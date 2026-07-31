// Case definitions for the consistency verification suite.

package main

import "fmt"

// TestCase describes one consistency check: an HTML document rendered by both
// Edge and wb-ui, with per-element data compared.
type TestCase struct {
	Name string // unique case name
	Desc string // human-readable description
	HTML string // full HTML document
	// Viewport used by both renderers.
	ViewportW, ViewportH int
}

// ElementSnapshot is one element's observable state, collected by both
// renderers so they can be compared directly.
type ElementSnapshot struct {
	Tag     string // e.g. "div"
	ID      string
	Class   string
	X, Y    float64
	W, H    float64
	Display string // computed display
	Color   string // computed color (rgb string)
	BG      string // computed background-color
	FontSz  string // computed font-size
	// Interaction state (filled by interaction cases).
	TextContent string
	Checked     string // "true"/"false" for checkbox/radio
	Value       string // input value
	Count       int    // JS event counter
}

// CaseResult aggregates the comparison for one case.
type CaseResult struct {
	Case    TestCase
	Edge    []ElementSnapshot
	WBUi    []ElementSnapshot
	Matches []ElementMatch // per-element verdict
	Warnings []string
	Err     error
}

// ElementMatch is the verdict for one element.
type ElementMatch struct {
	Key     string // id or tag#index
	Fields  []FieldDiff
	Passed  bool
}

// FieldDiff records a single differing observable field.
type FieldDiff struct {
	Field    string
	EdgeVal  string
	WBUiVal  string
	Tolerance string // e.g. "±2px" when numeric tolerance applied
}

// Passed reports whether every element matched within tolerance.
func (r *CaseResult) Passed() bool {
	if r.Err != nil {
		return false
	}
	for _, m := range r.Matches {
		if !m.Passed {
			return false
		}
	}
	return len(r.Matches) > 0
}

// Summary renders a one-line result summary.
func (r *CaseResult) Summary() string {
	if r.Err != nil {
		return fmt.Sprintf("ERROR: %v", r.Err)
	}
	total, failed := 0, 0
	for _, m := range r.Matches {
		total++
		if !m.Passed {
			failed++
		}
	}
	if failed == 0 {
		return fmt.Sprintf("OK  (%d elements matched)", total)
	}
	return fmt.Sprintf("FAIL (%d/%d elements differ)", failed, total)
}
