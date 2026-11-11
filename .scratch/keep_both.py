#!/usr/bin/env python3
# 用法: keep_both.py <file> <ours_first|head_first> [块序号(1 起), 默认全部]
# 把冲突块解决为两侧都保留、按给定顺序拼接（块内整体拼接，不做行内合并）。
import re, sys

p, order = sys.argv[1], sys.argv[2]
only = set(int(x) for x in sys.argv[3:]) if len(sys.argv) > 3 else None
t = open(p, encoding='utf-8').read()
pat = re.compile(r'<<<<<<< HEAD\n(.*?)=======\n(.*?)>>>>>>> [^\n]*\n', re.S)
out, i, n = [], 0, 0
for m in pat.finditer(t):
    n += 1
    out.append(t[i:m.start()])
    head, ours = m.group(1), m.group(2)
    if only is not None and n not in only:
        out.append(m.group(0))
    elif order == 'ours_first':
        out.append(ours + head)
    else:
        out.append(head + ours)
    i = m.end()
out.append(t[i:])
open(p, 'w', encoding='utf-8').write(''.join(out))
print(f'{p}: {n} block(s) processed')
