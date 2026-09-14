// Tests for the resource loading pipeline: ResourceRequest, ResourceResponse,
// CachedResource, MemoryCache, ResourceHandle, CachedResourceLoader.
//
// These tests exercise the complete resource lifecycle from URL resolution
// through cache lookup, network transfer (via file://) and client notification.
// HTTP-dependent tests use file:// URLs to avoid real network dependencies.

package page

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testCachedResourceClient implements CachedResourceClient for testing.
// It records the resource passed to NotifyFinished so the test can inspect it.
type testCachedResourceClient struct {
	mu       sync.Mutex
	notified bool
	resource *CachedResource
	ch       chan struct{} // closed on NotifyFinished, for blocking waits
}

func newTestCachedResourceClient() *testCachedResourceClient {
	return &testCachedResourceClient{ch: make(chan struct{})}
}

func (c *testCachedResourceClient) NotifyFinished(resource *CachedResource) {
	c.mu.Lock()
	c.notified = true
	c.resource = resource
	c.mu.Unlock()
	close(c.ch)
}

func (c *testCachedResourceClient) Notified() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.notified
}

func (c *testCachedResourceClient) Resource() *CachedResource {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.resource
}

func (c *testCachedResourceClient) Wait(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-c.ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for NotifyFinished")
	}
}

// stringAsError is a simple error type wrapping a string message.
type stringAsError struct{ msg string }

func (e stringAsError) Error() string { return e.msg }

// testResourceHandleClient implements ResourceHandleClient for testing.
// It records the callbacks received during a resource load.
type testResourceHandleClient struct {
	mu         sync.Mutex
	response   *ResourceResponse
	dataChunks [][]byte
	finished   bool
	failErr    error
	ch         chan struct{} // closed on DidFinishLoading or DidFail
}

func newTestResourceHandleClient() *testResourceHandleClient {
	return &testResourceHandleClient{ch: make(chan struct{})}
}

func (c *testResourceHandleClient) DidReceiveResponse(handle *ResourceHandle, resp *ResourceResponse) {
	c.mu.Lock()
	c.response = resp
	c.mu.Unlock()
}

func (c *testResourceHandleClient) DidReceiveData(handle *ResourceHandle, data []byte) {
	c.mu.Lock()
	chunk := make([]byte, len(data))
	copy(chunk, data)
	c.dataChunks = append(c.dataChunks, chunk)
	c.mu.Unlock()
}

func (c *testResourceHandleClient) DidFinishLoading(handle *ResourceHandle) {
	c.mu.Lock()
	c.finished = true
	c.mu.Unlock()
	close(c.ch)
}

func (c *testResourceHandleClient) DidFail(handle *ResourceHandle, err error) {
	c.mu.Lock()
	c.failErr = err
	c.mu.Unlock()
	close(c.ch)
}

func (c *testResourceHandleClient) Response() *ResourceResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.response
}

func (c *testResourceHandleClient) Finished() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.finished
}

func (c *testResourceHandleClient) FailErr() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.failErr
}

func (c *testResourceHandleClient) CombinedData() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	var combined []byte
	for _, chunk := range c.dataChunks {
		combined = append(combined, chunk...)
	}
	return combined
}

func (c *testResourceHandleClient) Wait(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-c.ch:
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for resource handle completion")
	}
}

// writeTempFile writes content to a temp file and returns its file:// URL.
func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return "file://" + path
}

// ---------------------------------------------------------------------------
// Test group 1: ResourceRequest / ResourceResponse
// ---------------------------------------------------------------------------

func TestResourceRequest_NewRequest(t *testing.T) {
	getReq := NewResourceRequest("http://example.com/page", "GET")
	if getReq.URL != "http://example.com/page" {
		t.Errorf("GET URL = %q, want http://example.com/page", getReq.URL)
	}
	if getReq.Method != "GET" {
		t.Errorf("GET Method = %q, want GET", getReq.Method)
	}

	postReq := NewResourceRequest("http://example.com/submit", "POST")
	if postReq.Method != "POST" {
		t.Errorf("POST Method = %q, want POST", postReq.Method)
	}

	// Empty method defaults to GET.
	defaultReq := NewResourceRequest("http://example.com", "")
	if defaultReq.Method != "GET" {
		t.Errorf("default Method = %q, want GET", defaultReq.Method)
	}
}

