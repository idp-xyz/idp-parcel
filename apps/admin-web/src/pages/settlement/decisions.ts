import type { RegistrationResponseBody, RegistrationTarget } from '../../components/registration';
import { postMasterData, type ApiResult } from '../catalogue-api';

/**
 * 账务的运营决定口：费用确认与调整登记（费用页），对账单发布、作废与后续账期纳入（对账页）。
 *
 * 后端口都已接上，这里先用登记面板把它们接进页面：操作员照提示填一份 JSON 提交，页面示出服务端的答复。不是引导式
 * 表单。所属公司由系统按登录身份记下；金额一律填最小货币单位的整数串（例如 96.00 元填 "9600"）。
 */

export type SettlementDecisionKind =
  | 'charge-confirmation'
  | 'charge-adjustment'
  | 'statement-publication'
  | 'statement-void'
  | 'late-charge-inclusion'
  | 'adjustment-inclusion';

export const settlementDecisionEndpoints: Record<SettlementDecisionKind, string> = {
  'charge-confirmation': '/settlement-charge-confirmations',
  'charge-adjustment': '/settlement-charge-adjustments',
  'statement-publication': '/settlement-statement-publications',
  'statement-void': '/settlement-statement-voids',
  'late-charge-inclusion': '/settlement-late-charge-inclusions',
  'adjustment-inclusion': '/settlement-adjustment-inclusions',
};

export const settlementDecisionTitles: Record<SettlementDecisionKind, string> = {
  'charge-confirmation': '确认费用',
  'charge-adjustment': '登记费用调整',
  'statement-publication': '发布对账单',
  'statement-void': '作废对账单',
  'late-charge-inclusion': '后到费用纳入后续账期',
  'adjustment-inclusion': '调整纳入后续账期',
};

const identityNote = ' 所属公司由系统按登录身份记下，不用填；需要持有这一种运营决定的授予。';

export const settlementDecisionHints: Record<SettlementDecisionKind, string> = {
  'charge-confirmation': '请填 charge（费用标识）。确认条件与确认事实齐了才确认得了；没齐会说明还差什么。' + identityNote,
  'charge-adjustment':
    '请填 adjustment（给这笔调整起的标识）、charge（已确认的费用）、kind（PRICING_CORRECTION 计价纠错，必带 evaluation 新评价引用 / ' +
    'COMMERCIAL_CONCESSION 商业让利，必带 authorization 授权引用）、direction（DEBIT 加收 / CREDIT 减收）、originalCurrency、originalMinor、' +
    'settlementCurrency、settlementMinor。结算币种要与费用相同，减收不能把费用减成负数。金额填最小货币单位的整数串。' +
    identityNote,
  'statement-publication':
    '请填 account（结算账户）、period（账期）、version（版本，如 v1）、currency（币种）、charges（费用标识数组）、lastAttributionDate（本期最后归属日，' +
    'YYYY-MM-DD）、declaredTotal（申明总额，最小货币单位整数串）、cutOffAt（截单时间，RFC 3339）。单号由系统取。' +
    identityNote,
  'statement-void': '请填 statement（对账单号）、basis（作废依据）。原单保留，作废只成立一次；替代单另行发布、用新单号。' + identityNote,
  'late-charge-inclusion':
    '请填 inclusion（给这次纳入起的标识）、statement（原对账单号）、charge（后到的已确认费用）、subsequentPeriod（纳入的后续账期）。' +
    '已经在某张有效对账单上出过账的费用不是后到费用；不能纳回原账期。' +
    identityNote,
  'adjustment-inclusion':
    '请填 inclusion（给这次纳入起的标识）、statement（原对账单号）、adjustment（已登记的调整）、subsequentPeriod（纳入的后续账期）。' +
    '一笔调整只纳入一次；不能纳回原账期。' +
    identityNote,
};

/** 各口答复的中文。没收录的原样示出英文原名。 */
export const settlementDecisionOutcomeLabels: Record<string, string> = {
  CHARGE_CONFIRMED: '已确认',
  ALREADY_CONFIRMED: '此前已经确认',
  CONDITION_NOT_MET: '确认条件还没满足',
  CONFIRM_UNDECIDED: '暂时确认不了，原因见下',
  ADJUSTMENT_RECORDED: '已登记调整',
  ALREADY_RECORDED: '这笔调整已经登记过',
  ADJUSTMENT_CONFLICT: '同一个标识下登的是另一笔内容',
  ADJUSTMENT_EXCEEDS_CHARGE: '减收超过了费用连同已有调整的金额',
  CHARGE_NOT_CONFIRMED: '这笔费用还没有确认',
  STATEMENT_PUBLISHED: '已发布',
  EXISTING_STATEMENT: '这一期已经发布过',
  PUBLICATION_INTENT_CONFLICT: '与已发布的内容不一致',
  STATEMENT_IMBALANCE: '申明总额与费用合计对不上',
  NO_CHARGE_IN_PERIOD: '本期没有可出账的费用',
  STATEMENT_CUT_UNCONFIGURED: '截单安排还没有登记',
  NUMBERING_RULE_COLLISION: '单号规则撞号，请先调整编号规则',
  STATEMENT_VOIDED: '已作废',
  ALREADY_VOIDED: '此前已经作废',
  LATE_CHARGE_INCLUDED: '已纳入后续账期',
  ADJUSTMENT_INCLUDED: '已纳入后续账期',
  EXISTING_INCLUSION: '这次纳入已经登记过',
  INCLUSION_CONFLICT: '同一个纳入标识下登的是另一份内容',
  INCLUSION_BACKFILLS_PERIOD: '不能纳回原账期',
  SUBJECT_ALREADY_INCLUDED: '这笔费用或调整已经纳入过',
  STATEMENT_VOIDED_TAKES_NO_INCLUSION: '原对账单已作废，不再往它上面纳入',
  SOURCE_NOT_ACCEPTED: '内容不完整或不成立，没有受理',
  SETTLEMENT_MOMENT_UNCONFIGURED: '这一步的结算时点还没有登记',
  SETTLEMENT_MOMENT_NOT_ACCEPTED: '这一步的结算时点没有放行',
};

export function submitSettlementDecision(kind: SettlementDecisionKind, snapshot: unknown): Promise<ApiResult<RegistrationResponseBody>> {
  return postMasterData<RegistrationResponseBody>(settlementDecisionEndpoints[kind], snapshot);
}

function target(kind: SettlementDecisionKind): RegistrationTarget {
  return {
    id: kind,
    label: settlementDecisionTitles[kind],
    title: settlementDecisionTitles[kind],
    endpoint: `POST ${settlementDecisionEndpoints[kind]}`,
    snapshotHint: settlementDecisionHints[kind],
    submit: (snapshot: unknown) => submitSettlementDecision(kind, snapshot),
    outcomeLabels: settlementDecisionOutcomeLabels,
  };
}

/** 费用页「运营决定」签上的两个口。 */
export const chargeDecisionTargets: RegistrationTarget[] = (['charge-confirmation', 'charge-adjustment'] as const).map(target);

/** 对账页「运营决定」签上的四个口。 */
export const statementDecisionTargets: RegistrationTarget[] = (
  ['statement-publication', 'statement-void', 'late-charge-inclusion', 'adjustment-inclusion'] as const
).map(target);
