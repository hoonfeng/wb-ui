package rendering

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestBackgroundImageAsyncHTTP: an http(s) background image loads off-thread.
// The first load call returns nil (not loaded yet); after the fetch the
// cache serves it and the loaded callback fires.
func TestBackgroundImageAsyncHTTP(t *testing.T) {
	// Build a small red PNG served over http.
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	// Initial load: returns nil (async in flight).
	if got := loadBackgroundImage(srv.URL, ""); got != nil {
		t.Fatalf("first load returned %v, want nil (async)", got)
	}

	// Callback must fire with the URL.
	var mu sync.Mutex
	gotURL := ""
	SetBackgroundImageLoadedCallback(func(u string) {
		mu.Lock()
		gotURL = u
		mu.Unlock()
	})
	defer SetBackgroundImageLoadedCallback(nil)

	// Poll until cached.
	deadline := time.Now().Add(5 * time.Second)
	for {
		backgroundImageCache.mu.Lock()
		cached := backgroundImageCache.imgs[srv.URL]
		loading := backgroundImageCache.loading[srv.URL]
		backgroundImageCache.mu.Unlock()
		if cached != nil && !loading {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for async load (cached=%v loading=%v)", cached != nil, loading)
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	if gotURL != srv.URL {
		t.Fatalf("callback url=%q, want %q", gotURL, srv.URL)
	}
	mu.Unlock()

	// Now the cache serves it synchronously.
	got := loadBackgroundImage(srv.URL, "")
	if got == nil || !got.Loaded() {
		t.Fatalf("cached load returned nil/unloaded")
	}
	if got.Width() != 4 || got.Height() != 4 {
		t.Fatalf("size = %vx%v, want 4x4", got.Width(), got.Height())
	}
}
