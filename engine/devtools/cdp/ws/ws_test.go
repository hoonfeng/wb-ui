package ws

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// wsTestServer 起一个回显服务：把收到的每条文本消息原样发回。返回 server 与 ws URL。
func wsTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := Upgrade(w, r)
		if err != nil {
			return // Upgrade 已写错误响应
		}
		defer conn.Close()
		for {
			op, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if op == OpText {
				if err := conn.WriteText(payload); err != nil {
					return
				}
			}
		}
	}))
	u, _ := url.Parse(srv.URL)
	u.Scheme = "ws"
	u.Path = "/ws"
	return srv, u.String()
}

// dialWS 手写客户端握手（Go 标准库没有 WS 客户端；手写才能把真实字节格式当
// 被测输入，而不是拿被测实现自己给自己出题）。返回连接与已缓冲的读端。
func dialWS(t *testing.T, wsURL, key string) (net.Conn, *bufio.Reader, *http.Response) {
	t.Helper()
	u, err := url.Parse(wsURL)
	if err != nil {
		t.Fatalf("解析 ws URL 失败: %v", err)
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatalf("连接测试服务失败: %v", err)
	}
	req := "GET " + u.Path + " HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatalf("写握手请求失败: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatalf("设置读超时失败: %v", err)
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("读握手响应失败: %v", err)
	}
	return conn, br, resp
}

// writeClientFrame 按 RFC6455 写一个**带掩码**的客户端帧。
func writeClientFrame(t *testing.T, w io.Writer, fin bool, opcode byte, payload []byte) {
	t.Helper()
	var buf bytes.Buffer
	b0 := opcode
	if fin {
		b0 |= 0x80
	}
	buf.WriteByte(b0)
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	n := len(payload)
	switch {
	case n < 126:
		buf.WriteByte(byte(n) | 0x80)
	case n <= 0xFFFF:
		buf.WriteByte(126 | 0x80)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		buf.Write(ext[:])
	default:
		buf.WriteByte(127 | 0x80)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		buf.Write(ext[:])
	}
	buf.Write(mask)
	masked := make([]byte, n)
	for i := 0; i < n; i++ {
		masked[i] = payload[i] ^ mask[i%4]
	}
	buf.Write(masked)
	if _, err := w.Write(buf.Bytes()); err != nil {
		t.Fatalf("写客户端帧失败: %v", err)
	}
}

// readServerFrame 读一个服务端帧（服务端帧不带掩码）。
func readServerFrame(t *testing.T, r *bufio.Reader) (byte, []byte, bool) {
	t.Helper()
	var head [2]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		t.Fatalf("读服务端帧头失败: %v", err)
	}
	fin := head[0]&0x80 != 0
	opcode := head[0] & 0x0F
	if head[1]&0x80 != 0 {
		t.Fatal("服务端帧不应带掩码（RFC6455 §5.1）")
	}
	length := uint64(head[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatalf("读扩展长度失败: %v", err)
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatalf("读扩展长度失败: %v", err)
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("读负载失败: %v", err)
	}
	return opcode, payload, fin
}

// TestUpgradeAcceptsRFCExampleVector 用 RFC6455 §1.3 的标准测试向量校验
// Sec-WebSocket-Accept（key → accept 的映射错了，所有真实客户端都会拒绝连接）。
func TestUpgradeAcceptsRFCExampleVector(t *testing.T) {
	_, wsURL := wsTestServer(t)
	conn, _, resp := dialWS(t, wsURL, "dGhlIHNhbXBsZSBub25jZQ==")
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("状态 = %d，want 101", resp.StatusCode)
	}
	if got, want := resp.Header.Get("Sec-WebSocket-Accept"), "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="; got != want {
		t.Errorf("Sec-WebSocket-Accept = %q，want %q", got, want)
	}
	if up := resp.Header.Get("Upgrade"); !strings.EqualFold(up, "websocket") {
		t.Errorf("Upgrade = %q", up)
	}
}

// TestUpgradeRejectsBadHandshake 覆盖三类握手错误必须被拒绝（否则服务会接受
// 残缺握手并在后续帧解析上跑偏）。
func TestUpgradeRejectsBadHandshake(t *testing.T) {
	srv, _ := wsTestServer(t)
	defer srv.Close()
	cases := []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"缺 Upgrade", map[string]string{"Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ==", "Sec-WebSocket-Version": "13"}, http.StatusBadRequest},
		{"版本不支持", map[string]string{"Upgrade": "websocket", "Connection": "Upgrade", "Sec-WebSocket-Key": "dGhlIHNhbXBsZSBub25jZQ==", "Sec-WebSocket-Version": "8"}, http.StatusUpgradeRequired},
		{"缺 Key", map[string]string{"Upgrade": "websocket", "Connection": "Upgrade", "Sec-WebSocket-Version": "13"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", srv.URL+"/ws", nil)
			if err != nil {
				t.Fatal(err)
			}
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("请求失败: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Errorf("状态 = %d，want %d", resp.StatusCode, tc.want)
			}
		})
	}
}

