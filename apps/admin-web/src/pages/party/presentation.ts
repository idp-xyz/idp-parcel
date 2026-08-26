// 参与方与商业目录查阅词表。kind 取值与传输层封闭集同词;中文取 CONTEXT 原词。

import type { CommercialPolicyKind } from './api';

export const serviceFormLabels: Record<string, string> = {
  NETWORK_SERVICE: '网络服务产品',
  LABEL_CHANNEL_SERVICE: '面单渠道服务',
};

export const commercialStatusLabels: Record<string, string> = {
  DRAFT: '草稿',
  PUBLISHED: '已发布',
  EFFECTIVE: '已生效',
  EXPIRED: '已到期',
  RETIRED: '已退役',
  SUPERSEDED: '已替代',
};

export const policyKindLabels: Record<CommercialPolicyKind, string> = {
  ACCEPTANCE_RULE_PACKAGE: '接单规则包',
  PRE_ACCEPTANCE_CONTROL: '接受前财务控制',
  PRICE_POLICY: '商业价格政策',
  SETTLEMENT_POLICY: '结算政策',
  AS_OF_POLICY: '时点锚声明',
  AUTHORIZATION_RULE: '授权规则',
};

export const commercialPolicyKinds: CommercialPolicyKind[] = [
  'ACCEPTANCE_RULE_PACKAGE',
  'PRE_ACCEPTANCE_CONTROL',
  'PRICE_POLICY',
  'SETTLEMENT_POLICY',
  'AS_OF_POLICY',
  'AUTHORIZATION_RULE',
];

// 商业方向封闭三格(domain CommercialDirection),中文与计价方向同词——同一个方向
// 概念不因出现在不同页而换名。
export const commercialDirectionLabels: Record<string, string> = {
  BUY: '买价',
  SELL: '卖价',
  INTERNAL: '内部',
};

// 结算方式封闭两格(domain SettlementMethod)。
export const settlementMethodLabels: Record<string, string> = {
  PREPAID: '预付',
  TERMS: '账期',
};

// 接受前财务控制要求封闭两格(domain PreAcceptanceControl)。
export const controlRequirementLabels: Record<string, string> = {
  REQUIRED: '要求',
  NOT_APPLICABLE: '不适用',
};

// 收寄来源封闭二值(domain DeclaredIntakeSource),对应 parcel-shipment 来源联合的两格。
export const intakeSourceLabels: Record<string, string> = {
  NODE_INTAKE: '节点收寄',
  OFFSITE_PICKUP: '场外揽收',
};

// 终局责任结果封闭四值(domain 的终局规则声明),中文取 CONTEXT 原词。
export const finalOutcomeLabels: Record<string, string> = {
  EFFECTIVE_DELIVERY: '有效送达',
  RETURN_COMPLETED: '退回完成',
  SERVICE_TERMINATED: '服务终止',
  REGULATORY_DISPOSITION: '监管处置',
};

// 取消请求方封闭二值(domain DeclaredCancellationParty)。
export const cancellationPartyLabels: Record<string, string> = {
  CUSTOMER: '客户',
  OPERATIONS: '运营',
};

export const problemCodeNotes: Record<string, string> = {
  METHOD_NOT_ALLOWED: '请求方法不被该端点允许。这是调用方式问题,不是业务答案。',
  MALFORMED_REQUEST:
    '请求构造不出查询(kind 缺席或不在封闭集),重发同样的内容不会改变结果。',
  INTAKE_FAILED: '接入解析未能完成,本次没有形成任何业务答案,可稍后重试。',
  NO_ANSWER_FORMED: '服务端处理未能完成,本次没有形成任何业务答案,可稍后重试。',
};

export function problemNote(code: string): string {
  return problemCodeNotes[code] ?? '未知错误码。请携带关联标识查询服务端记录。';
}

export function labelOf(table: Record<string, string>, code: string): string {
  return table[code] ?? code;
}
