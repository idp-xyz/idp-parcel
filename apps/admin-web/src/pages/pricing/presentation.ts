// 计价目录查阅的结果词表。词取 parcel-pricing CONTEXT 与传输层原词,不自造译法。

export const directionLabels: Record<string, string> = {
  BUY: '买价',
  SELL: '卖价',
  INTERNAL: '内部',
};

export const purposeLabels: Record<string, string> = {
  SUPPLIER_COST: '供应商成本',
  CUSTOMER_CHARGE: '客户计费',
  INTERNAL_TRANSFER: '内部划转',
};

export const seriesKindLabels: Record<string, string> = {
  FUEL_RATE: '燃油费率',
  EXCHANGE_RATE: '汇率',
  PUBLISHED_AMOUNT: '按期公布金额',
};

// 键取 domain.SeriesEvidenceGrade 原词（库上 CHECK 同一组）。此前这里写的是 ASSERTION，与服务端
// 的 ASSERTED 对不上，断言强度那一格一直原样显示英文——票 08 顺手改正。
export const evidenceGradeLabels: Record<string, string> = {
  VERIFIABLE: '可复核',
  ASSERTED: '断言强度',
};

// 复核结论封闭两格（domain.SeriesReviewDecision 原词）。
export const reviewDecisionLabels: Record<string, string> = {
  APPROVED: '通过',
  RETURNED: '退回',
};

// ---- 登记前预览（票 pricing-reference-series-operations/08）----

export const previewOutcomeLabels: Record<string, string> = {
  PREVIEWED: '已预览（未登记；等级、内容摘要与逐期差异如下）',
  NOT_ACCEPTED: '请求不受理（登记本体立不起来，什么也没算）',
  UNDECIDED: '未决（取对照版本时依赖故障，可重试）',
};

// 对照那一格的四种下场：三种「没比出来」按续办动作分格，不折成一句「无差异」。
export const comparisonOutcomeLabels: Record<string, string> = {
  NOT_REQUESTED: '未要求对照（不是更正、也没指名对照版本）',
  COMPARED: '已与对照版本逐期比对',
  BASE_UNKNOWN: '对照版本不在册（查版本号）',
  BASE_INCOMPARABLE: '对照版本在册但不是同一条序列（种类不同——查绑错了哪条）',
};

export const periodChangeKindLabels: Record<string, string> = {
  UNCHANGED: '未变',
  CHANGED: '有改动',
  ADDED: '新增',
  REMOVED: '移除',
};

// 评价状态封闭五格(migrations/parcel_pricing/0001 CHECK 原词)。四种非完成结果
// 不得互相冒充——FAILED 是「未形成」(该重试的故障),UNRATABLE 是「不可计价」
// (终局业务判断),两词分开是 CONTEXT 硬句,词表不折叠。
export const evaluationStatusLabels: Record<string, string> = {
  COMPLETED: '已完成',
  PENDING: '待判断',
  CONFLICT: '冲突',
  FAILED: '未形成',
  UNRATABLE: '不可计价',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST: '请求构造不出查询,重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
