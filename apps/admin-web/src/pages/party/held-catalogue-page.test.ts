import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import { heldCataloguePage } from './held-catalogue-page';

// 本文件钉「在途时垫不垫上一页」：只有这一问还没答出、上一页又有行可摆时才垫；垫着时翻页条照上一页写。

interface Body {
  rows: string[];
  page: { size: number; next: string | null; total: number | null };
}

const body = (rows: string[], next: string | null = null): Body => ({ rows, page: { size: 2, next, total: 5 } });
const hasRows = (candidate: Body) => candidate.rows.length > 0;

test('这一问在途、上一页有行：垫着，翻页条照上一页写', () => {
  const previous = { body: body(['a', 'b'], 'c1'), page: 2 };
  const held = heldCataloguePage<Body>(null, previous, hasRows);
  equal(held?.body, previous.body);
  equal(held?.pagination.page, 2);
  equal(held?.pagination.size, 2);
  equal(held?.pagination.total, 5);
  equal(held?.pagination.next, 'c1');
});

test('这一问已经答出就不垫——成了摆新的，败了摆状态块', () => {
  const previous = { body: body(['a']), page: 1 };
  const answered: ApiResult<Body> = { kind: 'outcome', status: 200, body: body([]) };
  const failed: ApiResult<Body> = { kind: 'noAnswer', status: 502, code: 'NO_ANSWER_FORMED' };
  equal(heldCataloguePage(answered, previous, hasRows), null);
  equal(heldCataloguePage(failed, previous, hasRows), null);
});

test('没有上一页可垫：第一次加载照旧是骨架', () => {
  equal(heldCataloguePage<Body>(null, null, hasRows), null);
});

test('上一页在此刻的页内筛选下一行不剩：不垫', () => {
  // 上一页本来就是空的（筛空），或页内筛选把它的行都筛掉了：摆出来的只会是「没有匹配」那句话，说的却是上一组条件。
  equal(heldCataloguePage<Body>(null, { body: body([]), page: 1 }, hasRows), null);
  equal(heldCataloguePage<Body>(null, { body: body(['a']), page: 1 }, () => false), null);
});
