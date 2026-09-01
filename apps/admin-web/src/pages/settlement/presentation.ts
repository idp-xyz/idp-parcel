// 结算与核算四页的查阅词表。中文一律取 settlement-accounting 领域枚举与用例文档的原词，
// 不自造译法；集外取值由 labelOf 原样回显——坏数据该露出来，不该被译成一句像样的话。

import type { ChargeRegistry, OperatingRegistry, StatementRegistry } from './api';

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
