import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { answerFreshForMs, answerStaleTimeOf, answerToShow } from './freshness';

// 钉 ADR-0360 决定二的两条判读：只有答到了才算新鲜；重读在途时显什么。接 TanStack Query 那半由浏览器核验覆盖。

const outcome = { kind: 'outcome' as const };
const transport = { kind: 'transport' as const };
const unconfigured = { kind: 'unconfigured' as const };

test('只有答到了的答案在新鲜期内算新的；读不到、未配置与还没答都即刻过期', () => {
  equal(answerStaleTimeOf(outcome), answerFreshForMs);
  equal(answerStaleTimeOf(transport), 0);
  equal(answerStaleTimeOf(unconfigured), 0);
  equal(answerStaleTimeOf(undefined), 0);
});

test('重读在途时照旧显上一份答到了的；上一份不是答到了的按加载画', () => {
  equal(answerToShow(outcome, true), outcome);
  equal(answerToShow(transport, true), null);
  equal(answerToShow(unconfigured, true), null);
});

test('不在途时显手上那一份，读不到也如实显；还没有答案是加载', () => {
  equal(answerToShow(outcome, false), outcome);
  equal(answerToShow(transport, false), transport);
  equal(answerToShow(undefined, false), null);
  equal(answerToShow(undefined, true), null);
});
