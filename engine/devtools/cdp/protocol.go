package cdp

import "encoding/json"

// CDP 走 JSON-RPC 风格消息（Chrome 的 wire 格式）：
//
//	→ {"id":1,"method":"Runtime.evaluate","params":{…},"sessionId":"…"}
//	← {"id":1,"result":{…},"sessionId":"…"}  |  {"id":1,"error":{"code":-32601,"message":"…"}}
//	← {"method":"Page.loadEventFired","params":{…},"sessionId":"…"}   （事件无 id）

// request 是一条客户端消息。id 缺省（=0）时按 CDP 约定忽略（通知）。
type request struct {
	ID        int             `json:"id"`
	Method    string          `json:"method"`
	Params    json.RawMessage `json:"params,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// response 是一条响应。
type response struct {
	ID        int             `json:"id"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *protocolError  `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// event 是一条事件推送（无 id）。
type event struct {
	Method    string      `json:"method"`
	Params    interface{} `json:"params,omitempty"`
	SessionID string      `json:"sessionId,omitempty"`
}

// protocolError 是 JSON-RPC 错误对象（CDP 沿用其错误码空间）。
type protocolError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC 标准错误码（CDP 的解析错误、方法不存在等都用这套）。
const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
	// CDP 自定义错误码区间（-32000 起，Chrome 用它表达「协议内业务错误」）。
	errServer  = -32000
	errInvalid = -32001
)

// jsonRPCVersion 只用于文档/日志，不参与 wire 格式。
const jsonRPCVersion = "2.0"
