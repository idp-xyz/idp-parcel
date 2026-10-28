import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import { entryHint, heldGrantsOf, hiddenEntryHint, mayWriteRegistry, type OperatorGrantsResponseBody } from './operator-grants';

// 本文件钉授予自查答复的判读与「摆不摆写入口」的口径（ADR-0315）：只有业务成格才算拿到；明确没有登记册配置写授予才收起
// 入口，各种答不出一律当不知道、入口照摆——摆不摆只是呈现，服务端仍是裁决方。

function resolved(grants: string[], decisions: string[] = []): { kind: 'outcome'; status: number; body: OperatorGrantsResponseBody } {
  return { kind: 'outcome', status: 200, body: { outcome: 'OPERATOR_GRANTS_RESOLVED', grants, decisions } };
}

test('成格才答出授予，面与决定种类各一集', () => {
  const held = heldGrantsOf(resolved(['MASTER_DATA_AND_OPERATIONS_READ', 'REGISTRY_CONFIGURATION_WRITE'], ['MANUAL_REVIEW_COMPLETION']));
  ok(held);
  equal(held.faces.has('REGISTRY_CONFIGURATION_WRITE'), true);
  equal(held.decisions.has('MANUAL_REVIEW_COMPLETION'), true);
});

test('答不出一律是不知道', () => {
  equal(heldGrantsOf({ kind: 'outcome', status: 200, body: { outcome: 'SOMETHING_ELSE', grants: [], decisions: [] } }), undefined);
  equal(heldGrantsOf({ kind: 'outcome', status: 200, body: { outcome: 'OPERATOR_GRANTS_RESOLVED' } as unknown as OperatorGrantsResponseBody }), undefined);
  equal(heldGrantsOf({ kind: 'outcome', status: 200, body: null as unknown as OperatorGrantsResponseBody }), undefined);
  equal(heldGrantsOf({ kind: 'unconfigured' }), undefined);
  equal(heldGrantsOf({ kind: 'loginRequired' }), undefined);
  equal(heldGrantsOf({ kind: 'notGranted' }), undefined);
  equal(heldGrantsOf({ kind: 'noAnswer', status: 503, code: 'IDENTITY_DEPENDENCY_UNAVAILABLE' }), undefined);
  equal(heldGrantsOf({ kind: 'transport', message: 'ECONNREFUSED' }), undefined);
  equal(heldGrantsOf(null), undefined);
});

test('明确没有登记册配置写授予才收起写入口', () => {
  equal(mayWriteRegistry(heldGrantsOf(resolved(['MASTER_DATA_AND_OPERATIONS_READ']))), false);
  equal(mayWriteRegistry(heldGrantsOf(resolved(['MASTER_DATA_AND_OPERATIONS_READ', 'REGISTRY_CONFIGURATION_WRITE']))), true);
  // 没有任何授予也是「明确知道没有」，不是不知道；服务端对这种操作者本来就连读都拒，入口收不收无所谓，口径不为它开例外。
  equal(mayWriteRegistry(heldGrantsOf(resolved([]))), false);
  equal(mayWriteRegistry(undefined), true);
});

test('指向写入口的文案跟着入口走：摆着时说它在哪，收起时说谁能用', () => {
  equal(entryHint(true, '在「登记」页签登记第一个。'), '在「登记」页签登记第一个。');
  equal(entryHint(false, '在「登记」页签登记第一个。'), hiddenEntryHint);
});
