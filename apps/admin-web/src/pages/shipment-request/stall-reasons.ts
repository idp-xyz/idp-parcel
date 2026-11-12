// 受理任务「最近一次没能推进」的原因（lastAttemptReason）的屏上写法，一个码在各页读成同一句。纯函数，node:test 直接钉；
// 词表跟不跟得上 Go 侧的取值，由 stall-reasons.contract.test.ts 对着 Go 源核。
//
// 任务上记的是小包托运编排给的未决原因原词（application 包的 JudgmentPendingReason），财务控制那一步停下时是
// FINANCIAL_CONTROL_* 那一族。结算答的细原因另在一格（lastAttemptSettlementReason，不带前缀，ADR-0380 决定三）；它与适配器
// 写进停因码时加了 `SA-` 的那一族是同一组码，查同一张表，免得两份词表各自漂。
//
// 认得的译成中文，认不得的照原码——页面落后于服务端时宁可露出原码，也不把它并进某个认得的说法。

interface StallWords {
  text: string;
  /** 停在受理前财务控制那一步：这时估价可能还没形成，「受理前财务控制」一块要把它说出来。 */
  financialControl?: true;
  /** 停在结算估价那一口（采用册、试算、留痕与合计各格）：细原因前说「估价没形成」，不笼统说财务控制没形成。 */
  estimate?: true;
  /** 登一张价卡解得开（ADR-0380 决定一）：等运营登记时说清登什么，链到价格表。 */
  priceCardSide?: true;
}

const financial = (text: string): StallWords => ({ text, financialControl: true });
const estimate = (text: string): StallWords => ({ text, financialControl: true, estimate: true });
const priceCardSide = (text: string): StallWords => ({ text, financialControl: true, estimate: true, priceCardSide: true });

// 结算的未形成原因与两种拒收，前缀 `SA-` 是小包托运的结算适配器加的（结算一侧的原因码本身不带前缀）。
const settlementStallWords: Record<string, StallWords> = {
  'SA-CONTROL_POLICY_UNAVAILABLE': financial('财务控制策略读不到'),
  'SA-CONTROL_POLICY_NOT_CONFIGURED': financial('财务控制策略未配置'),
  'SA-BALANCE_UNAVAILABLE': financial('运营余额读不到'),
  'SA-FREEZE_LEDGER_UNAVAILABLE': financial('冻结账本读写不通'),
  'SA-CREDIT_STANDING_UNAVAILABLE': financial('信用状况读不到'),
  'SA-EXPOSURE_LEDGER_UNAVAILABLE': financial('信用占用账本读写不通'),
  'SA-CREDIT_BASIS_UNAVAILABLE': financial('授信依据读不到'),
  'SA-CREDIT_BASIS_NOT_CONFIGURED': financial('授信依据未配置'),
  'SA-CREDIT_RATIO_BASE_UNDECIDED': financial('比例额度没有声明基数'),
  'SA-CREDIT_RATIO_BASE_UNAVAILABLE': financial('比例额度的基数读不到'),
  'SA-CREDIT_RATIO_BASE_NOT_ESTABLISHED': financial('比例额度的基数还没有事实（未登记运营余额或还没有有效对账单）'),
  'SA-CONTROL_AMOUNT_NOT_CONFIGURED': estimate('这户没有登记受理前估价采用'),
  'SA-ESTIMATE_ADOPTION_UNAVAILABLE': estimate('受理前估价采用读不到'),
  'SA-ESTIMATE_TRIALS_UNAVAILABLE': estimate('试算调不通'),
  'SA-ESTIMATE_TRACE_UNAVAILABLE': estimate('估价留痕读写不通'),
  'SA-ESTIMATE_PRICE_CARD_NOT_CONFIGURED': priceCardSide('没有在用价卡'),
  'SA-ESTIMATE_PRICE_CARD_CONFLICT': priceCardSide('价卡适用冲突'),
  'SA-ESTIMATE_SUBMISSION_NOT_VISIBLE': estimate('估价时读不到这一提交版本'),
  // 申报不全（ADR-0381）：缺什么、谁来补由 last-attempt.ts 按缺项码与任务缺口再说细；这里是没有那两格时的底句。
  'SA-ESTIMATE_DECLARATION_INCOMPLETE': estimate('申报缺了计价要的资料'),
  'SA-ESTIMATE_HAS_NO_TRIAL': estimate('没有一件试算'),
  'SA-ESTIMATE_TRIAL_WITHOUT_TOTAL': priceCardSide('有一件试算没有合计'),
  // 币种对不上不归价卡一侧：登一张卡解不开，结算一侧照旧内部重试，续办不指到价格表。
  'SA-ESTIMATE_CURRENCY_MISMATCH': estimate('各件试算币种不一致'),
  'SA-ESTIMATE_NOT_POSITIVE': priceCardSide('估价合计不是正数'),
  'SA-ESTIMATE_TRIAL_NEGATIVE': priceCardSide('有一件试算为负'),
  'SA-ESTIMATE_SUM_OVERFLOW': estimate('估价合计超出可记范围'),
  'SA-REQUEST_CONFLICT': financial('同一控制请求两次内容不一致'),
  'SA-REQUEST_NOT_ACCEPTED': financial('结算没受理这次控制请求'),
};

