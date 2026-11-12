#!/usr/bin/env python3
# 重放 1cdece260（我方 ADR-0378 落档）时与 main 的 0380 落档并排：
# Status 行两注并排（ours 的注记回插索引注记之后，HEAD 的 0380 注记留在原位）；
# Links 两行都留；CONTEXT-MAP 与计价 CONTEXT 把两份替换文字合成一句。
import re, os

MARK = '<!-- 索引注记 -->'

def blocks(t):
    return list(re.finditer(r'<<<<<<< HEAD\n(.*?)=======\n(.*?)>>>>>>> [^\n]*\n', t, re.S))

def replace_span(t, m, new):
    return t[:m.start()] + new + t[m.end():]

def status_merge(head, ours):
    hi = head.index(MARK) + len(MARK)
    oi = ours.index(MARK) + len(MARK)
    head_after = head[hi:hi+40]
    j = ours.index(head_after, oi)
    seg = ours[oi:j]
    assert seg.startswith('｜'), seg[:20]
    return head[:hi] + seg + head[hi:]

p = 'docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md'
t = open(p, encoding='utf-8').read()
bs = blocks(t)
assert len(bs) == 2
t2 = replace_span(t, bs[1], bs[1].group(1) + bs[1].group(2))
bs = blocks(t2)
t2 = replace_span(t2, bs[0], status_merge(bs[0].group(1), bs[0].group(2)))
open(p, 'w', encoding='utf-8').write(t2)
print('0179 ok')

p = 'docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md'
t = open(p, encoding='utf-8').read()
bs = blocks(t)
assert len(bs) == 2
t2 = replace_span(t, bs[1], bs[1].group(1) + bs[1].group(2))
bs = blocks(t2)
t2 = replace_span(t2, bs[0], status_merge(bs[0].group(1), bs[0].group(2)))
open(p, 'w', encoding='utf-8').write(t2)
print('0260 ok')

p = 'docs/domain/CONTEXT-MAP.md'
t = open(p, encoding='utf-8').read()
bs = blocks(t)
assert len(bs) == 1
head, ours = bs[0].group(1), bs[0].group(2)
hl, ol = head.splitlines(True), ours.splitlines(True)
assert len(hl) == 3 and len(ol) == 3
a = '选卡口径同受理前试算。'
b = '选卡口径同受理前试算，服务产品取覆盖这个包裹的已接受委托的申报，结算的请求不带产品。'
assert hl[0].count(a) == 1 and ol[0].count(b) == 1
l1 = hl[0].replace(a, b)
assert hl[1] == ol[1]
c = '计价按（租户、计价范围、`SELL`、客户费用、该时点）解析唯一在用价卡——与正式 `SELL` 客户费用评价同一口径'
d = ('计价按（租户、计价范围、`SELL`、客户费用、该时点）与这一版申报的服务产品解析唯一在用价卡'
     '——先找指名这个产品的卡、再看不声明产品的卡（[ADR-0378](../adr/0378-a-sell-price-card-may-name-the-service-products-it-prices-and-an-unnamed-card-prices-the-rest.md)），'
     '与正式 `SELL` 客户费用评价同一口径')
assert hl[2].count(c) == 1, 'anchor c'
l3 = hl[2].replace(c, d)
t2 = replace_span(t, bs[0], l1 + hl[1] + l3)
open(p, 'w', encoding='utf-8').write(t2)
print('CONTEXT-MAP ok')

p = 'docs/domain/parcel-pricing/CONTEXT.md'
t = open(p, encoding='utf-8').read()
bs = blocks(t)
assert len(bs) == 2
t2 = replace_span(t, bs[0], bs[0].group(1) + bs[0].group(2))
bs = blocks(t2)
head, ours = bs[0].group(1), bs[0].group(2)
e = '取舍见 [ADR-0171](../../adr/0171-pricing-input-resolver-uses-declaration-until-a-measurement-port-exists.md)。'
i = head.index(e) + len(e)
tail = head[i:]
assert '0377' in tail
merged = ours.rstrip('\n').rstrip()
assert merged.endswith(e.rstrip())
merged = merged + tail
t2 = replace_span(t2, bs[0], merged)
open(p, 'w', encoding='utf-8').write(t2)
print('PP CONTEXT ok')

for p in ['docs/adr/0179-pre-acceptance-control-amount-is-estimated-by-settlement-from-per-parcel-sell-trial-evaluations.md',
          'docs/adr/0260-an-evaluation-selects-its-price-card-by-scope-and-reads-its-pricing-terms-only-from-the-card.md',
          'docs/domain/CONTEXT-MAP.md', 'docs/domain/parcel-pricing/CONTEXT.md']:
    x = open(p, encoding='utf-8').read()
    assert '<<<<<<<' not in x and '>>>>>>>' not in x, p
print('ALL_CLEAN')
