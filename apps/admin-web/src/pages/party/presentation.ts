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

// 参与方身份生命周期封闭三格(domain IdentityStatus)。与 commercialStatusLabels 分表:
// 身份状态按时点导出,商业对象状态是发布生命周期,同词 EFFECTIVE 在两套代数里含义
// 不同,并表会让一套的封闭性替另一套背书。
export const identityStatusLabels: Record<string, string> = {
  REGISTERED: '已登记',
  EFFECTIVE: '已生效',
  DEACTIVATED: '已停用',
};

// 参与方关系生命周期封闭五格(domain RelationshipStatus),中文取 CONTEXT 原词。
export const relationshipStatusLabels: Record<string, string> = {
  CANDIDATE: '候选关系',
  EFFECTIVE: '已生效',
  EXPIRED: '已到期',
  REVOKED: '已撤销',
  SUPERSEDED: '已替代',
};

// 参与方角色封闭五格(domain PartyRole),中文取 CONTEXT 原词。
export const partyRoleLabels: Record<string, string> = {
  CUSTOMER: '客户',
  SUPPLIER: '供应商',
  CARRIER_AGENT: '承运商代理',
  RESELLER: '转售',
  ACCOUNT_HOLDER: '渠道账号持有',
};

// 法人册对象类型今天只有一格(传输层 kindResponsibleLegalEntity):经营组织没有
// 登记面,如实不上列,不预开空格。
export const legalEntityKindLabels: Record<string, string> = {
  RESPONSIBLE_LEGAL_ENTITY: '责任法人',
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
