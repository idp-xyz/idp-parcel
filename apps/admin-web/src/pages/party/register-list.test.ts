import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { ApiResult } from '../catalogue-api';
import { registerListReducer, type RegisterListState } from './register-list';

// 本文件钉的是册页列表状态在「重取」这一格上的规则（票 admin-web-group-legal-entities/13 第 6 条，出处票 10 评审 S1）：
// 重取期间登记签看到的册面是上一份业务答案，不是「没取到」——否则修订建议闪回 1、标识格提示句闪成「列表里没有」。
// 上一份不是业务答案（未配置 / 出错）时照旧回到加载中：那时留着它，按下「重试」的人看不到任何反应。

type Body = { rows: string[] };

const outcome: ApiResult<Body> = { kind: 'outcome', status: 200, body: { rows: ['SYN-PARTY-01'] } };
const nextOutcome: ApiResult<Body> = { kind: 'outcome', status: 200, body: { rows: ['SYN-PARTY-01', 'SYN-PARTY-02'] } };
const unconfigured: ApiResult<Body> = { kind: 'unconfigured' };
const transport: ApiResult<Body> = { kind: 'transport', message: 'ECONNREFUSED' };

const state = (answer: ApiResult<Body> | null, version = 0): RegisterListState<Body> => ({ answer, version });

// Covers: 重取时上一份业务答案留着、重取序号加一——列表与登记签在新答案到来前继续按旧册面算，不闪回「没取到」。
test('重取保留上一份业务答案', () => {
  deepEqual(registerListReducer(state(outcome, 3), { kind: 'reload' }), state(outcome, 4));
});

// Covers: 上一份不是业务答案时重取回到加载中——未配置 / 出错态下的「重试」要有可见反应；首取之前也是加载中。
test('上一份不是业务答案时重取回到加载中', () => {
  deepEqual(registerListReducer(state(unconfigured, 1), { kind: 'reload' }), state(null, 2));
  deepEqual(registerListReducer(state(transport, 1), { kind: 'reload' }), state(null, 2));
  deepEqual(registerListReducer(state(null), { kind: 'reload' }), state(null, 1));
});

// Covers: 答案到来只换答案、不动重取序号——序号是「问了几次」，不是「答了几次」。
test('答案到来只换答案', () => {
  const after = registerListReducer(state(outcome, 4), { kind: 'answered', answer: nextOutcome });
  deepEqual(after, state(nextOutcome, 4));
  equal(registerListReducer(state(null, 0), { kind: 'answered', answer: transport }).answer, transport);
});
