import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import type { BusinessPartyListResponseBody, GroupLegalEntityListResponseBody } from './api';
import { emptyNameDirectory, nameDirectoryOf, refHash, refName } from './ref-links';

test('引用的去处是对象所在的页，标识作检索词', () => {
  equal(refHash('party', 'SYN-PARTY-01'), '#/business-parties?q=SYN-PARTY-01');
  equal(refHash('legal-entity', 'SYN-LE-01'), '#/group-legal-entities?q=SYN-LE-01');
  equal(refHash('contract', 'SYN-CONTRACT-01'), '#/party-contracts?q=SYN-CONTRACT-01');
  equal(refHash('service-product', 'SYN-PROD-1'), '#/service-products?q=SYN-PROD-1');
});

test('规则包与财务控制策略落到商业规则与策略页对应的册', () => {
  // 接单规则包是那一页的缺省册，地址里不写册。
  equal(refHash('rule-package', 'SYN-RULEPKG-01'), '#/commercial-policies?q=SYN-RULEPKG-01');
  equal(
    refHash('financial-control-policy', 'SYN-FIN-CONTROL-01'),
    '#/commercial-policies?register=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY&q=SYN-FIN-CONTROL-01',
  );
});

test('标识里的特殊字符经地址编码，不被当成别的键', () => {
  equal(refHash('party', 'A&b=c'), '#/business-parties?q=A%26b%3Dc');
});

test('名称目录：参与方按标识取名，法人取它所钉参与方的名称，悬空的不进目录', () => {
  const directory = nameDirectoryOf(
    { parties: [{ partyId: 'P1', partyName: '货主甲' }] } as unknown as BusinessPartyListResponseBody,
    {
      entities: [
        { legalEntityId: 'LE1', partyName: '运营方乙', partyNameKnown: true },
        { legalEntityId: 'LE2', partyNameKnown: false },
      ],
    } as unknown as GroupLegalEntityListResponseBody,
  );
  equal(refName(directory, 'party', 'P1'), '货主甲');
  equal(refName(directory, 'legal-entity', 'LE1'), '运营方乙');
  equal(refName(directory, 'legal-entity', 'LE2'), null);
  equal(refName(directory, 'party', 'P9'), null);
});

test('合同、产品、规则包没有名称；目录没取到时一律只显标识', () => {
  equal(refName(emptyNameDirectory, 'party', 'P1'), null);
  equal(refName(nameDirectoryOf(null, null), 'legal-entity', 'LE1'), null);
  equal(refName(emptyNameDirectory, 'contract', 'C1'), null);
});
