// Package ws 是最小 WebSocket 服务端实现（RFC6455 子集），只为 CDP 传输服务。
//
// 覆盖范围（够 CDP 用，且刻意不多做）：
//   - 握手：Sec-WebSocket-Key → Sec-WebSocket-Accept（SHA-1 + base64）
//   - 帧：文本帧读写、客户端掩码解码、126/127 扩展长度、分片（continuation）拼装
//   - 控制帧：ping 自动回 pong、close 回执、pong 忽略
//   - 不做：permessage-deflate 等扩展、二进制消息、子协议协商
//
// 为什么自研而不引入 gorilla/websocket：CDP 客户端只用「掩码文本帧 + ping/
// pong/close」这一小块，真正的工作量在协议分派与会话（cdp 包），不在帧层。
// 见 docs/implementation-path.md §3.4（决策 2 采纳「自研最小 WS」）。
package ws

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 帧 opcode（RFC6455 §5.2）。
const (
	OpContinuation byte = 0x0
	OpText         byte = 0x1
	OpBinary       byte = 0x2
	OpClose        byte = 0x8
	OpPing         byte = 0x9
	OpPong         byte = 0xA
)

// MaxPayload 是单条消息的上限（CDP 消息里有截图 base64，给足余量）。
const MaxPayload = 32 << 20

// wsGUID 是 RFC6455 §1.3 规定的握手魔数。
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ErrClosed 表示对端发来 close 帧（或底层连接已断）。
var ErrClosed = errors.New("ws: 连接已关闭")

// Conn 是一条已握手的 WebSocket 连接（服务端侧）。
type Conn struct {
	conn    net.Conn
	br      *bufio.Reader
	bw      *bufio.Writer
	writeMu sync.Mutex
}

// Upgrade 在 HTTP 处理器里完成握手：校验请求头、劫持连接、回 101。
// 失败时已写好 HTTP 错误响应，调用方直接返回即可。
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "ws: 需要 Upgrade: websocket", http.StatusBadRequest)
		return nil, errors.New("ws: 缺少 Upgrade 头")
	}
	if !headerHasToken(r.Header, "Connection", "upgrade") {
		http.Error(w, "ws: 需要 Connection: Upgrade", http.StatusBadRequest)
		return nil, errors.New("ws: 缺少 Connection: Upgrade")
	}
	if v := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Version")); v != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "ws: 只支持 Sec-WebSocket-Version: 13", http.StatusUpgradeRequired)
		return nil, fmt.Errorf("ws: 不支持的版本 %q", v)
	}
	key := strings.TrimSpace(r.Header.Get("Sec-WebSocket-Key"))
	if key == "" {
		http.Error(w, "ws: 缺少 Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("ws: 缺少 Sec-WebSocket-Key")
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "ws: 该连接不支持劫持（需要 HTTP/1.1）", http.StatusInternalServerError)
		return nil, errors.New("ws: ResponseWriter 不支持 Hijack")
	}
	netConn, rw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("ws: 劫持连接失败: %w", err)
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
	if _, err := rw.WriteString(resp); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		_ = netConn.Close()
		return nil, err
	}
	return &Conn{conn: netConn, br: rw.Reader, bw: rw.Writer}, nil
}

