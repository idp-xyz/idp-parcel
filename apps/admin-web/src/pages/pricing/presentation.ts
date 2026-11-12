// 计价目录查阅的结果词表。词取 parcel-pricing CONTEXT 与传输层原词,不自造译法。

import { chargeCodeLabel } from '../../vocabulary/status';

// 价格方向与计算目的的界面词只在用语词典定义一处（vocabulary/price-card 的 priceDirectionLabels、priceCardPurposeLabels）；
// 这里转出给本目录沿用旧名的几页，不另抄一份——此前这里另有一套「买价 / 卖价」，与价格表页的「成本 / 售价」打架。
export { priceDirectionLabels as directionLabels, priceCardPurposeLabels as purposeLabels } from '../../vocabulary/price-card';

export const seriesKindLabels: Record<string, string> = {
  FUEL_RATE: '燃油费率',
  EXCHANGE_RATE: '汇率',
  PUBLISHED_AMOUNT: '按期公布金额',
};

// 键取 domain.SeriesEvidenceGrade 原词（库上 CHECK 同一组）。此前这里写的是 ASSERTION，与服务端
// 的 ASSERTED 对不上，断言强度那一格一直原样显示英文——票 08 顺手改正。
export const evidenceGradeLabels: Record<string, string> = {
  VERIFIABLE: '有凭证可查',
  ASSERTED: '仅声明、无凭证',
};

// 复核结论封闭两格（domain.SeriesReviewDecision 原词）。
export const reviewDecisionLabels: Record<string, string> = {
  APPROVED: '通过',
  RETURNED: '退回',
};

// ---- 登记前预览（票 pricing-reference-series-operations/08）----

export const previewOutcomeLabels: Record<string, string> = {
  PREVIEWED: '预览通过，还没登记；核对下面的内容后再点「登记」',
  NOT_ACCEPTED: '内容有误，没法登记；按提示改好后重新预览',
  UNDECIDED: '这次没查完（对照版本没取回来），稍后重新预览',
};

// 对照那一格的四种下场：三种「没比出来」按续办动作分格，不折成一句「无差异」。
export const comparisonOutcomeLabels: Record<string, string> = {
  NOT_REQUESTED: '没有指定对照版本，不做比对',
  COMPARED: '已和对照版本逐期比对',
  BASE_UNKNOWN: '找不到对照版本，检查版本号是否填对',
  BASE_INCOMPARABLE: '对照版本属于另一种数值（种类不同），检查是不是填错了哪一条',
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

// 评价的证据层级（docs/product/PILOT-ACCEPTANCE-MATRIX.md「证据层级」原词）。与上面参考序列的 evidenceGradeLabels 是两套词，不混用。
export const evidenceLevelLabels: Record<string, string> = {
  P: '真实生产',
  R: '历史数据回放',
  S: '受控模拟',
};

// 运营试算（ADR-0152 决定七）：`outcome` 只说编排，逐卡评价自己的状态用上面的 evaluationStatusLabels，不折进这里。
export const estimateOutcomeLabels: Record<string, string> = {
  FORMED: '已算出：每张适用的价卡各算一份，并排列出，不分先后',
  PRICE_CARD_NOT_CONFIGURED: '价卡未配置：这个范围、方向和时间下还没有适用的价卡',
  PRICE_CARD_APPLICABILITY_CONFLICT: '价卡冲突：同一张价卡有两个版本同时有效，请价卡负责人处理后再试；本页不替你挑',
  NOT_ACCEPTED: '包裹信息不完整或格式不对，改好后再试',
  UNDECIDED: '这次没算完（有数据没读回来），稍后再试',
};

export const estimateAnswerLabels: Record<string, string> = {
  EVALUATED: '已算出',
  INPUT_INCOMPLETE: '信息不够',
};

// 缺项按卡的目录绑定定：绑了分区目录的卡要邮编路线、没绑的卡要分区（ADR-0152 决定五）；计价在估价口把「卡要而申报没给」
// 一律点名为缺项，封闭集另有寄件邮编与尺寸两码（ADR-0381 决定二）。
export const estimateMissingLabels: Record<string, string> = {
  ZONE: '分区（这张价卡不按邮编查分区，要直接填分区）',
  POSTAL_ROUTE: '始发与目的邮编（这张价卡按邮编查分区或偏远地区）',
  ADDRESS_NATURE: '收件地址性质（这张价卡对商业 / 住宅地址收费不同，要选一个，系统不替你假定）',
  ORIGIN_POSTAL: '寄件邮编（这张价卡的分区目录按始发邮编前缀分区）',
  DIMENSIONS: '尺寸（这张价卡按尺寸或体积重判价，要逐件给长宽高）',
  MEASUREMENT: '重量或量纲（这一件的重量或重量单位读不出，计价只取申报的测量，不替你补）',
};

// 收件地址性质（ADR-0347 决定四）：封闭词，声明由发起方给，计价不推断。
export const addressNatureLabels: Record<string, string> = {
  COMMERCIAL: '商业地址',
  RESIDENTIAL: '住宅地址',
};

export const estimateReasonLabels: Record<string, string> = {
  PRICE_CARD_LOAD_UNAVAILABLE: '读适用价卡时没读回来',
  CONFLICT_CANDIDATES_UNAVAILABLE: '读冲突的价卡版本时没读回来',
  READING_COMPLETION_UNAVAILABLE: '读汇率、费率或分区目录时没读回来',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '页面与系统版本不一致，请刷新页面后再试；仍不行请联系管理员。',
  MALFORMED_REQUEST: '请求内容的格式不对，原样重发不会变；请检查填写的内容后再试。',
  INTAKE_FAILED: '系统这次没能处理请求，没有任何改动；请稍后重试。',
  NO_ANSWER_FORMED: '系统这次没能处理完，没有任何改动；请稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? `出了没预料到的问题（问题码 ${code}）；请把这个问题码告诉管理员。`;
}

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

// 费用行的主列用词：价卡为这条规则写的说明本身是中文就直接用（它是租户在卡上起的名）；说明缺失或没有中文（基础运费行是引擎
// 固定写的英文 "Base rate"）就按费用代码查词典。不拿英文说明上屏，也不替租户改写他写的中文。
export function chargeLineName(line: { code: string; description: string }): string {
  return /[\u3400-\u9fff]/.test(line.description) ? line.description : chargeCodeLabel(line.code);
}
