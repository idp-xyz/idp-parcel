// 价格政策与授权规则对象页列出引用它的价卡。价卡只直接记下授权规则；价格政策页先找出指名这份政策的授权规则，再按那些引用列价卡。

import { catalogueRequestPath, type CatalogueConditions } from '../catalogue-query';
import type { ApiResult } from '../catalogue-api';
import { listPriceCards, type PriceCardRecord } from '../pricing/api';
import {
  commercialPolicyConditions,
  listCommercialPolicyPage,
  type AuthorizationRuleRecord,
  type CommercialPolicyConditions,
} from './api';

export interface CitingPriceCardsBody {
  cards: PriceCardRecord[];
  /** 某一页后面还有，对象页没有把后面的页也拼进来。 */
  truncated: boolean;
}

export function citingPriceCardConditions(authorizationId: string, authorizationVersion: string): CatalogueConditions {
  return { filters: { authorizationId: [authorizationId], authorizationVersion: [authorizationVersion] } };
}

export function citingAuthorizationConditions(pricePolicyId: string): CommercialPolicyConditions {
  return commercialPolicyConditions('AUTHORIZATION_RULE', { filters: { pricePolicy: [pricePolicyId] } });
}

export function citingPriceCardPath(authorizationId: string, authorizationVersion: string): string {
  return catalogueRequestPath('/pricing-price-cards', citingPriceCardConditions(authorizationId, authorizationVersion), null);
}

export function citingAuthorizationPath(pricePolicyId: string): string {
  return catalogueRequestPath('/commercial-policies', citingAuthorizationConditions(pricePolicyId), null);
}

export async function loadCitingPriceCards(
  register: 'PRICE_POLICY' | 'AUTHORIZATION_RULE',
  objectId: string,
  version: string,
): Promise<ApiResult<CitingPriceCardsBody>> {
  if (register === 'AUTHORIZATION_RULE') {
    return cardsForAuthorizations([{ id: objectId, version }], false);
  }
  const listed = await listCommercialPolicyPage(citingAuthorizationConditions(objectId));
  if (listed.kind !== 'outcome') return listed;
  if (listed.body.kind !== 'AUTHORIZATION_RULE') {
    return { kind: 'noAnswer', status: listed.status, code: 'UNEXPECTED_CATALOGUE' };
  }
  return cardsForAuthorizations(
    listed.body.policies.map((row: AuthorizationRuleRecord) => ({ id: row.objectId, version: row.version })),
    listed.body.page.next !== null,
  );
}

async function cardsForAuthorizations(
  authorizations: readonly { id: string; version: string }[],
  truncated: boolean,
): Promise<ApiResult<CitingPriceCardsBody>> {
  const cards: PriceCardRecord[] = [];
  for (const authorization of authorizations) {
    const listed = await listPriceCards(citingPriceCardConditions(authorization.id, authorization.version));
    if (listed.kind !== 'outcome') return listed;
    cards.push(...listed.body.cards);
    if (listed.body.page.next !== null) truncated = true;
  }
  return { kind: 'outcome', status: 200, body: { cards, truncated } };
}
