package page

import (
	"testing"
)

func TestLocation_Href(t *testing.T) {
	page := NewPage(NewSettings())
	frame := page.MainFrame()
	frame.LoadHTML("<html><body></body></html>")
	doc := frame.Document()
	doc.SetURL("https://example.com/page.html?q=hello#section")

	loc := NewLocation(doc)
	if loc.Href() != "https://example.com/page.html?q=hello#section" {
		t.Errorf("Href() = %q, want %q", loc.Href(), "https://example.com/page.html?q=hello#section")
	}
	if loc.Protocol() != "https:" {
		t.Errorf("Protocol() = %q, want 'https:'", loc.Protocol())
	}
	if loc.Host() != "example.com" {
		t.Errorf("Host() = %q, want 'example.com'", loc.Host())
	}
	if loc.Hostname() != "example.com" {
		t.Errorf("Hostname() = %q, want 'example.com'", loc.Hostname())
	}
	if loc.Port() != "" {
		t.Errorf("Port() = %q, want ''", loc.Port())
	}
	if loc.Pathname() != "/page.html" {
		t.Errorf("Pathname() = %q, want '/page.html'", loc.Pathname())
	}
	if loc.Search() != "?q=hello" {
		t.Errorf("Search() = %q, want '?q=hello'", loc.Search())
	}
	if loc.Hash() != "#section" {
		t.Errorf("Hash() = %q, want '#section'", loc.Hash())
	}
}

func TestLocation_NoURL(t *testing.T) {
	page := NewPage(NewSettings())
	frame := page.MainFrame()
	frame.LoadHTML("<html><body></body></html>")
	loc := NewLocation(frame.Document())
	if loc.Host() != "" {
		t.Errorf("Host() = %q, want '' for empty URL", loc.Host())
	}
}

func TestLocation_SetHref(t *testing.T) {
	page := NewPage(NewSettings())
	frame := page.MainFrame()
	frame.LoadHTML("<html><body></body></html>")
	doc := frame.Document()
	loc := NewLocation(doc)
	loc.SetHref("http://new.example.com/page")
	if doc.URL() != "http://new.example.com/page" {
		t.Errorf("doc.URL() = %q, want 'http://new.example.com/page'", doc.URL())
	}
}

func TestLocation_AssignAndReplace(t *testing.T) {
	page := NewPage(NewSettings())
	frame := page.MainFrame()
	frame.LoadHTML("<html><body></body></html>")
	doc := frame.Document()
	loc := NewLocation(doc)
	loc.Assign("http://assigned.com/")
	if doc.URL() != "http://assigned.com/" {
		t.Errorf("doc.URL() = %q after Assign", doc.URL())
	}
	loc.Replace("http://replaced.com/")
	if doc.URL() != "http://replaced.com/" {
		t.Errorf("doc.URL() = %q after Replace", doc.URL())
	}
}

func TestLocation_WithPort(t *testing.T) {
	page := NewPage(NewSettings())
	frame := page.MainFrame()
	frame.LoadHTML("<html><body></body></html>")
	doc := frame.Document()
	doc.SetURL("http://example.com:8080/path")
	loc := NewLocation(doc)
	if loc.Host() != "example.com:8080" {
		t.Errorf("Host() = %q, want 'example.com:8080'", loc.Host())
	}
	if loc.Port() != "8080" {
		t.Errorf("Port() = %q, want '8080'", loc.Port())
	}
}

func TestHistory_Initial(t *testing.T) {
	h := NewHistory("http://example.com/")
	if h.Length() != 1 {
		t.Errorf("Length() = %d, want 1", h.Length())
	}
	if h.CurrentURL() != "http://example.com/" {
		t.Errorf("CurrentURL() = %q, want 'http://example.com/'", h.CurrentURL())
	}
	if h.State() != nil {
		t.Errorf("State() = %v, want nil", h.State())
	}
}

func TestHistory_PushState(t *testing.T) {
	h := NewHistory("http://example.com/")
	h.PushState("page1", "Page 1", "/page1")
	if h.Length() != 2 {
		t.Errorf("Length() = %d, want 2", h.Length())
	}
	if h.CurrentURL() != "/page1" {
		t.Errorf("CurrentURL() = %q, want '/page1'", h.CurrentURL())
	}
	if h.State() != "page1" {
		t.Errorf("State() = %v, want 'page1'", h.State())
	}
}

func TestHistory_BackForward(t *testing.T) {
	h := NewHistory("http://example.com/")
	h.PushState("p1", "", "/p1")
	h.PushState("p2", "", "/p2")
	h.Back()
	if h.CurrentURL() != "/p1" {
		t.Errorf("After Back: CurrentURL() = %q, want '/p1'", h.CurrentURL())
	}
	if h.State() != "p1" {
		t.Errorf("After Back: State() = %v, want 'p1'", h.State())
	}
	h.Back()
	if h.CurrentURL() != "http://example.com/" {
		t.Errorf("After 2nd Back: CurrentURL() = %q, want initial URL", h.CurrentURL())
	}
	// Can't go back further
	h.Back()
	if h.CurrentURL() != "http://example.com/" {
		t.Errorf("After 3rd Back: CurrentURL() = %q, still at initial", h.CurrentURL())
	}
	h.Forward()
	if h.CurrentURL() != "/p1" {
		t.Errorf("After Forward: CurrentURL() = %q, want '/p1'", h.CurrentURL())
	}
}

func TestHistory_ReplaceState(t *testing.T) {
	h := NewHistory("http://example.com/")
	h.PushState("old", "", "/page")
	h.ReplaceState("new", "New Title", "/newpage")
	if h.CurrentURL() != "/newpage" {
		t.Errorf("CurrentURL() = %q, want '/newpage'", h.CurrentURL())
	}
	if h.State() != "new" {
		t.Errorf("State() = %v, want 'new'", h.State())
	}
	if h.Length() != 2 {
		t.Errorf("Length() = %d, want 2 (replaceState doesn't add)", h.Length())
	}
}

func TestHistory_PushStateTrimsFuture(t *testing.T) {
	h := NewHistory("http://example.com/")
	h.PushState("a", "", "/a")
	h.PushState("b", "", "/b")
	h.PushState("c", "", "/c")
	h.Back()       // at /b
	h.Back()       // at /a
	h.PushState("d", "", "/d") // should trim /b and /c
	if h.Length() != 3 {
		t.Errorf("Length() = %d, want 3 (initial, /a, /d)", h.Length())
	}
	if h.CurrentURL() != "/d" {
		t.Errorf("CurrentURL() = %q, want '/d'", h.CurrentURL())
	}
}

func TestHistory_GoDelta(t *testing.T) {
	h := NewHistory("http://example.com/")
	h.PushState("p1", "", "/p1")
	h.PushState("p2", "", "/p2")
	h.Go(-2)
	if h.CurrentURL() != "http://example.com/" {
		t.Errorf("Go(-2): CurrentURL() = %q, want initial", h.CurrentURL())
	}
	h.Go(10) // clamp to last
	if h.CurrentURL() != "/p2" {
		t.Errorf("Go(10): CurrentURL() = %q, want '/p2'", h.CurrentURL())
	}
}
