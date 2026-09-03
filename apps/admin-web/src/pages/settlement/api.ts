// 本文件是 settlement-accounting 上下文的 fetch 出口：费用与计费、对账单、收付款核销、
// 经营核算四页共四个查阅端点（票 admin-skeleton-closure-batch/04）。形状以
// internal/settlementaccounting/adapters/http 下四个 query_settlement_* 为准，此处只做
// 镜像不虚构。传输与五格判读收敛在共享 catalogue-api，本文件只保留本上下文的类型与查询函数。
//
// 四个端点合在这一个文件而不按页面分四份：四页同属一个上下文、镜像的是同一个 Go 包，
// 拆开就会有几处各自维护同一批字段，而 Go 那侧改一个键时只会有一处跟上。对账单与收付款
// 核销两页曾落在 pages/governance/ 时也是从此处引入的，归位本目录后这条理由不变。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

// —— 费用与计费页（GET /settlement-charges）——

/** 费用册封闭两格，与传输层 ?registry= 分派同词。 */
export type ChargeRegistry = 'customer-charge' | 'supplier-expected-cost';

/**
 * 一种已到达的确认依据。「到了哪几种」与「这类费用要哪一种」是两张表两个答案，
 * 服务端分两格上列不代算交集，页面同样不算——「确认条件已满足」不该有第三条成立路径。
 */
export interface ChargeConfirmationBasisRecord {
  basisKind: string;
  basis: string;
  recordedAt: string;
}

/**
 * 一条客户费用。币种三件组（原币、合同结算币、换算依据）是从同一个评价采用来的一组，
 * 整组重述不拆散；同币种时 conversionStep 整键不出现，那是「这一步不存在」的正面形状。
 *
 * requiredBasisKind 缺席表示该费用项目在确认条件目录里没有行，与「配了但依据没到」
 * （有 requiredBasisKind 而 confirmationBases 里没有那一种）是两格，续办不同：前者要人去配
 * 条件，后者要人去催依据。confirmedAt 未确认时整键不出现，不为它编造零时刻。
 *
 * 确认时固定的七项（责任法人、结算相对方、收付方向、结算账户、合同或责任依据、主要计费
 * 范围、来源事实）随 ADR-0087 决定一入册，未确认行整键不出现——与 confirmedAt 同一处置，
 * 那是「这一行还没确认」的正面形状，不是缺数据。**页面不得为它们补默认值或由别处推断**：
 * CONTEXT 那句明禁「通过当前组织、当前客户属性或报表筛选临时推断」。
 */
export interface CustomerChargeRecord {
  charge: string;
  feeItem: string;
  evaluation: string;
  /** 封闭三格 ESTIMATED / PROVISIONAL / CONFIRMED；词表在 presentation.ts。 */
  stage: string;
  originalCurrency: string;
  originalAmount: string;
  settlementCurrency: string;
  settlementAmount: string;
  conversionStep?: string;
  confirmationBasis?: string;
  responsibleEntity?: string;
  counterparty?: string;
  /** 封闭二格 RECEIVABLE / PAYABLE；是**收付**方向不是借贷方向，词表在 presentation.ts。 */
  chargeDirection?: string;
  settlementAccount?: string;
  contractBasis?: string;
  primaryChargingScope?: string;
  sourceFact?: string;
  formedAt: string;
  confirmedAt?: string;
  requiredBasisKind?: string;
  confirmationBases: ChargeConfirmationBasisRecord[];
}

/**
 * 一版供应商预期成本。没有阶段键——整册属预估口径（CONTEXT「预期成本属预估口径，从不进
 * 对账单」），那是册级事实不是行级取值。priorVersion 与 correctionReason 成对缺席表示首版：
 * 计价纠错换版本、原版本保留，一份成本的历史因此是多行而不是一行被改写。
 */
export interface SupplierExpectedCostRecord {
  version: string;
  occurrence: string;
  occurrenceReason: string;
  occurrenceVersion: string;
  occurredAt: string;
  feeItem: string;
  purchaseRuleVersion: string;
  agreement: string;
  evaluation: string;
  originalCurrency: string;
  originalAmount: string;
  settlementCurrency: string;
  settlementAmount: string;
  conversionStep?: string;
  priorVersion?: string;
  correctionReason?: string;
  recordedAt: string;
}

export interface CustomerChargeListResponseBody {
  outcome: 'CUSTOMER_CHARGES_LISTED';
  charges: CustomerChargeRecord[];
}

export interface SupplierExpectedCostListResponseBody {
  outcome: 'SUPPLIER_EXPECTED_COSTS_LISTED';
  costs: SupplierExpectedCostRecord[];
}

// registry → 结果格的对照，判据同 customs 目录的同名对照表：调用侧按 registry 收窄后拿到
// 单格类型，不必写空分支。
interface ChargeBodyByRegistry {
  'customer-charge': CustomerChargeListResponseBody;
  'supplier-expected-cost': SupplierExpectedCostListResponseBody;
}

