// 最小 RFC6455 客户端：wb-ui 引擎不内置真实 WebSocket 传输（bindings 里只有
// stub），桌面壳需要自己连后端的两条长连接：
//
//	/ws                全局事件流（agent 事件 / 运行状态）
//	/api/terminal/ws   PTY 终端（二进制帧 = PTY I/O）
//
// 只实现桌面壳需要的子集：客户端握手、文本/二进制帧、ping/pong、close。
// 刻意不引第三方依赖（wb-ui 的 go.mod 里没有 gorilla/websocket），也不复刻
// engine/devtools/cdp/ws 的服务端实现（那是 Upgrade 方向）。
package ideapp

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const wsMagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsOpcode 是 RFC6455 帧类型。
type wsOpcode byte

const (
	wsOpContinuation wsOpcode = 0x0
	wsOpText         wsOpcode = 0x1
	wsOpBinary       wsOpcode = 0x2
	wsOpClose        wsOpcode = 0x8
	wsOpPing         wsOpcode = 0x9
	wsOpPong         wsOpcode = 0xA
)

// wsClient 是一条已建立的客户端连接。写操作加锁（多个 goroutine 可能同时
// 发送：终端输入、心跳、close）。
type wsClient struct {
	conn net.Conn
	br   *bufio.Reader
	wmu  sync.Mutex
}

// dialWS 连接 ws://host/path 并完成握手。timeout 覆盖 TCP 连接与握手。
func dialWS(rawURL string, timeout time.Duration) (*wsClient, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("ws url 解析失败：%w", err)
	}
	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("不支持的 scheme %q（只支持 ws/wss）", u.Scheme)
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "wss" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败：%w", host, err)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := u.RequestURI()
	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读取握手响应失败：%w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("握手失败：HTTP %d", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Sec-WebSocket-Accept"), acceptKey(key); got != want {
		conn.Close()
		return nil, fmt.Errorf("握手校验失败：Sec-WebSocket-Accept=%q", got)
	}
	_ = conn.SetDeadline(time.Time{})
	return &wsClient{conn: conn, br: br}, nil
}

func acceptKey(key string) string {
	sum := sha1.Sum([]byte(key + wsMagicGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// ReadMessage 读一条完整消息（自动拼接分片、处理 ping）。返回 opcode 与负载。
// 收到 close 时返回 io.EOF。
func (c *wsClient) ReadMessage() (wsOpcode, []byte, error) {
	var op wsOpcode
	var payload []byte
	first := true
	for {
		fin, frameOp, data, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch frameOp {
		case wsOpPing:
			_ = c.writeFrame(wsOpPong, data)
			continue
		case wsOpPong:
			continue
		case wsOpClose:
			_ = c.writeFrame(wsOpClose, nil)
			return 0, nil, io.EOF
		}
		if first {
			op = frameOp
			first = false
		}
		payload = append(payload, data...)
		if fin {
			return op, payload, nil
		}
	}
}

// readFrame 读一帧，返回 fin / opcode / payload。
func (c *wsClient) readFrame() (bool, wsOpcode, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.br, hdr[:]); err != nil {
		return false, 0, nil, err
	}
	fin := hdr[0]&0x80 != 0
	op := wsOpcode(hdr[0] & 0x0F)
	masked := hdr[1]&0x80 != 0
	length := int64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return false, 0, nil, err
		}
		length = int64(binary.BigEndian.Uint64(ext[:]))
	}
	if length < 0 || length > 64<<20 {
		return false, 0, nil, errors.New("ws: 帧长度非法")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return false, 0, nil, err
		}
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(c.br, data); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for i := range data {
			data[i] ^= mask[i%4]
		}
	}
	return fin, op, data, nil
}

// writeFrame 写一帧（客户端必须掩码）。
func (c *wsClient) writeFrame(op wsOpcode, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	head := []byte{0x80 | byte(op)}
	n := len(payload)
	switch {
	case n <= 125:
		head = append(head, 0x80|byte(n))
	case n <= 0xFFFF:
		head = append(head, 0x80|126, byte(n>>8), byte(n))
	default:
		head = append(head, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		head = append(head, ext[:]...)
	}
	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	head = append(head, mask[:]...)
	buf := make([]byte, 0, len(head)+n)
	buf = append(buf, head...)
	for i := 0; i < n; i++ {
		buf = append(buf, payload[i]^mask[i%4])
	}
	_, err := c.conn.Write(buf)
	return err
}

// WriteText / WriteBinary 供桥接层使用。
func (c *wsClient) WriteText(p []byte) error   { return c.writeFrame(wsOpText, p) }
func (c *wsClient) WriteBinary(p []byte) error { return c.writeFrame(wsOpBinary, p) }

// Close 发 close 帧并断开（幂等）。
func (c *wsClient) Close() error {
	_ = c.writeFrame(wsOpClose, []byte{0x03, 0xE8}) // 1000 normal closure
	return c.conn.Close()
}
