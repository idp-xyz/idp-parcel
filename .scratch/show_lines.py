import re, sys
p = sys.argv[1]
t = open(p, encoding='utf-8').read()
for i, m in enumerate(re.finditer(r'<<<<<<< HEAD\n(.*?)=======\n(.*?)>>>>>>> [^\n]*\n', t, re.S), 1):
    print(f'=== block {i} HEAD lines:')
    for l in m.group(1).splitlines():
        print('H|', l[:160])
    print(f'=== block {i} OURS lines:')
    for l in m.group(2).splitlines():
        print('O|', l[:160])
    print()
