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
  | 'manual-lodgement'
  | 'external-result-entry'
  | 'regulatory-restriction'
  | 'regulatory-restriction-release';

export const customsDecisionEndpoints: Record<CustomsDecisionKind, string> = {
  'case-establishment': '/customs-cases',
  'submission-authorization': '/customs-submission-authorizations',
  'declaration-submission': '/customs-declaration-submissions',
  'release-gate-verification': '/customs-release-gate-verifications',
  'case-closure': '/customs-case-closures',
  'manual-lodgement': '/customs-declaration-manual-lodgements',
  'external-result-entry': '/customs-external-result-manual-registrations',
  'regulatory-restriction': '/customs-regulatory-restrictions',
  'regulatory-restriction-release': '/customs-regulatory-restriction-releases',
};

export const customsDecisionTitles: Record<CustomsDecisionKind, string> = {
  'case-establishment': '建立清关单',
  'submission-authorization': '确认提交授权',
  'declaration-submission': '提交申报',
  'release-gate-verification': '放行检查',
  'case-closure': '关闭清关单',
  'manual-lodgement': '标记申报已线下递交',
  'external-result-entry': '录入海关结果',
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
  'manual-lodgement':
    '没有对接报关行或单一窗口时用这一个：这一版申报你已经在线下递交了，在这里记一笔。请填 version（提交版本，取「提交申报」答复里的 versionId）、' +
    'original（递交凭据：回单、邮件或受理单的编号或存档引用，必填）。可选：lodgedAt（实际递交的时间，RFC 3339，不能晚于现在、不能早于这一版形成的时间；不填取现在）、' +
    'reference（对方给的受理号）。系统不发送任何东西；这一版记为「已递交、待确认」，海关的受理与放行随后用「录入海关结果」录进来。' +
    '登记之后申报准备或提交授权已经失效的，要先重新取得。' +
    identityNote,
  'external-result-entry':
    '没有对接海关或报关行系统时，照回执在这里录入。请填 sourceId（给这条结果起的标识，同一份回执只用一个）、layer（结果层：REGULATORY_RECEIPT 海关已接收 / ' +
    'BUSINESS_ACCEPTANCE 海关已受理 / PROCESS_DECISION 过程决定 / ASSESSED_DUTY 核定税费 / RELEASE_RESULT 放行结果 / DISPOSITION_DECISION 处置决定）、' +
    'role（发出结果的机构角色）、rawSemantics（回执原文）、claimedVersion（回执对应的申报版本）、attempt（第几次提交，整数）、scope（申报范围）、' +
    'occurredAt（回执上的业务时间，RFC 3339）、original（所凭原件的编号或存档引用，必填）。' +
    '放行结果（layer 填 RELEASE_RESULT）必须另填 release，是一个对象：kind（FULL 完整放行 / PARTIAL 部分放行 / CONDITIONAL 附条件放行）、authority（放行机构）、' +
    'condition（附条件放行时填条件原文）。更正前一条时填 correctsSourceId。系统会在你填的标识前加上 MANUAL/，与接口送来的结果分开；' +
    '要更正一条人工录入的结果，correctsSourceId 填带 MANUAL/ 的那个标识。' +
    identityNote,
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

/** 录入海关结果这一口自己的答复词：与别的口重名的格（已有结果、未决）在这里说的是另一件事。 */
export const externalResultEntryOutcomeLabels: Record<string, string> = {
  RESULT_RECORDED: '已记下这条海关结果',
  EXISTING_RESULT: '这条结果已经录过，内容相同',
  SOURCE_CONFLICT: '同一个标识下已经录过另一份内容；要改请用更正',
  UNATTRIBUTABLE: '归属不到已有的申报版本，原文已留存',
  LAYER_CONFLICT: '与同一层已有的结果矛盾，两份都已留存',
  SOURCE_NOT_ACCEPTED: '内容不完整或不成立，没有受理',
  RESULT_UNDECIDED: '暂时定不了，原因见下',
};

/** 线下递交标记这一口自己的答复词。 */
export const manualLodgementOutcomeLabels: Record<string, string> = {
  LODGEMENT_RECORDED: '已记为已递交，等待海关结果',
  DECLARATION_ALREADY_SENT: '这一版此前已经发出或已经记过，没有再记一次',
  DECLARATION_SEND_HELD: '申报准备或提交授权已经失效，没有记；请先重新取得',
  SOURCE_NOT_ACCEPTED: '内容不完整或不成立（版本不在册、没有递交凭据、递交时间在未来或早于这一版形成的时间），没有受理',
};

export const externalResultEntryUndecidedLabels: Record<string, string> = {
  RELEASE_SEMANTICS_UNINTERPRETED: '放行结果没有填 release，或读不出放行内容',
  EVALUATION_INSTANT_UNTRUSTED: '回执上的业务时间没填，或晚于现在',
  INTERPRETATION_RULE_UNCONFIGURED: '这一层结果的读法规则还没有登记',
  JURISDICTION_UNRESOLVED: '定不了这条结果属于哪个辖区',
  CORRECTED_RESULT_NOT_RECORDED: '要更正的那一条不在册',
};

export function customsDecisionTarget(kind: CustomsDecisionKind): RegistrationTarget {
  if (kind === 'manual-lodgement') {
    return {
      id: kind,
      label: customsDecisionTitles[kind],
      title: customsDecisionTitles[kind],
      endpoint: `POST ${customsDecisionEndpoints[kind]}`,
      snapshotHint: customsDecisionHints[kind],
      submit: (snapshot: unknown) => submitCustomsDecision(kind, snapshot),
      outcomeLabels: manualLodgementOutcomeLabels,
    };
  }
  if (kind === 'external-result-entry') {
    return {
      id: kind,
      label: customsDecisionTitles[kind],
      title: customsDecisionTitles[kind],
      endpoint: `POST ${customsDecisionEndpoints[kind]}`,
      snapshotHint: customsDecisionHints[kind],
      submit: (snapshot: unknown) => submitCustomsDecision(kind, snapshot),
      outcomeLabels: externalResultEntryOutcomeLabels,
      undecidedReasonLabels: externalResultEntryUndecidedLabels,
    };
  }
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

/** 清关单页「运营决定」签上的七个口，按一张清关单从建到关的顺序排；海关结果在没有对接时由人照回执录入。 */
export const caseDecisionTargets: RegistrationTarget[] = (
  ['case-establishment', 'submission-authorization', 'declaration-submission', 'manual-lodgement', 'external-result-entry', 'release-gate-verification', 'case-closure'] as const
).map(customsDecisionTarget);

/** 限制页登记签上追加的两个口。 */
export const restrictionDecisionTargets: RegistrationTarget[] = (['regulatory-restriction', 'regulatory-restriction-release'] as const).map(
  customsDecisionTarget,
);
