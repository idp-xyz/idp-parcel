// ⚠️ 隔离合成 S，不得作生产默认。
// 本文件是三个页面模板的演示专用假数据，证据层级为「隔离合成 S」：
// 全部标识、客户名、时间与金额均为虚构，仅供模板开发期的视觉与交互演示；
// 不得写入任何模板的默认 props，不得被生产页面 import，不得当作
// 任何租户实例参数的样板。标识一律带 SYN- 前缀以便肉眼与检索识别。
//
// 模板本体不引用本文件——demo 数据只从演示入口注入，这条单向依赖
// 保证删掉本文件不伤任何模板。

import type { DetailField, AuditEntry } from './types';
import type { ListSortOption } from './ListPageTemplate';

/* ── ListPageTemplate 演示数据 ── */

/** 演示行形状：字段名对齐托运申报的领域语言，便于演示页零改动映射列。 */
export interface DemoShipmentRow {
  /** 申报单号（合成）。 */
  requestId: string;
  /** 货主客户（合成名，非任何真实锚点客户）。 */
  shipperClient: string;
  /** 目的国/地区（仅演示排版用）。 */
  destination: string;
  /** 状态展示文案与徽章档位：语义映射由演示页决定，demo 不定档位真值。 */
  statusLabel: string;
  statusKind: 'success' | 'warning' | 'danger' | 'info' | 'pending' | 'neutral';
  /** 已格式化的提交时间（合成）。 */
  submittedAt: string;
}

/** 隔离合成 S：托运申报单列表演示行。 */
export const demoShipmentRows: DemoShipmentRow[] = [
  {
    requestId: 'SYN-PS-240819-0001',
    shipperClient: '合成商贸（演示A）',
    destination: 'US',
    statusLabel: '已提交',
    statusKind: 'success',
    submittedAt: '2024-08-19 09:12',
  },
  {
    requestId: 'SYN-PS-240819-0002',
    shipperClient: '合成电商（演示B）',
    destination: 'DE',
    statusLabel: '待人工复核',
    statusKind: 'warning',
    submittedAt: '2024-08-19 09:47',
  },
  {
    requestId: 'SYN-PS-240819-0003',
    shipperClient: '合成供应链（演示C）',
    destination: 'JP',
    statusLabel: '已提交',
    statusKind: 'success',
    submittedAt: '2024-08-19 10:03',
  },
  {
    requestId: 'SYN-PS-240820-0004',
    shipperClient: '合成商贸（演示A）',
    destination: 'GB',
    statusLabel: '已取消',
    statusKind: 'neutral',
    submittedAt: '2024-08-20 08:30',
  },
  {
    requestId: 'SYN-PS-240820-0005',
    shipperClient: '合成物流（演示D）',
    destination: 'AU',
    statusLabel: '来源保全中',
    statusKind: 'pending',
    submittedAt: '2024-08-20 11:21',
  },
];

/* ── ListPageTemplate Filter Bar 结构位演示数据（票 admin-web-ux-alignment/03 第 6 条） ── */

/** 演示排序键：只对 demoShipmentRows 的字段起作用，不对应任何读口的排序参数。 */
export type DemoListSortKey = 'submitted-desc' | 'submitted-asc' | 'request-id';

/** 隔离合成 S：排序位演示选项。文案含方向，照模板对 ListSortOption.label 的要求由调用方给全。 */
export const demoListSortOptions: (ListSortOption & { value: DemoListSortKey })[] = [
  { value: 'submitted-desc', label: '提交时间 ↓' },
  { value: 'submitted-asc', label: '提交时间 ↑' },
  { value: 'request-id', label: '申报单号' },
];

/**
 * 演示保存视图：一份筛选态的快照（状态 / 目的地 / 排序），'all' 表示该维不筛。
 * 保存视图的真实实现（存哪、存多久）归票 admin-web-ux-alignment/02；这里只让演示页能切、能存到内存里。
 */
export interface DemoListSavedView {
  id: string;
  label: string;
  status: string | 'all';
  destination: string | 'all';
  sort: DemoListSortKey;
}

/** 隔离合成 S：预置的两个演示视图，值只引用 demoShipmentRows 里实际出现的状态文案。 */
export const demoListSavedViews: DemoListSavedView[] = [
  { id: 'syn-view-all', label: '全部申报（演示）', status: 'all', destination: 'all', sort: 'submitted-desc' },
  {
    id: 'syn-view-review',
    label: '待人工复核（演示）',
    status: '待人工复核',
    destination: 'all',
    sort: 'submitted-desc',
  },
];

/* ── DetailPageTemplate 演示数据 ── */

