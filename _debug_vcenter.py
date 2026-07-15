path = r"f:\syproject\wb-ui\layout\inlineformattingcontext.go"
with open(path, "r", encoding="utf-8") as f:
    content = f.read()

old = '''	vCenterOffset := (lineHeight - ascent - descent) / 2
	if vCenterOffset < 0 {
		vCenterOffset = 0
	}'''

new = '''	vCenterOffset := (lineHeight - ascent - descent) / 2
	if vCenterOffset < 0 {
		vCenterOffset = 0
	}
	if box.Element != nil {
		fmt.Printf("[DEBUG vcenter] tag=%s lineHeight=%.1f ascent=%.1f descent=%.1f vCenterOffset=%.1f contentY=%.1f\\n",
			box.Element.TagName(), lineHeight, ascent, descent, vCenterOffset, box.Rect.ContentY())
	} else {
		fmt.Printf("[DEBUG vcenter] anon lineHeight=%.1f ascent=%.1f descent=%.1f vCenterOffset=%.1f contentY=%.1f\\n",
			lineHeight, ascent, descent, vCenterOffset, box.Rect.ContentY())
	}'''

if old in content:
    # Add fmt import if not present
    if '"fmt"' not in content:
        content = content.replace('import (\n\t"wb-ui/style"\n)', 'import (\n\t"fmt"\n\t"wb-ui/style"\n)')
    content = content.replace(old, new, 1)
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)
    print("OK")
else:
    print("NOT FOUND")
    # Find the relevant lines
    lines = content.split("\n")
    for i, line in enumerate(lines):
        if "vCenterOffset" in line and "lineHeight" in line:
            print(f"Line {i+1}: {repr(line)}")
