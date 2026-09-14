"""pixel_compare.py — 像素级对比：浏览器 vs wb-ui 渲染结果"""
import asyncio, os
from PIL import Image
from playwright.async_api import async_playwright

async def main():
    # Read wb-ui PNG
    wb_png = os.path.join(os.path.dirname(__file__), "wbui_output.png")
    wb = Image.open(wb_png).convert("RGBA")
    
    # Capture browser rendering of ide_bench.html via playwright
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page(viewport={"width": 1280, "height": 800})
        await pg.goto("http://localhost:9091/ide_bench.html", wait_until="networkidle")
        await pg.wait_for_timeout(1000)
        br_png = os.path.join(os.path.dirname(__file__), "browser_pixels.png")
        await pg.screenshot(path=br_png)
        await b.close()
    
    br = Image.open(br_png).convert("RGBA")
    
    wb_px = wb.load()
    br_px = br.load()
    w, h = wb.size
    
    # Define regions of interest (ROI) based on known element positions
    regions = {
        # (name, x, y, w, h)
        "actbar_bg":       (0, 0, 48, 800),
        "sidebar_bg":      (48, 0, 280, 800),
        "separator_bar":   (328, 0, 3, 800),
        "menubar_bg":      (331, 0, 949, 30),
        "tabs_area":       (331, 30, 949, 36),
        "editor_area":     (331, 66, 949, 710),
        "statusbar":       (331, 776, 949, 24),
        "files_item":      (48, 31, 280, 37),
        "explorer_label":  (48, 0, 280, 31),
        "sidebar_items":   (48, 150, 280, 200),
        "actbar_icon_A":   (0, 8, 48, 48),
        "menu_file_text":  (343, 2, 35, 25),
        "tab_main_go":     (331, 30, 71, 35),
        "editor_line1":    (331, 66, 949, 21),
    }
    
    total = 0
    mismatched = 0
    region_reports = []
    
    for name, (rx, ry, rw, rh) in regions.items():
        wb_count = 0
        br_count = 0
        wb_colors = {}
        br_colors = {}
        
        for y in range(ry, min(ry+rh, h)):
            for x in range(rx, min(rx+rw, w)):
                wc = wb_px[x, y]
                bc = br_px[x, y]
                
                wb_has = wc[3] > 10  # visible pixel
                br_has = bc[3] > 10
                
                if wb_has or br_has:
                    total += 1
                    if wb_has:
                        wb_count += 1
                        wb_key = hex_color(wc)
                        wb_colors[wb_key] = wb_colors.get(wb_key, 0) + 1
                    if br_has:
                        br_count += 1
                        br_key = hex_color(bc)
                        br_colors[br_key] = br_colors.get(br_key, 0) + 1
                    
                    if self_px_diff(wc, bc) > 48:
                        mismatched += 1
        
        # Only report regions with significant differences
        if wb_count == 0 and br_count == 0:
            continue
            
        diff_pct = mismatched / max(total, 1) * 100
        region_reports.append({
            "name": name,
            "pos": f"({rx},{ry}) {rw}x{rh}",
            "wb_pixels": wb_count,
            "br_pixels": br_count,
            "wb_top_colors": sorted(wb_colors.items(), key=lambda x: -x[1])[:3],
            "br_top_colors": sorted(br_colors.items(), key=lambda x: -x[1])[:3],
        })
    
    # Print report
    print("=" * 80)
    print(f"PIXEL COMPARISON: wb-ui vs browser")
    print(f"Total visible pixels: {total}, mismatched: {mismatched} ({mismatched/max(total,1)*100:.1f}%)")
    print("=" * 80)
    
    for r in region_reports:
        match = "OK" if mismatched_region(r, mismatched, total) < 5 else "MISMATCH"
        print(f"\n{match} {r['name']} {r['pos']}")
        print(f"   WB pixels: {r['wb_pixels']}  Browser pixels: {r['br_pixels']}")
        
        # Compare top colors
        wb_colors_set = set(c for c, _ in r['wb_top_colors'])
        br_colors_set = set(c for c, _ in r['br_top_colors'])
        shared = wb_colors_set & br_colors_set
        only_wb = wb_colors_set - br_colors_set
        only_br = br_colors_set - wb_colors_set
        
        if only_wb:
            print(f"   ONLY IN WB: {only_wb}")
        if only_br:
            print(f"   ONLY IN BR: {only_br}")
        print(f"   WB top colors: {', '.join(f'{c}({n})' for c,n in r['wb_top_colors'])}")
        print(f"   BR top colors: {', '.join(f'{c}({n})' for c,n in r['br_top_colors'])}")
    
    print(f"\n{'='*80}")
    print("LEGEND: OK = OK  MISMATCH = mismatch  MISSING = missing pixels")

def mismatched_region(region, total_mismatch, total):
    """Estimate per-region mismatch"""
    if region['wb_pixels'] == 0 and region['br_pixels'] > 0:
        return 100  # fully missing
    if region['br_pixels'] == 0 and region['wb_pixels'] > 0:
        return 100  # extra
    if region['wb_pixels'] > 0 and region['br_pixels'] > 0:
        # Compare by color sets
        wb_set = set(c for c, _ in region['wb_top_colors'])
        br_set = set(c for c, _ in region['br_top_colors'])
        if not (wb_set & br_set):
            return 100
        return len(wb_set - br_set) / max(len(wb_set | br_set), 1) * 100
    return 0

def hex_color(c):
    return f"#{c[0]:02x}{c[1]:02x}{c[2]:02x}"

def self_px_diff(a, b):
    return abs(int(a[0])-int(b[0])) + abs(int(a[1])-int(b[1])) + abs(int(a[2])-int(b[2]))

if __name__ == '__main__':
    asyncio.run(main())
