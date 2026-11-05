import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  customerAccountLocalProblems,
  customerAccountPayloadOf,
  emptyCustomerAccountDraft,
  suggestedCustomerAccountRevision,
} from './customer-account-form';

// Covers: 载荷一项一批、不带租户；身份串原样（不裁空白）；修订号与生效时刻编不出来时键缺席、本地点名；建议修订号按账户原串查。
test('客户账户草稿编成登记载荷', () => {
  const draft = { ...emptyCustomerAccountDraft(), accountId: ' SYN-ACCOUNT-09', customerPartyId: 'SYN-PARTY-SHIPPER-01', basis: 'B-1', revision: '1', effectiveFrom: '2026-11-05T08:00' };
  deepEqual(customerAccountPayloadOf(draft, 'Asia/Shanghai'), {
    customerAccounts: [{ accountId: ' SYN-ACCOUNT-09', customerPartyId: 'SYN-PARTY-SHIPPER-01', basis: 'B-1', revision: 1, effectiveFrom: '2026-11-05T00:00:00Z' }],
  });
  const bad = { ...draft, revision: 'x', effectiveFrom: 'not-a-time' };
  deepEqual(Object.keys(customerAccountLocalProblems(bad, 'Asia/Shanghai')).sort(), ['customerAccounts[0].effectiveFrom', 'customerAccounts[0].revision']);
  deepEqual(customerAccountPayloadOf(bad, 'Asia/Shanghai').customerAccounts[0].revision, undefined);
  const known = [{ accountId: 'SYN-ACCOUNT-01', revision: 2 }] as never;
  equal(suggestedCustomerAccountRevision(known, 'SYN-ACCOUNT-01'), 3);
  equal(suggestedCustomerAccountRevision(known, ' SYN-ACCOUNT-01'), 1);
  equal(suggestedCustomerAccountRevision(null, 'X'), 1);
});