/** 隔离合成 S：详情页「基本信息」区演示字段。 */
export const demoDetailFields: DetailField[] = [
  { label: '申报单号', value: 'SYN-PS-240819-0002' },
  { label: '货主客户', value: '合成电商（演示B）' },
  { label: '目的国/地区', value: 'DE' },
  { label: '包裹件数', value: '3' },
  { label: '提交时间', value: '2024-08-19 09:47' },
  { label: '当前状态', value: '待人工复核' },
];

/** 演示业务区块：content 是纯文本，演示页可直接塞给 DetailSection.content。 */
export interface DemoDetailSectionText {
  id: string;
  title: string;
  description?: string;
  body: string;
}

/** 隔离合成 S：详情页业务区块演示文案。 */
export const demoDetailSections: DemoDetailSectionText[] = [
  {
    id: 'source-preservation',
    title: '来源保全',
    description: '原始提交内容的保全摘要',
    body: '原始报文已保全（合成演示摘要：SYN-DIGEST-9f21…）。本区块在真实接线后展示保全凭据与原文对照入口。',
  },
  {
    id: 'production-attribution',
    title: '生产归属',
    description: '归属判定的演示占位',
    body: '生产归属判定结果（合成演示：归属演示网络节点 SYN-NODE-01）。真实判定依据接线后由后端事实供数。',
  },
];

/** 隔离合成 S：详情页审计留痕演示记录。 */
export const demoDetailAuditTrail: AuditEntry[] = [
  {
    id: 'syn-audit-001',
    title: '收到托运申报',
    description: '渠道：API（合成演示）',
    timestamp: '2024-08-19 09:47:02',
    variant: 'default',
  },
  {
    id: 'syn-audit-002',
    title: '来源保全完成',
    description: '保全摘要 SYN-DIGEST-9f21…（合成演示）',
    timestamp: '2024-08-19 09:47:03',
    variant: 'success',
  },
  {
    id: 'syn-audit-003',
    title: '转入接受前人工复核',
    description: '适用规则显式要求人工业务判断（合成演示场景）',
    timestamp: '2024-08-19 09:47:05',
    variant: 'warning',
  },
];

/* ── DetailPageTemplate 对象工作区演示数据（票 admin-web-ux-alignment/04 第 6 条） ── */

/**
 * 隔离合成 S：对象工作区演示的主对象，就是上面详情演示那份申报单——同一对象换成头区 + 指标带 + 签的排布，
 * 看的人能把两张演示里的字段对上；基本信息 / 区块 / 审计留痕因此直接复用上面三份，不另造一套。
 */
export const demoWorkspaceIdentifier = 'SYN-PS-240819-0002';

/** 隔离合成 S：头区第二行元信息。只放对象自身的事实；负责人 / 风险没有领域来源，不留位（票面裁决 2）。 */
export const demoWorkspaceMeta: { label: string; value: string }[] = [
  { label: '提交时间', value: '2024-08-19 09:47' },
  { label: '系统接收时间', value: '2024-08-19 09:47:02' },
  { label: '来源', value: 'API' },
  { label: '当前提交版本', value: 'SYN-SV-0002-01' },
];

/**
 * 隔离合成 S：指标带四格。值都是对象自己陈述的事实（件数、版本数、目的地、批次），
 * 没有一格是从列表数出来的派生 KPI——指标带只回答对象「有多大 / 多少件」，不替页面算账。
 */
export const demoWorkspaceSummary: { label: string; value: string }[] = [
  { label: '声明包裹件数', value: '3' },
  { label: '此前版本数', value: '0' },
  { label: '目的国/地区', value: 'DE' },
  { label: '提交批次', value: 'SYN-BATCH-240819-07' },
];

/** 关联对象一行：对象类型 / 标识 / 一句说明。 */
export interface DemoWorkspaceRelatedObject {
  kind: string;
  id: string;
  note: string;
}

/** 隔离合成 S：「关联」签的内容——与本申报单相关的批次、声明包裹与接受判断任务。 */
export const demoWorkspaceRelated: DemoWorkspaceRelatedObject[] = [
  { kind: '提交批次', id: 'SYN-BATCH-240819-07', note: '本申报单所属的提交批次' },
  { kind: '包裹', id: 'SYN-PARCEL-0002-1', note: '声明毛重 1.2 kg' },
  { kind: '包裹', id: 'SYN-PARCEL-0002-2', note: '声明毛重 1.6 kg' },
  { kind: '包裹', id: 'SYN-PARCEL-0002-3', note: '声明毛重 1.4 kg' },
  { kind: '接受判断任务', id: 'SYN-AT-0002-01', note: '待人工复核' },
];

/** 隔离合成 S：右侧上下文的几格事实。 */
export const demoWorkspaceAsideFacts: { label: string; value: string }[] = [
  { label: '货主客户', value: '合成电商（演示B）' },
  { label: '接入渠道', value: 'API' },
  { label: '所属批次', value: 'SYN-BATCH-240819-07' },
];

/* ── ReviewFlowTemplate 演示数据 ── */

