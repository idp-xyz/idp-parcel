import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  customerDisplayOf,
  customerNameIndexOf,
  defaultShipmentSort,
  shipmentListConditions,
  shipmentStates,
} from './shipment-request-list';
import { catalogueConditionsNarrow, catalogueRequestPath } from '../catalogue-query';
import { toggleWorkbenchSort } from '../../templates/workbench';
import type { ShipmentRequestSummary } from './api';

// 本文件钉运单工作台的纯逻辑：客户名称从客户账户册转写、转写不到显编码；检索词、状态胶囊与提交时间先后下推成读口的查询条件
// （票 catalogue-read-pagination-rollout/14，契约 ADR-0144 决定三、四）。

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

const path = '/shipment-request-views';

// Covers: 不带检索、状态「全部」、默认新→旧时不收窄，查询串为空——第一页即裸路径（缺省序不写进去）。
test('默认条件即裸路径，不收窄', () => {
  const conditions = shipmentListConditions('', 'ALL', defaultShipmentSort);
  equal(catalogueRequestPath(path, conditions, null), path);
  equal(catalogueConditionsNarrow(conditions), false);
});

// Covers: 状态胶囊下推成 state 一维、检索词去首尾空白成 q；两样都算收窄（零行说「这组条件下没有」）。
test('状态胶囊下推成 state，检索词下推成 q', () => {
  const conditions = shipmentListConditions(' P2026 ', 'ACCEPTED', defaultShipmentSort);
  equal(catalogueRequestPath(path, conditions, null), `${path}?state=ACCEPTED&q=P2026`);
  equal(catalogueConditionsNarrow(conditions), true);
  equal(catalogueConditionsNarrow(shipmentListConditions('', 'WITHDRAWN', defaultShipmentSort)), true);
});

// Covers: 表头点提交时间翻成旧→新时下推 sort=submittedAt；再点回到新→旧即缺省序，不写 sort。排序不收窄。
test('提交时间翻成旧→新时下推 sort，翻回来不写', () => {
  const ascending = toggleWorkbenchSort(defaultShipmentSort, 'submitted-at');
  const conditions = shipmentListConditions('', 'ALL', ascending);
  equal(catalogueRequestPath(path, conditions, null), `${path}?sort=submittedAt`);
  equal(catalogueConditionsNarrow(conditions), false);
  equal(catalogueRequestPath(path, shipmentListConditions('', 'ALL', toggleWorkbenchSort(ascending, 'submitted-at')), null), path);
});

// Covers: 状态胶囊只摆状态维词表里的码，按生命周期顺序。
test('状态胶囊按状态维词表、生命周期顺序', () => {
  deepEqual([...shipmentStates], ['SUBMITTED', 'ACCEPTED', 'REJECTED', 'WITHDRAWN']);
});
