import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { priceCardUploadField, priceCardUploadForm } from './api';
import { draftSubmittable, sortPriceCardProblems } from './price-card-import';
import type { PriceCardProblem } from './api';

function problem(over: Partial<PriceCardProblem>): PriceCardProblem {
  return { sheet: 'card', row: 1, column: 'planId', code: 'BLANK', message: '空', ...over };
}

test('逐格问题按表、行、列排，同一格保持原顺序', () => {
  const sorted = sortPriceCardProblems([
    problem({ sheet: 'rates', row: 3, column: 'b', code: 'SECOND', message: '后' }),
    problem({ sheet: 'card', row: 2, column: 'a', code: 'CARD', message: '卡' }),
    problem({ sheet: 'rates', row: 3, column: 'a', code: 'COL', message: '列' }),
    problem({ sheet: 'rates', row: 3, column: 'b', code: 'FIRST', message: '先' }),
  ]);
  deepEqual(
    sorted.map((item) => item.code),
    ['CARD', 'COL', 'SECOND', 'FIRST'],
  );
});

test('上传表单只有一个 file 格，文件名原样带上', () => {
  const form = priceCardUploadForm(new Blob(['bytes']), 'card.xlsx');
  equal([...form.keys()].join(','), priceCardUploadField);
  const file = form.get(priceCardUploadField);
  okFile(file);
  equal(file.name, 'card.xlsx');
});

test('存草稿只在这一份文件的预览形成答案之后才开，未配置与传输失败不算看过预览', () => {
  equal(draftSubmittable(null), false);
  equal(draftSubmittable({ kind: 'unconfigured' }), false);
  equal(draftSubmittable({ kind: 'transport', message: '请求未到达' }), false);
  equal(draftSubmittable({ kind: 'noAnswer', status: 503, code: 'STORE_UNAVAILABLE' }), false);
  equal(draftSubmittable({ kind: 'outcome', status: 200, body: { outcome: 'HAS_PROBLEMS' } }), true);
});

function okFile(value: FormDataEntryValue | null): asserts value is File {
  if (!(value instanceof File)) {
    throw new Error('file 格不是文件');
  }
}
