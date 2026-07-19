package page

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"wb-ui/jsc"
)

// RegisterFetch registers the global fetch() function on the JS interpreter.
func RegisterFetch(rt *jsc.Interpreter) {
	fetchFn := jsc.NewNativeFunction("fetch", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if len(args) < 1 {
			return rejectPromise(in, fmt.Errorf("fetch: missing url argument"))
		}
		url := jscToString(args[0])
		if url == "" {
			return rejectPromise(in, fmt.Errorf("fetch: url must be a non-empty string"))
		}
		method := "GET"
		var body io.Reader
		headers := http.Header{}
		if len(args) >= 2 {
			opts := args[1]
			if opts.IsObject() {
				o := opts.AsObject()
				if o != nil {
					if m, ok := o.GetByKey("method"); ok && !m.IsUndefined() && !m.IsNull() {
						method = jscToString(m)
					}
					if b, ok := o.GetByKey("body"); ok && !b.IsUndefined() && !b.IsNull() {
						body = strings.NewReader(jscToString(b))
					}
					if h, ok := o.GetByKey("headers"); ok && h.IsObject() {
						hObj := h.AsObject()
						if hObj != nil {
						for _, key := range hObj.Keys() {
								if val, ok2 := hObj.GetByKey(key); ok2 {
									headers.Set(key, jscToString(val))
								}
							}
						}
					}
				}
			}
		}
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: invalid request: %w", err))
		}
		req.Header = headers
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: request failed: %w", err))
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return rejectPromise(in, fmt.Errorf("fetch: read response failed: %w", err))
		}
		respObj := jsc.NewObject(in.ObjectPrototype())
		respObj.SetClassName("Response")
		respObj.Set("status", jsc.NumberValue(float64(resp.StatusCode)))
		respObj.Set("ok", jsc.BooleanValue(resp.StatusCode >= 200 && resp.StatusCode < 300))
		respObj.Set("statusText", jsc.StringValue(resp.Status))
		respObj.Set("url", jsc.StringValue(url))
		bodyText := string(respBody)
		textFn := jsc.NewNativeFunction("text", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			return resolvePromise(in2, jsc.StringValue(bodyText))
		}, 0)
		respObj.Set("text", jsc.FunctionValue(textFn))
		jsonFn := jsc.NewNativeFunction("json", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
			val, err := in2.Run(bodyText)
			if err != nil {
				return rejectPromise(in2, fmt.Errorf("fetch: json parse failed: %w", err))
			}
			return resolvePromise(in2, val)
		}, 0)
		respObj.Set("json", jsc.FunctionValue(jsonFn))
		headersObj := jsc.NewObject(in.ObjectPrototype())
		for k, vs := range resp.Header {
			headersObj.Set(k, jsc.StringValue(strings.Join(vs, ", ")))
		}
		respObj.Set("headers", jsc.ObjectValue(headersObj))
		return resolvePromise(in, jsc.ObjectValue(respObj))
	}, 1)
	rt.GlobalObject().Set("fetch", jsc.FunctionValue(fetchFn))
}

