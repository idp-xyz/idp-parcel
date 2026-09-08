// 供应商协议页的行转写：把 GET /commercial-supplier-agreements 的响应体转成列表模板的行与列
// （票 admin-write-faces/19）。抽出 .tsx 的理由与 policy-rows.ts 同一条：让它在 Node 里跑得起来；这里没有 React，
// 只有形状与词表。

import type { ListColumn } from '../../templates';
import { formatInstant, formatRange } from '../catalogue-view';
import type { SupplierAgreementListResponseBody, SupplierAgreementRecord } from './api';
import { commercialStatusLabels, labelOf } from './presentation';

export interface SupplierAgreementRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function col(id: string, header: string, mono = false): ListColumn<SupplierAgreementRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// 壳的列在前、正文（0021）的列在后。范围与区间各出现两次，前缀「版本」「协议」照发布签的格名分清：壳上的
// 是版本的（登记册逐列比对的项），正文里的是协议自己的，两样不是同一件事（SupplierAgreementPublicationForm
// 版本壳一节的说明原话）。方向不上列：后端刻意不透，前端转写常量等于第二个口径。
export const supplierAgreementColumns: ListColumn<SupplierAgreementRow>[] = [
  col('identity', '协议 / 版本', true),
  col('scope', '版本适用范围', true),
  col('status', '生命周期状态'),
  col('effective', '版本有效区间', true),
  col('publishedAt', '发布时间', true),
  col('contentRegistered', '正文'),
  col('supplier', '供应商', true),
  col('legalEntity', '责任法人', true),
  col('purchasePlan', '采购方案引用', true),
  col('agreementScope', '协议适用范围', true),
  col('agreementEffective', '协议有效区间', true),
  col('registeredAt', '登记时间', true),
];

// 后端 supplierAgreementBody 在正文在场时必然写满的键；agreementEffectiveEndsAt 不在其列——协议区间无上界是
// 登记方说出的合法声明，不是键缺。
const requiredBodyKeys = [
  'supplier',
  'legalEntity',
  'purchasePlan',
  'agreementScope',
  'agreementEffectiveStartsAt',
  'registeredAt',
] as const;

const missingNote = '缺失(响应不合契约)';

// 正文在场与否由服务端的显式布尔说，页面不拿正文键的有无去推：没登记正文与登记了正文但某键为空都表现为键缺席，
// 恢复动作相反（前者去发布正文，后者去查写侧）。布尔为真而键缺是响应不合契约，点名而不是折成「—」——那会让
// 一次坏响应长得像一格正常的空（判据同 policy-rows.ts 的 contentRegisteredCell）。
function contentRegisteredCell(record: SupplierAgreementRecord): string {
  if (!record.contentRegistered) return '未登记';
  if (requiredBodyKeys.some((key) => !record[key])) return '正文缺失(响应不合契约)';
  return '已登记';
}

function orMissing(value: string | undefined): string {
  return value ? value : missingNote;
}

// 只有壳的行正文各格不给值，让模板显「—」：正文那一格已经说了「未登记」，其余各格再各写一遍是噪音；
// 布尔为真时每格各自作答，缺哪一键点名哪一格。
function bodyValues(record: SupplierAgreementRecord): Record<string, string> {
  if (!record.contentRegistered) return {};
  return {
    supplier: orMissing(record.supplier),
    legalEntity: orMissing(record.legalEntity),
    purchasePlan: orMissing(record.purchasePlan),
    agreementScope: orMissing(record.agreementScope),
    agreementEffective: record.agreementEffectiveStartsAt
      ? formatRange(record.agreementEffectiveStartsAt, record.agreementEffectiveEndsAt)
      : missingNote,
    registeredAt: record.registeredAt ? formatInstant(record.registeredAt) : missingNote,
  };
}

export function supplierAgreementRowsOf(body: SupplierAgreementListResponseBody): SupplierAgreementRow[] {
  return body.agreements.map((record) => ({
    key: `${record.objectId}@${record.version}`,
    values: {
      identity: `${record.objectId}@${record.version}`,
      scope: record.scope,
      status: labelOf(commercialStatusLabels, record.status),
      effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
      publishedAt: formatInstant(record.publishedAt),
      contentRegistered: contentRegisteredCell(record),
      ...bodyValues(record),
    },
  }));
}
