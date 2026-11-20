import { createElement } from 'react';
import { test } from 'node:test';
import { equal, match, ok } from 'node:assert/strict';
import { renderToStaticMarkup } from 'react-dom/server';
import type { PriceCardOverlap } from './api';
import { priceCardOverlapLines } from './price-card-overlap';
import { PriceCardOverlapNotice } from './PriceCardOverlapNotice';

// 三处都摆这一块：预览步 StepPreview、导入签 PriceCardImportPanel、草稿签 PriceCardDraftsPanel。
// 那三个组件还拉着按钮库，node:test 的 CommonJS 装载不了它；话与琥珀色在这里钉死，三处只负责把 overlaps 交进来。

const doesNotBlock = '不阻止保存、批准或发布';

function overlap(over: Partial<PriceCardOverlap> & Pick<PriceCardOverlap, 'resolution' | 'samePlan'>): PriceCardOverlap {
  return {
    plan: { id: 'SYN-PLAN', version: 'v1' },
    selectionStart: '2026-01-01T00:00:00Z',
    overlapStart: '2026-06-01T00:00:00Z',
    ...over,
  };
}

const samples: PriceCardOverlap[] = [
  overlap({ resolution: 'SUCCESSOR_CUT', samePlan: true }),
  overlap({ resolution: 'RETIRE_PREDECESSOR', samePlan: true, plan: { id: 'SYN-PLAN', version: 'v0' } }),
  overlap({ resolution: 'GOVERNANCE_DECISION', samePlan: true, overlapEnd: '2026-09-01T00:00:00Z' }),
  overlap({ resolution: 'GOVERNANCE_DECISION', samePlan: false, plan: { id: 'SYN-OTHER', version: 'v1' } }),
];

test('四种收尾分开说，并且写明不阻止保存、批准或发布', () => {
  const lines = priceCardOverlapLines(samples);
  equal(lines.length, 4);
  match(lines[0].text, /后继截止/);
  match(lines[1].text, /退役/);
  match(lines[2].text, /盖不住旧版的尾部/);
  match(lines[2].text, /空档/);
  ok(!/另一个方案/.test(lines[2].text));
  match(lines[3].text, /另一个方案 SYN-OTHER@v1/);
  match(lines[3].text, /要人来裁/);
  ok(!/盖不住/.test(lines[3].text));
  for (const line of lines) match(line.text, new RegExp(doesNotBlock));
  equal(priceCardOverlapLines(undefined).length, 0);
  equal(priceCardOverlapLines([]).length, 0);
});

test('提示是琥珀色说明，没有重叠就不占一行', () => {
  const html = renderToStaticMarkup(createElement(PriceCardOverlapNotice, { overlaps: samples }));
  match(html, /role="note"/);
  match(html, /aria-label="适用期重叠"/);
  match(html, /--ds-status-warning-fg/);
  match(html, /后继截止/);
  match(html, /应走退役/);
  match(html, /盖不住旧版的尾部/);
  match(html, /另一个方案/);
  match(html, new RegExp(doesNotBlock));
  equal(renderToStaticMarkup(createElement(PriceCardOverlapNotice, { overlaps: [] })), '');
  equal(renderToStaticMarkup(createElement(PriceCardOverlapNotice, {})), '');
});
