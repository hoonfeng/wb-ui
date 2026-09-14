"""pixel_scan.py — 逐像素扫描编辑器区域，检查文本颜色"""
from PIL import Image
import os

wb_png = os.path.join(os.path.dirname(__file__), "ide_output.png")
wb = Image.open(wb_png).convert("RGBA")
px = wb.load()

# Editor area: x=331 to x=1280, y=66 to y=213 (7 lines x 21px)
# Line 1: y=66..87
# Line 3: y=108..129
# Line 5: y=150..171
# Line 6: y=171..192

print("=== WB-UI Editor Text Color Scan ===")
print("Scanning each pixel row in editor area for non-background colors...\n")

# Background colors: body=#0d1117, sidebar=#161b22
BG1 = (13, 17, 23)   # #0d1117
BG2 = (22, 27, 34)   # #161b22
BG3 = (48, 54, 61)   # #30363d (border)

for line_name, y_start, y_end in [
    ("Line1 (package main)", 66, 87),
    ("Line3 (import fmt)", 108, 129),
    ("Line5 (func main)", 150, 171),
    ("Line6 (Println)", 171, 192),
]:
    print(f"\n--- {line_name} y={y_start}..{y_end} ---")
    colors_found = {}
    text_pixels = []
    for y in range(y_start, y_end):
        for x in range(331, 1280):
            c = px[x, y]
            if c[3] > 10:
                key = (c[0], c[1], c[2])
                colors_found[key] = colors_found.get(key, 0) + 1
                # Is this a "text" color (not background)?
                if key != BG1 and key != BG2 and key != BG3:
                    text_pixels.append((x, y, key))
    
    # Print unique colors sorted by frequency
    sorted_colors = sorted(colors_found.items(), key=lambda x: -x[1])
    print(f"  Background pixels: {sum(1 for k,n in sorted_colors if k in (BG1,BG2,BG3))}")
    print(f"  Text/other pixels: {len(text_pixels)}")
    
    for (r,g,b), n in sorted_colors[:10]:
        tag = ""
        if (r,g,b) == (255, 123, 114): tag = " ← #ff7b72 (package/func/import)"
        elif (r,g,b) == (230, 237, 243): tag = " ← #e6edf3 (main/code text)"
        elif (r,g,b) == (72, 79, 88): tag = " ← #484f58 (line numbers)"
        elif (r,g,b) == (165, 214, 255): tag = " ← #a5d6ff (string)"
        elif (r,g,b) == (210, 168, 255): tag = " ← #d2a8ff (func name)"
        hex_c = f"#{r:02x}{g:02x}{b:02x}"
        print(f"    {hex_c}: {n:5d} pixels{tag}")
    
    if text_pixels:
        print(f"  Sample text pixels (first 5):")
        for x, y, c in text_pixels[:5]:
            print(f"    ({x},{y}) = #{c[0]:02x}{c[1]:02x}{c[2]:02x}")

print("\n\n=== Done ===")
