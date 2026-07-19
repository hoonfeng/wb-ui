import re
inv = r'F:\syproject\ref\JSC_File_Inventory.md'
with open(inv, 'r', encoding='utf-8') as f:
    lines = f.readlines()

count = 0
for line in lines:
    m = re.match(r'- \[( |x)\] `(.+?)`', line)
    if m:
        if count < 5:
            print(f"  [{m.group(1)}] {m.group(2)}")
        count += 1
print(f"Total task entries: {count}")