/** 演示队列项：statusKind 档位由演示页映射成 StatusBadge，demo 不携带 JSX。 */
export interface DemoReviewQueueEntry {
  id: string;
  title: string;
  subtitle: string;
  statusLabel: string;
  statusKind: 'warning' | 'pending';
  meta: string;
}

/** 隔离合成 S：接受前人工复核队列演示项。 */
export const demoReviewQueue: DemoReviewQueueEntry[] = [
  {
    id: 'SYN-PS-240819-0002',
    title: 'SYN-PS-240819-0002',
    subtitle: '合成电商（演示B） → DE',
    statusLabel: '待复核',
    statusKind: 'warning',
    meta: '进入队列 2024-08-19 09:47',
  },
  {
    id: 'SYN-PS-240820-0006',
    title: 'SYN-PS-240820-0006',
    subtitle: '合成供应链（演示C） → FR',
    statusLabel: '待复核',
    statusKind: 'warning',
    meta: '进入队列 2024-08-20 14:05',
  },
  {
    id: 'SYN-PS-240821-0007',
    title: 'SYN-PS-240821-0007',
    subtitle: '合成物流（演示D） → SG',
    statusLabel: '复核中',
    statusKind: 'pending',
    meta: '进入队列 2024-08-21 08:16',
  },
];

/** 隔离合成 S：复核详情区演示字段（对应队列首项）。 */
export const demoReviewDetailFields: DetailField[] = [
  { label: '申报单号', value: 'SYN-PS-240819-0002' },
  { label: '货主客户', value: '合成电商（演示B）' },
  { label: '目的国/地区', value: 'DE' },
  { label: '触发原因', value: '适用规则显式要求人工业务判断（合成演示）' },
  { label: '进入队列', value: '2024-08-19 09:47' },
];

/** 隔离合成 S：复核工作流审计留痕演示记录。 */
export const demoReviewAuditTrail: AuditEntry[] = [
  {
    id: 'syn-review-audit-001',
    title: '进入接受前人工复核',
    description: '触发：适用规则显式要求人工业务判断（合成演示）',
    timestamp: '2024-08-19 09:47:05',
    variant: 'warning',
  },
  {
    id: 'syn-review-audit-002',
    title: '复核人打开案件',
    description: '操作者：演示复核员（合成）',
    timestamp: '2024-08-19 10:02:11',
    variant: 'default',
  },
];

/* ── ReviewFlowTemplate 可配置决定集（分诊）演示数据 ── */

/** 演示分诊信号队列项：字段名对齐异常信号的领域语言；statusKind 档位由演示页映射。 */
export interface DemoTriageSignalEntry {
  id: string;
  title: string;
  subtitle: string;
  statusLabel: string;
  statusKind: 'warning' | 'pending';
  meta: string;
}

/** 隔离合成 S：进入分诊的异常信号演示队列。 */
export const demoTriageSignalQueue: DemoTriageSignalEntry[] = [
  {
    id: 'SYN-VE-SIG-240821-0001',
    title: 'SYN-VE-SIG-240821-0001',
    subtitle: '可见性缺口 · 包裹 SYN-PARCEL-88021',
    statusLabel: '待分诊',
    statusKind: 'warning',
    meta: '信号生成 2024-08-21 07:40',
  },
  {
    id: 'SYN-VE-SIG-240821-0002',
    title: 'SYN-VE-SIG-240821-0002',
    subtitle: '疑似重复信号 · 包裹 SYN-PARCEL-88034',
    statusLabel: '待分诊',
    statusKind: 'warning',
    meta: '信号生成 2024-08-21 09:12',
  },
  {
    id: 'SYN-VE-SIG-240822-0003',
    title: 'SYN-VE-SIG-240822-0003',
    subtitle: '关务异常信号 · 包裹 SYN-PARCEL-88102',
    statusLabel: '资料补充中',
    statusKind: 'pending',
    meta: '信号生成 2024-08-22 15:03',
  },
];

/**
 * 隔离合成 S：分诊详情区演示字段（对应队列首项）。
 * 字段清单对齐 visibility-exception CONTEXT.md 信号必存项：
 * 对象、类型、规则版本、判断时间、事实依据、可信度、当前发作期。
 */
export const demoTriageDetailFields: DetailField[] = [
  { label: '信号对象', value: 'SYN-PARCEL-88021（包裹）' },
  { label: '信号类型', value: '可见性缺口' },
  { label: '规则版本', value: 'SYN-RULE-VISGAP-03' },
  { label: '判断时间', value: '2024-08-21 07:40' },
  { label: '事实依据', value: '观察窗口届满仍无预期扫描（合成演示）' },
  { label: '可信度', value: '低（演示档位）' },
  { label: '当前发作期', value: 'SYN-EPISODE-88021-01（进行中）' },
];
