"""extract_full_dom.py — 从 9090 端口抓取 Vue 渲染后的完整 DOM + 计算样式"""
import asyncio, json, re
from playwright.async_api import async_playwright

async def extract():
    async with async_playwright() as p:
        browser = await p.chromium.launch()
        page = await browser.new_page(viewport={"width": 1280, "height": 800})
        await page.goto("http://localhost:9090", wait_until="networkidle")
        await page.wait_for_timeout(3000)

        # 抓取完整 DOM 树（含 computed styles）
        data = await page.evaluate("""() => {
            function getComputedStyleObj(el) {
                const cs = window.getComputedStyle(el);
                const out = {};
                const keys = [
                    'display','flexDirection','alignItems','justifyContent',
                    'width','height','minWidth','minHeight','maxWidth','maxHeight',
                    'padding','paddingLeft','paddingRight','paddingTop','paddingBottom',
                    'margin','marginLeft','marginRight','marginTop','marginBottom',
                    'border','borderWidth','borderStyle','borderColor','borderRadius',
                    'fontSize','fontFamily','fontWeight','color','lineHeight','textAlign',
                    'backgroundColor','background','backgroundImage',
                    'position','top','right','bottom','left','zIndex',
                    'overflow','overflowX','overflowY',
                    'visibility','opacity','boxSizing','gap','flexWrap',
                    'whiteSpace','wordBreak','textOverflow',
                ];
                for (const k of keys) out[k] = cs.getPropertyValue(k);
                return out;
            }

            function walk(el, depth) {
                if (depth > 30) return null;
                const tag = el.tagName ? el.tagName.toLowerCase() : '#text';
                if (tag === 'script' || tag === 'style' || tag === 'link') return null;

                const node = {tag};
                try {
                    const rect = el.getBoundingClientRect();
                    node.rect = {x: Math.round(rect.x), y: Math.round(rect.y),
                                w: Math.round(rect.width), h: Math.round(rect.height)};
                } catch(e) {
                    node.rect = {x:0, y:0, w:0, h:0};
                }

                if (tag === '#text') {
                    node.text = el.textContent.trim();
                    if (!node.text) return null;
                } else {
                    node.styles = getComputedStyleObj(el);
                    if (el.id) node.id = el.id;
                    if (el.className && typeof el.className === 'string') node.cls = el.className;
                    // 文本内容截断
                    const tc = el.textContent ? el.textContent.trim() : '';
                    if (tc && tc.length < 200) node.text = tc;
                }

                // 子节点
                if (el.childNodes) {
                    const kids = [];
                    for (const c of el.childNodes) {
                        const kid = walk(c, depth + 1);
                        if (kid) kids.push(kid);
                    }
                    if (kids.length) node.children = kids;
                }
                return node;
            }

            const root = walk(document.body, 0);
            return {
                title: document.title,
                bodyRect: {w: document.body.clientWidth, h: document.body.clientHeight},
                tree: root
            };
        }""")

        # 保存完整 JSON
        with open("F:/syproject/wb-ui/dev/static_test/full_dom.json", "w", encoding="utf-8") as f:
            json.dump(data, f, ensure_ascii=False, indent=2)

        # 同时也截图
        await page.screenshot(path="F:/syproject/wb-ui/dev/static_test/browser_screenshot.png")

        print(f"OK: {json.dumps(data, ensure_ascii=False)[:500]}")
        await browser.close()

asyncio.run(extract())
