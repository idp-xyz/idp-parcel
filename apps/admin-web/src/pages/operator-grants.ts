// 操作者授予自查答复的判读（ADR-0315）。`GET /operator-grants` 答这个会话的操作者此刻持有哪几格能力面授予与哪几种运营决定授予；
// 管理台据此不摆这个操作者做不了的写入口。纯函数，node:test 钉着；取数与钩子在 use-operator-grants.ts。只做类型导入，好让
// run-tests 的 CommonJS 发射直接加载本文件。
//
// **摆不摆只是呈现，不是裁决**：每个写口照旧在服务端判授予，拒绝仍是服务端的答案。所以这里只在「明确知道没有」时才收起入口——
// 答不出（未配置、拒绝、依赖故障、读不到、答复形状不对）一律当作不知道，入口照摆，让服务端去答。

import type { ApiResult } from './catalogue-api';

export interface OperatorGrantsResponseBody {
  outcome: string;
  grants: string[];
  decisions: string[];
}

/** 登记册配置写：ADR-0085 那一族登记与发布端点的能力面，原词取自 accessidentity.CapabilityRegistryConfigurationWrite。 */
export const registryWriteFace = 'REGISTRY_CONFIGURATION_WRITE';

export interface HeldGrants {
  faces: ReadonlySet<string>;
  decisions: ReadonlySet<string>;
}

/**
 * 判成持有的授予。只有业务成格且两个数组都在场才算拿到；其余各态一律 undefined——「不知道」，不折成「什么都没有」。
 */
export function heldGrantsOf(answer: ApiResult<OperatorGrantsResponseBody> | null): HeldGrants | undefined {
  if (answer?.kind !== 'outcome' || answer.body?.outcome !== 'OPERATOR_GRANTS_RESOLVED') return undefined;
  const { grants, decisions } = answer.body;
  if (!Array.isArray(grants) || !Array.isArray(decisions)) return undefined;
  return { faces: new Set(grants), decisions: new Set(decisions) };
}

/** 收寄后服务处置决定的授予（运营决定一族），原词取自 accessidentity.DecisionPostIntakeServiceDisposition。 */
export const postIntakeDispositionDecision = 'POST_INTAKE_SERVICE_DISPOSITION';

/** 待人工处理项登记的授予，与处置决定分开授予（ADR-0341 决定三），原词取自 accessidentity.DecisionPostIntakeManualItemHandling。 */
export const postIntakeManualItemDecision = 'POST_INTAKE_MANUAL_ITEM_HANDLING';

// 线路与运力区几个决定入口的授予原词，各取自 accessidentity 同名的 DecisionKind。写成常量、在单测里钉住等于服务端原词：
// 拼错一个字，持这一种授予的人会悄悄看不到按钮，而服务端那边照常放行，没有任何地方会报。
/** 授权改路，原词取自 accessidentity.DecisionAuthorizedReroute；改路建议与初始路由成本并列两个决定口认同一种。 */
export const authorizedRerouteDecision = 'AUTHORIZED_REROUTE';
/** 送站交接确认，原词取自 accessidentity.DecisionDeliveryHandoverConfirmation。 */
export const deliveryHandoverConfirmationDecision = 'DELIVERY_HANDOVER_CONFIRMATION';
/** 外部轨迹有效时间判断，原词取自 accessidentity.DecisionEffectiveTimeJudgment。 */
export const effectiveTimeJudgmentDecision = 'EFFECTIVE_TIME_JUDGMENT';
/** 待识别复核，原词取自 accessidentity.DecisionPendingIdentityRecheck。 */
export const pendingIdentityRecheckDecision = 'PENDING_IDENTITY_RECHECK';

/** 代客户登记对账单争议，原词取自 accessidentity.DecisionStatementDisputeRegistration（ADR-0354 决定一）。 */
export const statementDisputeRegistrationDecision = 'STATEMENT_DISPUTE_REGISTRATION';
/** 核实后处理对账单争议，与登记分开授予，原词取自 accessidentity.DecisionStatementDisputeResolution（ADR-0354 决定二）。 */
export const statementDisputeResolutionDecision = 'STATEMENT_DISPUTE_RESOLUTION';

/** 照来件登记一份供应商账单并逐行裁决匹配，原词取自 accessidentity.DecisionSupplierBillReception（ADR-0340 决定一）。 */
export const supplierBillReceptionDecision = 'SUPPLIER_BILL_RECEPTION';
/** 对账单的一行形成审核应付，与接收分开授予，原词取自 accessidentity.DecisionSupplierBillAudit（ADR-0340 决定二）。 */
export const supplierBillAuditDecision = 'SUPPLIER_BILL_AUDIT';

/** 运营决定入口摆不摆：同登记入口，明确知道没有那一种运营决定授予才收起，不知道就摆。 */
export function mayMakeDecision(held: HeldGrants | undefined, decision: string): boolean {
  return held === undefined || held.decisions.has(decision);
}

/** 登记与发布入口摆不摆：明确知道没有登记册配置写授予才收起，不知道就摆。 */
export function mayWriteRegistry(held: HeldGrants | undefined): boolean {
  return held === undefined || held.faces.has(registryWriteFace);
}

/** 入口按授予收起时，原本指向它的文案改说这一句：说谁能用，不让人去找本页没有摆出的签或按钮。 */
export const hiddenEntryHint = '登记与发布入口只对持登记册配置写授予的操作者显示。';

/**
 * 空态与说明里指向写入口的那半句。入口摆着时照说它在哪（`pointer`）；入口按授予收起时换成 `whenHidden`，默认 hiddenEntryHint——
 * 文案跟着入口走，否则查阅员会被指去一个看不见的「登记」签。本页本来就没有那个入口、只是在说载体从哪来的，另给一句或给空串。
 */
export function entryHint(mayWrite: boolean, pointer: string, whenHidden: string = hiddenEntryHint): string {
  return mayWrite ? pointer : whenHidden;
}

/**
 * 运营决定入口按授予收起时，原处改说的那一句：说这个动作只对持哪一种授予的人显示，与 hiddenEntryHint 同一个说法，不让人以为
 * 页面坏了、也不指去一个看不见的按钮。授予的中文名由调用方取用语词典（本文件只做类型导入，不引词典）。
 */
export function hiddenDecisionHint(action: string, grant: string): string {
  return `${action}入口只对持${grant}授予的操作者显示。`;
}
