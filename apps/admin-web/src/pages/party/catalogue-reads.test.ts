import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  commercialPolicyConditions,
  listBusinessParties,
  listCommercialPolicies,
  listCommercialPolicyPage,
  listCustomerContracts,
  listGroupLegalEntities,
  listPartyRelationships,
  listProductChannelMappings,
  listRegistrationNumberTypes,
  listServiceProducts,
  listSupplierAgreements,
} from './api';
import { listOpenPublicationDrafts } from './publication-draft-api';
import { listIdentityRegistrationNumberTypes, registrationNumberTypeCandidateConditions } from './registration-number-type-candidates';
import { businessPartyListConditions } from './business-party-list';
import { legalEntityListConditions } from './legal-entity-list';
import { partyRelationshipListConditions } from './party-relationship-list';
import { pendingListConditions } from './pending-publications';
import { policyListConditions } from './policy-rows';
import { versionListConditions } from './version-groups';
import type { CatalogueConditions } from '../catalogue-query';

// 本文件钉参与方各列表读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144 决定一、
// 三、四、五；各册的可排维与筛选维以后端 partycommercial ports 包的 *Pages 声明为准）：不带参数即裸路径——表单候选、名称转写这些
// 不翻页的调用方照旧拿第一页；带条件时经 catalogueRequestPath 拼成同一个查询串，游标殿后；策略目录的 kind 是册子选择器。客户账户
// 那一册由 customer-account-read.test.ts 钉。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 2, next: 'c2', total: 3 };

function answerWith(status: number, body: unknown): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
  return urls;
}

const captureRequests = () => answerWith(200, { outcome: 'ANY', page });

type Read = (conditions?: CatalogueConditions, after?: string | null) => Promise<unknown>;

// Covers: 九个目录读口各自裸路径照旧，带检索词与游标时一并下推（检索词去首尾空白）。
test('参与方各列表读口不带参数即裸路径，带检索词与游标时一并下推', async () => {
  const reads: Array<[string, Read]> = [
    ['/commercial-business-parties', listBusinessParties],
    ['/commercial-party-relationships', listPartyRelationships],
    ['/commercial-group-legal-entities', listGroupLegalEntities],
    ['/commercial-registration-number-types', listRegistrationNumberTypes],
    ['/commercial-service-products', listServiceProducts],
    ['/commercial-customer-contracts', listCustomerContracts],
    ['/commercial-supplier-agreements', listSupplierAgreements],
    ['/commercial-product-channel-mappings', listProductChannelMappings],
    ['/commercial-open-publication-drafts', listOpenPublicationDrafts],
  ];
  for (const [path, read] of reads) {
    const urls = captureRequests();
    await read();
    await read({ q: ' SYN-PARTY ' }, 'c1');
    deepEqual(urls, [path, `${path}?q=SYN-PARTY&after=c1`], path);
  }
});

// Covers: 身份三册的页面条件——状态、类别、注册地、角色按参数名下推，排序只带读口点了名的维、键按字典序；页面缺省序与读口缺省序相同的
// 两册不带 sort，关系册的生效起倒序总带上。
test('身份三册按页面条件拼出查询串', async () => {
  const urls = captureRequests();
  await listBusinessParties(businessPartyListConditions('承运', 'EFFECTIVE', 'CUSTOMER', { key: 'effective-from', dir: -1 }), 'c1');
  await listGroupLegalEntities(legalEntityListConditions('', 'REGISTERED', 'CN', { key: 'registered-at', dir: -1 }));
  await listPartyRelationships(partyRelationshipListConditions('', 'CUSTOMER', 'CANDIDATE', { key: 'effective-start', dir: -1 }));
  deepEqual(urls, [
    '/commercial-business-parties?sort=-effectiveFrom&categories=CUSTOMER&status=EFFECTIVE&q=%E6%89%BF%E8%BF%90&after=c1',
    '/commercial-group-legal-entities?registrationCountry=CN&status=REGISTERED',
    '/commercial-party-relationships?sort=-effectiveStartsAt&role=CUSTOMER&status=CANDIDATE',
  ]);
});

