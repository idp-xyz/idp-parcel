// 异常与索赔区四页（异常处理、异常案件、索赔与追偿、索赔资格与授权）的页面口径。
//
// 状态与枚举码的界面词一律在用语词典（vocabulary/status.ts）一处定义，这里不再另立词表；这里只放词典之外、本区自己的话：
// 处置请求交给哪个环节、错误码怎么说、索赔前置两册的登记口径，以及几个封闭集的码名单（单测拿它们钉「已知码全部有中文」）。
// 目录侧的 presentation.ts 由设置区的判断规则页与披露策略页共用，本区不改它，也不再从它取界面话。

import { scopedStatusVocabulary } from '../../vocabulary/status';

// —— 封闭集码名单（各取后端 CHECK 约束与传输层的封闭词）——

/** 案件主状态封闭三相（0008 CHECK）。 */
export const casePhaseCodes = ['AWAITING_RESPONSE', 'IN_PROGRESS', 'CLOSED'] as const;
/** 分诊结果封闭四格（domain TriageOutcome）。 */
export const triageOutcomeCodes = ['AUTO_ESTABLISH', 'ATTACH_TO_EXISTING', 'MANUAL_REVIEW', 'NO_CASE'] as const;
/** 处置请求的接收方判断封闭四走向（0005 CHECK）。 */
export const dispositionJudgmentCodes = ['ACCEPTED', 'PARTIALLY_ACCEPTED', 'REFUSED', 'SUPPLEMENT_REQUIRED'] as const;
/** 处置取消的答复封闭四值（0005 CHECK）。 */
export const dispositionCancellationCodes = [
  'CANCELLATION_ACCEPTED',
  'PARTIALLY_CANCELLED',
  'NO_LONGER_CANCELLABLE',
  'CANCELLATION_REFUSED',
] as const;
/** 索赔资格初筛封闭三值（ADR-0051 的第三态）。 */
export const claimScreenCodes = ['ELIGIBLE', 'INELIGIBLE', 'AWAITING_SUPPLEMENT'] as const;
/** 索赔责任结论封闭四值（0004 CHECK）。 */
export const claimConclusionCodes = ['FULLY_ESTABLISHED', 'PARTIALLY_ESTABLISHED', 'NOT_ESTABLISHED', 'UNDETERMINABLE'] as const;
/** 客户异常通知过程节点封闭六值（domain NotificationMilestone）。 */
export const notificationMilestoneCodes = [
  'GENERATED',
  'SUBMITTED_TO_CHANNEL',
  'CHANNEL_ACCEPTED',
  'DELIVERED',
  'FAILED',
  'CUSTOMER_CONFIRMED',
] as const;
/** 追偿动作过程节点封闭七值（0004 CHECK）。 */
export const recoveryMilestoneCodes = [
  'PREPARED',
  'SUBMITTED',
  'CHANNEL_ACCEPTED',
  'DELIVERED',
  'ACKNOWLEDGED',
  'SUBMISSION_FAILED',
  'DELIVERY_FAILED',
] as const;

/**
 * 追偿节点码查哪本册：送达失败在 recovery-action 一册（送的是给责任方的主张，不是派送包裹），其余节点在 claim-recovery。
 * 两本册分开是因为词典只追加、不改已有册的条目。
 */
export function recoveryMilestoneScope(code: string): string {
  return code in scopedStatusVocabulary['recovery-action'] ? 'recovery-action' : 'claim-recovery';
}

/** 客户通知节点码的词典分册：册里有的取册里的词，其余（已提交渠道、渠道已接受）落总表。 */
export const notificationScope = 'customer-notification';

/**
 * 处置请求交给哪个环节去办。码是源上下文名（封闭五词），界面上说一线认的环节名：托运对一线就是运单本身，关务就是清关。
 * 查不到的码原样显示。
 */
export const handlerLabels: Record<string, string> = {
  PARCEL_SHIPMENT: '运单',
  NETWORK_ROUTING: '路线',
  NODE_OPERATIONS: '网点',
  TRANSPORT_FULFILLMENT: '运输',
  CUSTOMS_COMPLIANCE: '清关',
};

