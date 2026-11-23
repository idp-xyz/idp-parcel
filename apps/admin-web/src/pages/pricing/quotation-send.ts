/** 发出报价的载荷。不带租户、发出人、时刻或报价号：那些由服务端从信封和时钟取。 */

export type QuotationPriceLane = 'APPROVED_DRAFT' | 'REGISTERED_PRICE_CARD';

export interface QuotationSendDraft {
  lane: QuotationPriceLane;
  planId: string;
  planVersion: string;
  contentDigest: string;
  counterpartyPartyId: string;
  legalEntityId: string;
  validFrom: string;
  validUntil: string;
  terms: string;
}

export interface QuotationSendPayload {
  lane: QuotationPriceLane;
  planId: string;
  planVersion: string;
  contentDigest: string;
  counterpartyPartyId: string;
  legalEntityId: string;
  validFrom?: string;
  validUntil?: string;
  terms: string;
}

export const quotationSendOutcomeLabels: Record<string, string> = {
  SENT: '已发出。送达不在这里记。',
  ALREADY_RECORDED: '这一条发出记录已经在册，没有再插一行。送达不在这里记。',
  CONTENT_CONFLICT: '这个报价号已经发出过不同的内容',
  NOT_REGISTERED: '所引的价格内容不在册',
  NOT_SELL: '所引的价格内容不是销售方向',
  CONTENT_NOT_FIXED: '价格内容还没固定',
  WITHDRAWN: '所引的价卡草稿已撤下',
  ALREADY_REGISTERED: '这一版已经在册，改走价卡路',
  CONTENT_CHANGED: '内容已变，与你看到的摘要不同',
  COUNTERPARTY_NOT_REGISTERED: '相对方不在册',
  COUNTERPARTY_NOT_YET_EFFECTIVE: '相对方还没到生效时点',
  COUNTERPARTY_DEACTIVATED: '相对方已停用',
  LEGAL_ENTITY_NOT_REGISTERED: '责任法人不在册',
  LEGAL_ENTITY_NOT_YET_EFFECTIVE: '责任法人还没到生效时点',
  LEGAL_ENTITY_DEACTIVATED: '责任法人已停用',
  NOT_ACCEPTED: '有效期或条款未受理',
  NOT_REPRESENTABLE: '这份价格内容表达不成对客价格表',
};

export const quotationDeliveryNote = '送达不在这里记。';

/** 日期时间框按写下的钟点当作协调世界时，不另取业务时区。空着就缺席，交给服务端答未受理。 */
export function quotationInstant(local: string): string | undefined {
  const trimmed = local.trim();
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(trimmed)) return `${trimmed}:00Z`;
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(trimmed)) return `${trimmed}Z`;
  return undefined;
}

export function quotationSendPayload(draft: QuotationSendDraft): QuotationSendPayload {
  return {
    lane: draft.lane,
    planId: draft.planId,
    planVersion: draft.planVersion,
    contentDigest: draft.contentDigest,
    counterpartyPartyId: draft.counterpartyPartyId,
    legalEntityId: draft.legalEntityId,
    validFrom: quotationInstant(draft.validFrom),
    validUntil: quotationInstant(draft.validUntil),
    terms: draft.terms,
  };
}