// acceptKey 计算 Sec-WebSocket-Accept（RFC6455 §4.2.2）。
func acceptKey(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// headerHasToken 判定逗号分隔的头部里是否含某个 token（大小写不敏感）。
func headerHasToken(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// ReadMessage 读一条完整消息（自动拼装分片、自动回 pong、close 时返回
// ErrClosed）。返回 opcode 与 payload。
func (c *Conn) ReadMessage() (byte, []byte, error) {
	var opcode byte
	var payload []byte
	for {
		h, err := c.readFrameHeader()
		if err != nil {
			return 0, nil, err
		}
		switch h.opcode {
		case OpPing:
			_ = c.writeFrame(OpPong, h.payload)
			continue
		case OpPong:
			continue
		case OpClose:
			// 回执 close 后返回：连接不可再用。
			_ = c.writeFrame(OpClose, h.payload)
			return 0, nil, ErrClosed
		case OpText, OpBinary:
			if opcode != 0 {
				return 0, nil, errors.New("ws: 未收到 continuation 帧即开始新消息")
			}
			opcode = h.opcode
			payload = append(payload, h.payload...)
		case OpContinuation:
			if opcode == 0 {
				return 0, nil, errors.New("ws: 意外的 continuation 帧")
			}
			payload = append(payload, h.payload...)
		default:
			return 0, nil, fmt.Errorf("ws: 不支持的 opcode 0x%x", h.opcode)
		}
		if len(payload) > MaxPayload {
			return 0, nil, errors.New("ws: 消息超过上限")
		}
		if h.fin {
			return opcode, payload, nil
		}
	}
}

// frameHeader 是一帧的头部（payload 已按掩码解码）。
type frameHeader struct {
	fin     bool
	opcode  byte
	payload []byte
}

// readFrameHeader 读一帧头与负载（客户端帧必须带掩码，RFC6455 §5.1）。
func (c *Conn) readFrameHeader() (frameHeader, error) {
	var h frameHeader
	var head [2]byte
	if _, err := io.ReadFull(c.br, head[:]); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return h, ErrClosed
		}
		return h, err
	}
	h.fin = head[0]&0x80 != 0
	if head[0]&0x70 != 0 {
		return h, errors.New("ws: RSV 位非零（不支持扩展）")
	}
	h.opcode = head[0] & 0x0F
	masked := head[1]&0x80 != 0
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return h, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.br, ext[:]); err != nil {
			return h, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > MaxPayload {
		return h, errors.New("ws: 帧超过上限")
	}
	// 控制帧（close/ping/pong）必须 ≤125 字节且不可分片（RFC6455 §5.5）。
	if h.opcode&0x08 != 0 && (length > 125 || !h.fin) {
		return h, errors.New("ws: 控制帧不合法")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return h, err
		}
	} else {
		return h, errors.New("ws: 客户端帧缺少掩码")
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return h, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	h.payload = payload
	return h, nil
}

// WriteText 发一条文本消息（CDP 消息全是 JSON 文本）。
func (c *Conn) WriteText(payload []byte) error { return c.writeFrame(OpText, payload) }

// WriteClose 发 close 帧（code 0 表示不带状态码）。
func (c *Conn) WriteClose(code uint16, reason string) error {
	var payload []byte
	if code != 0 {
		payload = make([]byte, 2+len(reason))
		binary.BigEndian.PutUint16(payload[:2], code)
		copy(payload[2:], reason)
	}
	return c.writeFrame(OpClose, payload)
}

// writeFrame 写一帧（服务端帧不加掩码）。
func (c *Conn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	var hdr [10]byte
	hdr[0] = 0x80 | opcode
	n := 2
	switch {
	case len(payload) < 126:
		hdr[1] = byte(len(payload))
	case len(payload) <= 0xFFFF:
		hdr[1] = 126
		binary.BigEndian.PutUint16(hdr[2:4], uint16(len(payload)))
		n = 4
	default:
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:10], uint64(len(payload)))
		n = 10
	}
	if _, err := c.bw.Write(hdr[:n]); err != nil {
		return err
	}
	if _, err := c.bw.Write(payload); err != nil {
		return err
	}
	return c.bw.Flush()
}

// SetReadDeadline 设置读超时（0 表示不超时）。
func (c *Conn) SetReadDeadline(t time.Time) error { return c.conn.SetReadDeadline(t) }

// RemoteAddr 返回对端地址（日志用）。
func (c *Conn) RemoteAddr() net.Addr { return c.conn.RemoteAddr() }

// Close 关闭底层连接。
func (c *Conn) Close() error { return c.conn.Close() }
