package jsc

import (
	"strings"
	"testing"
)

func TestBlobURLStoreLifecycle(t *testing.T) {
	u := RegisterBlobURL("https://example.com", "blob-body", "text/plain")
	if u == "" {
		t.Fatal("RegisterBlobURL returned empty URL")
	}
	if !strings.HasPrefix(u, "blob:https://example.com/") {
		t.Errorf("URL = %q, want blob:<origin>/<uuid>", u)
	}

	entry, ok := LookupBlobURL(u)
	if !ok {
		t.Fatalf("LookupBlobURL(%q) = not found", u)
	}
	if entry.Data != "blob-body" {
		t.Errorf("data = %q, want %q", entry.Data, "blob-body")
	}
	if entry.MIMEType != "text/plain" {
		t.Errorf("mime = %q, want %q", entry.MIMEType, "text/plain")
	}

	RevokeBlobURL(u)
	if _, ok := LookupBlobURL(u); ok {
		t.Errorf("revoked URL %q still resolvable", u)
	}
}

func TestBlobURLsAreUnique(t *testing.T) {
	a := RegisterBlobURL("", "a", "")
	b := RegisterBlobURL("", "b", "")
	defer RevokeBlobURL(a)
	defer RevokeBlobURL(b)
	if a == b {
		t.Fatalf("two createObjectURL calls returned the same URL %q", a)
	}
	if !strings.HasPrefix(a, "blob:null/") {
		t.Errorf("empty origin URL = %q, want blob:null/ prefix", a)
	}
}
