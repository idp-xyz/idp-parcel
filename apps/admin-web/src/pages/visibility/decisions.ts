import type { RegistrationResponseBody, RegistrationTarget } from '../../components/registration';
import { postMasterData, type ApiResult } from '../catalogue-api';

/**
 * 异常的运营决定口：案件的接单、关闭与归并，证据的登记、评价与披露版本准备，客户披露的判断与授权确认，客户异常通知。
 *
 * 后端口都已接上，这里先用登记面板把它们接进页面：操作员照提示填一份 JSON 提交，页面示出服务端的答复。不是引导式
 * 表单。所属公司与操作者由系统按登录身份记下。
 */

export type ExceptionDecisionKind =
  | 'case-take-up'
  | 'case-closure'
  | 'case-merge'
  | 'evidence-submission'
  | 'evidence-appraisal'
  | 'evidence-disclosure-preparation'
  | 'customer-disclosure-decision'
  | 'customer-disclosure-authorization'
  | 'customer-exception-notification'
  | 'customer-exception-notification-manual-record';

export const exceptionDecisionEndpoints: Record<ExceptionDecisionKind, string> = {
  'case-take-up': '/exception-case-take-ups',
  'case-closure': '/exception-case-closures',
  'case-merge': '/exception-case-merges',
  'evidence-submission': '/evidence-submissions',
  'evidence-appraisal': '/evidence-appraisals',
  'evidence-disclosure-preparation': '/evidence-disclosure-preparations',
  'customer-disclosure-decision': '/customer-disclosure-decisions',
  'customer-disclosure-authorization': '/customer-disclosure-authorizations',
  'customer-exception-notification': '/customer-exception-notifications',
  'customer-exception-notification-manual-record': '/customer-exception-notification-manual-records',
};

export const exceptionDecisionTitles: Record<ExceptionDecisionKind, string> = {
  'case-take-up': '接单',
  'case-closure': '关闭案件',
  'case-merge': '并入另一件案件',
  'evidence-submission': '登记证据',
  'evidence-appraisal': '评价证据',
  'evidence-disclosure-preparation': '准备证据披露版本',
  'customer-disclosure-decision': '判断是否向客户披露',
  'customer-disclosure-authorization': '确认向客户披露',
  'customer-exception-notification': '通知客户（系统推送）',
  'customer-exception-notification-manual-record': '已电话或邮件通知客户',
};

const identityNote = ' 所属公司与操作者由系统按登录身份记下，不用填；需要持有这一种运营决定的授予。';
const signalTarget =
  '请填 parcel（包裹号）、signalKind（异常信号类型）、episodeId（哪一期信号，在「异常处理」页的技术详情里）。同一件包裹同一类信号可以同时有几期，' +
  '判断、确认和通知都钉在填的那一期上。';

export const exceptionDecisionHints: Record<ExceptionDecisionKind, string> = {
  'case-take-up': '请填 caseId（案件标识）。接单即形成首次响应时间。' + identityNote,
  'case-closure': '请填 caseId（案件标识）、conclusion（结论）。' + identityNote,
  'case-merge': '请填 caseId（要并走的重复案件）、mainCaseId（主案件）。并走的案件关闭并指向主案件，记录都保留。' + identityNote,
  'evidence-submission':
    '请填 provider（交来这份材料的一方）、contentDigest（材料内容的指纹）。本产品只记指纹，材料本体放在贵司约定的受限存储里。' + identityNote,
  'evidence-appraisal': '请填 evidenceId（证据项）、appraisal（CREDITED 采信 / DISCREDITED 不采信）、basis（评价依据）。评价只落一次。' + identityNote,
  'evidence-disclosure-preparation':
    '请填 evidenceId（证据项）、redactedDigest（脱敏后内容的指纹，不得与原件相同）、scope（这份版本给谁看）。' + identityNote,
  'customer-disclosure-decision': signalTarget + ' 披露还是不披露由披露规则给，这里只发起判断。' + identityNote,
  'customer-disclosure-authorization':
    signalTarget + ' 确认的是一份停在「待授权」的判断；披露内容取规则里的那一份，规则已改成不披露则不放行。' + identityNote,
  'customer-exception-notification':
    signalTarget + ' 通知的是已经成立的披露；首次发送与失败后重发都在这里。通知策略与通知渠道要先登记好。' + identityNote,
  'customer-exception-notification-manual-record':
    '没有对接客户系统时用这一个：你已经用电话、邮件等方式通知了客户，在这里记一笔。' +
    signalTarget +
    ' 另填 method（通知方式，如 电话、邮件）、original（所凭原件：通话记录、邮件的编号或存档引用，必填）。系统不发送任何东西，也不需要登记通知渠道；' +
    '通知策略仍要先登记（时限从它来）。系统推送失败过的通知也可以在这里改记为已人工通知。' +
    identityNote,
};

