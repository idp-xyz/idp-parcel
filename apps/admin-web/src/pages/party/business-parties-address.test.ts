import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { deactivationLinkHash, deactivationPresetOf, withoutDeactivationPreset } from './business-parties-address';

test('深链带种类与标识，读回来是同一份预填', () => {
  const hash = deactivationLinkHash({ kind: 'LEGAL_ENTITY', id: 'SYN-LE-01' });
  equal(hash, '#/business-parties?deactivate=LEGAL_ENTITY&id=SYN-LE-01');
  deepEqual(deactivationPresetOf(hash), { kind: 'LEGAL_ENTITY', id: 'SYN-LE-01' });
});

test('种类不在封闭集或标识为空都当没带', () => {
  equal(deactivationPresetOf('#/business-parties?deactivate=PARTY_RELATIONSHIP&id=X'), null);
  equal(deactivationPresetOf('#/business-parties?deactivate=LEGAL_ENTITY'), null);
  equal(deactivationPresetOf('#/business-parties?deactivate=LEGAL_ENTITY&id=%20'), null);
  equal(deactivationPresetOf('#/business-parties'), null);
});

test('用过之后去掉预填两键，检索词照留', () => {
  equal(withoutDeactivationPreset('#/business-parties?q=SYN&deactivate=LEGAL_ENTITY&id=SYN-LE-01'), '#/business-parties?q=SYN');
  equal(withoutDeactivationPreset('#/business-parties?deactivate=LEGAL_ENTITY&id=SYN-LE-01'), '#/business-parties');
});

test('标识里的特殊字符经地址往返不走样', () => {
  const id = 'LE/上海 01&x';
  deepEqual(deactivationPresetOf(deactivationLinkHash({ kind: 'CUSTOMER_ACCOUNT', id })), { kind: 'CUSTOMER_ACCOUNT', id });
});
