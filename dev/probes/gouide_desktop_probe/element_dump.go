package main

import (
	"fmt"

	"wb-ui/webkit"
)

// domDumpJS 输出关键元素的几何 / 计算样式 / 属性列表——用于判断
// 「CSS 规则为何没生效」（scoped 属性选择器是否命中、height/display 是否解析）。
const domDumpJS = `(function () {
  function one(sel) {
    var e = null;
    try { e = document.querySelector(sel); } catch (err) { return sel + ': query ERR ' + err; }
    if (!e) return sel + ': missing';
    var attrs = [];
    try { for (var i = 0; i < e.attributes.length; i++) { attrs.push(e.attributes[i].name); } } catch (err) { attrs.push('ERR:' + err); }
    var cls = e.className;
    if (cls && typeof cls === 'object' && cls.baseVal !== undefined) cls = cls.baseVal;
    var rect = '';
    try { var b = e.getBoundingClientRect(); rect = Math.round(b.width) + 'x' + Math.round(b.height); } catch (err) { rect = 'ERR'; }
    var disp = '?', h = '?', align = '?';
    try { var cs = getComputedStyle(e); disp = cs.display; h = cs.height; align = cs.alignItems; } catch (err) {}
    return sel + ': ' + e.tagName.toLowerCase() + '.' + cls + ' | rect=' + rect + ' | ' + disp + ' h=' + h + ' align=' + align + ' | attrs=[' + attrs.join(',') + ']';
  }
  var sels = ['.titlebar', '.tb-logo', '.tb-nav', '.app-root', '.plugin-area-titlebar', '.main-area', '.sidebar'];
  var out = [];
  for (var i = 0; i < sels.length; i++) out.push(one(sels[i]));
  out.push('--- .titlebar 子项（运行时插入的插件样式是否生效）---');
  try {
    var tb = document.querySelector('.titlebar');
    var kids = tb.querySelectorAll('*');
    for (var k = 0; k < kids.length && k < 16; k++) {
      var e = kids[k];
      var r = e.getBoundingClientRect();
      var c = e.className;
      if (c && typeof c === 'object' && c.baseVal !== undefined) c = c.baseVal;
      out.push('   ' + e.tagName.toLowerCase() + '.' + c + ' rect=' + Math.round(r.x) + ',' + Math.round(r.y) + ',' + Math.round(r.width) + 'x' + Math.round(r.height) + ' text=' + String(e.textContent || '').replace(/s+/g, ' ').slice(0, 20));
    }
  } catch (err) { out.push('   ERR ' + err); }
  out.push('--- .plugin-area-titlebar 子节点（校验注释是否被渲染）---');
  try {
    var host = document.querySelector('.plugin-area-titlebar');
    var nk = host.childNodes;
    out.push('   childNodes=' + nk.length);
    for (var i = 0; i < nk.length && i < 10; i++) {
      var k = nk[i];
      var kr = '';
      try { var kb = k.getBoundingClientRect(); kr = ' rect=' + Math.round(kb.x) + ',' + Math.round(kb.y) + ',' + Math.round(kb.width) + 'x' + Math.round(kb.height); } catch (e2) {}
      out.push('   [' + i + '] type=' + k.nodeType + ' name=' + k.nodeName + kr + ' value=' + String(k.nodeValue || '').slice(0, 26));
    }
  } catch (err) { out.push('   ERR ' + err); }
  out.push('--- 左上区域含文本元素（x<300,y<240 / 左列 x<70 单独标记）---');
  try {
    var all = document.querySelectorAll('*');
    var cnt = 0;
    for (var i = 0; i < all.length && cnt < 22; i++) {
      var e = all[i];
      var r = e.getBoundingClientRect();
      if (r.width <= 0 || r.height <= 0 || r.x >= 300 || r.y >= 240 || r.y < 0) continue;
      var t = String(e.textContent || '').replace(/\s+/g, ' ').trim();
      if (t.length === 0) continue;
      cnt++;
      var tag = (r.x < 70) ? '[左列] ' : '';
      out.push('   ' + tag + e.tagName.toLowerCase() + '.' + e.className + ' rect=' + Math.round(r.x) + ',' + Math.round(r.y) + ',' + Math.round(r.width) + 'x' + Math.round(r.height) + ' text=' + t.slice(0, 34));
    }
  } catch (err) { out.push('   ERR ' + err); }
  out.push('--- .tb-nav 定位实验（变量消除）---');
  out.push('--- 「快速执行」按钮几何（引擎真值）---');
  try {
    var b = document.querySelector('.qexec-btn');
    if (!b) { out.push('   .qexec-btn missing'); }
    else {
      var r = b.getBoundingClientRect();
      var cs = getComputedStyle(b);
      out.push('   btn rect=' + r.x.toFixed(1) + ',' + r.y.toFixed(1) + ' ' + r.width.toFixed(1) + 'x' + r.height.toFixed(1)
        + ' display=' + cs.display + ' padding=' + cs.padding + ' border=' + cs.borderWidth
        + ' gap=' + cs.gap + ' boxSizing=' + cs.boxSizing + ' alignItems=' + cs.alignItems);
      out.push('   btn 右边界=' + (r.x + r.width).toFixed(1) + ' / padding box 右边界='
        + (r.x + r.width - parseFloat(cs.borderRightWidth || 0) - parseFloat(cs.paddingRight || 0)).toFixed(1));
      var kids = b.childNodes;
      for (var qi = 0; qi < kids.length; qi++) {
        var k = kids[qi];
        if (k.nodeType !== 1) { out.push('   [' + qi + '] #text ' + JSON.stringify(String(k.textContent).slice(0, 20))); continue; }
        var kb = k.getBoundingClientRect();
        var kc = getComputedStyle(k);
        var kn = k.className;
        if (kn && typeof kn === 'object' && kn.baseVal !== undefined) kn = kn.baseVal;
        out.push('   [' + qi + '] ' + k.tagName.toLowerCase() + '.' + kn
          + ' rect=' + kb.x.toFixed(1) + ',' + kb.y.toFixed(1) + ' ' + kb.width.toFixed(1) + 'x' + kb.height.toFixed(1)
          + ' display=' + kc.display + ' marginLeft=' + kc.marginLeft + ' overflow=' + kc.overflow);
        var gk = k.childNodes;
        for (var gj = 0; gj < gk.length; gj++) {
          var g = gk[gj];
          if (g.nodeType !== 1) continue;
          var gb = g.getBoundingClientRect();
          var gc = getComputedStyle(g);
          out.push('       > ' + g.tagName.toLowerCase()
            + ' rect=' + gb.x.toFixed(1) + ',' + gb.y.toFixed(1) + ' ' + gb.width.toFixed(1) + 'x' + gb.height.toFixed(1)
            + ' display=' + gc.display + ' widthAttr=' + (g.getAttribute ? g.getAttribute('width') : '?'));
        }
      }
    }
  } catch (err) { out.push('   ERR ' + err); }
  out.push('--- 祖先链（谁给了按钮这个宽度）---');
  try {
    var cur = document.querySelector('.qexec-btn');
    var depth = 0;
    while (cur && depth < 10) {
      var cb = cur.getBoundingClientRect();
      var cc = getComputedStyle(cur);
      var cn = cur.className;
      if (cn && typeof cn === 'object' && cn.baseVal !== undefined) cn = cn.baseVal;
      out.push('   ' + depth + ' ' + cur.tagName.toLowerCase() + '.' + cn
        + ' rect=' + cb.x.toFixed(1) + ',' + cb.y.toFixed(1) + ' ' + cb.width.toFixed(1) + 'x' + cb.height.toFixed(1)
        + ' cssW=' + cc.width + ' display=' + cc.display
        + ' flex=' + cc.flexGrow + '/' + cc.flexShrink + '/' + cc.flexBasis
        + ' clientW=' + cur.clientWidth + ' scrollW=' + cur.scrollWidth);
      cur = cur.parentElement;
      depth++;
    }
  } catch (err) { out.push('   ERR ' + err); }
  try {
    var nv = document.querySelector('.tb-nav');
    var navInfo = function (tag) {
      var r = nv.getBoundingClientRect();
      out.push('   ' + tag + ': rect=' + Math.round(r.x) + ',' + Math.round(r.y) + ' offsetTop=' + nv.offsetTop + ' offsetLeft=' + nv.offsetLeft);
    };
    navInfo('原始');
    nv.style.transform = 'none';
    navInfo('transform:none');
    nv.style.transform = 'translateX(-50%)';
    navInfo('显式 translateX(-50%)');
    nv.style.transform = 'translate(-50%)';
    navInfo('单值 translate(-50%)');
    nv.style.transform = 'translate(-50%,-50%)';
    navInfo('双值 translate(-50%,-50%)');
    nv.style.transform = '';
    nv.style.maxWidth = 'none';
    navInfo('max-width:none');
    nv.style.maxWidth = '';
    nv.style.overflow = 'visible';
    navInfo('overflow:visible');
  } catch (err) { out.push('   ERR ' + err); }
  out.push('--- 工具集/模型下拉 .sp-wrap / .sp-select / .sp-chevron（引擎真值）---');
  try {
    var wraps = document.querySelectorAll('.sp-wrap');
    out.push('   .sp-wrap 数量=' + wraps.length);
    for (var wi = 0; wi < wraps.length; wi++) {
      var w0 = wraps[wi];
      var wb = w0.getBoundingClientRect();
      var wc = getComputedStyle(w0);
      out.push('   [' + wi + '] WRAP rect=' + wb.x.toFixed(1) + ',' + wb.y.toFixed(1) + ' ' + wb.width.toFixed(1) + 'x' + wb.height.toFixed(1)
        + ' display=' + wc.display + ' position=' + wc.position + ' alignItems=' + wc.alignItems);
      var se = w0.querySelector('select');
      if (!se) { out.push('       select missing'); continue; }
      var sb = se.getBoundingClientRect();
      var sc = getComputedStyle(se);
      out.push('       SELECT rect=' + sb.x.toFixed(1) + ',' + sb.y.toFixed(1) + ' ' + sb.width.toFixed(1) + 'x' + sb.height.toFixed(1)
        + ' fontSize=' + sc.fontSize + ' fontFamily=' + String(sc.fontFamily).slice(0, 24)
        + ' pad=' + sc.padding + ' border=' + sc.borderWidth + ' lineHeight=' + sc.lineHeight
        + ' appearance=' + sc.appearance + ' maxWidth=' + sc.maxWidth + ' cssW=' + sc.width + ' cssH=' + sc.height);
      out.push('       SELECT 子节点数=' + se.childNodes.length + ' 选项数=' + se.options.length
        + ' 首项文本="' + (se.options.length ? String(se.options[0].textContent).slice(0, 24) : '') + '"'
        + ' 值="' + String(se.value) + '"');
      var opts = se.options.length;
      for (var oi = 0; oi < opts && oi < 3; oi++) {
        var ob = se.options[oi].getBoundingClientRect();
        out.push('         opt[' + oi + '] text="' + String(se.options[oi].textContent).slice(0, 18) + '" rect=' + ob.width.toFixed(1) + 'x' + ob.height.toFixed(1));
      }
      var chs = w0.querySelectorAll('.sp-chevron');
      out.push('       chevron 数量=' + chs.length);
      for (var ci = 0; ci < chs.length; ci++) {
        var cb = chs[ci].getBoundingClientRect();
        var cc = getComputedStyle(chs[ci]);
        out.push('         chevron[' + ci + '] ' + chs[ci].tagName.toLowerCase()
          + ' rect=' + cb.x.toFixed(1) + ',' + cb.y.toFixed(1) + ' ' + cb.width.toFixed(1) + 'x' + cb.height.toFixed(1)
          + ' color=' + cc.color + ' position=' + cc.position + ' right=' + cc.right + ' top=' + cc.top
          + ' transform=' + cc.transform + ' overflow=' + cc.overflow + ' display=' + cc.display);
      }
    }
  } catch (err) { out.push('   ERR ' + err); }
  out.push('--- toast 容器 / toast-item（引擎真值）---');
  try {
    var tc = document.querySelector('.toast-container');
    if (!tc) { out.push('   .toast-container missing'); }
    else {
      var tb = tc.getBoundingClientRect();
      var tcs = getComputedStyle(tc);
      out.push('   CONTAINER rect=' + tb.x.toFixed(1) + ',' + tb.y.toFixed(1) + ' ' + tb.width.toFixed(1) + 'x' + tb.height.toFixed(1)
        + ' display=' + tcs.display + ' flexDirection=' + tcs.flexDirection + ' position=' + tcs.position
        + ' maxWidth=' + tcs.maxWidth + ' cssW=' + tcs.width + ' gap=' + tcs.gap
        + ' clientW=' + tc.clientWidth + ' scrollW=' + tc.scrollWidth);
      var items = tc.querySelectorAll('.toast-item');
      out.push('   toast-item 数量=' + items.length + '（Vue v-for 每项渲染 1 个节点；>N 表示引擎重复建节点）');
      for (var ti = 0; ti < items.length; ti++) {
        var it = items[ti];
        var ib = it.getBoundingClientRect();
        var ic = getComputedStyle(it);
        out.push('   item[' + ti + '] rect=' + ib.x.toFixed(1) + ',' + ib.y.toFixed(1) + ' ' + ib.width.toFixed(1) + 'x' + ib.height.toFixed(1)
          + ' fontSize=' + ic.fontSize + ' pad=' + ic.padding + ' wordBreak=' + ic.wordBreak
          + ' writingMode=' + ic.writingMode + ' whiteSpace=' + ic.whiteSpace + ' display=' + ic.display
          + ' cssW=' + ic.width + ' clientW=' + it.clientWidth + ' scrollW=' + it.scrollWidth
          + ' 子元素数=' + it.children.length + ' 文本="' + String(it.textContent).slice(0, 40) + '"');
      }
      if (items.length) {
        var probe = document.createElement('span');
        probe.style.position = 'absolute';
        probe.style.whiteSpace = 'nowrap';
        probe.style.visibility = 'hidden';
        probe.style.font = getComputedStyle(items[0]).font;
        probe.textContent = String(items[0].textContent);
        document.body.appendChild(probe);
        var pwb = probe.getBoundingClientRect();
        out.push('   量宽对照：同字体 nowrap 文本宽=' + pwb.width.toFixed(1) + '（应 ≈ item clientW − padding 左右）');
        if (probe.parentNode) probe.parentNode.removeChild(probe);
      }
    }
  } catch (err) { out.push('   ERR ' + err); }
  out.push('--- [data-case] 用例矩阵（最小复现页，判定竖排根因所在层）---');
  try {
    var cases = document.querySelectorAll('[data-case]');
    out.push('   命中 ' + cases.length + ' 个');
    for (var qi = 0; qi < cases.length; qi++) {
      var qe = cases[qi];
      var qr = qe.getBoundingClientRect();
      var qc = getComputedStyle(qe);
      out.push('   ' + qe.getAttribute('data-case')
        + ' rect=' + qr.x.toFixed(1) + ',' + qr.y.toFixed(1) + ' ' + qr.width.toFixed(1) + 'x' + qr.height.toFixed(1)
        + ' clientW=' + qe.clientWidth + ' scrollW=' + qe.scrollWidth + ' scrollH=' + qe.scrollHeight
        + ' pad=' + qc.padding + ' cssW=' + qc.width + ' wordBreak=' + qc.wordBreak + ' whiteSpace=' + qc.whiteSpace
        + ' 文本=' + String(qe.textContent).slice(0, 30));
    }
  } catch (err) { out.push('   ERR ' + err); }
  return out.join('\n');
})()`

func runDomDump(wv *webkit.WebView) {
	fmt.Println("=== DOM/样式 dump ===")
	fmt.Println(evalStr(wv, domDumpJS))
}
