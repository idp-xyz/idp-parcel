// 送站预报与送站交接单的运营查阅读，与交接单的运营决定确认口（票 ps-external-mark-relations/13；UC-NO-006）。
//
// 形状以 internal/nodeoperations/adapters/http 的 query_delivery_scope_records.go 与 delivery_handover_note.go 为准，此处只做镜像不虚构。
// 读行走 exchangeMasterData（主数据与运营查阅读），确认走 postMasterData（运营决定的送站交接确认一种）：两者不是同一个准入，调用点分开写。

import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

/**
 * 一份送站预报。号码照客户交来的顺序；交付方线索没给即缺席；withdrawn 恒在场，withdrawnAt 只随已撤回出现。来源请求键与内容摘要不回显。
 */
export interface DeliveryPreAdviceRecord {
  preAdviceId: string;
  customerAccount: string;
  node: string;
  expectedArrivalAt: string;
  customerParcelNumbers: string[];
  deliveringParty?: string;
  formedAt: string;
  withdrawn: boolean;
  withdrawnAt?: string;
}

export interface DeliveryPreAdviceListResponseBody {
  outcome: 'DELIVERY_PRE_ADVICES_LISTED';
  preAdvices: DeliveryPreAdviceRecord[];
}

export interface DeliveryPreAdviceFoundResponseBody {
  outcome: 'DELIVERY_PRE_ADVICE_FOUND';
  preAdvice: DeliveryPreAdviceRecord;
}

/**
 * 一个声称的客户账户与它的绑定结论。conclusion 缺席即还没有人作结论（待确认），其余各格随之缺席；deliveringParticipant 与 basis 只随
 * 运营决定的确认出现。conclusion 取 CONFIRMED / DENIED，route 取 CUSTOMER_CHANNEL / OPERATION_DECISION；类型留 string，理由同
 * records-api 的 serviceResultCheck。
 */
export interface HandoverClaimRecord {
  customerAccount: string;
  conclusion?: string;
  route?: string;
  concluder?: string;
  concludedAt?: string;
  deliveringParticipant?: string;
  basis?: string;
}

/** 一张送站交接单。closed 与 scopeEstablished 恒在场；关闭两键随已关闭、scopeEstablishedAt 随范围成立出现。 */
export interface DeliveryHandoverNoteRecord {
  noteId: string;
  node: string;
  deliveringParty: string;
  openedAt: string;
  closed: boolean;
  closureFactId?: string;
  closedAt?: string;
  memberCount: number;
  claims: HandoverClaimRecord[];
  scopeEstablished: boolean;
  scopeEstablishedAt?: string;
}

/** 交接单的一个成员：membership 取 RECEPTION（收寄带来）/ ASSIGNMENT（归入）。 */
export interface HandoverMemberRecord {
  factId: string;
  unit: string;
  membership: string;
  formedAt: string;
}

export interface DeliveryHandoverNoteListResponseBody {
  outcome: 'DELIVERY_HANDOVER_NOTES_LISTED';
  handoverNotes: DeliveryHandoverNoteRecord[];
}

export interface DeliveryHandoverNoteFoundResponseBody {
  outcome: 'DELIVERY_HANDOVER_NOTE_FOUND';
  handoverNote: DeliveryHandoverNoteRecord;
  members: HandoverMemberRecord[];
}

/**
 * 运营决定确认的载荷：交接单、客户账户、交付方参与方（参与方与商业的参与方标识）、依据引用。**没有租户、操作者与时间**——前两格取自操作者
 * 信封、形成时刻取服务端时刻；服务端对载荷里出现的身份键一律按未知键拒。
 */
export interface DeliveryHandoverConfirmationRequest {
  noteId: string;
  customerAccount: string;
  deliveringParticipant: string;
  basis: string;
}

/** 确认口回显的一份结论。deliveringParticipant 与 basis 只随运营决定的确认出现。 */
export interface HandoverBindingRecord {
  customerAccount: string;
  conclusion: string;
  route: string;
  concludedAt: string;
  deliveringParticipant?: string;
  basis?: string;
}

/**
 * 确认口的封闭响应形状。outcome 取编排结果原名（BINDING_CONFIRMED / EXISTING_RESULT / EXISTING_CONCLUSION /
 * NOT_IN_AUTHORIZED_DELIVERY_RELATIONSHIP / AUTHORIZED_DELIVERY_NOT_CONFIGURED，另有 HANDOVER_NOTE_STATE_MISMATCH 带 stateMismatch）；binding
 * 是答案所指的那一份（新落的或在册的）；scopeEstablishedAt 只在交接单此刻已成立范围时出现。
 */
export interface DeliveryHandoverConfirmationResponseBody {
  outcome: string;
  stateMismatch?: string;
  binding?: HandoverBindingRecord;
  scopeEstablishedAt?: string;
}

/** 列表路径由 delivery-pre-advice 的 deliveryScopeListPathOf 拼好交进来：筛选的写法只在那一处。 */
export function listDeliveryPreAdvices(path: string): Promise<ApiResult<DeliveryPreAdviceListResponseBody>> {
  return exchangeMasterData<DeliveryPreAdviceListResponseBody>(path);
}

export function readDeliveryPreAdvice(preAdviceId: string): Promise<ApiResult<DeliveryPreAdviceFoundResponseBody>> {
  return exchangeMasterData<DeliveryPreAdviceFoundResponseBody>(
    `/node-operations-delivery-pre-advices/${encodeURIComponent(preAdviceId)}`,
  );
}

export function listDeliveryHandoverNotes(path: string): Promise<ApiResult<DeliveryHandoverNoteListResponseBody>> {
  return exchangeMasterData<DeliveryHandoverNoteListResponseBody>(path);
}

export function readDeliveryHandoverNote(noteId: string): Promise<ApiResult<DeliveryHandoverNoteFoundResponseBody>> {
  return exchangeMasterData<DeliveryHandoverNoteFoundResponseBody>(
    `/node-operations-delivery-handover-notes/${encodeURIComponent(noteId)}`,
  );
}

export function confirmDeliveryHandover(
  request: DeliveryHandoverConfirmationRequest,
): Promise<ApiResult<DeliveryHandoverConfirmationResponseBody>> {
  return postMasterData<DeliveryHandoverConfirmationResponseBody>('/node-operations/delivery-handover-confirmations', request);
}
