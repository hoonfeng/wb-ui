with open(r'F:\syproject\ref\JSC_File_Inventory.md', 'r', encoding='utf-8') as f:
    lines = f.readlines()
print(f'Total lines: {len(lines)}')
print('--- Last 30 lines ---')
for line in lines[-30:]:
    print(line.rstrip())
