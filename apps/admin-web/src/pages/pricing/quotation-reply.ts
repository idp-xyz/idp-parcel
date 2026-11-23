/** 代录报价答复的载荷。不带租户、代录人或登记时刻：那些由服务端从信封和时钟取。 */

export interface QuotationReplyDraft {
  quotationId: string;
  decision: 'ACCEPTED' | 'REJECTED';
  repliedAt: string;
  evidenceIndex: string;
  rejectionReason: string;
  contentDigest: string;
}

export interface QuotationReplyPayload {
  quotationId: string;
  decision: 'ACCEPTED' | 'REJECTED';
  repliedAt?: string;
  evidenceIndex: string;
  rejectionReason: string;
  contentDigest: string;
}

export const quotationReadingLabels: Record<string, string> = {
  PENDING: '待答复',
  ACCEPTED: '已接受',
  REJECTED: '已拒绝',
  EXPIRED: '已过期',
  REFERENCED_PRICE_WITHDRAWN: '所引价格已撤下',
};

export const quotationReplyOutcomeLabels: Record<string, string> = {
  RECORDED: '已记下。',
  ALREADY_RECORDED: '已经记下，没有再插一行。',
  ALREADY_REPLIED: '这一条已经有答复，没有替换。',
  EXPIRED: '客户答复的时刻不在有效期内，没有记下。',
  WITHDRAWN: '所引的价卡草稿已撤下，没有记下。',
  CONTENT_CHANGED: '内容已变，与发出记录上的摘要不同，没有记下。',
  NOT_ACCEPTED: '证据索引、客户时刻或原因未受理，没有记下。',
  NOT_FOUND: '没有这一条发出记录。',
};

/** 日期时间框按写下的钟点当作协调世界时。空着就缺席，交给服务端答未受理。 */
export function quotationReplyPayload(draft: QuotationReplyDraft): QuotationReplyPayload {
  return {
    quotationId: draft.quotationId,
    decision: draft.decision,
    repliedAt: quotationReplyInstant(draft.repliedAt),
    evidenceIndex: draft.evidenceIndex,
    rejectionReason: draft.decision === 'ACCEPTED' ? '' : draft.rejectionReason,
    contentDigest: draft.contentDigest,
  };
}

function quotationReplyInstant(local: string): string | undefined {
  const trimmed = local.trim();
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(trimmed)) return `${trimmed}:00Z`;
  if (/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/.test(trimmed)) return `${trimmed}Z`;
  return undefined;
}
