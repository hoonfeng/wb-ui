// Test case catalog: one HTML per concern. Each page embeds a JS collector
// (see jsCollector in edge.go) that serializes element state into
// document.title as "GEO:..." lines. The same data is extracted from wb-ui's
// render tree for comparison.

package main

// collectorSnippet is injected before </body> of every case. It walks all
// elements and emits a compact per-element record. The injected script is
// identical across cases so wb-ui can mirror the same fields.
const collectorSnippet = `
<script>
(function(){
  var out = [];
  var all = document.querySelectorAll('body *');
  for (var i = 0; i < all.length; i++) {
    var el = all[i];
    var tag = el.tagName.toLowerCase();
    if (tag === 'script' || tag === 'style' || tag === 'head' || tag === 'title' || tag === 'link' || tag === 'meta' || tag === 'option') continue;
    try {
      var r = el.getBoundingClientRect();
      var cs = getComputedStyle(el);
      var parts = [
        el.tagName.toLowerCase(),
        el.id || '',
        el.className || '',
        Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height),
        cs.display, cs.color, cs.backgroundColor, cs.fontSize,
        (el.textContent||'').trim().slice(0,20),
        el.checked !== undefined ? (el.checked?'1':'0') : '',
        el.value !== undefined ? String(el.value).slice(0,20) : '',
        el.dataset && el.dataset.count !== undefined ? el.dataset.count : ''
      ];
      out.push(parts.join('|'));
    } catch(e) {
      out.push(tag + '|' + (el.id||'') + '|ERR:' + e.message.slice(0,40));
    }
  }
  document.title = 'VP:' + window.innerWidth + 'x' + window.innerHeight + ';GEO:' + out.join(';');
})();
</script>
`

func baseDoc(body, extraHead string) string {
	return "<!DOCTYPE html><html><head><meta charset='utf-8'>" + extraHead + "</head><body>" + body + collectorSnippet + "</body></html>"
}