// Covers: 版本壳各册按对象号排、「在用」下推成 status；策略目录的 kind 作选择器排在最前，声明册按所挂靠的对象号排，没有状态维的册
// 不带 status（带了读口答 400）。按一种取第一页的无参调用只带 kind，与迁移前同一个地址。
test('版本壳各册与策略目录按页面条件拼出查询串', async () => {
  const urls = captureRequests();
  await listSupplierAgreements(versionListConditions('', 'in-use'));
  await listCommercialPolicyPage(commercialPolicyConditions('AUTHORIZATION_RULE', policyListConditions('AUTHORIZATION_RULE', 'SYN', 'other')), 'c1');
  await listCommercialPolicyPage(commercialPolicyConditions('PRE_ACCEPTANCE_CONTROL', policyListConditions('PRE_ACCEPTANCE_CONTROL', '', 'in-use')));
  await listCommercialPolicyPage(commercialPolicyConditions('AS_OF_POLICY', policyListConditions('AS_OF_POLICY', '', 'all')));
  await listCommercialPolicies('PRICE_POLICY');
  deepEqual(urls, [
    '/commercial-supplier-agreements?sort=objectId&status=EFFECTIVE',
    '/commercial-policies?kind=AUTHORIZATION_RULE&sort=objectId&status=EXPIRED&status=PUBLISHED&status=RETIRED&status=SUPERSEDED&q=SYN&after=c1',
    '/commercial-policies?kind=PRE_ACCEPTANCE_CONTROL&sort=contractObjectId',
    '/commercial-policies?kind=AS_OF_POLICY&sort=rulePackageObjectId',
    '/commercial-policies?kind=PRICE_POLICY',
  ]);
});

// Covers: 待批准清单的两格状态下推，读不出不下推；注册号类型候选按层与国家（去首尾空白）收窄，国家候选只按身份层。
test('待批准清单与注册号类型候选按条件拼出查询串', async () => {
  const urls = captureRequests();
  await listOpenPublicationDrafts(pendingListConditions('', 'APPROVED'));
  await listOpenPublicationDrafts(pendingListConditions('', 'UNREADABLE'));
  await listRegistrationNumberTypes(registrationNumberTypeCandidateConditions('PROFILE', ' SG '));
  await listIdentityRegistrationNumberTypes();
  deepEqual(urls, [
    '/commercial-open-publication-drafts?status=APPROVED',
    '/commercial-open-publication-drafts',
    '/commercial-registration-number-types?countryCode=SG&layer=PROFILE',
    '/commercial-registration-number-types?layer=IDENTITY',
  ]);
});

// Covers: 答复的 page 原样交给页面（next 与 total 不改写），集合键照旧。
test('答复的 page 原样交出', async () => {
  answerWith(200, { outcome: 'BUSINESS_PARTIES_LISTED', parties: [], page });
  const answer = await listBusinessParties({ q: 'SYN' });
  equal(answer.kind, 'outcome');
  if (answer.kind === 'outcome') {
    deepEqual(answer.body.page, page);
    deepEqual(answer.body.parties, []);
  }
});

// Covers: 400 带理由散文时进 detail，页面经 catalogueViewState 原样接在问题码说明之后；不给就没有这一格。
test('400 的理由散文进 detail', async () => {
  answerWith(400, { error: { code: 'MALFORMED_REQUEST', detail: '游标与本次的排序或筛选不符，请从第一页重取' } });
  const refused = await listPartyRelationships({ q: 'SYN' }, 'stale');
  equal(refused.kind, 'callerProblem');
  if (refused.kind === 'callerProblem') equal(refused.detail, '游标与本次的排序或筛选不符，请从第一页重取');
  answerWith(400, { error: { code: 'MALFORMED_REQUEST' } });
  const bare = await listGroupLegalEntities({ q: 'SYN' });
  deepEqual(bare, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
});
