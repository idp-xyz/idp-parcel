// 一家单位名下的合作关系（票 business-party-redesign/09）：按持有方、按相对方各读一问——读口的 holderId、counterpartyId 两维
// （ADR-0378 决定六）——并成一份。往来单位右栏的概要与「关联」签、对象页的「关联」、「我方关联」的右栏都读它。
//
// 此前是从合作关系册不带条件的那一页里挑这家单位的行（party-overview.ts 的 relationshipsOfParty）：册不止一页时挑不全，只能在
// 页上写「只按已取到的那一页列」。现在读口按这家单位筛，除非它一家在某一侧的关系就超过一页，读到的就是全部。
//
// 这里是并的那一半与读法，纯函数，node:test 钉着；接上取数层的钩子在 use-party-relationships.ts——拆开是因为钩子那一半引着
// 取数层，测试加载不了。

import type { ApiResult } from '../catalogue-api';
import { listPartyRelationships, type PartyRelationshipListResponseBody } from './api';

/**
 * 把两问的答复并成一份。任何一问没答成就交出那一问的答复（先看持有方那一问）：页面照它说读不到，不拿半份冒充全部。
 * 两问都答成时，行并在一起、按关系编号去重（同一段关系不会两侧都是它，去重只是防万一）；`page.next` 两问里只要有一问还有
 * 下一页就不为 null，调用方据此知道没读全（partyRelationshipsComplete）；总数两问都给才相加。
 */
export function mergePartyRelationshipAnswers(
  asHolder: ApiResult<PartyRelationshipListResponseBody>,
  asCounterparty: ApiResult<PartyRelationshipListResponseBody>,
): ApiResult<PartyRelationshipListResponseBody> {
  if (asHolder.kind !== 'outcome') return asHolder;
  if (asCounterparty.kind !== 'outcome') return asCounterparty;
  const seen = new Set<string>();
  const relationships = [...asHolder.body.relationships, ...asCounterparty.body.relationships].filter((row) => {
    if (seen.has(row.relationshipId)) return false;
    seen.add(row.relationshipId);
    return true;
  });
  const first = asHolder.body.page;
  const second = asCounterparty.body.page;
  return {
    kind: 'outcome',
    status: asHolder.status,
    body: {
      outcome: 'PARTY_RELATIONSHIPS_LISTED',
      relationships,
      page: {
        size: first.size,
        next: first.next ?? second.next,
        total: first.total === null || second.total === null ? null : first.total + second.total,
      },
    },
  };
}

/** 并出来的那一份是不是这家单位名下的全部：答成了，两问都没有下一页。还没答出、没答成都不算。 */
export function partyRelationshipsComplete(answer: ApiResult<PartyRelationshipListResponseBody> | null): boolean {
  return answer?.kind === 'outcome' && answer.body.page.next === null;
}

/** 读一家单位名下的合作关系：两问并发，并成一份。 */
export function listRelationshipsOfParty(partyId: string): Promise<ApiResult<PartyRelationshipListResponseBody>> {
  return Promise.all([
    listPartyRelationships({ filters: { holderId: [partyId] } }),
    listPartyRelationships({ filters: { counterpartyId: [partyId] } }),
  ]).then(([asHolder, asCounterparty]) => mergePartyRelationshipAnswers(asHolder, asCounterparty));
}