export function listSettlementCharges<Registry extends ChargeRegistry>(
  registry: Registry,
): Promise<ApiResult<ChargeBodyByRegistry[Registry]>> {
  return exchangeMasterData<ChargeBodyByRegistry[Registry]>(
    `/settlement-charges?registry=${encodeURIComponent(registry)}`,
  );
}

// —— 对账单页（GET /settlement-statements）——

/** 对账册封闭两格，与传输层 ?registry= 分派同词。 */
export type StatementRegistry = 'customer-statement' | 'supplier-bill-reception';

/**
 * 一项异议。裁定三件（结论、依据、时刻）同在或同缺；未裁定时三键皆不出现，页面不代填
 * 「待处理」——那会把「还没人裁」写成一个看起来已经有人处置过的结论。异议挂在单上而不改
 * 单的总额：无争议部分继续确认、开票、付款或核销。
 */
export interface StatementDisputeRecord {
  dispute: string;
  charge: string;
  disputedAmount: string;
  reason: string;
  openedAt: string;
  /** 封闭四格 ACCEPTED / PARTIALLY_ACCEPTED / REJECTED / PENDING_REVIEW；词表在 presentation.ts。 */
  resolution?: string;
  resolutionRef?: string;
  resolvedAt?: string;
}

/**
 * 一笔后续账期纳入。没有金额键——金额永远在费用或调整本体上，纳入只拥有关系。
 * adjustment 缺席是 LATE_CHARGE 的正面形状（迟到费用不得指名调整），不是漏登。
 */
export interface SubsequentInclusionRecord {
  inclusion: string;
  /** 封闭二格 ADJUSTMENT / LATE_CHARGE；词表在 presentation.ts。 */
  kind: string;
  originalPeriod: string;
  subsequentPeriod: string;
  charge: string;
  adjustment?: string;
  includedAt: string;
}

/**
 * 一张已发布对账单。lineCount 与 adjustmentCount 是费用行与调整行的行数不是金额：明细归
 * 详情面，但「这张单里有几行」与「一行都没有」得分得开。总额不由行数派生，两者各自照实
 * 转写——总额严格等于所含明细之和是写口的不变量，读侧重算一遍只会在两处各说一套。
 *
 * voidBasis 与 voidedAt 成对缺席表示未作废。作废不删行不改总额、替代单用新单号，所以
 * 「已作废」是这一行上的留痕而不是它的消失。
 */
export interface CustomerStatementRecord {
  statementNumber: string;
  account: string;
  period: string;
  currency: string;
  totalAmount: string;
  lineCount: number;
  adjustmentCount: number;
  publishedAt: string;
  voidBasis?: string;
  voidedAt?: string;
  disputes: StatementDisputeRecord[];
  subsequentInclusions: SubsequentInclusionRecord[];
}

/**
 * 一份供应商账单主张。auditAuthorityConfigured 照实转写而不折成「可审核」：授权未配置时
 * 审核停在未决，不默认放行也不虚构授权人；译成一个动作可用性就是在读面上替审核步骤
 * 做了那个判断。
 */
export interface SupplierBillReceptionRecord {
  claim: string;
  claimVersion: string;
  supplier: string;
  legalEntity: string;
  period: string;
  currency: string;
  lineCount: number;
  matchCount: number;
  auditAuthorityConfigured: boolean;
  recordedAt: string;
}

export interface CustomerStatementListResponseBody {
  outcome: 'CUSTOMER_STATEMENTS_LISTED';
  statements: CustomerStatementRecord[];
}

export interface SupplierBillReceptionListResponseBody {
  outcome: 'SUPPLIER_BILL_RECEPTIONS_LISTED';
  receptions: SupplierBillReceptionRecord[];
}

interface StatementBodyByRegistry {
  'customer-statement': CustomerStatementListResponseBody;
  'supplier-bill-reception': SupplierBillReceptionListResponseBody;
}

export function listSettlementStatements<Registry extends StatementRegistry>(
  registry: Registry,
): Promise<ApiResult<StatementBodyByRegistry[Registry]>> {
  return exchangeMasterData<StatementBodyByRegistry[Registry]>(
    `/settlement-statements?registry=${encodeURIComponent(registry)}`,
  );
}

// —— 收付款核销页（GET /settlement-funds-applications）——

/**
 * 一条真实收付映射。映射不是核销——事实接收、金额责任确认、真实到账与核销是不同结果，
 * 所以映射与核销分两个数组，不合成一格。
 */
export interface FundsMappingRecord {
  mapping: string;
  /** 封闭三格 STATEMENT / PAYABLE / CREDIT_NOTE；词表在 presentation.ts。 */
  targetKind: string;
  target: string;
  basis: string;
  mappedAt: string;
}

/**
 * 一次核销。allocationCount 是带方向分配片段的片段数，片段明细属详情面。
 * reversalBasis 与 reversedAt 成对缺席表示未撤销——撤销形成可追溯的反向关系，不删除原核销历史。
 */
export interface SettlementApplicationRecord {
  application: string;
  appliedAmount: string;
  basis: string;
  appliedAt: string;
  allocationCount: number;
  reversalBasis?: string;
  reversedAt?: string;
}