func allCases() []TestCase {
	return []TestCase{
		// ── Layout ──────────────────────────────────────────────
		{
			Name: "layout_block", Desc: "block flow: margins, padding, box-sizing",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<div id="a" style="width:200px;height:100px;background:#f00;margin:10px"></div>
				<div id="b" style="width:50%;height:80px;background:#00f;padding:5px;box-sizing:border-box"></div>
				<div id="c" style="width:150px;height:60px;background:#0f0;margin:10px 20px"></div>
			`, ""),
		},
		{
			Name: "layout_flex", Desc: "flex row/column, justify/align, gap",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<div style="display:flex;gap:10px;justify-content:space-between;align-items:center;width:400px;height:100px;background:#eee">
					<div id="f1" style="width:50px;height:50px;background:#f00"></div>
					<div id="f2" style="width:80px;height:30px;background:#0f0"></div>
					<div id="f3" style="width:60px;height:80px;background:#00f"></div>
				</div>
				<div style="display:flex;flex-direction:column;gap:5px;width:200px;height:150px;background:#ddd">
					<div id="c1" style="height:40px;background:#f80"></div>
					<div id="c2" style="height:40px;background:#08f"></div>
				</div>
			`, ""),
		},
		{
			Name: "layout_position", Desc: "absolute/relative/fixed positioning",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<div id="rel" style="position:relative;left:30px;top:20px;width:100px;height:50px;background:#f00"></div>
				<div id="abs" style="position:absolute;left:200px;top:100px;width:120px;height:60px;background:#0f0"></div>
				<div id="fixed" style="position:fixed;right:10px;bottom:10px;width:80px;height:80px;background:#00f"></div>
				<div id="cont" style="position:relative;width:300px;height:200px;background:#eee">
					<div id="inner" style="position:absolute;right:0;bottom:0;width:40px;height:40px;background:#f80"></div>
				</div>
			`, ""),
		},
		{
			Name: "layout_float", Desc: "float left/right text wrap",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<div id="fl" style="float:left;width:100px;height:100px;background:#f00">L</div>
				<div id="fr" style="float:right;width:80px;height:120px;background:#00f">R</div>
				<div id="txt" style="width:400px">Some text that should wrap around the floats on both sides.</div>
			`, ""),
		},
		{
			Name: "layout_grid", Desc: "grid columns, rows, gap, auto placement",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<div style="display:grid;grid-template-columns:100px 100px 100px;gap:10px;width:330px;background:#eee">
					<div id="g1" style="background:#f00">A</div>
					<div id="g2" style="background:#0f0">B</div>
					<div id="g3" style="background:#00f">C</div>
					<div id="g4" style="background:#ff0">D</div>
				</div>
				<div style="display:grid;grid-template-columns:repeat(2,80px);grid-template-rows:50px 50px;gap:5px;width:180px;background:#ddd">
					<div id="gr1" style="background:#f80">1</div>
					<div id="gr2" style="background:#08f">2</div>
					<div id="gr3" style="background:#8f0">3</div>
					<div id="gr4" style="background:#f0f">4</div>
				</div>
			`, ""),
		},
		{
			Name: "layout_table", Desc: "table/tr/td geometry, border-collapse",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<table id="t1" style="border-collapse:collapse;width:300px;background:#eee">
					<tr><td id="td1" style="border:1px solid #000">A</td><td id="td2" style="border:1px solid #000">B</td></tr>
					<tr><td id="td3" style="border:1px solid #000">C</td><td id="td4" style="border:1px solid #000">D</td></tr>
				</table>
			`, ""),
		},
		// ── Styles ──────────────────────────────────────────────
		{
			Name: "style_cascade", Desc: "specificity, inheritance, important",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<style>
					div { color: rgb(1,2,3); font-size: 14px; height: 20px; }
					.blue { color: rgb(0,0,255); }
					#red { color: rgb(255,0,0); }
					div.blue { color: rgb(0,255,0); }
					div { color: rgb(9,9,9) !important; }
					em { color: rgb(255,0,255); }
				</style>
				<div id="s1">plain</div>
				<div id="s2" class="blue">blue class</div>
				<div id="s3" class="blue" style="color:rgb(10,20,30)">inline</div>
				<div id="s4" style="color:rgb(255,255,0) !important">inline important</div>
				<p id="s5"><em>em text</em> and <strong>strong</strong></p>
			`, ""),
		},
		{
			Name: "style_box", Desc: "border, radius, shadow, opacity",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<div id="box1" style="width:200px;height:100px;border:2px solid rgb(255,0,0);border-radius:10px;background:rgb(240,240,240)"></div>
				<div id="box2" style="width:150px;height:80px;opacity:0.5;background:rgb(0,0,255)"></div>
				<div id="box3" style="width:120px;height:60px;box-shadow:4px 4px 8px rgba(0,0,0,0.3);background:rgb(255,255,0)"></div>
			`, ""),
		},
		// ── Animations ──────────────────────────────────────────
		{
			Name: "anim_opacity", Desc: "CSS opacity keyframe animation",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<style>
					@keyframes fade { from {opacity:0} to {opacity:1} }
					#anim { width:100px; height:100px; background:rgb(255,0,0); animation: fade 1s linear; }
				</style>
				<div id="anim"></div>
			`, ""),
		},
		{
			Name: "anim_transform", Desc: "CSS transform translate animation",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<style>
					@keyframes slide { from {transform:translateX(0)} to {transform:translateX(200px)} }
					#slide { width:80px; height:80px; background:rgb(0,0,255); animation: slide 2s linear; }
				</style>
				<div id="slide"></div>
			`, ""),
		},
		// ── Component polymorphism ─────────────────────────────
		{
			Name: "form_controls", Desc: "input/button/select/textarea geometry + value",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<form>
					<input id="txt" type="text" value="hello">
					<input id="chk" type="checkbox" checked>
					<input id="rad" type="radio" name="g" checked>
					<button id="btn">Click</button>
					<select id="sel"><option>one</option><option>two</option></select>
					<textarea id="ta">line1</textarea>
					<input id="range" type="range" value="50">
					<progress id="prog" value="30" max="100"></progress>
				</form>
			`, ""),
		},
		{
			Name: "component_list", Desc: "ul/ol list markers, headings, links",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<h1 id="h1">Title</h1>
				<h2 id="h2">Subtitle</h2>
				<ul id="ul"><li id="li1">apple</li><li id="li2">banana</li></ul>
				<ol id="ol"><li>first</li><li>second</li></ol>
				<a id="link" href="x">link text</a>
				<p id="p">a <strong>bold</strong> and <em>italic</em> word</p>
			`, ""),
		},
		// ── Interaction ────────────────────────────────────────
		{
			Name: "interact_click", Desc: "click event handler fires with correct target",
			ViewportW: 800, ViewportH: 600,
			HTML: baseDoc(`
				<div id="clicker" style="width:200px;height:100px;background:#eee" onclick="this.dataset.count='1';document.title=document.title.replace('GEO:','EVT:click:'+this.id+';GEO:')">click me</div>
			`, ""),
		},
	}
}
