"""extract_clean.py — 从浏览器抓取清洗后的静态 DOM（去 SVG/script/复杂内容）"""
import asyncio, json, os
from playwright.async_api import async_playwright

LAYOUT_KEYS = [
    'display','flex-direction','align-items','justify-content','flex-wrap','gap',
    'width','height','min-width','min-height','max-width','max-height',
    'margin-top','margin-bottom','margin-left','margin-right',
    'padding-top','padding-bottom','padding-left','padding-right',
    'font-size','font-family','font-weight','color','line-height','text-align',
    'background-color','border-radius','position',
    'top','right','bottom','left','z-index',
    'overflow','overflow-x','overflow-y','opacity','box-sizing',
    'white-space','word-break','text-overflow','visibility',
]

def clean_style(cs):
    """提取布局关键属性"""
    out = {}
    for k in LAYOUT_KEYS:
        v = cs.getPropertyValue(k)
        if v and v != 'none' and v != 'normal' and v != 'auto' and v != 'rgba(0, 0, 0, 0)' and v != '0px':
            out[k] = v
    return out

async def extract():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await pg.goto("http://localhost:9090", wait_until="networkidle")
        await pg.wait_for_timeout(3000)

        data = await pg.evaluate("""() => {
            const LK = """ + json.dumps(LAYOUT_KEYS) + """;
            function cleanStyle(cs) {
                const o = {};
                for (const k of LK) {
                    const v = cs.getPropertyValue(k.replace(/-/g, '_'));
                    let v2 = cs.getPropertyValue(k);
                    if (v2 && v2 !== 'none' && v2 !== 'normal' && v2 !== 'auto' && 
                        v2 !== 'rgba(0, 0, 0, 0)' && v2 !== '0px') {
                        o[k] = v2;
                    }
                }
                return o;
            }
            function visibleRect(el) {
                try {
                    const r = el.getBoundingClientRect();
                    return {x:Math.round(r.x),y:Math.round(r.y),w:Math.round(r.width),h:Math.round(r.height)};
                } catch(e) { return {x:0,y:0,w:0,h:0}; }
            }
            function walk(el, depth, parentTag) {
                if (depth > 25) return null;
                if (!el.tagName) {
                    // Text node
                    const t = el.textContent.trim();
                    if (!t) return null;
                    // Skip if parent is already capturing text
                    return {tag:'#text', text:t.substring(0,200)};
                }
                const tag = el.tagName.toLowerCase();
                // Skip hidden, scripts, styles, SVGs
                if (tag === 'script' || tag === 'style' || tag === 'link' || 
                    tag === 'svg' || tag === 'path' || tag === 'circle' ||
                    tag === 'meta' || tag === 'head' || tag === 'title') return null;
                
                const cs = window.getComputedStyle(el);
                const disp = cs.getPropertyValue('display');
                if (disp === 'none') return null;
                const vis = cs.getPropertyValue('visibility');
                if (vis === 'hidden') return null;

                const node = {
                    tag,
                    styles: cleanStyle(cs),
                    rect: visibleRect(el),
                };
                if (el.id) node.id = el.id;
                if (el.className && typeof el.className === 'string' && el.className.length < 100) 
                    node.cls = el.className;

                // Children
                const kids = [];
                let hasTextChild = false;
                for (const c of el.childNodes) {
                    const kid = walk(c, depth + 1, tag);
                    if (kid) {
                        // Merge consecutive text nodes
                        if (kid.tag === '#text' && kids.length > 0 && kids[kids.length-1].tag === '#text') {
                            kids[kids.length-1].text += ' ' + kid.text;
                        } else {
                            kids.push(kid);
                            if (kid.tag === '#text') hasTextChild = true;
                        }
                    }
                }
                if (kids.length) node.children = kids;
                
                // Trim text for leaf nodes
                if (hasTextChild && node.children) {
                    node.children = node.children.filter(c => {
                        if (c.tag === '#text') return c.text.length > 0;
                        return true;
                    });
                }
                
                return node;
            }
            return walk(document.body, 0, '');
        }""")

        out_dir = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "output")
        os.makedirs(out_dir, exist_ok=True)
        with open(os.path.join(out_dir, "clean_dom.json"), "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)
        
        await pg.screenshot(path=os.path.join(out_dir, "browser_ref.png"))
        print(f"OK: {len(json.dumps(data, ensure_ascii=False))} bytes")
        await b.close()

asyncio.run(extract())