/** 各口答复的中文。没收录的原样示出英文原名。 */
export const exceptionDecisionOutcomeLabels: Record<string, string> = {
  TAKEN_UP: '已接单',
  ALREADY_IN_PROGRESS: '这件案件已经在处理中',
  CLOSED: '已关闭',
  ALREADY_CLOSED: '此前已经关闭',
  MERGED: '已并入主案件',
  MAIN_CASE_NOT_ELIGIBLE: '主案件不能接收并入（已关闭、已并走或就是它自己）',
  NOT_FOUND: '找不到这件案件',
  EVIDENCE_SUBMITTED: '已登记证据',
  EVIDENCE_EXISTING_RESULT: '这份材料已经登记过',
  EVIDENCE_APPRAISED: '已评价',
  EVIDENCE_ALREADY_APPRAISED: '这项证据已经评价过',
  EVIDENCE_NOT_FOUND: '找不到这项证据',
  DISCLOSURE_PREPARED: '已准备披露版本',
  DECIDED: '已形成披露判断，结论见下',
  AUTHORIZED: '已确认披露',
  EXISTING_RESULT: '此前已经有结果，见下',
  NOT_AWAITING_AUTHORIZATION: '这一期信号没有等待确认的披露判断',
  DISCLOSURE_CONDITION_NO_LONGER_MET: '披露规则已改成不披露，不能确认',
  SUBMITTED: '已提交给通知渠道',
  SUBMISSION_FAILED: '提交给通知渠道没有成功，可以再发一次',
  UNDECIDED: '暂时定不了，原因见下',
  NOT_ACCEPTED: '内容不完整或不成立，没有受理',
};

/** 手工标记这一口自己的答复词：「已提交」「已有结果」在这里说的是另一件事。 */
export const manualNotificationOutcomeLabels: Record<string, string> = {
  ...exceptionDecisionOutcomeLabels,
  SUBMITTED: '已记为已通知客户',
  EXISTING_RESULT: '这份通知此前已经发出或已经记过，没有再记一次',
};

export const exceptionDecisionUndecidedLabels: Record<string, string> = {
  DISCLOSURE_RULE_NOT_CONFIGURED: '披露规则还没有登记',
  DISCLOSURE_CUSTOMER_ACCOUNT_UNDETERMINABLE: '这件包裹还没有已接受的委托，定不了是哪个客户',
  NOTIFICATION_POLICY_NOT_CONFIGURED: '通知策略还没有登记',
  NOTIFICATION_CHANNEL_NOT_CONFIGURED: '通知渠道的推送地址还没有登记',
  NOTIFICATION_CHANNEL_CREDENTIAL_MISSING: '通知渠道的凭据在运行环境里没有配置',
  NOTIFICATION_CUSTOMER_ACCOUNT_UNDETERMINABLE: '这件包裹还没有已接受的委托，定不了是哪个客户',
};

/**
 * 披露与通知三口把未决原因放在 `reason` 一格，登记面板认的是 `undecidedReason`：这里挪一下，面板才示得出中文原因。
 */
export function withUndecidedReason(
  answer: ApiResult<RegistrationResponseBody & { reason?: string }>,
): ApiResult<RegistrationResponseBody> {
  if (answer.kind !== 'outcome' || !answer.body.reason || answer.body.undecidedReason) return answer;
  return { ...answer, body: { ...answer.body, undecidedReason: answer.body.reason } };
}

export async function submitExceptionDecision(kind: ExceptionDecisionKind, snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return withUndecidedReason(await postMasterData<RegistrationResponseBody & { reason?: string }>(exceptionDecisionEndpoints[kind], snapshot));
}

/** 异常案件页「运营决定」签上的十个口：案件、证据、披露与通知，各按办事顺序排。 */
export const exceptionDecisionTargets: RegistrationTarget[] = (Object.keys(exceptionDecisionEndpoints) as ExceptionDecisionKind[]).map((kind) => ({
  id: kind,
  label: exceptionDecisionTitles[kind],
  title: exceptionDecisionTitles[kind],
  endpoint: `POST ${exceptionDecisionEndpoints[kind]}`,
  snapshotHint: exceptionDecisionHints[kind],
  submit: (snapshot: unknown) => submitExceptionDecision(kind, snapshot),
  outcomeLabels: kind === 'customer-exception-notification-manual-record' ? manualNotificationOutcomeLabels : exceptionDecisionOutcomeLabels,
  undecidedReasonLabels: exceptionDecisionUndecidedLabels,
}));