// 小包托运自己的未决原因。说的是停在哪一步、哪样东西没到；等运营登记的那一族照 Go 侧的续办路径点明，因为那是唯一要人去做的。
const shipmentStallWords: Record<string, StallWords> = {
  COMMERCIAL_BASIS_NOT_APPLICABLE: { text: '商务依据不适用这票运单' },
  COMMERCIAL_BASIS_UNDETERMINED: { text: '商务依据适不适用判不出' },
  // 与用语词典里同一个码同一句。
  COMMERCIAL_BASIS_UNAVAILABLE: { text: '商务依据暂不可用' },
  ADOPTED_COMMERCIAL_BASIS_NOT_RECORDED: { text: '采用的商务依据没记下' },
  COMMERCIAL_BASIS_SUPERSEDED: { text: '商务依据已换版，要按新版重判' },
  COMMERCIAL_REVALIDATION_BASIS_NOT_RESOLVED: { text: '受理前重校商务依据：依据未解析' },
  COMMERCIAL_REVALIDATION_INPUT_NOT_ACCEPTED: { text: '受理前重校商务依据：入参未受理' },
  REACHABILITY_AS_OF_NOT_DECLARED: { text: '可达性判断时点没有声明' },
  REACHABILITY_AS_OF_BASIS_NOT_RESOLVED: { text: '可达性判断时点的依据未解析' },
  REACHABILITY_AS_OF_NOT_CONFIGURED: { text: '可达性判断时点未配置，等运营登记' },
  REACHABILITY_AS_OF_UNAVAILABLE: { text: '可达性判断时点读不到' },
  REACHABILITY_AS_OF_VALUE_REJECTED: { text: '可达性判断时点的取值被拒' },
  REACHABILITY_AS_OF_INPUT_NOT_ACCEPTED: { text: '可达性判断时点的入参未受理' },
  REACHABILITY_AUTHORITY_UNAVAILABLE: { text: '可达性判断调不通' },
  REACHABILITY_JUDGMENT_NOT_FORMED: { text: '可达性判断没形成' },
  REACHABILITY_REQUEST_CONFLICT: { text: '可达性判断请求两次内容不一致' },
  REACHABILITY_REQUEST_NOT_ACCEPTED: { text: '可达性判断请求未受理' },
  REACHABILITY_JUDGMENT_SUPERSEDED: { text: '可达性判断已换代，要按新时点重判' },
  REACHABILITY_JUDGMENT_FORMED_UNDER_ANOTHER_RESOLUTION: { text: '可达性判断形成于别的商务解析之下，要按现在的解析重判' },
  REACHABILITY_REVALIDATION_JUDGMENT_NOT_FOUND: { text: '受理前重校可达性：没找回原判断' },
  REACHABILITY_REVALIDATION_UNDETERMINED: { text: '受理前重校可达性：无法判定' },
  REACHABILITY_REVALIDATION_UNAVAILABLE: { text: '受理前重校可达性：判断读不到' },
  REACHABILITY_REVALIDATION_INPUT_NOT_ACCEPTED: { text: '受理前重校可达性：入参未受理' },
  FINANCIAL_CONTROL_AS_OF_NOT_DECLARED: financial('财务控制的判断时点没有声明'),
  FINANCIAL_CONTROL_AS_OF_BASIS_NOT_RESOLVED: financial('财务控制的判断时点依据未解析'),
  FINANCIAL_CONTROL_AS_OF_NOT_CONFIGURED: financial('财务控制的判断时点未配置，等运营登记'),
  FINANCIAL_CONTROL_AS_OF_UNAVAILABLE: financial('财务控制的判断时点读不到'),
  FINANCIAL_CONTROL_AS_OF_VALUE_REJECTED: financial('财务控制的判断时点取值被拒'),
  FINANCIAL_CONTROL_AS_OF_INPUT_NOT_ACCEPTED: financial('财务控制的判断时点入参未受理'),
  FINANCIAL_CONTROL_UNAVAILABLE: financial('结算的财务控制调不通'),
  FINANCIAL_CONTROL_NOT_FORMED: financial('财务控制没形成（结算没能估价或没能占用）'),
  FINANCIAL_CONTROL_NOT_CONFIGURED: financial('财务控制缺一项登记，等运营登记'),
  FINANCIAL_CONTROL_REQUEST_CONFLICT: financial('财务控制请求两次内容不一致'),
  FINANCIAL_CONTROL_REQUEST_NOT_ACCEPTED: financial('结算没受理这次财务控制请求'),
  JUDGMENT_NOT_RECORDED: { text: '判断结果没记下' },
  SHIPMENT_REQUEST_UNAVAILABLE: { text: '运单读不到' },
  MANUAL_REVIEW_POLICY_NOT_DECLARED: { text: '人工复核规则没有声明' },
  RECORDED_JUDGMENTS_UNAVAILABLE: { text: '已记下的判断读不到' },
  DECISION_IDENTITY_UNAVAILABLE: { text: '决定标识签发不出' },
  ACCEPTANCE_JUDGMENT_INCOMPLETE: { text: '受理判断还没齐' },
  DECISION_NOT_RECORDED: { text: '决定没记下' },
  CONTROL_RELEASE_PENDING: { text: '占用的资金或信用还没放掉' },
  REJECTION_AUTHORITY_UNAVAILABLE: { text: '拒绝授权读不到' },
  CUSTOMER_SUPPLEMENT_PENDING: { text: '等客户补充' },
  MANUAL_REVIEW_PENDING: { text: '等人工复核' },
  WITHDRAWAL_AUTHORITY_UNAVAILABLE: { text: '撤回授权读不到' },
  SOURCE_DATA_AMENDMENT_AUTHORITY_UNAVAILABLE: { text: '资料修订授权读不到' },
  SOURCE_DATA_RULE_UNAVAILABLE: { text: '资料修订规则读不到' },
  SOURCE_DATA_VERSION_IDENTITY_UNAVAILABLE: { text: '资料版本标识签发不出' },
  AMENDED_REQUEST_NOT_SAVED: { text: '修订后的运单没保存下来' },
  SUBMISSION_IDENTITY_UNAVAILABLE: { text: '提交版本标识签发不出' },
  SUPPLEMENTED_REQUEST_NOT_SAVED: { text: '补充后的运单没保存下来' },
  SOURCE_DATA_VERSION_NOT_HANDED_OFF: { text: '资料版本没交出' },
  ACCEPTANCE_DECISION_NOT_HANDED_OFF: { text: '受理决定没交出' },
  STALE_SHIPMENT_REQUEST_REVISION: { text: '保存时被别的操作抢先' },
  REJECTION_AUTHORITY_RULES_NOT_CONFIGURED: { text: '拒绝授权规则未配置，等运营登记' },
  WITHDRAWAL_AUTHORITY_RULES_NOT_CONFIGURED: { text: '撤回授权规则未配置，等运营登记' },
  SOURCE_DATA_AMENDMENT_AUTHORITY_RULES_NOT_CONFIGURED: { text: '资料修订授权规则未配置，等运营登记' },
  OPERATOR_REGISTRATION_WAIT_NOT_SAVED: { text: '等待运营登记（这一停没能保存）' },
  CUSTOMER_SUPPLEMENT_WAIT_NOT_SAVED: { text: '等待受控补充（这一停没能保存）' },
  SOURCE_DATA_AMENDMENT_STAGE_UNDETERMINED: { text: '资料修订阶段判不出' },
  SOURCE_DATA_AMENDMENT_STAGE_FACT_UNAVAILABLE: { text: '资料修订阶段的事实读不到' },
  AUTHORIZED_DISPOSITION_PENDING: { text: '等授权处置' },
  CONTROL_DISPOSITION_UNAVAILABLE: { text: '受限项的失败处置读不到' },
  CONTROL_DISPOSITION_NOT_FORMED: { text: '受限项的失败处置没形成' },
  REJECTION_AUTHORITY_APPLICABILITY_CONFLICT: { text: '拒绝授权适用冲突，等运营登记' },
  WITHDRAWAL_AUTHORITY_APPLICABILITY_CONFLICT: { text: '撤回授权适用冲突，等运营登记' },
  SOURCE_DATA_AMENDMENT_AUTHORITY_APPLICABILITY_CONFLICT: { text: '资料修订授权适用冲突，等运营登记' },
  CONTROL_RELEASE_REFUSAL_NOT_RECORDED: { text: '释放被结算拒收，却没记进释放未受理册' },
};

