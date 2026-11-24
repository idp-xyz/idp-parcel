import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { asCatalogueRegistration, asStockImportWrite } from './answers';
import {
  asWarehouseAction,
  internalOpsActionSpecs,
  outboundWaveActionSpecs,
  pricingActionSpecs,
  receivingActionSpecs,
} from './actions';
import { catalogueReadPath } from './rows';

test('五册拒绝理由搬到登记面板认的键，指出的格与最新修订写成原因', () => {
  deepEqual(
    asCatalogueRegistration({
      outcome: 'NOT_ACCEPTED',
      refusal: 'CONTENT_INVALID',
      field: 'address.country',
      latestRevision: 2,
    }),
    {
      outcome: 'NOT_ACCEPTED',
      refusalReason: 'CONTENT_INVALID',
      cause: '指出的格：address.country；册上最新修订：2',
    },
  );
  deepEqual(asCatalogueRegistration({ outcome: 'REGISTERED' }), { outcome: 'REGISTERED' });
  deepEqual(asCatalogueRegistration({ outcome: 'NOT_ACCEPTED', refusal: 'REVISION_OUT_OF_SEQUENCE', latestRevision: 0 }), {
    outcome: 'NOT_ACCEPTED',
    refusalReason: 'REVISION_OUT_OF_SEQUENCE',
    cause: '册上最新修订：0',
  });
});

test('建账导入的不受理理由与逐行不合写成原因，没收录的种类原样留下', () => {
  equal(
    asStockImportWrite({
      outcome: 'CONTENT_INVALID',
      issues: [{ side: 'GOODS', line: 1, kind: 'QUANTITY_NOT_POSITIVE' }],
    }).cause,
    '商品行第 1 行：数量不是正数',
  );
  equal(asStockImportWrite({ outcome: 'WAREHOUSE_NOT_ACCEPTING', reason: 'AGENT' }).cause, '代理仓不进自营库存账');
  equal(asStockImportWrite({ outcome: 'CONTENT_INVALID', issues: [{ side: 'CARTON', line: 2, kind: 'NEW_KIND' }] }).cause, '整箱行第 2 行：NEW_KIND');
  equal(asStockImportWrite({ outcome: 'SUBMITTED' }).cause, undefined);
});

test('换册放进选择器，发出的路径不带这一维', () => {
  equal(
    catalogueReadPath({ selectors: { family: 'zone' }, q: 'SYN', filters: { deactivated: ['false'] } }, null),
    '/warehouse-fulfillment-zones?deactivated=false&q=SYN',
  );
  equal(catalogueReadPath({ selectors: { family: 'packaging' } }, 'cursor-1'), '/warehouse-fulfillment-packaging-materials?after=cursor-1');
  equal(catalogueReadPath({}, null), '/warehouse-fulfillment-warehouses');
});

test('仓配写口答复把单据号、发生项、状态和逐行不合折进原因', () => {
  equal(
    asWarehouseAction({ outcome: 'RESERVED', status: 'RESERVED', documentNumber: 'OB-20261124-00001' }).cause,
    '当前状态：RESERVED；单据号：OB-20261124-00001',
  );
  equal(
    asWarehouseAction({ outcome: 'FORMED', occurrenceId: 'CHO-1', version: 2 }).cause,
    '发生项：CHO-1（第 2 版）',
  );
  equal(
    asWarehouseAction({ outcome: 'CONTENT_INVALID', issues: [{ line: 1, kind: 'MOVE_IN_DEACTIVATED' }] }).cause,
    '第 1 行：MOVE_IN_DEACTIVATED',
  );
  equal(
    asWarehouseAction({ outcome: 'NOT_ACCEPTED', refusal: 'CONTENT_INVALID', field: 'warehouse' }).refusalReason,
    'CONTENT_INVALID',
  );
});

test('仓配其余四页的写动作端点与编号不重复且均以斜杠开头', () => {
  const all = [
    ...receivingActionSpecs,
    ...internalOpsActionSpecs,
    ...outboundWaveActionSpecs,
    ...pricingActionSpecs,
  ];
  const ids = new Set(all.map((spec) => spec.id));
  const endpoints = new Set(all.map((spec) => spec.endpoint));
  equal(ids.size, all.length);
  equal(endpoints.size, all.length);
  for (const spec of all) {
    equal(spec.endpoint.startsWith('/'), true, spec.endpoint);
  }
});
