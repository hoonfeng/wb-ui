package jsc

import (
	"crypto/rand"
	"fmt"
	"sync"
)

// BlobDataField 是 Blob 实例承载原始字节的内部字段名（见 webapi.go 的
// blobDataKey）。bindings 层的 URL.createObjectURL 需要读取它，故导出
// 供跨包使用，避免两处字面量漂移。
const BlobDataField = "__wbBlobData"

// BlobURLData 是 blob: URL 背后的内容。
type BlobURLData struct {
	Data     string
	MIMEType string
}

// blobURLStore 保存 URL.createObjectURL 生成的 blob: URL → 内容映射。
// 与浏览器一样，blob: URL 只在当前会话/文档生命周期内有效，revoke 后失效；
// 内容直接驻留内存（Blob 本来就是内存对象）。
var blobURLStore sync.Map // map[string]BlobURLData

// RegisterBlobURL 为一段内容生成 blob: URL（浏览器语义：
// `blob:<origin>/<uuid>`），并登记内容供后续 fetch/资源加载读取。
func RegisterBlobURL(origin, data, mimeType string) string {
	if origin == "" || origin == "null" {
		origin = "null"
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
	u := fmt.Sprintf("blob:%s/%x-%x-%x-%x-%x", origin, b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	blobURLStore.Store(u, BlobURLData{Data: data, MIMEType: mimeType})
	return u
}

// LookupBlobURL 查找 blob: URL 对应的内容。
func LookupBlobURL(u string) (BlobURLData, bool) {
	v, ok := blobURLStore.Load(u)
	if !ok {
		return BlobURLData{}, false
	}
	return v.(BlobURLData), true
}

// RevokeBlobURL 释放 blob: URL（revokeObjectURL）。浏览器语义：之后再
// fetch 该 URL 会失败。
func RevokeBlobURL(u string) { blobURLStore.Delete(u) }
