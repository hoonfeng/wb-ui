// Package html5 defaultcss provides the User-Agent (UA) default stylesheet
// for HTML form controls, mirroring WebCore/css/html.css.
// The stylesheet is injected into the style resolver with OriginUserAgent so
// that author stylesheets override it via the cascade.

package html5

import (
	"wb-ui/css"
)

// UAStyleSheetCSS is the CSS text for form control default styles, mirroring
// the relevant portions of WebCore/css/html.css. It covers form-associated
// elements (form, fieldset, legend, label, input, button, select, textarea,
// option, optgroup, datalist, output, progress, meter) plus a few structural
// defaults that affect form layout.
const UAStyleSheetCSS = `
/* --- Form element defaults (mirrors WebCore/css/html.css form section) --- */

form {
	display: block;
}

/* Default focus ring. Chrome/Edge UA uses :focus-visible, so mouse clicks do
 * NOT draw the ring (only keyboard Tab focus does). User styles targeting
 * :focus (e.g. .textarea:focus) still match on any focus. */
:focus-visible {
	outline: 1px solid #4d90fe;
	outline-offset: 0px;
}

fieldset {
	display: block;
	margin: 0 2px 0.8em 0;
	padding: 0.8em 1em 1em;
	border: 2px groove #c0c0c0;
}

legend {
	display: block;
	padding: 0 0.3em;
}

label {
	cursor: default;
}

/* Replaced elements: input, button, select, textarea are inline-block by
   default so they sit on the text baseline. The 13.33px font matches
   WebCore's -webkit-small-control default for form controls. */
input, button, select, textarea {
	display: inline-block;
	font-family: inherit;
	font-size: 13.3333px;
	color: inherit;
	vertical-align: middle;
}

/* Text-like controls share Chromium's geometry: the 2px control border with
   1px 2px padding is what turns an author width:100px into a 108px border
   box (form-control-geometry "author dimensions use content box"). The default
   sizing is content-box — quirks mode switches controls to border-box (see
   style.applyFormControlUserAgentDefaults). */
input {
	padding: 1px 2px;
	border: 2px solid #767676;
	background-color: #ffffff;
}

textarea {
	padding: 2px;
	border: 2px solid #767676;
	background-color: #ffffff;
	resize: both;
	overflow: auto;
	/* Browser UA default: soft-wrap at any character. An explicit
	   white-space: pre / nowrap overrides this and scrolls horizontally. */
	white-space: pre-wrap;
}

input[type="color"] {
	width: 2em;
	height: 1.5em;
	padding: 1px;
	border: 1px solid #c0c0c0;
}

/* Checkbox and radio are fixed 13×13 UA boxes carrying Chromium's margins
   (checkbox 3px 3px 3px 4px, radio 3px 3px 0 5px) and no border/padding of
   their own — the generic input rule above must not box them in. */
input[type="checkbox"], input[type="radio"] {
	display: inline-block;
	width: 13px;
	height: 13px;
	padding: 0;
	border: none;
	margin: 3px 3px 3px 4px;
	vertical-align: baseline;
}

input[type="radio"] {
	margin: 3px 3px 0 5px;
}

/* Range input renders as a slider. The background stays TRANSPARENT —
   browsers give appearance-based controls (range/checkbox/radio) a
   transparent background (the track/thumb are drawn by the theme, and a
   background-color would paint an opaque bar behind the slider). */
input[type="range"] {
	display: inline-block;
	/* Chromium: 129×16 (the width no longer scales with the control font, so
	   it stays 129px whatever font-size the page sets). */
	width: 129px;
	/* 高度对齐浏览器：Edge(Chromium) 实测 range 约 21px（1.6em @ 13px）。
	   此前 1.2em(≈16px) 导致温度行 row 高 24 vs 浏览器 30，modal 总高
	   少 5px（用户反馈「设置UI高度不对」）。 */
	height: 1.6em;
	padding: 0;
	border: none;
	margin: 2px;
	background-color: transparent;
	color: #101010;
	overflow: hidden;
}

/* File input. */
input[type="file"] {
	padding: 2px;
}

/* Hidden inputs have no box. */
input[type="hidden"] {
	display: none;
}

/* Image input behaves like img. */
input[type="image"] {
	display: inline-block;
}

/* Submit/reset/button inputs and <button> share button styling. The
   background mirrors Edge's Windows-style button: white body with a
   #efefef highlight band across the top ~25%, and a #767676 border. */
input[type="submit"], input[type="reset"], input[type="button"],
button {
	display: inline-block;
	padding: 1px 6px;
	border: 1px solid #767676;
	background-color: #f0f0f0;
	background-image: linear-gradient(to bottom, #efefef 0%, #efefef 28%, #ffffff 28%);
	color: #000000;
	text-align: center;
	cursor: default;
	box-sizing: border-box;
	-webkit-appearance: button;
}

button[disabled], input[disabled] {
	color: #808080;
}

/* Select and option. */
select {
	display: inline-block;
	padding: 1px;
	border: 1px solid #767676;
	background-color: #ffffff;
	box-sizing: border-box;
}

option {
	display: block;
	padding: 0 0.3em;
}

optgroup {
	display: block;
	font-weight: bold;
	padding: 0 0.3em;
}

optgroup option {
	font-weight: normal;
	padding-left: 1.2em;
}

/* Datalist is not rendered. */
datalist {
	display: none;
}

/* Output is inline. */
output {
	display: inline;
}

/* Progress and meter. */
progress {
	display: inline-block;
	width: 10em;
	height: 1em;
	vertical-align: middle;
}

meter {
	display: inline-block;
	width: 6em;
	height: 1em;
	vertical-align: middle;
}

/* Submit button alignment fix. */
input[type="submit"]::-webkit-inner-button,
input[type="reset"]::-webkit-inner-button,
input[type="button"]::-webkit-inner-button {
	padding: 0;
}

/* Details / Summary disclosure widget. */
details {
	display: block;
}

details > summary {
	display: block;
	cursor: pointer;
}

/* When details is not open, hide everything except the first summary. */
details:not([open]) > :not(summary) {
	display: none !important;
}

/* ── Table element defaults (mirrors WebCore/css/html.css table section) ── */
table {
	display: table;
	border-collapse: separate;
	/* border-spacing 的初始值是 0，但浏览器的 UA 样式表把表格默认间距设为
	   2px（对应 HTML 的 cellspacing 属性默认值）。缺这一条时所有未显式声明
	   border-spacing 的表格都少了 2px 外间距，单元格内容整体偏左上
	   （table-track-geometry 的 "UA cell padding is one pixel on every edge"
	   期望内容落在 (2,2)，实测 (0,0)）。 */
	border-spacing: 2px;
	text-indent: 0;
	box-sizing: border-box;
}
caption {
	display: table-caption;
	text-align: center;
}
/* 行组/行的 vertical-align 默认 middle，单元格用 inherit 链继承：
   td 的 vertical-align 初始值虽然是 baseline，但浏览器 UA 表让 td/th 继承
   行/行组的值，于是无显式声明时单元格内容垂直居中
   （table-track-geometry 的 "row-group default vertically centers cell
   content" 期望 60px 高单元格里的 20px 方块居中于 y=260，实测贴顶 240）。 */
thead { display: table-header-group; vertical-align: middle; }
tbody { display: table-row-group; vertical-align: middle; }
tfoot { display: table-footer-group; vertical-align: middle; }
tr { display: table-row; vertical-align: inherit; }
col { display: table-column; }
colgroup { display: table-column-group; }
th, td {
	display: table-cell;
	padding: 1px;
	vertical-align: inherit;
}
th {
	font-weight: bold;
	text-align: center;
}

/* Dialog. */
dialog {
	display: block;
	position: static;
	border: 1px solid rgba(0, 0, 0, 0.3);
	padding: 1em;
	background: #fff;
	color: #000;
}

dialog[open] {
	position: fixed;
	top: 50%;
	left: 50%;
	transform: translate(-50%, -50%);
	z-index: 1000;
}

/* 未打开的 <dialog> 不生成可见框（Chromium html.css 的同一规则）。
 * 注意这与模态状态无关：由 showModal() 打开的 dialog 即使 open 属性被移除
 * 仍是模态的（:modal 仍匹配、::backdrop 仍在），只是不再显示。 */
dialog:not([open]) {
	display: none;
}

/* ::backdrop：模态 <dialog> 的遮罩层（HTML 渲染规范 / Fullscreen spec §5）。
 * 本引擎没有 top layer，用 position:fixed + 高 z-index 近似：backdrop 由
 * rendering 在 dialog 之前绘制（见 rendertreebuilder 的 backdrop 插入），
 * z-index 略低于 dialog[open] 的 1000，高于普通内容。 */
::backdrop {
	position: fixed;
	inset: 0;
	background: rgba(0, 0, 0, 0.1);
	z-index: 999;
}

/* ── General element defaults (mirrors WebCore/css/html.css headings/lists/
       text sections; only properties the engine resolves are listed) ── */

html {
	display: block;
}

body {
	display: block;
	margin: 8px;
}

p {
	display: block;
	margin-top: 1em;
	margin-bottom: 1em;
}

address, article, aside, div, footer, header, hgroup, main, nav, section {
	display: block;
}

blockquote {
	display: block;
	margin-top: 1em;
	margin-bottom: 1em;
	margin-left: 40px;
	margin-right: 40px;
}

figure {
	display: block;
	margin-top: 1em;
	margin-bottom: 1em;
	margin-left: 40px;
	margin-right: 40px;
}

figcaption {
	display: block;
}

/* Headings: h1-h6 are bold block boxes with relative font sizes. */
h1, h2, h3, h4, h5, h6 {
	display: block;
	font-weight: bold;
}

h1 {
	font-size: 2em;
	margin-top: 0.67em;
	margin-bottom: 0.67em;
}

h2 {
	font-size: 1.5em;
	margin-top: 0.83em;
	margin-bottom: 0.83em;
}

h3 {
	font-size: 1.17em;
	margin-top: 1em;
	margin-bottom: 1em;
}

h4 {
	margin-top: 1.33em;
	margin-bottom: 1.33em;
}

h5 {
	font-size: 0.83em;
	margin-top: 1.67em;
	margin-bottom: 1.67em;
}

h6 {
	font-size: 0.67em;
	margin-top: 2.33em;
	margin-bottom: 2.33em;
}

/* Lists. */
ul, menu, dir {
	display: block;
	list-style-type: disc;
	margin-top: 1em;
	margin-bottom: 1em;
	padding-left: 40px;
}

ol {
	display: block;
	list-style-type: decimal;
	margin-top: 1em;
	margin-bottom: 1em;
	padding-left: 40px;
}

li {
	display: list-item;
}

dl {
	display: block;
	margin-top: 1em;
	margin-bottom: 1em;
}

dt {
	display: block;
}

dd {
	display: block;
	margin-left: 40px;
}

/* Inline text defaults. */
a {
	color: -webkit-link;
	cursor: pointer;
	text-decoration: underline;
}

strong, b {
	font-weight: bold;
}

em, i {
	font-style: italic;
}

big {
	font-size: 1.17em;
}

small {
	font-size: 0.83em;
}

sub {
	font-size: 0.83em;
	vertical-align: sub;
}

sup {
	font-size: 0.83em;
	vertical-align: super;
}

s, strike, del {
	text-decoration: line-through;
}

u, ins {
	text-decoration: underline;
}

code, kbd, samp, tt {
	font-family: monospace;
}

pre {
	display: block;
	font-family: monospace;
	white-space: pre;
	margin-top: 1em;
	margin-bottom: 1em;
}

hr {
	display: block;
	margin-top: 0.5em;
	margin-bottom: 0.5em;
	border-style: inset;
	border-width: 1px;
}

q {
	display: inline;
}

center {
	display: block;
	/* ★ -webkit-center（而非标准 center）：<center> 不仅居中行内内容，
	   还让块级子盒（定宽 div、表格等）在容器内水平居中。此前写成标准
	   center，legacy-center 夹具的 .block-box 期望 x=150 实测 x=0。 */
	text-align: -webkit-center;
}

/* --- Fullscreen (HTML §4.11.6 + Fullscreen spec §5) ---
 * The fullscreen element is promoted to the viewport by the UA: Chromium's
 * html.css pins :fullscreen:not(:root) with position fixed and inset 0.
 * The :root case is excluded here because a fullscreened root element is
 * already viewport-sized and pinning it would break document scrolling. */
:fullscreen:not(:root) {
	position: fixed;
	inset: 0;
	margin: 0;
	width: 100%;
	height: 100%;
	max-width: none;
	max-height: none;
}
`

// NewUAStyleSheet parses the UA default CSS and returns a CSSStyleSheet
// with OriginUserAgent set, ready to be added to a style.Resolver.
func NewUAStyleSheet() *css.CSSStyleSheet {
	sheet := css.NewCSSStyleSheet()
	sheet.SetOrigin(css.OriginUserAgent)
	p := css.NewParser(UAStyleSheetCSS)
	p.SetOrigin(css.OriginUserAgent)
	p.ParseStyleSheetInto(sheet)
	return sheet
}