/** 词表收录的码，给契约测试对着 Go 源核两边是否对得上。 */
export const knownSettlementStallCodes: readonly string[] = Object.keys(settlementStallWords);
export const knownShipmentStallCodes: readonly string[] = Object.keys(shipmentStallWords);

export interface StallReason {
  code: string;
  text: string;
  /** 认得这个码；认不得时 text 就是原码。 */
  known: boolean;
  financialControl: boolean;
}

export function stallReasonOf(code: string): StallReason {
  const words = settlementStallWords[code] ?? shipmentStallWords[code];
  if (words) return { code, text: words.text, known: true, financialControl: words.financialControl === true };
  // 认不得的 `SA-` 码仍是结算在财务控制那一步答的：前缀是适配器加的出处，不是从名字猜意思。
  return { code, text: code, known: false, financialControl: code.startsWith('SA-') };
}

export interface SettlementReason {
  /** 服务端交来的原码，不带前缀。 */
  code: string;
  /** 认不得时就是原码。 */
  text: string;
  known: boolean;
  estimate: boolean;
  priceCardSide: boolean;
}

/**
 * 申报不全而缺项在价卡一侧（ADR-0381 决定一，票 declared-goods-lines/27）：ZONE 是卡要发起方直接给分区、ADDRESS_NATURE 是卡按
 * 收件地址性质判价——两样受理前估价都给不出，客户补不了，归等运营登记。wants 接在「在用的卡」后面说这张卡为什么做不了，
 * registration 接在「登记一张」后面说该登什么样的售价卡；叫法与计价缺项的运营译法（pages/pricing/presentation.ts 的
 * estimateMissingLabels）同词。封闭集里其余四码客户可补，不在这张表。
 */
export const estimateGapCardSideWords: Record<string, { wants: string; registration: string }> = {
  ZONE: { wants: '要直接给分区', registration: '带分区目录' },
  ADDRESS_NATURE: { wants: '要收件地址性质', registration: '不按收件地址性质判价' },
};

/** 处理尝试上记的结算未形成原因（不带前缀）：补上前缀查 `SA-` 那张表。 */
export function settlementReasonOf(code: string): SettlementReason {
  const words = settlementStallWords[`SA-${code}`];
  if (!words) return { code, text: code, known: false, estimate: false, priceCardSide: false };
  return { code, text: words.text, known: true, estimate: words.estimate === true, priceCardSide: words.priceCardSide === true };
}
