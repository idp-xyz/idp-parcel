// 发布前给人读的「将要发布」那几项（票 party-commercial-uiux/11）。
//
// 预览这一步此前只显示一串内容摘要，人看不出要发的是什么；「发布」又不能撤回、改内容要换版本号。这里从表单此刻组出的
// 那份载荷里取版本信息——对象、版本号、适用范围、生效起止——在预览通过后与发布确认框里复述。正文各册形状不同、表单格已在
// 眼前，不在这里重复；摘要仍在，给要对日志的人。
//
// 生效起止给两份：操作者显示时区的墙钟与 UTC 原值，跨 CN/SG 两地对时不必换算。
// 纯函数，node:test 直接跑。

import { formatInstant } from '../moment';
import type { CommercialObjectKindName, CommercialPublicationPayload } from './publication-draft-api';

/** 发布类别的中文名，与各页、各册的叫法同词（价格规则的结果显示在「商业价格政策」册）。 */
export const publicationKindLabels: Record<CommercialObjectKindName, string> = {
  SERVICE_PRODUCT: '服务产品',
  CUSTOMER_CONTRACT: '客户合同',
  SUPPLIER_AGREEMENT: '供应商协议',
  ACCEPTANCE_RULE_PACKAGE: '接单规则包',
  PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY: '接受前财务控制策略',
  PRICE_RULE: '商业价格政策',
  SETTLEMENT_POLICY: '结算政策',
  CREDIT_POLICY: '信用政策',
  AUTHORIZATION_RULE: '授权规则',
  CUSTOMER_SERVICE_RULE: '客户服务规则',
};

export interface PublicationFact {
  label: string;
  value: string;
}

export function momentText(value: string, timeZone: string): string {
  return `${formatInstant(value, { timeZone })}（UTC ${value}）`;
}

/** 版本壳：载荷与读回的载体都有这几格，「将要发布」的复述两处共用。 */
export type PublicationShell = Pick<
  CommercialPublicationPayload,
  'kind' | 'objectId' | 'version' | 'scope' | 'effectiveStartsAt' | 'effectiveEndsAt'
>;

export function publicationSummary(payload: PublicationShell, timeZone: string): PublicationFact[] {
  return [
    { label: '类别', value: publicationKindLabels[payload.kind] ?? payload.kind },
    { label: '对象', value: payload.objectId },
    { label: '版本号', value: payload.version },
    { label: '适用范围', value: payload.scope },
    { label: '生效起点', value: momentText(payload.effectiveStartsAt, timeZone) },
    {
      label: '生效止点',
      value: payload.effectiveEndsAt ? momentText(payload.effectiveEndsAt, timeZone) : '持续有效',
    },
  ];
}
