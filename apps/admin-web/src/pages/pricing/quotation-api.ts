import { exchangeMasterData, exchangeMasterDataFile, postMasterData } from '../catalogue-api';
import type { ApiResult } from '../catalogue-api';
import type { QuotationSendPayload } from './quotation-send';
import type { QuotationReplyPayload } from './quotation-reply';

export const quotationSendRegistrationPath = '/commercial-quotation-send-registrations';
export const quotationSendListPath = '/commercial-quotation-sends';

export interface QuotationSendResponseBody {
  outcome: string;
  conclusion?: string;
  quotationId?: string;
  sentAt?: string;
  deliveryRecorded: boolean;
}

export interface QuotationSendBody {
  quotationId: string;
  lane: string;
  planId: string;
  planVersion: string;
  canonicalization: string;
  contentDigest: string;
  counterpartyPartyId: string;
  counterpartyName: string;
  legalEntityId: string;
  legalEntityName: string;
  validFrom: string;
  validUntil: string;
  terms: string;
  sender: string;
  sentAt: string;
  method: string;
  deliveryRecorded: boolean;
}

export interface QuotationSendListBody {
  outcome: string;
  deliveryNote: string;
  items: QuotationSendBody[];
  next?: { sentAt: string; quotationId: string };
}

export interface QuotationReplyBody {
  quotationId: string;
  decision: string;
  repliedAt: string;
  evidenceIndex: string;
  rejectionReason?: string;
  recordedBy: string;
  recordedAt: string;
}

export interface QuotationReplyResponseBody {
  outcome: string;
  conclusion?: string;
  reply?: QuotationReplyBody;
}

export interface QuotationSendDetailBody {
  outcome: string;
  deliveryNote: string;
  deliveryRecorded: boolean;
  send?: QuotationSendBody;
  reading?: string;
  conclusion?: string;
  referencedDraftWithdrawn?: boolean;
  reply?: QuotationReplyBody;
  priceOnRegister?: boolean;
  scopeNamed?: boolean;
  publicationRequirement?: string;
  effectivenessNextStep?: string;
}

export const quotationReplyRegistrationPath = '/commercial-quotation-reply-registrations';

export function recordQuotationReply(payload: QuotationReplyPayload): Promise<ApiResult<QuotationReplyResponseBody>> {
  return postMasterData<QuotationReplyResponseBody>(quotationReplyRegistrationPath, payload);
}

export function sendQuotation(payload: QuotationSendPayload): Promise<ApiResult<QuotationSendResponseBody>> {
  return postMasterData<QuotationSendResponseBody>(quotationSendRegistrationPath, payload);
}

export function listQuotationSends(cursor?: { sentAt: string; quotationId: string }): Promise<ApiResult<QuotationSendListBody>> {
  const params = new URLSearchParams();
  if (cursor) {
    params.set('sentAt', cursor.sentAt);
    params.set('quotationId', cursor.quotationId);
  }
  const query = params.toString();
  return exchangeMasterData<QuotationSendListBody>(query === '' ? quotationSendListPath : `${quotationSendListPath}?${query}`);
}

export function readQuotationSend(quotationId: string): Promise<ApiResult<QuotationSendDetailBody>> {
  return exchangeMasterData<QuotationSendDetailBody>(`${quotationSendListPath}/${encodeURIComponent(quotationId)}`);
}

export function downloadQuotationWorkbook(quotationId: string) {
  return exchangeMasterDataFile(`${quotationSendListPath}/${encodeURIComponent(quotationId)}/workbook`);
}
