// 结算与核算四页的查阅词表，加收付款核销页登记签的页面口径（票 sa-cc/31）。中文一律取
// settlement-accounting 领域枚举与用例文档的原词，不自造译法；集外取值由 labelOf 原样回显——
// 坏数据该露出来，不该被译成一句像样的话。

import type {
  ChargeRegistry,
  FundsRegistrationKind,
  OperatingRegistry,
  StatementRegistry,
} from './api';

// 三个端点的册名，与各自 ?registry= 分派同词。
export const chargeRegistryLabels: Record<ChargeRegistry, string> = {
  'customer-charge': '客户费用',
  'supplier-expected-cost': '供应商预期成本',
};

export const statementRegistryLabels: Record<StatementRegistry, string> = {
  'customer-statement': '客户对账单',
  'supplier-bill-reception': '供应商账单主张',
};

export const operatingRegistryLabels: Record<OperatingRegistry, string> = {
  'operating-result': '经营结果快照',
  'cost-allocation': '成本分摊',
};

// 客户费用三段封闭演进，中文取 ChargeStage 注释原词（费用可以经历预估、暂估和确认）。
// 「调整」不在此集：CONTEXT 把调整定为追加的调整明细，各由唯一创建用例形成，不是阶段的
// 第四个取值，词表也不为它留假格。
export const chargeStageLabels: Record<string, string> = {
  ESTIMATED: '预估',
  PROVISIONAL: '暂估',
  CONFIRMED: '确认',
};

// 异议处理的封闭四走向，中文取 UC-SA-003 原句（形成接受、部分接受、拒绝或待复核结果）。
export const disputeResolutionLabels: Record<string, string> = {
  ACCEPTED: '接受',
  PARTIALLY_ACCEPTED: '部分接受',
  REJECTED: '拒绝',
  PENDING_REVIEW: '待复核',
};

// 后续账期纳入的封闭二来源，中文取 InclusionKind 注释原词。
export const inclusionKindLabels: Record<string, string> = {
  ADJUSTMENT: '既有调整',
  LATE_CHARGE: '迟到的已确认费用',
};

// 外部资金事实的封闭三值，中文取 FundsFactKind 注释原词。通知不在其中——运营方收到付款
// 通知但财务未确认时保持待确认，词表跟着没有那一格。
export const fundsFactKindLabels: Record<string, string> = {
  RECEIPT_CONFIRMED: '确认到账',
  PAYMENT_FAILED: '付款失败',
  FUNDS_RETURNED: '资金退回',
};

// 资金可映射目标的封闭三值，中文取 SettlementTargetKind 注释原词。费用、赔付金额与追偿
// 认可本体不在其中——映射与核销改不了那些金额。
export const settlementTargetKindLabels: Record<string, string> = {
  STATEMENT: '客户对账单',
  PAYABLE: '供应商审核应付',
  CREDIT_NOTE: '供应商费用贷项',
};

// 经营指标口径的封闭三值，中文取 OperatingBasis 注释原词。索赔调整后口径必须声明所依附的
// 基础口径，属编排层组合，不是第四格。
export const operatingBasisLabels: Record<string, string> = {
  ESTIMATED: '预估',
  CONFIRMED: '已确认',
  SETTLED: '已结算',
};

// 组成项对指标的封闭二向，中文取 ComponentEffect 注释原词。
export const componentEffectLabels: Record<string, string> = {
  INCREASES: '增加',
  DECREASES: '减少',
};

// 组成项在本口径下的采用物身份，中文取 CONTEXT「经营毛利的客户与外部供应商基础必须按阶段
// 成对采用」那句逐口径点名的原词（ADR-0087 决定三）。词表按口径分组不是一个平集：同一个
// 「审核应付」在预估口径下不成立，那个口径采用的是预期成本。
export const componentRoleLabels: Record<string, string> = {
  CUSTOMER_ESTIMATE: '客户预估费用',
  SUPPLIER_EXPECTED_COST: '供应商预期成本',
  CUSTOMER_OPERATING_RECEIVABLE: '客户运营应收',
  AUDITED_PAYABLE: '审核应付',
  SUPPLIER_CREDIT_NOTE: '供应商费用贷项',
  SETTLED_CUSTOMER_RECEIVABLE: '已核销客户运营应收',
  SETTLED_AUDITED_PAYABLE: '已核销审核应付',
  SETTLED_SUPPLIER_CREDIT_NOTE: '已核销供应商费用贷项',
};

// 客户费用的收付方向，中文取 ChargeDirection 注释原词。**是收付不是借贷**：本模块另有
// DEBIT/CREDIT 的借贷方向词表（调整用），两者不是一回事，不共用（ADR-0087 决定一）。
export const chargeDirectionLabels: Record<string, string> = {
  RECEIVABLE: '应收',
  PAYABLE: '应付',
};

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}

/**
 * 可缺席字段的展示：缺席即「未登记」，不留白也不补默认值。留白读起来像渲染掉了东西，
 * 而这一格要说出的是「这一行上还没有这件事」——它是行级缺席的如实呈现，读的人据它去催
 * 那一行的登记。册级缺席（整册不记这件事）不走这里，那种栏直接不设。
 */