// TestTextEchoWithMasking 是核心路径：带掩码文本帧 → 解掩码 → 回显。
func TestTextEchoWithMasking(t *testing.T) {
	_, wsURL := wsTestServer(t)
	conn, br, resp := dialWS(t, wsURL, "dGhlIHNhbXBsZSBub25jZQ==")
	defer conn.Close()
	if resp.StatusCode != 101 {
		t.Fatalf("握手失败: %d", resp.StatusCode)
	}
	msg := []byte(`{"id":1,"method":"Runtime.evaluate","params":{"expression":"1+1"}}`)
	writeClientFrame(t, conn, true, OpText, msg)
	op, payload, _ := readServerFrame(t, br)
	if op != OpText {
		t.Fatalf("opcode = 0x%x，want 文本", op)
	}
	if !bytes.Equal(payload, msg) {
		t.Errorf("回显 = %q，want %q", payload, msg)
	}
}

// TestExtendedLengthFrames 覆盖 126/127 两个扩展长度分支（CDP 的截图消息会走
// 到 127 分支，长度字段宽度错了会截断负载）。
func TestExtendedLengthFrames(t *testing.T) {
	_, wsURL := wsTestServer(t)
	conn, br, _ := dialWS(t, wsURL, "dGhlIHNhbXBsZSBub25jZQ==")
	defer conn.Close()
	for _, n := range []int{300, 70000} {
		payload := bytes.Repeat([]byte("x"), n)
		writeClientFrame(t, conn, true, OpText, payload)
		_, got, _ := readServerFrame(t, br)
		if len(got) != n {
			t.Fatalf("payload 长度 = %d，want %d", len(got), n)
		}
	}
}

// TestFragmentedMessage 覆盖分片拼装：CDP 客户端可以把大消息分片发送。
func TestFragmentedMessage(t *testing.T) {
	_, wsURL := wsTestServer(t)
	conn, br, _ := dialWS(t, wsURL, "dGhlIHNhbXBsZSBub25jZQ==")
	defer conn.Close()
	writeClientFrame(t, conn, false, OpText, []byte(`{"id":1,`))
	writeClientFrame(t, conn, true, OpContinuation, []byte(`"method":"Runtime.evaluate"}`))
	_, payload, _ := readServerFrame(t, br)
	want := `{"id":1,"method":"Runtime.evaluate"}`
	if string(payload) != want {
		t.Errorf("拼装结果 = %q，want %q", payload, want)
	}
}

// TestPingPongAndClose 覆盖控制帧：ping 必须自动回 pong；close 要回执。
func TestPingPongAndClose(t *testing.T) {
	_, wsURL := wsTestServer(t)
	conn, br, _ := dialWS(t, wsURL, "dGhlIHNhbXBsZSBub25jZQ==")
	defer conn.Close()
	writeClientFrame(t, conn, true, OpPing, []byte("hb"))
	op, payload, _ := readServerFrame(t, br)
	if op != OpPong || string(payload) != "hb" {
		t.Errorf("ping 回执 = opcode 0x%x payload %q，want pong + hb", op, payload)
	}
	writeClientFrame(t, conn, true, OpClose, nil)
	op, _, _ = readServerFrame(t, br)
	if op != OpClose {
		t.Errorf("close 回执 opcode = 0x%x，want close", op)
	}
}

// TestUnmaskedClientFrameRejected 覆盖协议强校验：客户端帧必须带掩码
// （RFC6455 §5.1），否则服务端应报错而不是把掩码位当长度读。
func TestUnmaskedClientFrameRejected(t *testing.T) {
	_, wsURL := wsTestServer(t)
	conn, _, _ := dialWS(t, wsURL, "dGhlIHNhbXBsZSBub25jZQ==")
	defer conn.Close()
	// 不带掩码位的文本帧：0x81 0x02 'h' 'i'
	if _, err := conn.Write([]byte{0x81, 0x02, 'h', 'i'}); err != nil {
		t.Fatalf("写帧失败: %v", err)
	}
	// 服务端应关闭连接（ReadMessage 报错 → handler 返回）而不是回显。
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 16)
	n, err := conn.Read(buf)
	if err == nil && n > 0 {
		// 允许服务端回一个 close 帧（0x88），但不能回文本回显。
		if buf[0]&0x0F == OpText {
			t.Fatalf("服务端不应回显无掩码帧，收到 0x%x", buf[0])
		}
	}
}
