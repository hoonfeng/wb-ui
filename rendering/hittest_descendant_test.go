// TestHitTestDescendantBeatsLargerArea — regression test for the desktop
// title-bar "帮助" menu: a click on the menu button resolved to its parent
// flex container instead of the button, so the dropdown never opened.
//
// The area-based pick (smallest bounding box wins) breaks when a child is
// LARGER than its parent — .menu-btn is 46x30 while .menubar is 46x29
// (the button's 30px height exceeds the 29px bar by a pixel), so the
// container won and the click bubbled to the titlebar's close-all handler.
// Browsers always resolve to the deepest element; the pick rule now gives
// DOM-descendant candidates priority over smaller-area ancestors.
package rendering_test

import (
	"testing"

	"wb-ui/rendering"
)

func TestHitTestDescendantBeatsLargerArea(t *testing.T) {
	doc, rv, _, _ := mkRuntime(t, `<div class="menubar" style="display:flex;align-items:center;height:29px;width:46px;background:#222">
  <button class="menu-btn" style="height:30px;width:46px">帮助</button>
</div>`)
	btn := findEl(t, doc, "menu-btn")

	x, y := hitCenter(t, rv, "menu-btn")
	deepest := rendering.HitTest(rv, x, y, "")
	if deepest == nil {
		t.Fatal("HitTest nil")
	}
	if deepest != btn {
		t.Fatalf("HitTest = %v, want button.menu-btn (deeper element wins even with larger area)", deepest)
	}

	// Also verify the deepest-priority rule doesn't break the "click the
	// container itself" case: a point inside the bar but outside the button
	// (bar is 46 wide, button fills it — so use a wider bar with the button
	// on the left, click the right side) still resolves to the container.
	doc2, rv2, _, _ := mkRuntime(t, `<div class="bar" style="display:flex;align-items:center;height:29px;width:200px;background:#222">
  <button class="menu-btn2" style="height:30px;width:46px">帮助</button>
</div>`)
	bar := findEl(t, doc2, "bar")
	// bar 宽 200，按钮在左 0..46；点击 x=150 处只命中 bar
	hit := rendering.HitTest(rv2, 150, 14, "")
	if hit != bar {
		t.Fatalf("HitTest(150,14) = %v, want the bar container", hit)
	}
}
