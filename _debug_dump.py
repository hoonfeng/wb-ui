path = r"f:\syproject\wb-ui\examples\comprehensive_test\main.go"
with open(path, "r", encoding="utf-8") as f:
    lines = f.readlines()

for i in range(280, min(295, len(lines))):
    print(f"Line {i+1}: {repr(lines[i])}")
