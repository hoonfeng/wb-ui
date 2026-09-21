package main

import "testing"

// SVG 像素级用例。
//
// 两个回归点：
//  ① 渐变 + 多子路径：<path d="M…z M…z"> 的多个子路径必须各自独立填充，
//     不能连成一条多边形（否则出现幻觉斜线、渐变区域填错）。
//     samplePoints() 是扁平点序列，只可用于渐变轴向计算；绘制必须走
//     sampleSegments + segmentsToPath + Canvas.FillPathGradientPath。
//  ② clipPath 的两项单位语义：子元素自身的 transform、以及
//     clipPathUnits="objectBoundingBox"（0..1 相对引用元素 bbox）。

// TestPxSVGGradientMultiSubpath: 两个子路径各填渐变，中间空隙必须透明。
func TestPxSVGGradientMultiSubpath(t *testing.T) {
	pixelCompare(t, "px_svg_grad_multisub", `
		<svg width="220" height="110" viewBox="0 0 220 110" style="display:block">
			<defs>
				<linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
					<stop offset="0" stop-color="#ff0000"/>
					<stop offset="1" stop-color="#0000ff"/>
				</linearGradient>
			</defs>
			<path d="M10 10 H80 V80 H10 Z M140 10 H210 V80 H140 Z" fill="url(#g)"/>
		</svg>
	`, 0.01)
}

// TestPxSVGClipUnits: clipPath 子元素带 transform，且第二个引用元素的
// clipPath 用 objectBoundingBox（0..1 归一化坐标随元素 bbox 缩放）。
func TestPxSVGClipUnits(t *testing.T) {
	pixelCompare(t, "px_svg_clip_units", `
		<svg width="220" height="120" viewBox="0 0 220 120" style="display:block">
			<defs>
				<clipPath id="cUser">
					<rect x="0" y="0" width="40" height="60" transform="translate(10,10)"/>
				</clipPath>
				<clipPath id="cObj" clipPathUnits="objectBoundingBox">
					<rect x="0.5" y="0.25" width="0.5" height="0.5"/>
				</clipPath>
			</defs>
			<rect x="0" y="0" width="80" height="80" fill="#ff0000" clip-path="url(#cUser)"/>
			<rect x="100" y="10" width="100" height="80" fill="#008000" clip-path="url(#cObj)"/>
		</svg>
	`, 0.01)
}

// TestPxSVGGradientStrokeMultiSubpath: 渐变描边 + 多子路径。
//
// `M…z M…z` 的两个子路径必须各自描渐变（中间为空），不能连成一条折线：
// StrokePathGradient 收的是扁平点序列（MoveTo 首点 + LineTo 其余全部），
// 会把子路径之间连出「幻觉斜线」。修复后由调用方用 segmentsToPath 建独立
// 子路径，再走 Canvas.StrokePathGradientPath（只造 stroke shader + DrawPath）；
// 渐变轴/bbox 仍用扁平点算。
//
// 第三个元素是【单子路径】渐变描边的对照（确认未回归）。
func TestPxSVGGradientStrokeMultiSubpath(t *testing.T) {
	pixelCompare(t, "px_svg_grad_stroke", `
		<svg width="240" height="200" viewBox="0 0 240 200" style="display:block">
			<defs>
				<linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
					<stop offset="0" stop-color="#ff0000"/>
					<stop offset="1" stop-color="#0000ff"/>
				</linearGradient>
			</defs>
			<path d="M10 10 H80 V80 H10 Z M140 10 H210 V80 H140 Z"
			      fill="none" stroke="url(#g)" stroke-width="6"/>
			<path d="M20 120 H200 V180 H20 Z"
			      fill="none" stroke="url(#g)" stroke-width="6"/>
		</svg>
	`, 0.01)
}
