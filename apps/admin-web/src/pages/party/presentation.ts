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
};

export const commercialPolicyKinds: CommercialPolicyKind[] = [
  'ACCEPTANCE_RULE_PACKAGE',
  'PRE_ACCEPTANCE_CONTROL',
  'PRICE_POLICY',
  'SETTLEMENT_POLICY',
  'AS_OF_POLICY',
];

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
