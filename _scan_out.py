# -*- coding: utf-8 -*-
import io, sys
sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8', errors='replace')
lines = io.open('diag_out5.txt', encoding='utf-8', errors='replace').read().splitlines()
print('total lines:', len(lines))
for i, l in enumerate(lines):
    if 'UnicharToGlyph' in l or '.ttc' in l or 'not loaded' in l or l.startswith('=='):
        print(i, repr(l[:120]))
