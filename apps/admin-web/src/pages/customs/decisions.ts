import type { RegistrationResponseBody, RegistrationTarget } from '../../components/registration';
import { postMasterData, type ApiResult } from '../catalogue-api';

/**
 * 清关的运营决定口：建案、提交授权、申报提交、放行检查、关闭清关单，以及监管限制的建立与解除。
 *
 * 这些口后端早已接上，管理台此前没有任何一页调它们。这里先用登记面板把它们接进页面：操作员照提示填一份 JSON 提交，
 * 页面示出服务端的答复。它不是引导式表单——没有从列表行带出标识、没有逐格校验——但每一步在产品里有了路。所属公司与
 * 决定人由系统按登录身份记下，不在内容里填。
 */

export type CustomsDecisionKind =
  | 'case-establishment'
  | 'submission-authorization'
  | 'declaration-submission'
  | 'release-gate-verification'
  | 'case-closure'
  | 'regulatory-restriction'
  | 'regulatory-restriction-release';

export const customsDecisionEndpoints: Record<CustomsDecisionKind, string> = {
  'case-establishment': '/customs-cases',
  'submission-authorization': '/customs-submission-authorizations',
  'declaration-submission': '/customs-declaration-submissions',
  'release-gate-verification': '/customs-release-gate-verifications',
  'case-closure': '/customs-case-closures',
  'regulatory-restriction': '/customs-regulatory-restrictions',
  'regulatory-restriction-release': '/customs-regulatory-restriction-releases',
};

export const customsDecisionTitles: Record<CustomsDecisionKind, string> = {
  'case-establishment': '建立清关单',
  'submission-authorization': '确认提交授权',
  'declaration-submission': '提交申报',
  'release-gate-verification': '放行检查',
  'case-closure': '关闭清关单',
  'regulatory-restriction': '建立监管限制',
  'regulatory-restriction-release': '解除监管限制',
};

const identityNote = ' 所属公司与决定人由系统按登录身份记下，不用填；需要持有这一种运营决定的授予。';

export const customsDecisionHints: Record<CustomsDecisionKind, string> = {
  'case-establishment':
    '请填 jurisdiction（辖区）、direction（IMPORT 进口 / EXPORT 出口）、procedure（监管程序）、obligation（关务义务）、parcels（包裹号数组）、' +
    'roles（参与方角色数组，每项填 role、party、authority 三格）。包裹只给号：客户归属与来源资料由系统按已接受的委托取。' +
    identityNote,
  'submission-authorization': '请填 unitId（报关批次）。确认的是这一批次可以提交申报。' + identityNote,
  'declaration-submission':
    '请填 unitId（报关批次）、caseId（清关单）、procedure（监管程序）、members（成员包裹号数组）、dossier（申报资料快照引用）、' +
    'roles（角色快照引用）、target（申报通道）。这里只形成提交版本，发送由系统随后完成。' +
    identityNote,
  'release-gate-verification':
    '请填 scope（申报范围）、action（检查哪一步：OUTBOUND_RELEASE 出库 / LOADING_DEPARTURE 装载出发 / CROSS_CUSTOMS_MOVEMENT 跨关区移动 / ' +
    'FINAL_DELIVERY 交付）、boundary（边界）。' +
    identityNote,
  'case-closure': '请填 caseId（清关单）、cutoffAt（截点，RFC 3339 时间，例如 2026-11-13T08:00:00Z）。还有未了结的事项时答不能关闭并列出来。' + identityNote,
  'regulatory-restriction':
    '请填 restrictionId（给这条限制起的标识）、decision（依据的那条海关结果的来源标识）、constrains（限制哪几步，数组，取值同放行检查的 action）。' +
    '依据必须是已登记、可归属、未被更正的海关结果。' +
    identityNote,
  'regulatory-restriction-release':
    '请填 restrictionId（要解除的限制）、result（解除所依据的那条海关放行结果的来源标识）。只有完整放行的结果能解除。' + identityNote,
};

/** 各口答复的中文。没收录的原样示出英文原名。 */
export const customsDecisionOutcomeLabels: Record<string, string> = {
  CASE_ESTABLISHED: '已建立清关单',
  EXISTING_CASE: '这张清关单已经在了',
  AUTHORIZATION_CONFIRMED: '已确认提交授权',
  DECLARATION_SUBMITTED: '已形成提交版本',
  DECLARATION_CORRECTED: '已形成更正版本',
  EXISTING_VERSION: '这一版已经提交过',
  NOT_AUTHORIZED: '还没有提交授权',
  NOT_READY: '申报准备还没完成',
  CASE_UNKNOWN: '找不到这张清关单',
  UNIT_CONFLICT: '报关批次与已有内容不一致',
  SOURCE_CONFLICT: '与已提交的内容不一致',
  SOURCE_NOT_ACCEPTED: '内容不完整或不成立，没有受理',
  NOT_ACCEPTED: '内容不完整或不成立，没有受理',
  RECORDED: '已记下检查结果',
  EXISTING_RESULT: '这次检查已经记过',
  GATE_CATALOG_NOT_CONFIGURED: '放行检查条件还没有登记',
  DUTY_PAYMENT_GATE_RULE_NOT_CONFIGURED: '税费支付的检查规则还没有登记',
  DUTY_VERIFICATION_ABSENT: '还没有税费核验结果',
  DUTY_VERIFICATION_PENDING: '税费核验还在进行',
  CLOSED: '已关闭',
  ALREADY_CLOSED: '此前已经关闭',
  BLOCKED: '还有未了结的事项，不能关闭',
  OBLIGATION_INVENTORY_UNCONFIGURED: '关闭核对清单还没有登记',
  ESTABLISHED: '已建立限制',
  EXISTING: '这条限制已经在了',
  RELEASED: '已解除',
  ALREADY_RELEASED: '此前已经解除',
  BASIS_NOT_ACCEPTED: '依据的海关结果不能用（没登记、已被更正、层级不对或有冲突）',
  CONTENT_CONFLICT: '同一个标识下登的是另一份内容',
  NOT_FOUND: '找不到这条限制',
};

export function submitCustomsDecision(kind: CustomsDecisionKind, snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(customsDecisionEndpoints[kind], snapshot);
}

export function customsDecisionTarget(kind: CustomsDecisionKind): RegistrationTarget {
  return {
    id: kind,
    label: customsDecisionTitles[kind],
    title: customsDecisionTitles[kind],
    endpoint: `POST ${customsDecisionEndpoints[kind]}`,
    snapshotHint: customsDecisionHints[kind],
    submit: (snapshot: unknown) => submitCustomsDecision(kind, snapshot),
    outcomeLabels: customsDecisionOutcomeLabels,
  };
}

/** 清关单页「运营决定」签上的五个口，按一张清关单从建到关的顺序排。 */
export const caseDecisionTargets: RegistrationTarget[] = (
  ['case-establishment', 'submission-authorization', 'declaration-submission', 'release-gate-verification', 'case-closure'] as const
).map(customsDecisionTarget);

/** 限制页登记签上追加的两个口。 */
export const restrictionDecisionTargets: RegistrationTarget[] = (['regulatory-restriction', 'regulatory-restriction-release'] as const).map(
  customsDecisionTarget,
);