export const unregistered = '未登记';

// ——以下为登记签的页面口径（ADR-0085，票 sa-cc/31）。

/**
 * problem+json 错误码的中文说明，与 settlementhttp 的传输层错误码同字。业务判别走 outcome，这几个码
 * 只说明为什么没有 outcome。
 *
 * HANDOFF_NOT_SENT 是本上下文登记口独有的一格：事实的版本行已落，但向 customs-compliance 交的采用信封
 * 因依赖故障没出去。它折成 5xx 而不与已采用同格（判据与受控 CLI 的 fundsAnswer 同一条）：CC 的税费付款核对等的
 * 正是那封，答 2xx 会让操作者把一封永远不会到的信当成已出。续办是重发同一份——Outbox 按认领键吞重，重放交的是
 * 同一封，不会重复采用。这句只对依赖故障成立：信封被框架确定性拒收（引用太长把信封顶过上限）是另一格
 * HANDOFF_ENVELOPE_REJECTED，服务端整笔回滚、什么都没登记，重发同一份永远同一个结果（票 sa-cc/32）。
 */
export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题，不是业务答案。',
  MALFORMED_REQUEST: '请求构造不出命令（登记输入形状不合），重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成，本次没有形成任何业务答案，可稍后重试。',
  NO_ANSWER_FORMED:
    '服务端处理未能完成（资金事实库不可用或事务回滚），本次没有形成任何业务答案，登记与否未知，可稍后重试。',
  UNNAMED_OUTCOME: '用例交回了一个没有名字的答案——实现坏了，不是业务答案；请携带关联标识查询服务端记录。',
  HANDOFF_NOT_SENT:
    '事实已采用（版本行已落），但向 customs-compliance 交的采用信封因依赖故障未出——重发同一份即补发同一封，不会重复采用。',
  HANDOFF_ENVELOPE_REJECTED:
    '采用信封被框架确定性拒收（引用太长把信封顶过上限），本次什么都没登记；要改的是引用的长度或形，重发同一份永远同一个结果。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

/**
 * 两口登记签的标题。动词取用例原词（采用）而不叫「登记」：外部资金事实由外部系统拥有，本上下文对它只形成
 * 引用与待匹配入口（UC-SA-005「采用」），采用时刻取服务端时钟、不是登记输入；更正是采用的一种（同一事实
 * 回指链头的新版本），标题照实说。
 */
export const fundsRegistrationTitles: Record<FundsRegistrationKind, string> = {
  'external-funds-fact': '采用外部资金事实（首版）',
  'external-funds-fact-correction': '采用外部资金事实更正（回指链头）',
};

/**
 * 登记输入形状的提示句。两类只差子命令一词（与端点路径、CLI 子命令同字），所以由一处拼出：逐类抄一遍会让
 * 「不逐字段建表单」这条理由在其中一遍被改动时悄悄分叉。
 */
function snapshotHint(kind: FundsRegistrationKind, fields: string): string {
  return (
    `登记输入 JSON 的形状与受控登记口 parcel-settlement-register ${kind} -input 吃的同一份；` +
    '本页不逐字段建表单，因为「渠道原始载荷 → 登记输入」的翻译属渠道接入契约，随 PAR-INT-01 提供。' +
    fields
  );
}

/**
 * 各类登记输入的形状提示。逐类把键名与封闭集词列出来：未知键一律被译装拒绝（打错的键静默丢弃会让操作员以为
 * 登进去的比实际多），而封闭集里的词打错在族名上看不出来。金额与时刻原样递给领域：非正金额、零时刻、回指自己、
 * 回指非链头都在服务端答`未受理`，页面不代判。来源、账户、币种属实例半边，页面不预填任何真实值。
 */
export const fundsRegistrationSnapshotHints: Record<FundsRegistrationKind, string> = {
  'external-funds-fact': snapshotHint(
    'external-funds-fact',
    '键为 tenantId / factRef / sourceRef / payerRef / kind / currency / amountMinor / version / occurredAt；' +
      '种类取封闭三词 RECEIPT_CONFIRMED / PAYMENT_FAILED / FUNDS_RETURNED，缺席同样拒；' +
      'payerRef 可缺席（来源未提供付款人是事实的一个诚实状态），给了却全是空白则拒；' +
      'amountMinor 是最小单位整数；occurredAt 是外部事实的业务发生时刻，不是登记时刻。' +
      '同事实同版本同内容重发答已存在；同版本换内容、同事实第二个首版都是内容冲突，更正走另一签。',
  ),
  'external-funds-fact-correction': snapshotHint(
    'external-funds-fact-correction',
    '键为 tenantId / factRef / corrects / version / amountMinor / correctedAt；' +
      'corrects 必须等于该事实当前链头的版本，version 是这次更正形成的新版本，只有金额变——' +
      '来源、付款人、种类、币种与发生时刻从链头照抄，载荷带了它们按未知字段拒（其余若也变了那是另一条事实）。' +
      '回指自己、回指非链头、更正一条未采用的事实都答`未受理`；原版本保留，更正形成新有效版本。',
  ),
};
