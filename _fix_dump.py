path = r"f:\syproject\wb-ui\examples\comprehensive_test\main.go"
with open(path, "r", encoding="utf-8") as f:
    lines = f.readlines()

# Find and replace the text segment debug block (lines 283-289 approx)
# Replace the type assertion approach with IsRenderText() approach
new_lines = []
i = 0
replaced = False
while i < len(lines):
    line = lines[i]
    if "Print text segments for RenderText" in line and not replaced:
        # Found the start; replace through the closing brace
        # Skip until we find the closing "}" at the same indent level
        new_lines.append("\t// Print text segments for RenderText to diagnose vertical centering.\n")
        new_lines.append("\tif o.IsRenderText() {\n")
        new_lines.append("\t\tif rt, ok := o.(*rendering.RenderText); ok {\n")
        new_lines.append("\t\t\tsegs := rt.Segments()\n")
        new_lines.append("\t\t\tif len(segs) == 0 {\n")
        new_lines.append("\t\t\t\tfmt.Printf(\"%s  [no segments]\\n\", indent)\n")
        new_lines.append("\t\t\t}\n")
        new_lines.append("\t\t\tfor i, seg := range segs {\n")
        new_lines.append("\t\t\t\tfmt.Printf(\"%s  seg[%d] start=%d len=%d (%.1f,%.1f %.1fx%.1f)\\n\",\n")
        new_lines.append("\t\t\t\t\tindent, i, seg.Start, seg.Len, seg.X, seg.Y, seg.Width, seg.Height)\n")
        new_lines.append("\t\t\t}\n")
        new_lines.append("\t\t} else {\n")
        new_lines.append("\t\t\tfmt.Printf(\"%s  [IsRenderText but type assert failed: %T]\\n\", indent, o)\n")
        new_lines.append("\t\t}\n")
        new_lines.append("\t}\n")
        # Skip the old lines
        i += 1  # skip comment
        i += 1  # skip if rt, ok
        i += 1  # skip for
        i += 1  # skip fmt.Printf
        i += 1  # skip indent line
        i += 1  # skip }
        i += 1  # skip }
        replaced = True
        continue
    new_lines.append(line)
    i += 1

with open(path, "w", encoding="utf-8") as f:
    f.writelines(new_lines)

print("OK" if replaced else "NOT FOUND")
