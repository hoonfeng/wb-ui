import os

with open('jsc/globalobject.go', 'r', encoding='utf-8') as f:
    content = f.read()

# Check if String global is set up with fromCharCode
idx = content.find('String')
while idx >= 0:
    start = max(0, idx - 30)
    end = min(len(content), idx + 200)
    snippet = content[start:end]
    if 'fromCharCode' in snippet or 'stringProto' in snippet:
        print("--- String context ---")
        print(repr(snippet[:250]))
        print()
    idx = content.find('String', idx + 1)
    if idx > len(content) - 10:
        break

# Find fromCharCode specifically
idx = content.find('fromCharCode')
if idx >= 0:
    start = max(0, idx - 100)
    end = min(len(content), idx + 100)
    print("=== fromCharCode ===")
    print(repr(content[start:end]))
else:
    print("fromCharCode NOT FOUND in globalobject.go!")

# Find String constructor setup
idx = content.find('String =')
while idx >= 0:
    start = max(0, idx - 50)
    end = min(len(content), idx + 200)
    snippet = content[start:end]
    if 'fromCharCode' in snippet or 'stringProto' in snippet:
        print("=== String assignment ===")
        print(repr(snippet[:250]))
    idx = content.find('String =', idx + 1)
    if idx > len(content) - 10:
        break
