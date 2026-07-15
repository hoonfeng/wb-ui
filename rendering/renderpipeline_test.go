package rendering

import (
	"testing"

	"wb-ui/dom"
	"wb-ui/platform/graphics"
	"wb-ui/style"
)

// TestDirtyRect_MarkAndClear tests basic dirty rect marking and clearing.
func TestDirtyRect_MarkAndClear(t *testing.T) {
	doc := dom.NewDocument()
	st := &style.ComputedStyle{}
	rv := NewRenderView(doc, st)
	rv.SetViewportSize(800, 600)

	if rv.IsDirty() {
		t.Fatal("expected clean after creation")
	}

	rv.MarkDirty(Rect{X: 10, Y: 20, Width: 100, Height: 200})
	if !rv.IsDirty() {
		t.Fatal("expected dirty after MarkDirty")
	}
	dr := rv.GetDirtyRect()
	if dr.X != 10 || dr.Y != 20 || dr.Width != 100 || dr.Height != 200 {
		t.Fatalf("dirty rect = %+v, want (10,20,100,200)", dr)
	}

	rv.ClearDirty()
	if rv.IsDirty() {
		t.Fatal("expected clean after ClearDirty")
	}
}

// TestDirtyRect_Union tests that MarkDirty unions with existing dirty rect.
func TestDirtyRect_Union(t *testing.T) {
	doc := dom.NewDocument()
	st := &style.ComputedStyle{}
	rv := NewRenderView(doc, st)
	rv.SetViewportSize(800, 600)

	rv.MarkDirty(Rect{X: 0, Y: 0, Width: 100, Height: 100})
	rv.MarkDirty(Rect{X: 200, Y: 200, Width: 100, Height: 100})

	dr := rv.GetDirtyRect()
	if dr.X != 0 || dr.Y != 0 {
		t.Fatalf("union origin = (%v,%v), want (0,0)", dr.X, dr.Y)
	}
	if dr.Width != 300 || dr.Height != 300 {
		t.Fatalf("union size = (%v,%v), want (300,300)", dr.Width, dr.Height)
	}
}

// TestDirtyRect_MarkAllDirty tests MarkAllDirty marks the entire viewport.
func TestDirtyRect_MarkAllDirty(t *testing.T) {
	doc := dom.NewDocument()
	st := &style.ComputedStyle{}
	rv := NewRenderView(doc, st)
	rv.SetViewportSize(1024, 768)

	rv.MarkAllDirty()
	dr := rv.GetDirtyRect()
	if dr.Width != 1024 || dr.Height != 768 {
		t.Fatalf("MarkAllDirty = %+v, want (0,0,1024,768)", dr)
	}
}

// TestScrollOffset tests that scroll offset is stored and returned correctly.
func TestScrollOffset(t *testing.T) {
	doc := dom.NewDocument()
	st := &style.ComputedStyle{}
	rv := NewRenderView(doc, st)
	rv.SetViewportSize(800, 600)

	sx, sy := rv.ScrollOffset()
	if sx != 0 || sy != 0 {
		t.Fatalf("initial scroll = (%v,%v), want (0,0)", sx, sy)
	}

	rv.SetScrollOffset(100, 200)
	sx, sy = rv.ScrollOffset()
	if sx != 100 || sy != 200 {
		t.Fatalf("scroll = (%v,%v), want (100,200)", sx, sy)
	}

	// Setting scroll offset should mark all dirty
	if !rv.IsDirty() {
		t.Fatal("expected dirty after SetScrollOffset")
	}
}

// TestPaintInfo_DirtyCheckEnabled tests the dirty check flag behavior.
func TestPaintInfo_DirtyCheckEnabled(t *testing.T) {
	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()

	info := NewPaintInfo(canvas, Rect{X: 10, Y: 10, Width: 20, Height: 20})

	// Default should be enabled
	if !info.DirtyCheckEnabled() {
		t.Fatal("expected dirtyCheckEnabled by default")
	}

	// Intersects within dirty rect
	if !info.intersects(Rect{X: 15, Y: 15, Width: 5, Height: 5}) {
		t.Fatal("expected intersects for rect inside dirty rect")
	}

	// Intersects outside dirty rect
	if info.intersects(Rect{X: 100, Y: 100, Width: 5, Height: 5}) {
		t.Fatal("expected no intersect for rect outside dirty rect")
	}

	// Disable dirty check — everything should intersect
	info.SetDirtyCheckEnabled(false)
	if !info.intersects(Rect{X: 100, Y: 100, Width: 5, Height: 5}) {
		t.Fatal("expected intersect when dirtyCheckEnabled=false")
	}
}

// TestPaint_WithScrollOffset creates a simple RenderView, sets scroll offset,
// and verifies Paint does not crash (the translate is applied correctly).
func TestPaint_WithScrollOffset(t *testing.T) {
	doc := dom.NewDocument()
	st := &style.ComputedStyle{}
	rv := NewRenderView(doc, st)
	rv.SetViewportSize(100, 100)
	rv.SetScrollOffset(50, 25)

	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()

	// Should not panic: scroll translate applied, then cleared.
	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})
}

// TestPaint_WithDirtyRect creates a RenderView, marks a dirty rect,
// and verifies Paint clears the dirty flag after painting.
func TestPaint_WithDirtyRect(t *testing.T) {
	doc := dom.NewDocument()
	st := &style.ComputedStyle{}
	rv := NewRenderView(doc, st)
	rv.SetViewportSize(100, 100)
	rv.MarkDirty(Rect{X: 0, Y: 0, Width: 50, Height: 50})

	canvas := graphics.NewCanvas(100, 100)
	defer canvas.Release()

	Paint(rv, canvas, Rect{X: 0, Y: 0, Width: 100, Height: 100})

	// After Paint, dirty rect should be cleared
	if rv.IsDirty() {
		t.Fatal("expected dirty rect cleared after Paint")
	}
}
