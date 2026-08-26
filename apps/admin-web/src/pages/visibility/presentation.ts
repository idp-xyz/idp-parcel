// 运营追踪与目录查阅的结果词表。词取 CONTEXT.md 与传输层的原词,不自造译法。
//
// 刻意没有的几张表:事实类型(kind)与标准里程碑都是原词直示——kind 是源上下文
// 拥有的开放词表,里程碑目录及其映射属版本化登记的实例参数(CONTEXT「追踪投影与
// 里程碑」),本页翻译任何一个都是替租户造第二套口径。同理,分诊规则的信号类型与
// 可信度、通知策略的渠道与义务判据、索赔前置的种类与申请人都是开放引用,原词转写。

import type { VisibilityCatalogueKind } from './catalogue-api';

/**
 * 源上下文的中文词,取 CONTEXT「全程追踪投影」定义句自己数的五源:
 * 「对节点、运输、关务、路由和托运上下文已经接受的事实进行语义化编排」。
 * 词表没收录的源(枚举扩了、页面还没跟上)由调用方原样示码,不猜词。
 */
export const sourceContextLabels: Record<string, string> = {
  PARCEL_SHIPMENT: '托运',
  NETWORK_ROUTING: '路由',
  NODE_OPERATIONS: '节点',
  TRANSPORT_FULFILLMENT: '运输',
  CUSTOMS_COMPLIANCE: '关务',
};

// 传输层错误码说明。它们不是业务原因目录:出现即表示应用层没答过,响应体也
// 刻意不带自由文本(防泄露),所以措辞只指下一步动作。
export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST:
    '请求构造不出查询(定位参数为空,或 parcel 与 version 同时在场),重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

// ---- 六类目录查阅（/visibility-catalogues,票 admin-web-page-wiring-frontier/02）----

/** 目录种类的页签词。种类命名册子(与登记写口同词根),中文取 CONTEXT 原词。 */
export const visibilityCatalogueKindLabels: Record<VisibilityCatalogueKind, string> = {
  MILESTONE_MAPPING: '里程碑映射',
  TRIAGE_RULE: '分诊规则',
  NOTIFICATION_POLICY: '通知策略',
  CLAIM_ELIGIBILITY: '索赔资格',
  CLAIM_AUTHORIZATION: '索赔授权',
  DISCLOSURE_POLICY: '披露策略',
};

/**
 * 分诊结果封闭四格(domain TriageOutcome)。中文取 CONTEXT 语言:「自动建立或关联
 * 案件」「必须先进入分诊(人工复核)」;NO_CASE 是规则明说的不立案,不是没答。
 */
export const triageOutcomeLabels: Record<string, string> = {
  AUTO_ESTABLISH: '自动立案',
  ATTACH_TO_EXISTING: '关联既有案件',
  MANUAL_REVIEW: '进入人工复核',
  NO_CASE: '不立案',
};

/** 披露维态封闭三格(domain DimensionState,0012)。 */
export const disclosureStateLabels: Record<string, string> = {
  SHOWN: '展示',
  PENDING_CONFIRMATION: '待确认',
  NOT_DISCLOSED: '不披露',
};

/** 词表没收录的码原样示码,不猜词——开放集扩了、页面还没跟上时如实露码。 */
export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