// RegisterXMLHttpRequest registers the XMLHttpRequest constructor on the JS interpreter.
func RegisterXMLHttpRequest(rt *jsc.Interpreter) {
	proto := jsc.NewObject(rt.ObjectPrototype())
	proto.SetClassName("XMLHttpRequestPrototype")
	proto.Set("UNSENT", jsc.NumberValue(0))
	proto.Set("OPENED", jsc.NumberValue(1))
	proto.Set("HEADERS_RECEIVED", jsc.NumberValue(2))
	proto.Set("LOADING", jsc.NumberValue(3))
	proto.Set("DONE", jsc.NumberValue(4))

	openFn := jsc.NewNativeFunction("open", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() {
			return jsc.Undefined()
		}
		obj := this.AsObject()
		if len(args) >= 1 {
			method := strings.ToUpper(jscToString(args[0]))
			obj.Set("_method", jsc.StringValue(method))
		}
		if len(args) >= 2 {
			obj.Set("_url", args[1])
		}
		if len(args) >= 3 {
			obj.Set("_async", args[2])
		} else {
			obj.Set("_async", jsc.BooleanValue(true))
		}
		obj.Set("readyState", jsc.NumberValue(1))
		obj.Set("_requestHeaders", jsc.ObjectValue(jsc.NewObject(in.ObjectPrototype())))
		xhrFireReadyStateChange(in, obj)
		return jsc.Undefined()
	}, 5)
	proto.Set("open", jsc.FunctionValue(openFn))

	setReqHeaderFn := jsc.NewNativeFunction("setRequestHeader", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() || len(args) < 2 {
			return jsc.Undefined()
		}
		obj := this.AsObject()
	hv, ok := obj.GetByKey("_requestHeaders")
		if ok && hv.IsObject() {
			hObj := hv.AsObject()
			name := jscToString(args[0])
			val := jscToString(args[1])
			hObj.Set(name, jsc.StringValue(val))
		}
		return jsc.Undefined()
	}, 2)
	proto.Set("setRequestHeader", jsc.FunctionValue(setReqHeaderFn))

	sendFn := jsc.NewNativeFunction("send", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() {
			return jsc.Undefined()
		}
		obj := this.AsObject()
		method := jscToString2(obj, "_method")
		url := jscToString2(obj, "_url")
		if method == "" {
			method = "GET"
		}
		var body io.Reader
		if len(args) >= 1 && !args[0].IsUndefined() && !args[0].IsNull() {
			body = strings.NewReader(jscToString(args[0]))
		}
		headers := http.Header{}
	hv, ok := obj.GetByKey("_requestHeaders")
		if ok && hv.IsObject() {
			hObj := hv.AsObject()
		for _, key := range hObj.Keys() {
			if val, ok2 := hObj.GetByKey(key); ok2 {
					headers.Set(key, jscToString(val))
				}
			}
		}
		obj.Set("readyState", jsc.NumberValue(3))
		xhrFireReadyStateChange(in, obj)
		req, err := http.NewRequest(method, url, body)
		if err != nil {
			obj.Set("status", jsc.NumberValue(0))
			obj.Set("statusText", jsc.StringValue(err.Error()))
			obj.Set("readyState", jsc.NumberValue(4))
			xhrFireReadyStateChange(in, obj)
			return jsc.Undefined()
		}
		req.Header = headers
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			obj.Set("status", jsc.NumberValue(0))
			obj.Set("statusText", jsc.StringValue(err.Error()))
			obj.Set("readyState", jsc.NumberValue(4))
			xhrFireReadyStateChange(in, obj)
			return jsc.Undefined()
		}
		defer resp.Body.Close()
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			obj.Set("status", jsc.NumberValue(0))
			obj.Set("statusText", jsc.StringValue(err.Error()))
			obj.Set("readyState", jsc.NumberValue(4))
			xhrFireReadyStateChange(in, obj)
			return jsc.Undefined()
		}
		obj.Set("status", jsc.NumberValue(float64(resp.StatusCode)))
		var hdrLines []string
		for k, vs := range resp.Header {
			hdrLines = append(hdrLines, fmt.Sprintf("%s: %s", k, strings.Join(vs, ", ")))
		}
		obj.Set("_responseHeaders", jsc.StringValue(strings.Join(hdrLines, "\r\n")))
		getAllRespHeadersFn := jsc.NewNativeFunction("getAllResponseHeaders", func(in2 *jsc.Interpreter, this2 jsc.JSValue, args2 []jsc.JSValue) jsc.JSValue {
		if v, ok := obj.GetByKey("_responseHeaders"); ok {
				return v
			}
			return jsc.StringValue("")
		}, 0)
		obj.Set("getAllResponseHeaders", jsc.FunctionValue(getAllRespHeadersFn))
		obj.Set("responseText", jsc.StringValue(string(respBody)))
		obj.Set("response", jsc.StringValue(string(respBody)))
		obj.Set("readyState", jsc.NumberValue(4))
		xhrFireReadyStateChange(in, obj)
		return jsc.Undefined()
	}, 1)
	proto.Set("send", jsc.FunctionValue(sendFn))

	abortFn := jsc.NewNativeFunction("abort", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		if !this.IsObject() {
			return jsc.Undefined()
		}
		obj := this.AsObject()
		obj.Set("readyState", jsc.NumberValue(0))
		xhrFireReadyStateChange(in, obj)
		return jsc.Undefined()
	}, 0)
	proto.Set("abort", jsc.FunctionValue(abortFn))

	ctor := jsc.NewNativeFunction("XMLHttpRequest", func(in *jsc.Interpreter, this jsc.JSValue, args []jsc.JSValue) jsc.JSValue {
		xhr := jsc.NewObject(proto)
		xhr.SetClassName("XMLHttpRequest")
		xhr.Set("readyState", jsc.NumberValue(0))
		xhr.Set("status", jsc.NumberValue(0))
		xhr.Set("statusText", jsc.StringValue(""))
		xhr.Set("responseText", jsc.StringValue(""))
		xhr.Set("response", jsc.Null())
		xhr.Set("timeout", jsc.NumberValue(0))
		xhr.Set("withCredentials", jsc.BooleanValue(false))
		return jsc.ObjectValue(xhr)
	}, 0)
	rt.GlobalObject().Set("XMLHttpRequest", jsc.FunctionValue(ctor))
}

// --- Helpers ---

func jscToString(v jsc.JSValue) string {
	if v.IsUndefined() || v.IsNull() {
		return ""
	}
	if v.IsString() {
		return v.String()
	}
	return fmt.Sprintf("%v", v)
}
func jscToString2(obj *jsc.JSObject, key string) string {
	if v, ok := obj.GetByKey(key); ok {
		return jscToString(v)
	}
	return ""
}



func resolvePromise(in *jsc.Interpreter, val jsc.JSValue) jsc.JSValue {
	return in.ResolvePromise(val)
}

func rejectPromise(in *jsc.Interpreter, err error) jsc.JSValue {
	return in.RejectPromise(jsc.StringValue(err.Error()))
}

func xhrFireReadyStateChange(in *jsc.Interpreter, obj *jsc.JSObject) {
		hVal, ok := obj.GetByKey("onreadystatechange")
	if !ok || hVal.IsUndefined() || hVal.IsNull() {
		return
	}
	in.Call(hVal, jsc.ObjectValue(obj), nil)
}
