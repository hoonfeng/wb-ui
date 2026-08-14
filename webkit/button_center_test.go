package webkit

import (
	"testing"
)

// TestButtonTextVerticalCenter 验证 <button> 文字垂直居中：渲染一个
// 22x22 的 xnum − 按钮（配置器属性面板样式），扫描按钮内白色文字像素，
// 计算 glyph 垂直中心，应与按钮中心（y=21）接近（偏差 ≤1.5px）。
func TestButtonTextVerticalCenter(t *testing.T) {
	wv := NewWebView()
	wv.Resize(100, 60)
	wv.LoadHTML(`<html><head><style>
*{box-sizing:border-box}
body{margin:0;background:#000;font-family:'Segoe UI','Microsoft YaHei',sans-serif}
.xnum button{width:22px;height:22px;background:#263352;border:1px solid #35456e;color:#fff;
             border-radius:4px;cursor:pointer;font-size:12px;line-height:1}
</style></head><body>
<div class="xnum" style="position:absolute;left:10px;top:10px">
  <button>−</button>
</div>
</body></html>`)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	brect, err := wv.EvalJS(`JSON.stringify(document.querySelector('button').getBoundingClientRect())`)
	if err == nil {
		t.Logf("button rect = %s", brect.ToString())
	}
	// 先测半透明阈值（>100）找完整字形范围
	minY2, maxY2, count2 := -1, -1, 0
	for y := 8; y < 36; y++ {
		for x := 8; x < 36; x++ {
			i := (y*100 + x) * 4
			r, g, b, a := pix[i], pix[i+1], pix[i+2], pix[i+3]
			if r > 100 && g > 100 && b > 100 && a > 100 {
				count2++
				if minY2 < 0 || y < minY2 {
					minY2 = y
				}
				if y > maxY2 {
					maxY2 = y
				}
			}
		}
	}
	if count2 > 0 {
		center2 := float64(minY2+maxY2) / 2
		t.Logf("glyph soft pixels=%d y-range=[%d,%d] center=%.1f (button center=21)", count2, minY2, maxY2, center2)
	}
	// 白色像素（>200）范围
	minY, maxY, count := -1, -1, 0
	for y := 8; y < 36; y++ {
		for x := 8; x < 36; x++ {
			i := (y*100 + x) * 4
			r, g, b := pix[i], pix[i+1], pix[i+2]
			if r > 200 && g > 200 && b > 200 {
				count++
				if minY < 0 || y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if count == 0 {
		t.Fatalf("no white glyph pixels found")
	}
	center := float64(minY+maxY) / 2
	t.Logf("glyph white pixels=%d y-range=[%d,%d] center=%.1f (button center=21)", count, minY, maxY, center)
	if center < 19.5 || center > 22.5 {
		t.Errorf("glyph vertical center %.1f too far from button center 21 (range 19.5..22.5)", center)
	}
}

// TestButtonTextCJK 用中文"确定"按钮验证垂直居中（CJK glyph 大、像素多，
// 测量可靠）。
func TestButtonTextCJK(t *testing.T) {
	wv := NewWebView()
	wv.Resize(200, 100)
	wv.LoadHTML(`<html><head><style>
*{box-sizing:border-box}
body{margin:0;background:#000;font-family:'Segoe UI','Microsoft YaHei',sans-serif}
button{width:80px;height:32px;background:#3b6fd4;border:1px solid #35456e;color:#fff;
       border-radius:4px;cursor:pointer;font-size:14px;padding:0;line-height:1}
</style></head><body>
<button id="b" style="position:absolute;left:20px;top:20px">确定</button>
</body></html>`)
	if mf := wv.MainFrame(); mf != nil {
		if fr := mf.Frame(); fr != nil {
			fr.RebuildRenderTree()
			fr.SetNeedsLayout(true)
		}
	}
	wv.EnsureLayout()
	pix, err := wv.Render()
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	brect, err := wv.EvalJS(`JSON.stringify(document.querySelector('#b').getBoundingClientRect())`)
	if err == nil {
		t.Logf("button rect = %s", brect.ToString())
	}
	// 按钮 (20,20)-(100,52)，中心 y=36
	minY, maxY, count := -1, -1, 0
	for y := 18; y < 56; y++ {
		for x := 18; x < 104; x++ {
			i := (y*200 + x) * 4
			r, g, b := pix[i], pix[i+1], pix[i+2]
			if r > 200 && g > 200 && b > 200 {
				count++
				if minY < 0 || y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}
	if count == 0 {
		t.Fatalf("no white glyph pixels found")
	}
	center := float64(minY+maxY) / 2
	t.Logf("glyph white pixels=%d y-range=[%d,%d] center=%.1f (button center=36)", count, minY, maxY, center)
	if center < 34 || center > 38 {
		t.Errorf("glyph vertical center %.1f too far from button center 36 (range 34..38)", center)
	}
}
