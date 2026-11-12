import { useAnswer, type AnswerQuery } from '../../query/answers';
import type { PartyRelationshipListResponseBody } from './api';
import { listRelationshipsOfParty } from './party-relationships-of';
import { partyAnswersDomain } from './party-writes';

/**
 * 一家单位名下的合作关系（party-relationships-of.ts），经取数层读：同一家单位在右栏、对象页与「我方关联」是同一份缓存。
 * 键在参与方域下——参与方的写口答到后整域作废（party-writes.ts），登记或结束一段关系之后这里跟着重读，不必各处点名。
 */
export function usePartyRelationships(partyId: string): AnswerQuery<PartyRelationshipListResponseBody> {
  return useAnswer([partyAnswersDomain, 'relationships-of-party', partyId], () => listRelationshipsOfParty(partyId));
}
