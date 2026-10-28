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

// 阶段、方向、资金事实种类、口径、组成项角色这些封闭集的中文与色调都在 src/vocabulary/status.ts（界面用语词典，一处定义），
// 页面经 labelFor / StatusPill 取，本文件不再另存一份。

/**
 * 资金事实在客户应收作用域上怎么记入入账余额（ADR-0283），给收付款核销页一格。落到几个作用域时要说出未分配停在哪、
 * 怎么续办：收款没核销的那一截哪个作用域都不记，退回的那一截每个作用域都先扣——续办都是把钱核到作用域上，对余额的
 * 方向相反，不能用一句话带过。没落到客户应收作用域的事实（映到应付、贷项或金额本体，或还没映射）不是这一支。
 */
export function scopePostingText(fact: {
  kind: string;
  customerScopes: number;
  unscopedUnappliedAmount: string;
}): string {
  if (fact.customerScopes <= 0) return '不入客户应收作用域';
  if (fact.customerScopes === 1) return '记入 1 个作用域（核到别处的除外）';
  const head = `${fact.customerScopes} 个作用域按核销记入`;
  if (fact.unscopedUnappliedAmount === '0') return head;
  if (fact.kind === 'FUNDS_RETURNED') {
    return `${head}；未分配 ${fact.unscopedUnappliedAmount} 每个作用域都先扣，核到钱离开的那个账户上就只扣那一边`;
  }
  return `${head}；未分配 ${fact.unscopedUnappliedAmount} 未计入任何作用域，核到哪个作用域那里才记`;
}

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
  METHOD_NOT_ALLOWED: '这个入口不接受这种提交方式，请刷新页面后重试；仍然不行请联系系统管理员。',
  MALFORMED_REQUEST: '填写的内容格式不对，系统读不出一笔完整的登记；请按提示检查各项后再提交。',
  INTAKE_FAILED: '登录信息没能核对通过，这次没有登记任何内容；请稍后重试。',
  NO_ANSWER_FORMED: '系统暂时没能处理这笔登记，是否登记成功还不确定；请稍后刷新查看，必要时重新提交同一份。',
  UNNAMED_OUTCOME: '系统返回了无法识别的结果；请记下关联标识，联系系统管理员核查。',
  HANDOFF_NOT_SENT: '这笔收付款已登记，但通知关务那一步没有发出；重新提交同一份即可补发，不会重复登记。',
  HANDOFF_ENVELOPE_REJECTED: '引用太长，这笔登记没有保存；请缩短引用后重新提交。',
  OUTSIDE_ADMISSION_SCOPE: '当前阶段还没有放开这项登记；什么都没有保存，请联系负责阶段管理的同事。',
  IDENTITY_DEPENDENCY_UNAVAILABLE: '暂时无法核对登录身份，请稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? `这次提交没有成功（错误码 ${code}）；请记下关联标识联系系统管理员。`;
}

/**
 * 两口登记签的标题。动词取用例原词（采用）而不叫「登记」：外部资金事实由外部系统拥有，本上下文对它只形成
 * 引用与待匹配入口（UC-SA-005「采用」），采用时刻取服务端时钟、不是登记输入；更正是采用的一种（同一事实
 * 回指链头的新版本），标题照实说。
 */
export const fundsRegistrationTitles: Record<FundsRegistrationKind, string> = {
  'external-funds-fact': '登记一笔收付款',
  'external-funds-fact-correction': '更正一笔收付款的金额',
};

/**
 * 登记输入形状的提示句。两类只差子命令一词（与端点路径、CLI 子命令同字），所以由一处拼出：逐类抄一遍会让
 * 同一句提示在其中一遍被改动时悄悄分叉。暂以 JSON 录入只是逐字段表单还没做（票 demo-chain-gaps/156）。
 */
function snapshotHint(kind: FundsRegistrationKind, fields: string): string {
  return `${fields} 填的是银行或支付系统给的原始记录，与集成接口送来的同一份形状（${kind}）；提交人与所属公司由系统记下，不用填。`;
}

/**
 * 各类登记输入的形状提示。逐类把键名与封闭集词列出来：未知键一律被译装拒绝（打错的键静默丢弃会让操作员以为
 * 登进去的比实际多），而封闭集里的词打错在族名上看不出来。金额与时刻原样递给领域：非正金额、零时刻、回指自己、
 * 回指非链头都在服务端答`未受理`，页面不代判。来源、账户、币种属实例半边，页面不预填任何真实值。
 */
export const fundsRegistrationSnapshotHints: Record<FundsRegistrationKind, string> = {
  'external-funds-fact': snapshotHint(
    'external-funds-fact',
    '请填 factRef（银行流水号或支付单号）、sourceRef（来源系统）、payerRef（付款人，没有可不填）、kind（RECEIPT_CONFIRMED 到账 / ' +
      'PAYMENT_FAILED 付款失败 / FUNDS_RETURNED 资金退回）、currency、amountMinor（金额，按币种最小单位的整数）、version（版本，首次填 v1）、' +
      'occurredAt（银行记录的发生时间，不是现在）。同一笔重复提交不会登两次；金额填错请用「更正金额」。',
  ),
  'external-funds-fact-correction': snapshotHint(
    'external-funds-fact-correction',
    '请填 factRef（要更正的那笔）、corrects（它现在的版本号）、version（这次更正的新版本号）、amountMinor（正确金额）、correctedAt（更正时间）。' +
      '只能改金额；来源、付款人、币种、发生时间要改的话那是另一笔收付款，请另行登记。原来的金额会保留在历史里。',
  ),
};