func TestResourceRequest_Headers(t *testing.T) {
	req := NewResourceRequest("http://example.com", "GET")
	req.SetHeader("Accept", "text/html")
	req.SetHeader("Authorization", "Bearer token123")

	if got := req.GetHeader("Accept"); got != "text/html" {
		t.Errorf("Accept header = %q, want text/html", got)
	}
	if got := req.GetHeader("Authorization"); got != "Bearer token123" {
		t.Errorf("Authorization header = %q, want Bearer token123", got)
	}
	if got := req.GetHeader("X-Nonexistent"); got != "" {
		t.Errorf("X-Nonexistent header = %q, want empty", got)
	}
}

func TestResourceResponse_NewResponse(t *testing.T) {
	resp := NewResourceResponse()
	if resp.Headers == nil {
		t.Error("Headers map is nil")
	}
	if resp.ContentLength != -1 {
		t.Errorf("ContentLength = %d, want -1", resp.ContentLength)
	}
	if resp.StatusCode != 0 {
		t.Errorf("StatusCode = %d, want 0", resp.StatusCode)
	}
}

func TestResourceResponse_SetContentType(t *testing.T) {
	resp := NewResourceResponse()
	resp.SetContentType("text/html; charset=utf-8")
	if resp.MimeType != "text/html" {
		t.Errorf("MimeType = %q, want text/html", resp.MimeType)
	}
	if ct := resp.Headers.Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type header = %q, want text/html; charset=utf-8", ct)
	}

	// Without charset.
	resp2 := NewResourceResponse()
	resp2.SetContentType("application/json")
	if resp2.MimeType != "application/json" {
		t.Errorf("MimeType = %q, want application/json", resp2.MimeType)
	}
}

func TestResourceResponse_IsHTTP(t *testing.T) {
	resp := NewResourceResponse()
	if resp.IsHTTP() {
		t.Error("IsHTTP() = true for zero-value response, want false")
	}

	resp.StatusCode = 200
	if !resp.IsHTTP() {
		t.Error("IsHTTP() = false for status 200, want true")
	}

	resp.StatusCode = 404
	if !resp.IsHTTP() {
		t.Error("IsHTTP() = false for status 404, want true")
	}
}

