import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  countShipmentsByState,
  customerDisplayOf,
  customerNameIndexOf,
  defaultShipmentSort,
  filterShipments,
  shipmentCountSummary,
  sortShipments,
} from './shipment-request-list';
import type { ShipmentRequestSummary } from './api';

// 本文件钉运单工作台的纯逻辑：客户名称从客户账户册转写、转写不到显编码；搜索命中运单号 / 客户单号 / 客户名；
// 默认提交时间新→旧；状态计数只列出现过的码且词经词典。

const row = (overrides: Partial<ShipmentRequestSummary>): ShipmentRequestSummary => ({
  shipmentRequestId: 'SHR-1',
  customerAccountId: 'SYN-ACCOUNT-01',
  source: 'https://issuer.example/#syn-customer-system-01',
  sourceRequestKey: 'WALK-1028-A',
  state: 'SUBMITTED',
  submissionVersionId: 'SV-1',
  declaredParcelCount: 1,
  submittedAt: '2026-10-28T10:00:00Z',
  ...overrides,
});

const names = new Map([['SYN-ACCOUNT-01', '合成货主一号']]);

test('客户账户册答复建名称索引；非业务答案交回 null；转写不到的账户不进索引', () => {
  const index = customerNameIndexOf({
    kind: 'outcome',
    status: 200,
    body: {
      outcome: 'CUSTOMER_ACCOUNTS_LISTED',
      accounts: [
        {
          tenantId: 't',
          accountId: 'SYN-ACCOUNT-01',
          customerPartyId: 'P-1',
          customerPartyName: '合成货主一号',
          customerPartyNameKnown: true,
          status: 'EFFECTIVE',
          revision: 1,
          basis: 'b',
          effectiveFrom: '2026-01-01T00:00:00Z',
          registeredAt: '2026-01-01T00:00:00Z',
        },
        {
          tenantId: 't',
          accountId: 'SYN-ACCOUNT-02',
          customerPartyId: 'P-2',
          customerPartyNameKnown: false,
          status: 'EFFECTIVE',
          revision: 1,
          basis: 'b',
          effectiveFrom: '2026-01-01T00:00:00Z',
          registeredAt: '2026-01-01T00:00:00Z',
        },
      ],
      page: { size: 100, next: null, total: null },
    },
  });
  deepEqual([...(index ?? new Map()).entries()], [['SYN-ACCOUNT-01', '合成货主一号']]);
  equal(customerNameIndexOf(null), null);
  equal(customerNameIndexOf({ kind: 'transport', message: 'x' }), null);
});

test('客户格有名称显名称，没有显编码并标明是编码', () => {
  deepEqual(customerDisplayOf(row({}), names), { text: '合成货主一号', isCode: false });
  deepEqual(customerDisplayOf(row({ customerAccountId: 'SYN-ACCOUNT-09' }), names), { text: 'SYN-ACCOUNT-09', isCode: true });
});

test('搜索命中运单号、客户单号、客户编码或客户名称；状态筛选只留那一格', () => {
  const rows = [row({}), row({ shipmentRequestId: 'SHR-2', customerAccountId: 'SYN-ACCOUNT-02', sourceRequestKey: 'K-2', state: 'ACCEPTED' })];
  equal(filterShipments(rows, { search: '货主一号', state: 'ALL' }, names).length, 1);
  equal(filterShipments(rows, { search: 'k-2', state: 'ALL' }, names).length, 1);
  equal(filterShipments(rows, { search: 'account-02', state: 'ALL' }, names).length, 1);
  equal(filterShipments(rows, { search: '', state: 'ACCEPTED' }, names)[0].shipmentRequestId, 'SHR-2');
  equal(filterShipments(rows, { search: '', state: 'ALL' }, names).length, 2);
});

test('默认按提交时间新→旧；件数与客户名可排', () => {
  deepEqual(defaultShipmentSort, { key: 'submitted-at', dir: -1 });
  const rows = [
    row({ shipmentRequestId: 'A', submittedAt: '2026-10-28T10:00:00Z', declaredParcelCount: 3 }),
    row({ shipmentRequestId: 'B', submittedAt: '2026-10-28T11:00:00Z', declaredParcelCount: 1, customerAccountId: 'SYN-ACCOUNT-02' }),
  ];
  deepEqual(sortShipments(rows, defaultShipmentSort, names).map((r) => r.shipmentRequestId), ['B', 'A']);
  deepEqual(sortShipments(rows, { key: 'parcel-count', dir: 1 }, names).map((r) => r.shipmentRequestId), ['B', 'A']);
  deepEqual(sortShipments(rows, { key: 'customer', dir: 1 }, names).map((r) => r.shipmentRequestId), ['B', 'A']);
  equal(sortShipments(rows, { key: 'nowhere', dir: 1 }, names).length, 2);
});

test('状态计数只列出现过的码、按生命周期顺序、词经词典', () => {
  const rows = [row({}), row({ state: 'WITHDRAWN' }), row({ state: 'ACCEPTED' }), row({})];
  deepEqual(countShipmentsByState(rows), [
    { code: 'SUBMITTED', label: '待受理', count: 2 },
    { code: 'ACCEPTED', label: '已受理', count: 1 },
    { code: 'WITHDRAWN', label: '已撤回', count: 1 },
  ]);
  deepEqual(countShipmentsByState([]), []);
  equal(shipmentCountSummary(4, 2), '共 4 张运单，当前显示 2 张');
});
