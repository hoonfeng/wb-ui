# -*- coding: utf-8 -*-
import io

with io.open(r'F:\syproject\wb-ui\layout\blockformattingcontext.go', 'r', encoding='utf-8', newline='') as f:
    lines = f.readlines()

for i in range(90, 103):
    if i < len(lines):
        print(f'L{i}: {repr(lines[i])}')
