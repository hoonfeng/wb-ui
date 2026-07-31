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
	margin: 0 0 1em 0;
}

/* Default focus ring (Edge/Chrome: blue outline on :focus elements). */
:focus {
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

/* Text inputs share a common border/padding. */
input[type="text"], input[type="password"], input[type="search"],
input[type="email"], input[type="url"], input[type="tel"],
input[type="number"], input[type="date"], input[type="time"],
input[type="month"], input[type="week"], input[type="datetime-local"] {
	padding: 2px 4px;
	border: 1px solid #767676;
	background-color: #ffffff;
	box-sizing: border-box;
	min-height: 1.2em;
}

textarea {
	padding: 2px 4px;
	border: 1px solid #767676;
	background-color: #ffffff;
	box-sizing: border-box;
	resize: both;
	overflow: auto;
}

input[type="color"] {
	width: 2em;
	height: 1.5em;
	padding: 1px;
	border: 1px solid #c0c0c0;
}

/* Checkbox and radio are inline with no border. */
input[type="checkbox"], input[type="radio"] {
	display: inline-block;
	width: 1em;
	height: 1em;
	margin: 0 0.2em 0 0;
	vertical-align: baseline;
}

/* Range input renders as a slider. */
input[type="range"] {
	display: inline-block;
	width: 9.7em;
	height: 1.2em;
	padding: 0;
	border: none;
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

/* Submit/reset/button inputs and <button> share button styling. The
   background mirrors Edge's Windows-style button: white body with a
   #efefef highlight band across the top ~25%, and a #767676 border. */
input[type="submit"], input[type="reset"], input[type="button"],
button {
	display: inline-block;
	padding: 4px 10px;
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
	border: 1px solid #c0c0c0;
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
	text-indent: 0;
	box-sizing: border-box;
}
caption {
	display: table-caption;
	text-align: center;
}
thead { display: table-header-group; }
tbody { display: table-row-group; }
tfoot { display: table-footer-group; }
tr { display: table-row; }
col { display: table-column; }
colgroup { display: table-column-group; }
th, td {
	display: table-cell;
	padding: 1px;
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
	text-align: center;
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
