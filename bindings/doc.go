// Package bindings implements Go <-> JS <-> DOM bridges
// (Source/WebCore/bindings) in Go.
//
//   - go2js.go:    register Go functions as JS global methods (Go->JS bridge)
//   - js_to_go.go: convert JSValue back to Go values (JS->Go bridge)
//   - dom.go:      expose DOM API to JS (document.getElementById etc.)
//   - dom_events.go: bridge JS event listeners to/from Go dom.EventTarget
//
// Note: the Go->JS bridge file is named go2js.go rather than go_to_js.go because Go
// applies an implicit GOOS=js build constraint to any file whose name ends in
// _js.go (or _js_test.go), which would exclude it from compilation on every other
// platform and break `go build ./...`.
package bindings