func TestResourceType_String(t *testing.T) {
	tests := []struct {
		typ  ResourceType
		want string
	}{
		{ResourceTypeMainResource, "MainResource"},
		{ResourceTypeStylesheet, "Stylesheet"},
		{ResourceTypeScript, "Script"},
		{ResourceTypeImage, "Image"},
		{ResourceTypeFont, "Font"},
		{ResourceTypeRaw, "Raw"},
		{ResourceTypeSVGDocument, "SVGDocument"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("ResourceType(%d).String() = %q, want %q", int(tt.typ), got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Test group 2: CachedResource state machine
// ---------------------------------------------------------------------------

func TestCachedResource_New(t *testing.T) {
	res := NewCachedResource("http://example.com/style.css", ResourceTypeStylesheet)
	if res.URL() != "http://example.com/style.css" {
		t.Errorf("URL() = %q", res.URL())
	}
	if res.Type() != ResourceTypeStylesheet {
		t.Errorf("Type() = %v, want Stylesheet", res.Type())
	}
	if res.Status() != CachedResourceStatusPending {
		t.Errorf("Status() = %v, want Pending", res.Status())
	}
	if res.Data() != nil {
		t.Errorf("Data() = %v, want nil", res.Data())
	}
	if res.Error() != nil {
		t.Errorf("Error() = %v, want nil", res.Error())
	}
}

func TestCachedResource_SetData(t *testing.T) {
	res := NewCachedResource("http://example.com/test", ResourceTypeRaw)
	data := []byte("hello world")
	res.SetData(data)

	if res.Status() != CachedResourceStatusLoaded {
		t.Errorf("Status() = %v, want Loaded", res.Status())
	}
	if string(res.Data()) != "hello world" {
		t.Errorf("Data() = %q, want hello world", string(res.Data()))
	}
	if res.Error() != nil {
		t.Errorf("Error() = %v, want nil", res.Error())
	}
}

func TestCachedResource_SetError(t *testing.T) {
	res := NewCachedResource("http://example.com/test", ResourceTypeRaw)
	testErr := "connection refused"
	res.SetError(stringAsError{msg: testErr})

	if res.Status() != CachedResourceStatusError {
		t.Errorf("Status() = %v, want Error", res.Status())
	}
	if res.Error() == nil || res.Error().Error() != testErr {
		t.Errorf("Error() = %v, want %q", res.Error(), testErr)
	}
}

func TestCachedResource_ClientNotify(t *testing.T) {
	res := NewCachedResource("http://example.com/test", ResourceTypeRaw)
	client := newTestCachedResourceClient()
	res.AddClient(client)

	data := []byte("test data")
	res.SetData(data)

	if !client.Notified() {
		t.Fatal("client was not notified")
	}
	if client.Resource() != res {
		t.Error("client.Resource() does not match original resource")
	}
	if string(client.Resource().Data()) != "test data" {
		t.Errorf("client resource data = %q, want test data", string(client.Resource().Data()))
	}
}

func TestCachedResource_MultipleClients(t *testing.T) {
	res := NewCachedResource("http://example.com/test", ResourceTypeRaw)
	client1 := newTestCachedResourceClient()
	client2 := newTestCachedResourceClient()
	client3 := newTestCachedResourceClient()

	res.AddClient(client1)
	res.AddClient(client2)
	res.AddClient(client3)

	res.SetData([]byte("data"))

	if !client1.Notified() || !client2.Notified() || !client3.Notified() {
		t.Error("not all clients were notified")
	}
}

func TestCachedResource_ClientCleared(t *testing.T) {
	res := NewCachedResource("http://example.com/test", ResourceTypeRaw)
	res.AddClient(newTestCachedResourceClient())
	res.AddClient(newTestCachedResourceClient())

	res.SetData([]byte("data"))

	// After notification, clients should be cleared.
	if res.HasClients() {
		t.Error("HasClients() = true after SetData, want false")
	}

	// Adding a client after terminal state should notify immediately.
	lateClient := newTestCachedResourceClient()
	res.AddClient(lateClient)
	if !lateClient.Notified() {
		t.Error("late client was not notified immediately")
	}
}

func TestCachedResource_AccessCount(t *testing.T) {
	res := NewCachedResource("http://example.com/test", ResourceTypeRaw)
	if res.AccessCount() != 0 {
		t.Errorf("initial AccessCount = %d, want 0", res.AccessCount())
	}

	res.IncrementAccessCount()
	res.IncrementAccessCount()
	if res.AccessCount() != 2 {
		t.Errorf("AccessCount after 2 increments = %d, want 2", res.AccessCount())
	}

	res.DecrementAccessCount()
	if res.AccessCount() != 1 {
		t.Errorf("AccessCount after decrement = %d, want 1", res.AccessCount())
	}

	// Should not go below zero.
	res.DecrementAccessCount()
	res.DecrementAccessCount()
	if res.AccessCount() != 0 {
		t.Errorf("AccessCount after decrement below zero = %d, want 0", res.AccessCount())
	}
}

func TestCachedResource_LastAccessed(t *testing.T) {
	res := NewCachedResource("http://example.com/test", ResourceTypeRaw)
	now := time.Now()
	if res.LastAccessed().Before(now.Add(-time.Second)) {
		t.Error("LastAccessed is in the past")
	}

	past := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	res.SetLastAccessed(past)
	if !res.LastAccessed().Equal(past) {
		t.Errorf("LastAccessed = %v, want %v", res.LastAccessed(), past)
	}
}

// ---------------------------------------------------------------------------
// Test group 3: MemoryCache LRU
// ---------------------------------------------------------------------------

func TestMemoryCache_AddGet(t *testing.T) {
	mc := NewMemoryCache(1024 * 1024) // 1 MB
	res := NewCachedResource("http://example.com/a.css", ResourceTypeStylesheet)
	res.SetData([]byte("body { color: red; }"))

	mc.Add(res)

	got := mc.Get("http://example.com/a.css")
	if got == nil {
		t.Fatal("Get returned nil after Add")
	}
	if got.URL() != "http://example.com/a.css" {
		t.Errorf("URL = %q", got.URL())
	}
	if string(got.Data()) != "body { color: red; }" {
		t.Errorf("Data = %q", string(got.Data()))
	}
}

func TestMemoryCache_GetMiss(t *testing.T) {
	mc := NewMemoryCache(1024 * 1024)
	if got := mc.Get("http://example.com/nonexistent"); got != nil {
		t.Errorf("Get for missing URL returned %v, want nil", got)
	}
}

func TestMemoryCache_Remove(t *testing.T) {
	mc := NewMemoryCache(1024 * 1024)
	res := NewCachedResource("http://example.com/test.js", ResourceTypeScript)
	res.SetData([]byte("var x = 1;"))
	mc.Add(res)

	mc.Remove("http://example.com/test.js")
	if got := mc.Get("http://example.com/test.js"); got != nil {
		t.Error("Get after Remove returned non-nil")
	}
}

func TestMemoryCache_RemoveAll(t *testing.T) {
	mc := NewMemoryCache(1024 * 1024)
	mc.Add(NewCachedResource("http://example.com/a.js", ResourceTypeScript))
	mc.Add(NewCachedResource("http://example.com/b.js", ResourceTypeScript))
	mc.Add(NewCachedResource("http://example.com/c.js", ResourceTypeScript))

	mc.RemoveAll()
	if mc.Count() != 0 {
		t.Errorf("Count after RemoveAll = %d, want 0", mc.Count())
	}
}

func TestMemoryCache_LRUOrder(t *testing.T) {
	mc := NewMemoryCache(1024 * 1024)

	// Add three resources.
	r1 := NewCachedResource("http://example.com/old.js", ResourceTypeScript)
	r1.SetData([]byte("a"))
	mc.Add(r1)

	r2 := NewCachedResource("http://example.com/mid.js", ResourceTypeScript)
	r2.SetData([]byte("b"))
	mc.Add(r2)

	r3 := NewCachedResource("http://example.com/new.js", ResourceTypeScript)
	r3.SetData([]byte("c"))
	mc.Add(r3)

	// Access the oldest one, making it the newest.
	mc.Get("http://example.com/old.js")

	// Now old.js should be most recently used and survive eviction.
	// Create a tiny cache that forces eviction.
	mc.SetCapacity(1)

	// Only one resource should remain. It should be the one we touched.
	if mc.Count() != 1 {
		t.Fatalf("after eviction Count = %d, want 1", mc.Count())
	}
	if got := mc.Get("http://example.com/old.js"); got == nil {
		t.Error("old.js was evicted despite being most recently used")
	}
}

func TestMemoryCache_Eviction(t *testing.T) {
	// Create a tiny cache: capacity = 10 bytes.
	mc := NewMemoryCache(10)

	// Add a 5-byte resource.
	r1 := NewCachedResource("http://example.com/small.js", ResourceTypeScript)
	r1.SetData([]byte("hello"))
	mc.Add(r1)

	// Add another 5-byte resource — still within capacity (10 bytes total).
	r2 := NewCachedResource("http://example.com/small2.js", ResourceTypeScript)
	r2.SetData([]byte("world"))
	mc.Add(r2)

	if mc.Count() != 2 {
		t.Fatalf("after 2 small resources Count = %d, want 2", mc.Count())
	}

	// Add a 10-byte resource — exceeds capacity, should evict the oldest.
	r3 := NewCachedResource("http://example.com/big.js", ResourceTypeScript)
	r3.SetData([]byte("1234567890"))
	mc.Add(r3)

	// r1 (the oldest) should be evicted.
	if got := mc.Get("http://example.com/small.js"); got != nil {
		t.Error("oldest resource was not evicted")
	}
	// r2 was added after r1 but before r3. With total 20 bytes > capacity 10,
	// both small resources get evicted, leaving only the big one.
	if mc.Get("http://example.com/big.js") == nil {
		t.Error("newly added resource was incorrectly evicted")
	}
}

func TestMemoryCache_PruneDeadResources(t *testing.T) {
	mc := NewMemoryCache(1024 * 1024)

	// Create a resource without clients.
	r1 := NewCachedResource("http://example.com/dead.css", ResourceTypeStylesheet)
	r1.SetData([]byte("dead"))
	mc.Add(r1)

	// Create a resource in Pending state with a live client (client added
	// before SetData so it stays in the clients map).
	r2 := NewCachedResource("http://example.com/live.css", ResourceTypeStylesheet)
	r2.AddClient(newTestCachedResourceClient())
	// r2 stays Pending — no SetData call, so clients map is intact.
	mc.Add(r2)

	mc.PruneDeadResources()

	if mc.Get("http://example.com/dead.css") != nil {
		t.Error("dead resource was not pruned")
	}
	if mc.Get("http://example.com/live.css") == nil {
		t.Error("live resource (Pending + has client) was incorrectly pruned")
	}
}

func TestMemoryCache_DefaultSize(t *testing.T) {
	mc := NewMemoryCache(0) // should use default
	if mc.Capacity() != DefaultMemoryCacheCapacity {
		t.Errorf("Capacity = %d, want %d", mc.Capacity(), DefaultMemoryCacheCapacity)
	}
}

// ---------------------------------------------------------------------------
// Test group 4: ResourceHandle
// ---------------------------------------------------------------------------

func TestResourceHandle_FileLoad(t *testing.T) {
	dir := t.TempDir()
	url := writeTempFile(t, dir, "hello.txt", "Hello, ResourceHandle!")

	client := newTestResourceHandleClient()
	req := NewResourceRequest(url, "GET")
	handle := NewResourceHandle(req, client)

	if err := handle.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	client.Wait(t, 5*time.Second)

	if !client.Finished() {
		t.Fatal("DidFinishLoading was not called")
	}
	if client.FailErr() != nil {
		t.Fatalf("DidFail called with error: %v", client.FailErr())
	}
	if got := string(client.CombinedData()); got != "Hello, ResourceHandle!" {
		t.Errorf("received data = %q, want Hello, ResourceHandle!", got)
	}
	if resp := client.Response(); resp == nil {
		t.Fatal("Response is nil")
	} else if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	} else if resp.MimeType != "application/octet-stream" {
		t.Errorf("MimeType = %q, want application/octet-stream", resp.MimeType)
	}
}

func TestResourceHandle_FileLoadHTML(t *testing.T) {
	dir := t.TempDir()
	url := writeTempFile(t, dir, "index.html", "<html><body><p>Hello</p></body></html>")

	client := newTestResourceHandleClient()
	req := NewResourceRequest(url, "GET")
	handle := NewResourceHandle(req, client)

	if err := handle.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	client.Wait(t, 5*time.Second)

	if !client.Finished() {
		t.Fatal("DidFinishLoading was not called")
	}
	if resp := client.Response(); resp == nil {
		t.Fatal("Response is nil")
	} else if resp.MimeType != "text/html" {
		t.Errorf("MimeType = %q, want text/html", resp.MimeType)
	}
}

func TestResourceHandle_Cancel(t *testing.T) {
	// Create a handle for a valid file but cancel before ReadFile completes
	// (in practice file I/O is fast, but Cancel should still work).
	dir := t.TempDir()
	url := writeTempFile(t, dir, "cancel.txt", "this should not arrive")

	client := newTestResourceHandleClient()
	req := NewResourceRequest(url, "GET")
	handle := NewResourceHandle(req, client)

	if err := handle.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Cancel immediately.
	handle.Cancel()

	// Wait briefly; the handle should finish (either DidFail due to cancel
	// or DidFinishLoading if it raced ahead).
	time.Sleep(100 * time.Millisecond)

	// Cancel is safe to call multiple times.
	handle.Cancel()
	handle.Cancel()
}

func TestResourceHandle_NilClient(t *testing.T) {
	// Should not panic when client is nil.
	dir := t.TempDir()
	url := writeTempFile(t, dir, "nil.txt", "no panic")

	req := NewResourceRequest(url, "GET")
	handle := NewResourceHandle(req, nil)

	if err := handle.Start(); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Give the goroutine time to complete.
	time.Sleep(200 * time.Millisecond)

	if handle.Response() == nil {
		t.Error("Response() is nil after nil-client load")
	}
}

func TestResourceHandle_InvalidURL(t *testing.T) {
	client := newTestResourceHandleClient()
	// Use a URL with an invalid scheme so Start fails fast.
	req := NewResourceRequest("ht-tp://invalid scheme", "GET")
	handle := NewResourceHandle(req, client)

	if err := handle.Start(); err == nil {
		// The goroutine should fail before connecting.
		client.Wait(t, 2*time.Second)
		if client.Finished() {
			t.Error("DidFinishLoading was called, expected DidFail")
		}
		if client.FailErr() == nil {
			t.Error("DidFail was not called, expected error")
		}
	}
}

// ---------------------------------------------------------------------------
// Test group 5: CachedResourceLoader
// ---------------------------------------------------------------------------

func TestCachedResourceLoader_RequestResource(t *testing.T) {
	dir := t.TempDir()
	url := writeTempFile(t, dir, "data.txt", "resource data")

	loader := NewCachedResourceLoader("file://" + dir + "/")

	client := newTestCachedResourceClient()
	_ = loader.RequestResource(url, ResourceTypeRaw, client)

	client.Wait(t, 5*time.Second)

	if !client.Notified() {
		t.Fatal("client was not notified")
	}
	res := client.Resource()
	if res == nil {
		t.Fatal("resource is nil")
	}
	if res.Status() != CachedResourceStatusLoaded {
		t.Fatalf("resource status = %v, want Loaded", res.Status())
	}
	if string(res.Data()) != "resource data" {
		t.Errorf("resource data = %q, want 'resource data'", string(res.Data()))
	}
}

func TestCachedResourceLoader_CacheHit(t *testing.T) {
	dir := t.TempDir()
	url := writeTempFile(t, dir, "cached.txt", "cached content")

	loader := NewCachedResourceLoader("file://" + dir + "/")

	// First request: loads from file.
	client1 := newTestCachedResourceClient()
	res1 := loader.RequestResource(url, ResourceTypeRaw, client1)
	client1.Wait(t, 5*time.Second)

	if res1.Status() != CachedResourceStatusLoaded {
		t.Fatalf("first request status = %v, want Loaded", res1.Status())
	}

	// Second request: should hit cache and notify immediately.
	client2 := newTestCachedResourceClient()
	res2 := loader.RequestResource(url, ResourceTypeRaw, client2)

	// The resource should be the same instance.
	if res1 != res2 {
		t.Error("second request returned a different CachedResource instance")
	}
	// The client should have been notified synchronously (since the resource
	// is already Loaded).
	if !client2.Notified() {
		t.Error("second client was not notified (cache hit)")
	}
	if string(res2.Data()) != "cached content" {
		t.Errorf("resource data = %q, want 'cached content'", string(res2.Data()))
	}
}

func TestCachedResourceLoader_LoadStylesheet(t *testing.T) {
	dir := t.TempDir()
	url := writeTempFile(t, dir, "style.css", "body { color: red; }")

	loader := NewCachedResourceLoader("file://" + dir + "/")

	client := newTestCachedResourceClient()
	res := loader.LoadStylesheet(url, client)
	client.Wait(t, 5*time.Second)

	if res == nil {
		t.Fatal("resource is nil")
	}
	if res.Type() != ResourceTypeStylesheet {
		t.Errorf("resource type = %v, want Stylesheet", res.Type())
	}
	if string(res.Data()) != "body { color: red; }" {
		t.Errorf("data = %q, want 'body { color: red; }'", string(res.Data()))
	}
}

func TestCachedResourceLoader_RelativeURL(t *testing.T) {
	dir := t.TempDir()
	// Create a subdirectory with a resource.
	subDir := filepath.Join(dir, "css")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	_ = writeTempFile(t, subDir, "relative.css", "/* relative */")

	// Set the document URL to the subdirectory.
	docURL := "file://" + subDir + "/"
	loader := NewCachedResourceLoader(docURL)

	// Request using a relative path.
	client := newTestCachedResourceClient()
	res := loader.RequestResource("relative.css", ResourceTypeStylesheet, client)
	client.Wait(t, 5*time.Second)

	if res == nil {
		t.Fatal("resource is nil")
	}
	if res.Status() != CachedResourceStatusLoaded {
		t.Fatalf("resource status = %v, want Loaded", res.Status())
	}
	if string(res.Data()) != "/* relative */" {
		t.Errorf("data = %q, want '/* relative */'", string(res.Data()))
	}
}

// ---------------------------------------------------------------------------
// Test group 6: ParseContentType helper
// ---------------------------------------------------------------------------

func TestParseContentType(t *testing.T) {
	tests := []struct {
		input     string
		wantMedia string
		wantCharset string
	}{
		{"text/html", "text/html", ""},
		{"text/html; charset=utf-8", "text/html", "utf-8"},
		{"application/json; charset=UTF-8", "application/json", "UTF-8"},
		{"text/css; charset=iso-8859-1", "text/css", "iso-8859-1"},
		{"image/png", "image/png", ""},
	}
	for _, tt := range tests {
		media, params := parseContentType(tt.input)
		if media != tt.wantMedia {
			t.Errorf("parseContentType(%q) media = %q, want %q", tt.input, media, tt.wantMedia)
		}
		if params["charset"] != tt.wantCharset {
			t.Errorf("parseContentType(%q) charset = %q, want %q", tt.input, params["charset"], tt.wantCharset)
		}
	}
}

// ---------------------------------------------------------------------------
// Test group 7: DefaultMemoryCache singleton
// ---------------------------------------------------------------------------

func TestDefaultMemoryCache(t *testing.T) {
	if DefaultMemoryCache == nil {
		t.Fatal("DefaultMemoryCache is nil")
	}
	if DefaultMemoryCache.Capacity() != DefaultMemoryCacheCapacity {
		t.Errorf("DefaultMemoryCache.Capacity() = %d, want %d",
			DefaultMemoryCache.Capacity(), DefaultMemoryCacheCapacity)
	}
}
