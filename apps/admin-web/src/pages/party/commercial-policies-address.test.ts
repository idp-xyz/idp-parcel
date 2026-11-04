import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { policiesHash, policiesRegisterOf, policiesTabOf, policyObjectKindOf, publishTargetOf } from './commercial-policies-address';
import { commercialPolicyKinds } from './presentation';

test('地址里没写签与册时落在政策册签、接单规则包', () => {
  equal(policiesTabOf('#/commercial-policies'), 'registers');
  equal(policiesRegisterOf('#/commercial-policies'), 'ACCEPTANCE_RULE_PACKAGE');
});

test('签与册写进地址再读回是同一处；缺省值不写', () => {
  const hash = policiesHash('#/commercial-policies', { tab: 'publish', register: 'CREDIT_POLICY' });
  equal(hash, '#/commercial-policies?tab=publish&register=CREDIT_POLICY');
  equal(policiesTabOf(hash), 'publish');
  equal(policiesRegisterOf(hash), 'CREDIT_POLICY');
  equal(policiesHash(hash, { tab: 'registers', register: 'ACCEPTANCE_RULE_PACKAGE' }), '#/commercial-policies');
});

test('认不得的签与册退回缺省，不猜', () => {
  equal(policiesTabOf('#/commercial-policies?tab=publish-credit-policy'), 'registers');
  equal(policiesRegisterOf('#/commercial-policies?register=PRICE_RULE'), 'ACCEPTANCE_RULE_PACKAGE');
});

test('写签与册时检索词照留', () => {
  equal(
    policiesHash('#/commercial-policies?q=SYN', { tab: 'json', register: 'SETTLEMENT_POLICY' }),
    '#/commercial-policies?q=SYN&tab=json&register=SETTLEMENT_POLICY',
  );
});

test('七本册有自己的发布表单，随所属版本发布的两本各指去向', () => {
  const targets = Object.fromEntries(commercialPolicyKinds.map((kind) => [kind, publishTargetOf(kind).kind]));
  deepEqual(targets, {
    ACCEPTANCE_RULE_PACKAGE: 'form',
    PRE_ACCEPTANCE_CONTROL: 'with-contract',
    PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY: 'form',
    PRICE_POLICY: 'form',
    SETTLEMENT_POLICY: 'form',
    AS_OF_POLICY: 'with-rule-package',
    AUTHORIZATION_RULE: 'form',
    CREDIT_POLICY: 'form',
    CUSTOMER_SERVICE_RULE: 'form',
  });
});

// Covers: 价格政策换对象类别名；两本声明册没有对象页；其余同名。
test('册到商业对象类别的映射', () => {
  equal(policyObjectKindOf('PRICE_POLICY'), 'PRICE_RULE');
  equal(policyObjectKindOf('AS_OF_POLICY'), null);
  equal(policyObjectKindOf('PRE_ACCEPTANCE_CONTROL'), null);
  equal(policyObjectKindOf('CREDIT_POLICY'), 'CREDIT_POLICY');
});
