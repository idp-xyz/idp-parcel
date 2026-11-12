import re, os, sys
p = sys.argv[1]
t = open(p, encoding='utf-8').read()
for i, m in enumerate(re.finditer(r'<<<<<<< HEAD\n(.*?)=======\n(.*?)>>>>>>> [^\n]*\n', t, re.S), 1):
    h, o = m.group(1), m.group(2)
    c = os.path.commonprefix([h, o])
    # 公共后缀
    k = 0
    while k < min(len(h), len(o)) - len(c) and h[len(h)-1-k] == o[len(o)-1-k]:
        k += 1
    print(f'=== block {i} | common prefix {len(c)} | common suffix {k}')
    print('HEAD mid:', h[max(0,len(c)-15):len(h)-k if k else len(h)][:900])
    print('--- ours mid:')
    print('OURS mid:', o[max(0,len(c)-15):len(o)-k if k else len(o)][:900])
    print()
