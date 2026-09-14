"""Extract rendered element positions from http://localhost:9090 using Playwright."""
import json
import os
import sys
import time
from playwright.sync_api import sync_playwright

OUTPUT = os.path.join(os.path.dirname(__file__), "browser_elements.json")

def extract_positions(page):
    """Extract key element positions, styles, and text from the rendered page."""
    result = page.evaluate("""() => {
        const elements = [];
        // Target the key layout containers and items
        const selectors = [
            '.menubar', '.sidebar', '.main-content', '.right-panel', '.statusbar',
            '.tabs', '.editor-content',
            '.sidebar-item', '.tab-item'
        ];
        selectors.forEach(sel => {
            const els = document.querySelectorAll(sel);
            els.forEach((el, i) => {
                const r = el.getBoundingClientRect();
                const cs = getComputedStyle(el);
                // Get all text nodes inside this element
                let textNodes = [];
                const walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
                let node;
                while (node = walker.nextNode()) {
                    const txt = node.textContent.trim();
                    if (txt && node.parentElement) {
                        const pr = node.parentElement.getBoundingClientRect();
                        textNodes.push({
                            text: txt.substring(0, 40),
                            parentTag: node.parentElement.tagName,
                            x: Math.round(pr.x), y: Math.round(pr.y),
                            w: Math.round(pr.width), h: Math.round(pr.height)
                        });
                    }
                }
                elements.push({
                    selector: sel + '[' + i + ']',
                    rect: { x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height) },
                    styles: {
                        display: cs.display,
                        flexDirection: cs.flexDirection,
                        alignItems: cs.alignItems,
                        justifyContent: cs.justifyContent,
                        padding: cs.padding,
                        fontSize: cs.fontSize,
                        fontFamily: cs.fontFamily,
                        color: cs.color,
                        bg: cs.backgroundColor,
                        lineHeight: cs.lineHeight
                    },
                    textContent: el.textContent.trim().substring(0, 100),
                    textNodes: textNodes
                });
            });
        });
        return elements;
    }""")
    return result

def main():
    with sync_playwright() as p:
        browser = p.chromium.launch(headless=True)
        page = browser.new_page(viewport={"width": 1280, "height": 800})
        print("Navigating to http://localhost:9090 ...")
        page.goto("http://localhost:9090", wait_until="networkidle", timeout=30000)
        time.sleep(3)  # Wait for Vue to mount

        # Get page dimensions
        vp = page.viewport_size
        title = page.title()
        print(f"Title: {title}, Viewport: {vp['width']}x{vp['height']}")

        # Extract element positions
        elements = extract_positions(page)
        print(f"Found {len(elements)} key elements")
        for el in elements:
            r = el['rect']
            print(f"  {el['selector']:25s} x={r['x']:4d} y={r['y']:4d} w={r['w']:4d} h={r['h']:4d}  {el['styles']['display']:8s} {el['styles']['fontSize']}")

        # Screenshot
        page.screenshot(path=os.path.join(os.path.dirname(__file__), "browser_screenshot.png"), full_page=False)

        # Save to JSON
        output = {
            "url": "http://localhost:9090",
            "title": title,
            "viewport": {"width": vp['width'], "height": vp['height']},
            "timestamp": time.strftime("%Y-%m-%dT%H:%M:%S"),
            "elementCount": len(elements),
            "elements": elements
        }
        with open(OUTPUT, "w", encoding="utf-8") as f:
            json.dump(output, f, indent=2, ensure_ascii=False)
        print(f"\nSaved to {OUTPUT}")
        browser.close()

if __name__ == "__main__":
    main()
