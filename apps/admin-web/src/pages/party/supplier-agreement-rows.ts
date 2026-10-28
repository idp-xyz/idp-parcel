// 供应商协议页的行转写：把 GET /commercial-supplier-agreements 的响应体转成列表模板的行与列
// （票 admin-write-faces/19）。抽出 .tsx 的理由与 policy-rows.ts 同一条：让它在 Node 里跑得起来；这里没有 React，
// 只有形状与词表。

import type { ListColumn } from '../../templates';
import { formatInstant, formatRange } from '../catalogue-view';
import type { SupplierAgreementListResponseBody, SupplierAgreementRecord } from './api';
import { commercialStatusLabels, labelOf } from './presentation';
import type { RowRefs } from './ref-links';
import type { VersionFacts } from './version-groups';

export interface SupplierAgreementRow {
  key: string;
  values: Readonly<Record<string, string>>;
  /** 供应商与责任法人两格的引用原值，页面据它画成带名称的链接（票 party-commercial-uiux/07）。 */
  refs: RowRefs;
  /** 版本排列与「在用」筛选用的几项（票 party-commercial-uiux/08）。 */
  version: VersionFacts;
  /** 这一行对应的读口记录；「发新版本」要从它还原发布表单的草稿（票 party-commercial-uiux/14）。 */
  source: SupplierAgreementRecord;
}

function col(id: string, header: string, mono = false): ListColumn<SupplierAgreementRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// 主列只留运营要看的：协议、供应商、我方公司主体、状态、采购价格表、协议有效期、正文登了没有。版本自己的范围与区间、
// 发布与登记时间、协议适用范围是登记册逐列比对的项（SupplierAgreementPublicationForm 版本壳一节的说明原话），运营日常不看，
// 收进详情抽屉底部的「技术详情」（supplierAgreementTechnicalRows）。方向不上列：后端刻意不透，前端转写常量等于第二个口径。
export const supplierAgreementColumns: ListColumn<SupplierAgreementRow>[] = [
  col('identity', '协议', true),
  col('supplier', '供应商', true),
  col('legalEntity', '公司主体', true),
  col('status', '状态'),
  col('purchasePlan', '采购价格表', true),
  col('agreementEffective', '协议有效期', true),
  col('contentRegistered', '正文'),
];

/** 详情抽屉「技术详情」里的行：版本壳的范围与区间、两处时间与协议适用范围。 */
export function supplierAgreementTechnicalRows(row: SupplierAgreementRow): { label: string; value: string }[] {
  return [
    { label: '协议标识', value: row.source.objectId },
    { label: '版本号', value: row.source.version },
    { label: '版本适用范围', value: row.values.scope ?? '' },
    { label: '版本有效区间', value: row.values.effective ?? '' },
    { label: '发布时间', value: row.values.publishedAt ?? '' },
    { label: '协议适用范围', value: row.values.agreementScope ?? '' },
    { label: '正文登记时间', value: row.values.registeredAt ?? '' },
  ];
}

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

const missingNote = '数据异常：缺这一项';

// 正文在场与否由服务端的显式布尔说，页面不拿正文键的有无去推：没登记正文与登记了正文但某键为空都表现为键缺席，
// 恢复动作相反（前者去发布正文，后者去查写侧）。布尔为真而键缺是响应不合契约，点名而不是折成「—」——那会让
// 一次坏响应长得像一格正常的空（判据同 policy-rows.ts 的 contentRegisteredCell）。
function contentRegisteredCell(record: SupplierAgreementRecord): string {
  if (!record.contentRegistered) return '未登记';
  if (requiredBodyKeys.some((key) => !record[key])) return '数据异常：缺正文';
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
    source: record,
    version: {
      objectId: record.objectId,
      version: record.version,
      status: record.status,
      effectiveStartsAt: record.effectiveStartsAt,
    },
    values: {
      identity: `${record.objectId}@${record.version}`,
      scope: record.scope,
      status: labelOf(commercialStatusLabels, record.status),
      effective: formatRange(record.effectiveStartsAt, record.effectiveEndsAt),
      publishedAt: formatInstant(record.publishedAt),
      contentRegistered: contentRegisteredCell(record),
      ...bodyValues(record),
    },
    refs: {
      ...(record.supplier ? { supplier: { kind: 'party', id: record.supplier } } : {}),
      ...(record.legalEntity ? { legalEntity: { kind: 'legal-entity', id: record.legalEntity } } : {}),
    },
  }));
}