/**
 * 一笔已采用的外部资金事实。appliedAmount 与 reversedAmount 是两笔各算各的和，不是一个净额：
 * 前者只加未撤销的核销，后者只加已撤销的，unappliedAmount = amount − appliedAmount。三个数
 * 摆开是因为「从未核销过」与「核销过又撤销了」在一个净额上长着同一张脸，而这两态的续办相反。
 *
 * kind 照实透出不译成收付方向：封闭三格里只有一格是「收到了钱」，折成收／付两向会把
 * 「付款失败」与「资金退回」压成同一格。
 */
export interface ExternalFundsFactRecord {
  fact: string;
  source: string;
  /** 封闭三格 RECEIPT_CONFIRMED / PAYMENT_FAILED / FUNDS_RETURNED；词表在 presentation.ts。 */
  kind: string;
  currency: string;
  amount: string;
  version: string;
  occurredAt: string;
  corrects?: string;
  correctedAt?: string;
  appliedAmount: string;
  reversedAmount: string;
  unappliedAmount: string;
  applicationCount: number;
  mappings: FundsMappingRecord[];
  applications: SettlementApplicationRecord[];
}

export interface ExternalFundsFactListResponseBody {
  outcome: 'EXTERNAL_FUNDS_FACTS_LISTED';
  facts: ExternalFundsFactRecord[];
}

/**
 * 资金事实册只有一本，故本函数不收分派参数——封闭集为一时参数只会造出一个恒定值
 * （判据同 listCodSubledgers）。资金冻结册不在本端点：冻结是接受前财务控制的产物，
 * 与真实收付是两条链。
 */
export function listSettlementFundsApplications(): Promise<
  ApiResult<ExternalFundsFactListResponseBody>
> {
  return exchangeMasterData<ExternalFundsFactListResponseBody>('/settlement-funds-applications');
}

// —— 经营核算页（GET /settlement-operating-results）——

/** 经营册封闭两格，与传输层 ?registry= 分派同词。 */
export type OperatingRegistry = 'operating-result' | 'cost-allocation';

/**
 * 一个组成项：来源金额身份、在本口径下的角色、对指标的封闭二向与金额。
 *
 * role 随 ADR-0087 决定三入册并由服务端照册透出——**不是页面判出来的**。此前元素上没有
 * 角色维，要归栏就得由读侧先猜某个 source 属于哪一类，而那正是 CONTEXT 硬要求「审核应付
 * 与贷项按各自借贷方向分别计入一次、不得净含贷项」想让人看见的东西；现在册上说得出，
 * 页面照它分组即可，仍然不自行判断、不代贴标签。
 *
 * 取值按口径分组（预估／已确认／已结算各有自己的采用物），集外取值原样回显。
 */
export interface OperatingComponentRecord {
  source: string;
  /** 封闭八格，按口径分组；词表在 presentation.ts。 */
  role: string;
  /** 封闭二格 INCREASES / DECREASES；词表在 presentation.ts。 */
  effect: string;
  amount: string;
}

/**
 * 一份经营结果快照。margin 照库上那一列透出不重算：派生它的门在写口，读侧再算一遍就成了
 * 第二处定义。负毛利即经营损失，不另立一键——同一个数按正负分两键会让「零」落进两键都
 * 不占的缝里。
 */
export interface OperatingResultRecord {
  scope: string;
  period: string;
  /** 封闭三格 ESTIMATED / CONFIRMED / SETTLED；词表在 presentation.ts。 */
  basis: string;
  currency: string;
  margin: string;
  version: string;
  asOf: string;
  corrects?: string;
  recordedAt: string;
  components: OperatingComponentRecord[];
}

/** 归因到某个分析对象的一份份额。 */
export interface AllocationPortionRecord {
  target: string;
  amount: string;
}

/**
 * 一次成本分摊。unallocatedAmount 照实透出：已分摊与未分摊之和严格等于来源金额是写口的
 * 不变量，未分摊余额是第一类结果不是尾差——没有合格对象或分母为零时来源金额整笔留在这里
 * 等新依据。portions 为空数组即「全额未分摊」。
 */
export interface CostAllocationRecord {
  allocation: string;
  source: string;
  sourceAmount: string;
  currency: string;
  rule: string;
  unallocatedAmount: string;
  version: string;
  allocatedAt: string;
  corrects?: string;
  recordedAt: string;
  portions: AllocationPortionRecord[];
}

export interface OperatingResultListResponseBody {
  outcome: 'OPERATING_RESULTS_LISTED';
  results: OperatingResultRecord[];
}

export interface CostAllocationListResponseBody {
  outcome: 'COST_ALLOCATIONS_LISTED';
  allocations: CostAllocationRecord[];
}

interface OperatingBodyByRegistry {
  'operating-result': OperatingResultListResponseBody;
  'cost-allocation': CostAllocationListResponseBody;
}

export function listSettlementOperatingResults<Registry extends OperatingRegistry>(
  registry: Registry,
): Promise<ApiResult<OperatingBodyByRegistry[Registry]>> {
  return exchangeMasterData<OperatingBodyByRegistry[Registry]>(
    `/settlement-operating-results?registry=${encodeURIComponent(registry)}`,
  );
}
