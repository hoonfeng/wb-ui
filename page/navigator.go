package page

import (
	"net/url"

	"wb-ui/dom"
)

// --- Location (window.location) ---

// Location mirrors the browser's window.location API. It wraps a
// dom.Document and derives its properties from the document's URL.
type Location struct {
	doc *dom.Document
}

// NewLocation creates a Location backed by the given document.
func NewLocation(doc *dom.Document) *Location {
	return &Location{doc: doc}
}

// Document returns the backing document.
func (l *Location) Document() *dom.Document { return l.doc }

// Href returns the full URL string. Mirrors Location::href().
func (l *Location) Href() string { return l.doc.URL() }

// SetHref navigates to the given URL by setting the document's URL.
// In a full browser this triggers page navigation; in this port it
// only updates the URL string.
func (l *Location) SetHref(h string) { l.doc.SetURL(h) }

// Protocol returns the URL scheme including the trailing colon (e.g. "https:").
func (l *Location) Protocol() string {
	u, err := url.Parse(l.doc.URL())
	if err != nil || u.Scheme == "" {
		return ":"
	}
	return u.Scheme + ":"
}

// Host returns the hostname:port portion of the URL.
func (l *Location) Host() string {
	u, err := url.Parse(l.doc.URL())
	if err != nil {
		return ""
	}
	return u.Host
}

// Hostname returns the hostname portion of the URL.
func (l *Location) Hostname() string {
	u, err := url.Parse(l.doc.URL())
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Port returns the port portion of the URL, or "" if not specified.
func (l *Location) Port() string {
	u, err := url.Parse(l.doc.URL())
	if err != nil {
		return ""
	}
	return u.Port()
}

// Pathname returns the path portion of the URL (e.g. "/page.html").
func (l *Location) Pathname() string {
	u, err := url.Parse(l.doc.URL())
	if err != nil {
		return ""
	}
	return u.Path
}

// Search returns the query string including the leading "?". Mirrors
// Location::search().
func (l *Location) Search() string {
	u, err := url.Parse(l.doc.URL())
	if err != nil {
		return ""
	}
	if u.RawQuery == "" {
		return ""
	}
	return "?" + u.RawQuery
}

// Hash returns the fragment identifier including the leading "#". Mirrors
// Location::hash().
func (l *Location) Hash() string {
	u, err := url.Parse(l.doc.URL())
	if err != nil {
		return ""
	}
	if u.Fragment == "" {
		return ""
	}
	return "#" + u.Fragment
}

// Assign sets the document URL to the given href. Mirrors Location::assign().
func (l *Location) Assign(href string) { l.doc.SetURL(href) }

// Replace sets the document URL without adding a history entry. Mirrors
// Location::replace().
func (l *Location) Replace(href string) { l.doc.SetURL(href) }

// Reload is a no-op in this port (no actual HTTP fetching). Mirrors
// Location::reload().
func (l *Location) Reload() {}

// ToMap serializes the Location properties to a map for JS binding via
// bindings.RegisterGoObject.
func (l *Location) ToMap() map[string]any {
	return map[string]any{
		"href":     l.Href(),
		"protocol": l.Protocol(),
		"host":     l.Host(),
		"hostname": l.Hostname(),
		"port":     l.Port(),
		"pathname": l.Pathname(),
		"search":   l.Search(),
		"hash":     l.Hash(),
	}
}

// --- History (window.history) ---

// HistoryEntry represents a single entry in the session history.
type HistoryEntry struct {
	URL   string
	State any
	Title string
}

// History mirrors the browser's window.history API. It maintains a simple
// in-memory stack of URL/state entries, with a current index.
type History struct {
	entries []HistoryEntry
	current int // index into entries
}

// NewHistory creates an empty session history with one initial entry
// at the given URL.
func NewHistory(initialURL string) *History {
	return &History{
		entries: []HistoryEntry{{URL: initialURL}},
		current: 0,
	}
}

// Length returns the number of entries in the session history, mirroring
// History::length().
func (h *History) Length() int { return len(h.entries) }

// State returns the state object at the current index, mirroring
// History::state().
func (h *History) State() any {
	if h.current < 0 || h.current >= len(h.entries) {
		return nil
	}
	return h.entries[h.current].State
}

// Back navigates to the previous entry. Mirrors History::back().
func (h *History) Back() { h.Go(-1) }

// Forward navigates to the next entry. Mirrors History::forward().
func (h *History) Forward() { h.Go(1) }

// Go navigates delta steps in the history. Mirrors History::go().
func (h *History) Go(delta int) {
	idx := h.current + delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(h.entries) {
		idx = len(h.entries) - 1
	}
	h.current = idx
}

// PushState adds a new entry to the history, mirroring History::pushState().
// If url is non-empty, it is set as the entry's URL. The current position
// becomes this new entry; all entries after the old current are discarded.
func (h *History) PushState(data any, title string, url string) {
	// Trim any entries after the current one.
	h.entries = h.entries[:h.current+1]
	entry := HistoryEntry{State: data, Title: title}
	if url != "" {
		entry.URL = url
	} else if len(h.entries) > 0 {
		entry.URL = h.entries[h.current].URL
	}
	h.entries = append(h.entries, entry)
	h.current = len(h.entries) - 1
}

// ReplaceState replaces the current history entry, mirroring
// History::replaceState().
func (h *History) ReplaceState(data any, title string, url string) {
	if h.current < 0 || h.current >= len(h.entries) {
		return
	}
	h.entries[h.current].State = data
	h.entries[h.current].Title = title
	if url != "" {
		h.entries[h.current].URL = url
	}
}

// CurrentURL returns the URL of the current entry.
func (h *History) CurrentURL() string {
	if h.current < 0 || h.current >= len(h.entries) {
		return ""
	}
	return h.entries[h.current].URL
}

// ToMap serializes the History properties to a map for JS binding.
func (h *History) ToMap() map[string]any {
	return map[string]any{
		"length": h.Length(),
	}
}
