// Package html5 defaultcss provides the User-Agent (UA) default stylesheet
// for HTML form controls, mirroring WebCore/css/html.css.
// The stylesheet is injected into the style resolver with OriginUserAgent so
// that author stylesheets override it via the cascade.

package html5

import (
	"wb-ui/engine/css"
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
	/* ★ Chromium 的 UA 规则是 font: 400 13.3333px Arial（html.css）：
	   表单控件**既不继承文档字号、也不继承文档字体族**。此前这里写
	   font-family: inherit，控件于是继承文档字体（默认 sans-serif →
	   Noto Sans SC）；同样 13.3333px 下 Noto Sans SC 的行盒是 19px，
	   而 Arial 是 15px —— input 的边框盒因此是 25px 而不是 Edge 的 21px
	   （content 19+2 padding+4 border vs 15+2+4）。quirks 模式 form
	   的 1em 下边距把后面 .form-marker 从 37 推到了 41
	   （cssprobe form-control-quirks 的唯一失败项，cssoracle 确认 Edge=37）。 */
	font-family: Arial;
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
	/* ★ H2：Chromium html.css 给 textarea 单独的 monospace 字体
	   （textarea { font-family: monospace }），而上面那条
	   input, button, select, textarea { font-family: Arial } 会被它覆盖。
	   Edge 实测 formtext_probe 的 t1：font=monospace/13.3333px。 */
	font-family: monospace;
	/* ★ textarea 的 UA 边框是 1px，不是 input 的 2px（两者不同！）：
	   实测（TestPxTextarea，Edge 参考）该用例的外盒 = content 150×50 +
	   padding 2px×2 + border 1px×2 = 156×56，Edge 的上下边框各只占 1 行
	   （y=16 与 y=71）、左右各 1 列；wb-ui 曾用 2px 占 2 行 2 列
	   （y=16..17 / y=72..73），整体宽高各多 2px，整行整列共 1015 个差异
	   像素。input 保持 2px（见上，108px 边框盒的实测结论）。 */
	border: 1px solid #767676;
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
   their own — the generic input rule above must not box them in.
   ★ background-color 必须显式透明，不能沿用上面 input 的 #ffffff：
   Chromium/Edge 对 appearance 型控件给**透明**背景（实底由主题控件自己绘制）。
   实测（一致性套件 form_controls）：Edge 的 input#chk / input#rad 计算值
   = rgba(0,0,0,0)，而 wb-ui 曾输出 rgb(255,255,255)。 */
input[type="checkbox"], input[type="radio"] {
	display: inline-block;
	width: 13px;
	height: 13px;
	padding: 0;
	border: none;
	background-color: transparent;
	margin: 3px 3px 3px 4px;
	vertical-align: baseline;
}

input[type="radio"] {
	margin: 3px 3px 0 5px;
}

/* Range input renders as a slider.
   ★ background-color 实测为**白色**（与 checkbox/radio 相反）：Chromium/Edge
   的 range 计算样式里 background-color = rgb(255,255,255)（滑轨与滑块由主题
   绘制在其上）。此前这里写成 transparent，一致性套件 form_controls 报出
   range 的 bg 差异（edge=白 / wbui=透明）。 */
input[type="range"] {
	display: inline-block;
	/* Chromium: 129×16 —— 宽高都不随控件字体缩放。 */
	width: 129px;
	/* ★ 高度是固定 16px。曾写 1.6em（≈21px @13.3333）并注释「Edge 实测约
	   21px」——那是错的，还与上面 width 那行的「129×16」自相矛盾，导致
	   form_controls 的 range 长期报 129x21 vs Edge 129x16（5px 偏差被套件
	   容差吞掉）。三处独立证据都是 16：
	     1. 实测：msedge --headless --dump-dom 注入 JS 读
	        getBoundingClientRect/getComputedStyle →
	        得到 rect=129x16、cssH=16px；且作者显式 height:40px 时确实生效为 40px
	        （所以 16px 是 UA 默认值，并非不可覆盖的固有值）；
	     2. 外部 oracle：ref/obscura/render-repros/checks.json 的
	        form-control-geometry 里 range marker 高度 = 16（夹具作者按浏览器的
	        16 定的）；
	     3. 引擎布局层常量 formControlRangeHeight = 16.0
	        （engine/layout/formcontrol.go）——布局层本就按 16 设计，是本条 UA
	        的 1.6em 把它覆盖成了 21px。
	   历史备注「1.2em(≈16px) 导致温度行 row 高 24 vs 浏览器 30、modal 少 5px」
	   在本仓找不到对应夹具或断言（app/host.go 里没有该场景），属无法复现的旧
	   反馈；以实测为准。 */
	height: 16px;
	padding: 0;
	border: none;
	margin: 2px;
	background-color: #ffffff;
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

/* Submit/reset/button inputs and <button> share button styling: a FLAT face
   with a #767676 border.
   ★ 曾经这里是「白 body + 顶部 28% #efefef 高光带」（靠 background-image
   linear-gradient 实现），但那是想当然 —— Edge 的按钮面没有渐变分界。
   实测（TestPxButton 逐列剖面，Edge 参考）x=70 列 y=19..50 全为 #efefef，
   而 wb-ui 只有 y=19..27（恰为 28% 高光带）是 #efefef、y=28..50 是
   #ffffff —— 2736 个差异像素的主体即此，故删掉 background-image。
   ★ 但 background-color 必须保持 #f0f0f0：一致性套件的 form_controls 比对
   computed 值，Edge = rgb(240,240,240)；而 Edge 实际绘制出的按钮面像素是
   #efefef(239)，与 #f0f0f0(240) 只差 1，在像素容差 3 内 —— 不要为了那 1
   个单位把计算值改掉（套件会 FAIL）。 */
input[type="submit"], input[type="reset"], input[type="button"],
button {
	display: inline-block;
	padding: 1px 6px;
	/* ★ H2/H6（Edge 实测）：Chromium 的 button UA 边框是 **2px**，不是 1px ——
	   g1_formctl 的 t5 计算值 borderWidth=2px（wbui 曾输出 1px），其 border-box
	   宽度差 2px（Edge 36.02 vs wbui 34.01）也与此吻合。 */
	border: 2px solid #767676;
	background-color: #f0f0f0;
	color: #000000;
	text-align: center;
	cursor: default;
	box-sizing: border-box;
	-webkit-appearance: button;
	/* ★ H8（Edge 实测）：按钮的内部行盒**不继承**父元素的 line-height。
	   h2_baseline_matrix 的 l_button_* 四例：父 line-height 取
	   20px / 1.5 / 40px 时 Edge 的 button 边框盒高**恒为 21**，而 wbui 曾
	   随父值膨胀到 26 / 26 / 51.5 —— 因为 line-height 是可继承属性，UA 没给
	   button 覆盖，内部匿名行盒就用了父的 line-height。Chromium 的按钮
	   内部内容区行高固定（等同 uaControlFontSize 的 normal 行高 ≈15px，
	   加 padding 2 + border 4 = 21）。显式覆盖为 normal 使内部行盒取
	   **按钮自身字体**的度量行高，与浏览器一致。 */
	line-height: normal;
}

button[disabled], input[disabled] {
	color: #808080;
}

/* Select and option. */
select {
	display: inline-block;
	/* ★ H2（Edge 实测）：Chromium 的 select UA padding 是 **0**，不是 1px
	   —— formtext_probe 的 s1 / g1_formctl 的 t6 计算值均为 pad=0px
	   （select 的内部留白由控件自身绘制，不占 CSS padding）。 */
	padding: 0;
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

/* Popover（HTML §6.12 的 UA 样式）。
 *
 * 规范里显示中的 popover 进入 top layer（绘制在文档内容与其它 top layer 元素
 * 之上）；本引擎没有 top layer（同上），用 position:fixed + z-index 近似：
 * z-index 1100 高于普通内容与 dialog[open]（1000），低于应用层的 toast 层
 * （1200）。popover stack 内部的先后顺序由作者样式/文档顺序决定，本引擎不做
 * 「后显示的必然在上」——这是本端口已知的 top layer 近似差异。
 *
 * 居中写法照抄规范 UA：inset:0 + width/height:fit-content + margin:auto。
 * 本引擎对 fit-content/min-content/max-content 统一按 shrink-to-fit 求值
 * （layout.isIntrinsicSizeKeyword），并对 left/right（top/bottom）都非 auto
 * 的绝对定位盒解 margin:auto（CSS 2.1 §10.3.7 / §10.6.4）——两侧 auto 即居中。
 * 这样作者样式能按正常级联覆盖 UA：夹具 popover.html 的 #centered 给的是
 * margin:0，抵消 UA 的 margin:auto 后盒子落在包含块左上角 (0,0)，与 Edge
 * 实测一致；此前用 top/left 50% + translate 恒居中且无法被 margin 覆盖，
 * cssoracle 报 MISMATCH。
 *
 * 未显示的 popover 不生成盒（规范规则的逐字翻译；dialog[open] 例外：同时带
 * popover 属性的 <dialog> 以 dialog 方式打开时仍要显示）。 */
[popover]:not(:popover-open):not(dialog[open]) {
	display: none;
}
dialog:popover-open {
	display: block;
}
[popover] {
	position: fixed;
	inset: 0;
	margin: auto;
	width: fit-content;
	height: fit-content;
	z-index: 1100;
	border: 1px solid rgba(0, 0, 0, 0.3);
	padding: 0.25em;
	overflow: auto;
	background: #fff;
	color: #000;
}

/* popover 的 ::backdrop：规范里它是「铺满视口 + 透明 + 不吃指针事件」——默认
 * 透明意味着 popover 不会自动把页面变暗（作者可自行给 ::backdrop 上色）。它
 * 比通用 ::backdrop 规则更具体（多一个 :popover-open），因此覆盖掉模态 dialog
 * 的半透明黑底；z-index 沿用 999（在 popover 之下、普通内容之上）。 */
:popover-open::backdrop {
	position: fixed;
	inset: 0;
	pointer-events: none;
	background: transparent;
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
