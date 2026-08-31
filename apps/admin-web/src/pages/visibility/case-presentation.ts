// 案件侧三页的结果词表。词取 CONTEXT.md、迁移 CHECK 约束与传输层的封闭词，不自造
// 译法；发作期的信号类型与可信度、处置动作与范围、案件结论、责任相对方等开放引用
// 一律原词直示。分诊结果四格的词表复用目录侧 presentation.ts 的 triageOutcomeLabels
// （只读导入，不动那边任何符号）。

/** 案件主状态封闭三相（0008 CHECK），中文与共享词表 domainStatusTones 同词。 */
export const casePhaseLabels: Record<string, string> = {
  AWAITING_RESPONSE: '待响应',
  IN_PROGRESS: '处理中',
  CLOSED: '已关闭',
};

/** 处置请求的源上下文判断封闭四走向（0005 CHECK）。 */
export const dispositionJudgmentLabels: Record<string, string> = {
  ACCEPTED: '已接受',
  PARTIALLY_ACCEPTED: '部分接受',
  REFUSED: '已拒绝',
  SUPPLEMENT_REQUIRED: '要求补充',
};

/** 处置取消的答复封闭四值（0005 CHECK）。 */
export const dispositionCancellationLabels: Record<string, string> = {
  CANCELLATION_ACCEPTED: '取消已接受',
  PARTIALLY_CANCELLED: '部分取消',
  NO_LONGER_CANCELLABLE: '已不可取消',
  CANCELLATION_REFUSED: '取消被拒',
};

/** 索赔资格初筛封闭三值（ADR-0051 的第三态）。 */
export const claimScreenLabels: Record<string, string> = {
  ELIGIBLE: '已通过',
  INELIGIBLE: '不符合',
  AWAITING_SUPPLEMENT: '等待补充材料',
};

/** 索赔责任结论封闭四值（0004 CHECK），中文取 CONTEXT 原词。 */
export const claimConclusionLabels: Record<string, string> = {
  FULLY_ESTABLISHED: '全部成立',
  PARTIALLY_ESTABLISHED: '部分成立',
  NOT_ESTABLISHED: '不成立',
  UNDETERMINABLE: '当前无法认定',
};

/** 追偿动作过程节点封闭七值（0004 CHECK）。 */
export const recoveryMilestoneLabels: Record<string, string> = {
  PREPARED: '已准备',
  SUBMITTED: '已提交',
  CHANNEL_ACCEPTED: '渠道已接受',
  DELIVERED: '已送达',
  ACKNOWLEDGED: '对方已确认',
  SUBMISSION_FAILED: '提交失败',
  DELIVERY_FAILED: '送达失败',
};

/** 词表没收录的码原样示码，不猜词——封闭集扩了、页面还没跟上时如实露码。 */
export function caseLabelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
