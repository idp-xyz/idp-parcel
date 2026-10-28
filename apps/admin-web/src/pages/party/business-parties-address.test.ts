import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  businessPartiesViewOf,
  businessPartyObjectHash,
  businessPartyRegisters,
  deactivationLinkHash,
  deactivationPresetOf,
  registerViewHash,
  withoutDeactivationPreset,
} from './business-parties-address';

// 本文件钉的是 hash 是位置的唯一权威：列表、三本册的登记视图与对象页都从地址判出，停用深链落到停用册，预填用过即从地址去掉。

test('深链带册、种类与标识，读回来是同一份预填且落在停用册', () => {
  const hash = deactivationLinkHash({ kind: 'LEGAL_ENTITY', id: 'SYN-LE-01' });
  equal(hash, '#/business-parties?register=identity-deactivation&deactivate=LEGAL_ENTITY&id=SYN-LE-01');
  deepEqual(deactivationPresetOf(hash), { kind: 'LEGAL_ENTITY', id: 'SYN-LE-01' });
  deepEqual(businessPartiesViewOf(hash), { kind: 'register', register: 'identity-deactivation' });
});

test('种类不在封闭集或标识为空都当没带', () => {
  equal(deactivationPresetOf('#/business-parties?deactivate=PARTY_RELATIONSHIP&id=X'), null);
  equal(deactivationPresetOf('#/business-parties?deactivate=LEGAL_ENTITY'), null);
  equal(deactivationPresetOf('#/business-parties?deactivate=LEGAL_ENTITY&id=%20'), null);
  equal(deactivationPresetOf('#/business-parties'), null);
});

// Covers: 旧式深链只带 deactivate 与 id、不带 register，照样落到停用册；用过之后去掉预填两键、登记键写实为停用册，检索词照留。
test('旧式停用深链不带登记键也落到停用册；去掉预填后登记键写实', () => {
  const legacy = '#/business-parties?q=SYN&deactivate=LEGAL_ENTITY&id=SYN-LE-01';
  deepEqual(businessPartiesViewOf(legacy), { kind: 'register', register: 'identity-deactivation' });
  equal(withoutDeactivationPreset(legacy), '#/business-parties?q=SYN&register=identity-deactivation');
  equal(
    withoutDeactivationPreset('#/business-parties?register=identity-deactivation&deactivate=LEGAL_ENTITY&id=SYN-LE-01'),
    '#/business-parties?register=identity-deactivation',
  );
  // 没有预填时什么都不加：别把一个普通列表地址改成登记视图。
  equal(withoutDeactivationPreset('#/business-parties?q=SYN'), '#/business-parties?q=SYN');
});

// Covers: register 与 deactivate 并存时登记键为准——预填只有停用表单会读，别的册照登记键开。
test('登记键与停用预填并存时登记键为准', () => {
  deepEqual(businessPartiesViewOf('#/business-parties?register=party-relationship&deactivate=LEGAL_ENTITY&id=X'), {
    kind: 'register',
    register: 'party-relationship',
  });
});

test('标识里的特殊字符经地址往返不走样', () => {
  const id = 'LE/上海 01&x';
  deepEqual(deactivationPresetOf(deactivationLinkHash({ kind: 'CUSTOMER_ACCOUNT', id })), { kind: 'CUSTOMER_ACCOUNT', id });
});

// Covers: 列表 / 三本册 / 对象页三种视图；认不得的册名落回列表；对象地址先认，带了登记键也是对象页。
test('地址判出列表、登记视图与对象页', () => {
  deepEqual(businessPartiesViewOf('#/business-parties'), { kind: 'list' });
  deepEqual(businessPartiesViewOf('#/business-parties?q=SYN'), { kind: 'list' });
  for (const register of businessPartyRegisters) {
    deepEqual(businessPartiesViewOf(`#/business-parties?register=${register}`), { kind: 'register', register });
  }
  deepEqual(businessPartiesViewOf('#/business-parties?register=no-such-book'), { kind: 'list' });
  deepEqual(businessPartiesViewOf('#/business-parties/SYN-PARTY-OPERATOR-01'), { kind: 'object', partyId: 'SYN-PARTY-OPERATOR-01' });
  deepEqual(businessPartiesViewOf('#/business-parties/SYN-PARTY-01?register=business-party'), {
    kind: 'object',
    partyId: 'SYN-PARTY-01',
  });
  // 标识叫 register 的参与方照样有对象页：登记视图不占第二段。
  deepEqual(businessPartiesViewOf('#/business-parties/register'), { kind: 'object', partyId: 'register' });
  deepEqual(businessPartiesViewOf('#/business-parties/%E0'), { kind: 'list' });
  deepEqual(businessPartiesViewOf('#/group-legal-entities/SYN-LE-01'), { kind: 'list' });
});

// Covers: 进出登记视图与换册只改登记键，检索词等别的键原样；对象地址编码标识、经判读往返不走样。
test('登记视图地址只改登记键；对象地址往返不走样', () => {
  equal(registerViewHash('#/business-parties?q=SYN', 'business-party'), '#/business-parties?q=SYN&register=business-party');
  equal(
    registerViewHash('#/business-parties?q=SYN&register=business-party', 'party-relationship'),
    '#/business-parties?q=SYN&register=party-relationship',
  );
  equal(registerViewHash('#/business-parties?q=SYN&register=business-party', null), '#/business-parties?q=SYN');
  const id = 'P/上海 01&x';
  deepEqual(businessPartiesViewOf(businessPartyObjectHash(id)), { kind: 'object', partyId: id });
});
