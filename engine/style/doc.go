// Package style implements the style resolver and computed style
// (Source/WebCore/style) in Go.
//
// It contains StyleResolver.cpp and ComputedStyle translations that consume
// CSS rules produced by package css and produce per-element computed style
// used by the layout engine.
package style
