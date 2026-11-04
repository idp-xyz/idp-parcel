import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { versionObjectHash, versionObjectOf } from './version-object-address';

// Covers: 两段齐才是对象；页名不对、缺版本段、畸形百分号都不是；查询串不进标识；写回时按段编码、可带键值。
test('商业版本对象页的地址是页名下的两段', () => {
  deepEqual(versionObjectOf('#/service-products/SYN-PROD-01/v1', 'service-products'), { objectId: 'SYN-PROD-01', version: 'v1' });
  deepEqual(versionObjectOf('#/commercial-policies/SYN-RULEPKG-01/v2?register=ACCEPTANCE_RULE_PACKAGE', 'commercial-policies'), {
    objectId: 'SYN-RULEPKG-01',
    version: 'v2',
  });
  equal(versionObjectOf('#/service-products/SYN-PROD-01', 'service-products'), null);
  equal(versionObjectOf('#/service-products/SYN-PROD-01/v1', 'party-contracts'), null);
  equal(versionObjectOf('#/service-products/SYN%ZZ/v1', 'service-products'), null);
  equal(versionObjectHash('service-products', 'A/B', 'v 1'), '#/service-products/A%2FB/v%201');
  equal(
    versionObjectHash('commercial-policies', 'SYN-RULEPKG-01', 'v2', { register: 'ACCEPTANCE_RULE_PACKAGE' }),
    '#/commercial-policies/SYN-RULEPKG-01/v2?register=ACCEPTANCE_RULE_PACKAGE',
  );
});
