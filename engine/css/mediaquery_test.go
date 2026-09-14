package css

import (
	"testing"
)

func TestParseMediaQueryList_Basic(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"screen"},
		{"print"},
		{"all"},
		{"screen and (max-width: 768px)"},
		{"not print"},
		{"(orientation: landscape)"},
		{"screen, print"},
	}
	for _, tc := range tests {
		queries, err := ParseMediaQueryList(tc.input)
		if err != nil {
			t.Errorf("ParseMediaQueryList(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if len(queries) == 0 {
			t.Errorf("ParseMediaQueryList(%q) returned empty list", tc.input)
		}
	}
}

func TestParseMediaQueryList_Empty(t *testing.T) {
	queries, err := ParseMediaQueryList("")
	if err != nil {
		t.Fatalf("ParseMediaQueryList('') error: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}
	if queries[0].MediaType != "all" {
		t.Fatalf("expected media type 'all', got %q", queries[0].MediaType)
	}
}

func TestParseMediaQueryList_Features(t *testing.T) {
	queries, err := ParseMediaQueryList("(max-width: 600px)")
	if err != nil {
		t.Fatalf("ParseMediaQueryList error: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("expected 1 query, got %d", len(queries))
	}
	q := queries[0]
	if q.MediaType != "" {
		t.Fatalf("expected empty media type, got %q", q.MediaType)
	}
	if len(q.Features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(q.Features))
	}
	if q.Features[0].Name != "max-width" {
		t.Fatalf("expected feature name 'max-width', got %q", q.Features[0].Name)
	}
	if q.Features[0].Value != "600px" {
		t.Fatalf("expected feature value '600px', got %q", q.Features[0].Value)
	}
}

func TestParseMediaQueryList_CommaSeparated(t *testing.T) {
	queries, err := ParseMediaQueryList("screen, print and (min-width: 600px)")
	if err != nil {
		t.Fatalf("ParseMediaQueryList error: %v", err)
	}
	if len(queries) != 2 {
		t.Fatalf("expected 2 queries, got %d", len(queries))
	}
	if queries[0].MediaType != "screen" {
		t.Fatalf("query[0] media type: expected 'screen', got %q", queries[0].MediaType)
	}
	if queries[1].MediaType != "print" {
		t.Fatalf("query[1] media type: expected 'print', got %q", queries[1].MediaType)
	}
	if len(queries[1].Features) != 1 {
		t.Fatalf("query[1] expected 1 feature, got %d", len(queries[1].Features))
	}
}

func TestMatches_SimpleType(t *testing.T) {
	ctx := MediaQueryContext{Width: 800, Height: 600}
	screen, _ := ParseMediaQueryList("screen")
	if !MatchesAny(screen, ctx) {
		t.Error("expected 'screen' to match")
	}
	printMQ, _ := ParseMediaQueryList("print")
	if MatchesAny(printMQ, ctx) {
		t.Error("expected 'print' NOT to match")
	}
}

func TestMatches_MaxWidth(t *testing.T) {
	ctx := MediaQueryContext{Width: 400, Height: 800}
	queries, _ := ParseMediaQueryList("(max-width: 600px)")
	if !MatchesAny(queries, ctx) {
		t.Error("expected (max-width: 600px) to match at width=400")
	}
	queries2, _ := ParseMediaQueryList("(max-width: 300px)")
	if MatchesAny(queries2, ctx) {
		t.Error("expected (max-width: 300px) NOT to match at width=400")
	}
}

func TestMatches_MinWidth(t *testing.T) {
	ctx := MediaQueryContext{Width: 800, Height: 600}
	queries, _ := ParseMediaQueryList("(min-width: 400px)")
	if !MatchesAny(queries, ctx) {
		t.Error("expected (min-width: 400px) to match at width=800")
	}
	queries2, _ := ParseMediaQueryList("(min-width: 900px)")
	if MatchesAny(queries2, ctx) {
		t.Error("expected (min-width: 900px) NOT to match at width=800")
	}
}

func TestMatches_Width(t *testing.T) {
	ctx := MediaQueryContext{Width: 768, Height: 600}
	queries, _ := ParseMediaQueryList("(width: 768px)")
	if !MatchesAny(queries, ctx) {
		t.Error("expected (width: 768px) to match at width=768")
	}
	queries2, _ := ParseMediaQueryList("(width: 600px)")
	if MatchesAny(queries2, ctx) {
		t.Error("expected (width: 600px) NOT to match at width=768")
	}
}

func TestMatches_Compound(t *testing.T) {
	ctx := MediaQueryContext{Width: 800, Height: 600}
	queries, _ := ParseMediaQueryList("screen and (min-width: 400px) and (max-width: 1200px)")
	if !MatchesAny(queries, ctx) {
		t.Error("expected compound query to match")
	}
	queries2, _ := ParseMediaQueryList("screen and (min-width: 900px)")
	if MatchesAny(queries2, ctx) {
		t.Error("expected compound query NOT to match (width < min-width)")
	}
}

func TestMatches_Not(t *testing.T) {
	ctx := MediaQueryContext{Width: 800, Height: 600}
	queries, _ := ParseMediaQueryList("not print")
	if !MatchesAny(queries, ctx) {
		t.Error("expected 'not print' to match on screen")
	}
	queries2, _ := ParseMediaQueryList("not screen")
	if MatchesAny(queries2, ctx) {
		t.Error("expected 'not screen' NOT to match on screen")
	}
}

func TestMatches_CommaSeparated(t *testing.T) {
	ctx := MediaQueryContext{Width: 400, Height: 800}
	queries, _ := ParseMediaQueryList("print, (max-width: 600px)")
	if !MatchesAny(queries, ctx) {
		t.Error("expected comma-separated query to match (second part matches)")
	}
	queries2, _ := ParseMediaQueryList("print, (max-width: 300px)")
	if MatchesAny(queries2, ctx) {
		t.Error("expected comma-separated query NOT to match (neither matches)")
	}
}

func TestMatches_Orientation(t *testing.T) {
	ctxLandscape := MediaQueryContext{Width: 1024, Height: 768, Orientation: "landscape"}
	ctxPortrait := MediaQueryContext{Width: 768, Height: 1024, Orientation: "portrait"}

	queries, _ := ParseMediaQueryList("(orientation: landscape)")
	if !MatchesAny(queries, ctxLandscape) {
		t.Error("expected (orientation: landscape) to match landscape")
	}
	if MatchesAny(queries, ctxPortrait) {
		t.Error("expected (orientation: landscape) NOT to match portrait")
	}
}

func TestMatches_DevicePixelRatio(t *testing.T) {
	ctx1x := MediaQueryContext{DevicePixelRatio: 1.0}
	ctx2x := MediaQueryContext{DevicePixelRatio: 2.0}

	queries, _ := ParseMediaQueryList("(min-device-pixel-ratio: 2)")
	if MatchesAny(queries, ctx1x) {
		t.Error("expected (min-device-pixel-ratio: 2) NOT to match 1x")
	}
	if !MatchesAny(queries, ctx2x) {
		t.Error("expected (min-device-pixel-ratio: 2) to match 2x")
	}
}

func TestMatches_PrefersColorScheme(t *testing.T) {
	ctxDark := MediaQueryContext{PrefersColorScheme: "dark"}
	ctxLight := MediaQueryContext{PrefersColorScheme: "light"}

	dark, _ := ParseMediaQueryList("(prefers-color-scheme: dark)")
	if !MatchesAny(dark, ctxDark) {
		t.Error("expected (prefers-color-scheme: dark) to match dark")
	}
	if MatchesAny(dark, ctxLight) {
		t.Error("expected (prefers-color-scheme: dark) NOT to match light")
	}
}

func TestMatches_Hover(t *testing.T) {
	ctxHover := MediaQueryContext{Hover: "hover"}
	ctxNone := MediaQueryContext{Hover: "none"}

	h, _ := ParseMediaQueryList("(hover: hover)")
	if !MatchesAny(h, ctxHover) {
		t.Error("expected (hover: hover) to match hover")
	}
	if MatchesAny(h, ctxNone) {
		t.Error("expected (hover: hover) NOT to match none")
	}
}

func TestMatches_Pointer(t *testing.T) {
	ctxFine := MediaQueryContext{Pointer: "fine"}
	ctxCoarse := MediaQueryContext{Pointer: "coarse"}

	p, _ := ParseMediaQueryList("(pointer: fine)")
	if !MatchesAny(p, ctxFine) {
		t.Error("expected (pointer: fine) to match fine")
	}
	if MatchesAny(p, ctxCoarse) {
		t.Error("expected (pointer: fine) NOT to match coarse")
	}
}

func TestMatches_Height(t *testing.T) {
	ctx := MediaQueryContext{Height: 900}
	queries, _ := ParseMediaQueryList("(min-height: 600px)")
	if !MatchesAny(queries, ctx) {
		t.Error("expected (min-height: 600px) to match at height=900")
	}
	queries2, _ := ParseMediaQueryList("(max-height: 800px)")
	if MatchesAny(queries2, ctx) {
		t.Error("expected (max-height: 800px) NOT to match at height=900")
	}
}

func TestMatches_EmptyList(t *testing.T) {
	ctx := MediaQueryContext{Width: 800}
	if !MatchesAny(nil, ctx) {
		t.Error("expected nil/empty list to always match")
	}
	if !MatchesAny([]MediaQuery{}, ctx) {
		t.Error("expected empty list to always match")
	}
}

func TestMatches_OnlyQualifier(t *testing.T) {
	ctx := MediaQueryContext{Width: 800, Height: 600}
	queries, _ := ParseMediaQueryList("only screen")
	if !MatchesAny(queries, ctx) {
		t.Error("expected 'only screen' to match on screen")
	}
}
