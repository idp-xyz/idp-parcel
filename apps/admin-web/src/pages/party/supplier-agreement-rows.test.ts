import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { SupplierAgreementListResponseBody } from './api';
import { supplierAgreementColumns, supplierAgreementRowsOf } from './supplier-agreement-rows';

// 输入照后端 query_commercial_relations_test.go 里
// TestSupplierAgreementsEndpointTranscribesContentOnlyWhenRegistered 钉住的两行：带正文（协议区间带结束、
// 登记时刻晚于发布一小时）与只有壳。正文各键只在 contentRegistered 为真时在场是后端契约，壳在正文缺是合法状态。
const agreements: SupplierAgreementListResponseBody = {
  outcome: 'SUPPLIER_AGREEMENTS_LISTED',
  agreements: [
    {
      objectId: 'agreement-1',
      version: 'v1',
      scope: 'scope-1',
      status: 'EFFECTIVE',
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      publishedAt: '2026-01-01T00:00:00Z',
      contentRegistered: true,
      supplier: 'supplier-1',
      legalEntity: 'legal-1',
      purchasePlan: 'plan-buy-1',
      agreementScope: 'scope-procurement',
      agreementEffectiveStartsAt: '2026-01-01T00:00:00Z',
      agreementEffectiveEndsAt: '2026-01-02T00:00:00Z',
      registeredAt: '2026-01-01T01:00:00Z',
    },
    {
      objectId: 'agreement-2',
      version: 'v1',
      scope: 'scope-1',
      status: 'EFFECTIVE',
      effectiveStartsAt: '2026-01-01T00:00:00Z',
      publishedAt: '2026-01-01T00:00:00Z',
      contentRegistered: false,
    },
  ],
};

// 票 admin-write-faces/19 完成判据的列那一半：壳的列在前、正文（0021）的列在后。范围与区间各出现两次，
// 前缀「版本」「协议」照发布签的格名分清——壳上的是版本的，正文里的是协议自己的，两样不是同一件事。
test('目录列集：壳的列之后是正文各列，壳与正文的范围、区间各带前缀分清', () => {
  deepEqual(
    supplierAgreementColumns.map((column) => column.header),
    [
      '协议 / 版本',
      '版本适用范围',
      '生命周期状态',
      '版本有效区间',
      '发布时间',
      '正文',
      '供应商',
      '责任法人',
      '采购方案引用',
      '协议适用范围',
      '协议有效区间',
      '登记时间',
    ],
  );
});

test('带正文的行逐键上列：正文「已登记」，协议区间带结束，登记时间单列', () => {
  const [full] = supplierAgreementRowsOf(agreements);

  equal(full.key, 'agreement-1@v1');
  equal(full.values.identity, 'agreement-1@v1');
  equal(full.values.scope, 'scope-1');
  equal(full.values.status, '已生效');
  equal(full.values.effective, '2026-01-01 00:00:00 UTC → 持续有效');
  equal(full.values.publishedAt, '2026-01-01 00:00:00 UTC');
  equal(full.values.contentRegistered, '已登记');
  equal(full.values.supplier, 'supplier-1');
  equal(full.values.legalEntity, 'legal-1');
  equal(full.values.purchasePlan, 'plan-buy-1');
  equal(full.values.agreementScope, 'scope-procurement');
  equal(full.values.agreementEffective, '2026-01-01 00:00:00 UTC → 2026-01-02 00:00:00 UTC');
  equal(full.values.registeredAt, '2026-01-01 01:00:00 UTC');
});

// 与合同页同一判据：壳可先入册、正文随发布登记，只有壳的行照列不消失；正文各格不上值（模板显「—」），
// 正文那一格说「未登记」——那是去发布正文的信号，与下面「响应不合契约」要人去查写侧是相反的两件事。
test('只有壳的行照列为「未登记」，正文各格不上值，不从目录上消失', () => {
  const [, bare] = supplierAgreementRowsOf(agreements);

  equal(bare.key, 'agreement-2@v1');
  equal(bare.values.identity, 'agreement-2@v1');
  equal(bare.values.status, '已生效');
  equal(bare.values.contentRegistered, '未登记');
  for (const id of ['supplier', 'legalEntity', 'purchasePlan', 'agreementScope', 'agreementEffective', 'registeredAt']) {
    ok(!(id in bare.values), `只有壳时 ${id} 不该有值`);
  }
});

test('布尔说已登记而正文键缺了是响应不合契约，如实点名而不是显示成空', () => {
  const [odd] = supplierAgreementRowsOf({
    ...agreements,
    agreements: [{ ...agreements.agreements[1], objectId: 'agreement-odd', contentRegistered: true }],
  });

  equal(odd.values.contentRegistered, '正文缺失(响应不合契约)');
  equal(odd.values.supplier, '缺失(响应不合契约)');
  equal(odd.values.legalEntity, '缺失(响应不合契约)');
  equal(odd.values.purchasePlan, '缺失(响应不合契约)');
  equal(odd.values.agreementScope, '缺失(响应不合契约)');
  equal(odd.values.agreementEffective, '缺失(响应不合契约)');
  equal(odd.values.registeredAt, '缺失(响应不合契约)');
});

// 协议区间无上界是登记方说出的合法声明（发布签那格「留空即无上界」），不是键缺：不得点名成不合契约。
test('协议区间不带结束是合法声明，按「持续有效」显示而不是点名缺失', () => {
  const [open] = supplierAgreementRowsOf({
    ...agreements,
    agreements: [{ ...agreements.agreements[0], agreementEffectiveEndsAt: undefined }],
  });

  equal(open.values.contentRegistered, '已登记');
  equal(open.values.agreementEffective, '2026-01-01 00:00:00 UTC → 持续有效');
});
