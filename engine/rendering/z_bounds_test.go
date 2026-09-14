package rendering

import (
	"math"
	"testing"
)

func TestTransformRect(t *testing.T) {
	// 1) translate(-50%,-50%)：弹窗居中（refW=280, refH=121.6, 布局 (640,400)）
	nx, ny, nw, nh, ok := TransformRect("translate(-50%,-50%)", 280, 121.6, 640, 400, 280, 121.6)
	if !ok || math.Abs(nx-500) > 0.01 || math.Abs(ny-339.2) > 0.01 ||
		math.Abs(nw-280) > 0.01 || math.Abs(nh-121.6) > 0.01 {
		t.Fatalf("translate: got (%v,%v,%v,%v) ok=%v want (500,339.2,280,121.6)", nx, ny, nw, nh, ok)
	}
	// 2) translateX(10px) + scale(2)：矩形 (0,0,10,20) → 角点 (10,0)-(30,40)
	nx, ny, nw, nh, ok = TransformRect("translateX(10px) scale(2)", 0, 0, 0, 0, 10, 20)
	if !ok || math.Abs(nx-10) > 0.01 || math.Abs(ny-0) > 0.01 ||
		math.Abs(nw-20) > 0.01 || math.Abs(nh-40) > 0.01 {
		t.Fatalf("translate+scale: got (%v,%v,%v,%v)", nx, ny, nw, nh)
	}
	// 3) rotate(90deg)：矩形 (0,0,10,20) 绕原点 → 包围盒 (-20,0,20,10)
	nx, ny, nw, nh, ok = TransformRect("rotate(90deg)", 0, 0, 0, 0, 10, 20)
	if !ok || math.Abs(nx-(-20)) > 0.01 || math.Abs(ny-0) > 0.01 ||
		math.Abs(nw-20) > 0.01 || math.Abs(nh-10) > 0.01 {
		t.Fatalf("rotate90: got (%v,%v,%v,%v)", nx, ny, nw, nh)
	}
	// 4) none / 空 → 不变
	nx, ny, nw, nh, ok = TransformRect("none", 280, 100, 640, 400, 280, 100)
	if ok || nx != 640 || ny != 400 || nw != 280 || nh != 100 {
		t.Fatalf("none: got (%v,%v,%v,%v) ok=%v", nx, ny, nw, nh, ok)
	}
	// 5) matrix(1,0,0,1,50,20) → 平移
	nx, ny, _, _, ok = TransformRect("matrix(1,0,0,1,50,20)", 0, 0, 100, 200, 10, 10)
	if !ok || math.Abs(nx-150) > 0.01 || math.Abs(ny-220) > 0.01 {
		t.Fatalf("matrix: got (%v,%v)", nx, ny)
	}
}
