import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ApiResult } from './catalogue-api';
import {
  catalogueCursorAnswer,
  catalogueCursorReducer,
  catalogueCursorRequest,
  startCatalogueCursor,
  type CatalogueCursorEvent,
  type CatalogueCursorState,
} from './catalogue-cursor';
import { catalogueConditionsQuery } from './catalogue-query';

// 本文件钉目录页游标翻页的状态转移（票 catalogue-read-pagination/05）。期望值取 ADR-0144 决定一：服务端只给向后的游标，上一页由
// 客户端回退；换了条件旧游标作废、从第一页重取。签名用 catalogueConditionsQuery 造，与页面同一个口。

interface Body {
  page: { size: number; next: string | null; total: number | null };
}

function outcome(next: string | null): ApiResult<Body> {
  return { kind: 'outcome', status: 200, body: { page: { size: 2, next, total: 5 } } };
}

function apply(state: CatalogueCursorState<Body>, ...events: CatalogueCursorEvent<Body>[]): CatalogueCursorState<Body> {
  return events.reduce(catalogueCursorReducer<Body>, state);
}

/** 收下此刻这一问的答复。 */
function answered(state: CatalogueCursorState<Body>, answer: ApiResult<Body>): CatalogueCursorState<Body> {
  return apply(state, { kind: 'answered', key: catalogueCursorRequest(state).key, answer });
}

const all = catalogueConditionsQuery({ q: 'SYN' });
const narrowed = catalogueConditionsQuery({ q: 'SHA' });

// Covers: 票面完成判据那一条走法——检索 → 翻页（带上答复的 next）→ 再翻 → 换检索词回第一页、不带游标。
test('检索 → 翻页 → 换检索词回第一页', () => {
  let state = startCatalogueCursor<Body>(all);
  equal(catalogueCursorRequest(state).after, null);

  state = apply(answered(state, outcome('c1')), { kind: 'next', next: 'c1' });
  equal(catalogueCursorRequest(state).after, 'c1');
  state = apply(answered(state, outcome('c2')), { kind: 'next', next: 'c2' });
  equal(catalogueCursorRequest(state).after, 'c2');
  equal(state.trail.afters.length, 2);

  state = apply(state, { kind: 'conditions', signature: narrowed });
  equal(catalogueCursorRequest(state).after, null);
  equal(state.trail.query, narrowed);
  equal(catalogueCursorAnswer(state), null);
});

// Covers: 上一页由走过的游标回退；回到第一页后再按上一页原样不动。
test('上一页沿走过的游标回退，第一页上原样不动', () => {
  let state = apply(startCatalogueCursor<Body>(all), { kind: 'next', next: 'c1' }, { kind: 'next', next: 'c2' });
  state = apply(state, { kind: 'previous' });
  equal(catalogueCursorRequest(state).after, 'c1');
  state = apply(state, { kind: 'previous' });
  equal(catalogueCursorRequest(state).after, null);
  equal(apply(state, { kind: 'previous' }), state);
});

// Covers: 回第一页这件事记进状态——检索词换走再换回原样，仍在第一页，换条件之前翻到的那一页不复活。
test('条件改回原样仍在第一页，旧轨迹不复活', () => {
  let state = apply(startCatalogueCursor<Body>(all), { kind: 'next', next: 'c1' }, { kind: 'next', next: 'c2' });
  state = apply(state, { kind: 'conditions', signature: narrowed }, { kind: 'conditions', signature: all });
  equal(state.trail.query, all);
  equal(catalogueCursorRequest(state).after, null);
});

// Covers: 条件没变、到头了再翻、同一个 next 再压一次，都原样交回同一个对象——页面在渲染里比对它，相等即不再派发、不重渲。
test('无事可做时原样交回同一个状态对象', () => {
  const state = answered(startCatalogueCursor<Body>(all), outcome('c1'));
  equal(apply(state, { kind: 'conditions', signature: all }), state);
  const second = apply(state, { kind: 'next', next: 'c1' });
  equal(apply(second, { kind: 'next', next: 'c1' }), second);
});

// Covers: 只收此刻这一问的答复——翻页之后才回来的上一页答复被丢掉；手上答复答的不是此刻这一问时交 null（加载中）。
test('慢答复不盖掉新的一问', () => {
  const first = startCatalogueCursor<Body>(all);
  const staleKey = catalogueCursorRequest(first).key;
  const paged = apply(first, { kind: 'next', next: 'c1' });

  const afterStale = apply(paged, { kind: 'answered', key: staleKey, answer: outcome('c1') });
  equal(afterStale, paged);
  equal(catalogueCursorAnswer(afterStale), null);

  const fresh = outcome('c2');
  deepEqual(catalogueCursorAnswer(answered(paged, fresh)), fresh);
});

// Covers: 重试换一个请求键——手上那份答复随之不算数（页面回到加载中），重取回来的那一份才交出；游标不动。
test('重试回到加载中、游标不动，新答复到了才交出', () => {
  const failed: ApiResult<Body> = { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' };
  const state = answered(apply(startCatalogueCursor<Body>(all), { kind: 'next', next: 'c1' }), failed);
  deepEqual(catalogueCursorAnswer(state), failed);

  const retried = apply(state, { kind: 'retry' });
  equal(catalogueCursorAnswer(retried), null);
  equal(catalogueCursorRequest(retried).after, 'c1');
  ok(catalogueCursorRequest(retried).key !== catalogueCursorRequest(state).key);

  const recovered = outcome('c2');
  deepEqual(catalogueCursorAnswer(answered(retried, recovered)), recovered);
});
