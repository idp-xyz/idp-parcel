import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { identityMayBeDeactivated, identityTakesRevisions } from './identity-write-entries';

// 本文件钉身份上几个写入口摆不摆：判据是「登过停用没有」（行上带不带停用时点），不是状态。

// Covers: 停用入口——登记册对同一个身份只落一笔停用：行上带着停用时点就不再摆；停用时点还没到（状态仍是已生效）那一笔
// 也已经登了，同样不摆。
test('identityMayBeDeactivated 登过停用的不再摆停用入口', () => {
  equal(identityMayBeDeactivated({}), true);
  equal(identityMayBeDeactivated({ deactivatedAt: '2026-06-01T00:00:00Z' }), false);
  equal(identityMayBeDeactivated({ deactivatedAt: '2099-01-01T00:00:00Z' }), false);
});

// Covers: 更正登记信息的入口——登过停用的身份不再收新修订，停用时点到没到都一样。
test('identityTakesRevisions 登过停用的不再收新修订', () => {
  equal(identityTakesRevisions({}), true);
  equal(identityTakesRevisions({ deactivatedAt: '2026-06-01T00:00:00Z' }), false);
  equal(identityTakesRevisions({ deactivatedAt: '2099-01-01T00:00:00Z' }), false);
});