export function handlerLabel(code: string): string {
  return handlerLabels[code] ?? code;
}

// —— 错误码说人话：说哪里错、怎么改，不说实现细节 ——

export const exceptionProblemNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。',
  MALFORMED_REQUEST: '填写的内容格式不对，系统读不出一笔完整的登记；请按提示检查各项后再提交。',
  INTAKE_FAILED: '登录信息没能核对通过，这次什么都没有保存；请稍后重试。',
  NO_ANSWER_FORMED: '系统暂时没能处理这次请求，结果还不确定；请稍后刷新查看，必要时重新提交同一份。',
  UNNAMED_OUTCOME: '系统返回了无法识别的结果，重试不会好转；请记下技术详情里的编号联系系统管理员。',
  UNNAMED_REFUSAL_REASON: '这笔登记没有通过，但系统没有说明原因；请记下技术详情里的编号联系系统管理员。',
};

export function exceptionProblemNote(code: string): string {
  return exceptionProblemNotes[code] ?? `这次请求没有成功（错误码 ${code}）；请记下技术详情里的编号联系系统管理员。`;
}

// —— 索赔资格与授权的登记签（ADR-0085）——
// 标题说这个任务做什么；提示说要填哪几项、各是什么意思。两册登的是租户自己的规则，提示里的码只示形状，不是默认值。

export type ClaimPrerequisiteKind = 'CLAIM_ELIGIBILITY' | 'CLAIM_AUTHORIZATION';

export const claimPrerequisiteKindLabels: Record<ClaimPrerequisiteKind, string> = {
  CLAIM_ELIGIBILITY: '可索赔范围',
  CLAIM_AUTHORIZATION: '代提索赔授权',
};

export const claimPrerequisiteRegistrationTitles: Record<ClaimPrerequisiteKind, string> = {
  CLAIM_ELIGIBILITY: '设定合同可索赔的类型',
  CLAIM_AUTHORIZATION: '设定谁可以代客户提索赔',
};

export const claimPrerequisiteSnapshotHints: Record<ClaimPrerequisiteKind, string> = {
  CLAIM_ELIGIBILITY:
    '请填 version（这一版的编号，换版时给新编号）、approvedBy（批准人）、contract（客户合同编号）、customer（客户编码）、' +
    'coveredKinds（这份合同可以索赔的类型，如 LOSS 丢失、DAMAGE 破损，至少一项）。' +
    '某份合同还没设定时，客户在它名下的索赔会先等待设定，而不是直接不予受理。',
  CLAIM_AUTHORIZATION:
    '请填 version（这一版的编号）、approvedBy（批准人）、customer（客户编码）、applicants（可以代这个客户提索赔的申请人编码）。' +
    '不允许任何人代提时填 []，那是一个明确的决定；漏掉 applicants 这一项不会被当成「不允许」。',
};

export const claimPrerequisiteOutcomeLabels: Record<string, string> = {
  REGISTERED: '已保存',
  REFUSED: '没有保存——原因见下方，已有的设定不受影响',
};

export const claimPrerequisiteRefusalLabels: Record<string, string> = {
  SCOPE_MISSING: '缺少客户：可索赔范围要填客户合同与客户编码，代提授权要填客户编码。',
  VERSION_MISSING: '缺少这一版的编号，请填一个新编号。',
  APPROVAL_MISSING: '缺少批准人，请填写谁批准了这一版。',
  EFFECTIVE_RANGE_MISSING: '缺少生效时间，请填写从什么时候开始用。',
  EFFECTIVE_RANGE_REVERSED: '生效时间的起止顺序反了，请检查后再提交。',
  ENTRIES_MISSING: '没有填任何索赔类型，至少要填一项。',
  ENTRY_INCOMPLETE: '有一项没有填完整，请检查后再提交。',
  ENTRY_DUPLICATED: '同一项填了两次，请删掉重复的那一项。',
  VERSION_NOT_OVERWRITABLE: '这个编号已经用过了，已保存的那一版不会被覆盖；请换一个新编号再提交。',
  VERSION_OVERLAPS_EXISTING: '同一时间已经有另一版在用；请核对生效时间后再提交。',
};
