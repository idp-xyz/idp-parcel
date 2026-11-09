import { afterEach, test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  findSupplierBillReceptions,
  getStatementLines,
  getSupplierBillLines,
  listAuditedPayables,
  listDisputedCustomerStatements,
  listSettlementCharges,
  listSettlementFundsApplications,
  listSettlementOperatingResults,
  listSettlementStatements,
  registryConditions,
} from './api';

// 本文件钉结算各列表读口在 api 层带上的查询参数与答复的 page（票 catalogue-read-pagination-rollout/14；契约 ADR-0144
// 决定一、四、五）：按册分派的三个读口把 registry 当选择器与条件一起拼进查询串，不带条件即迁移前的路径；账单册的精确检索与
// 两个取行端点请求一字不变。

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

const page = { size: 2, next: 'c2', total: 3 };

function captureRequests(): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    urls.push(String(input));
    return new Response(JSON.stringify({ outcome: 'ANY', page }), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  }) as typeof fetch;
  return urls;
}

// Covers: 对账页的账户归属一问与登记账单表单的预期成本候选都只带册名调，路径与迁移前一样。
test('按册分派的三个读口只带册名时路径同迁移前', async () => {
  const urls = captureRequests();
  await listSettlementCharges(registryConditions('customer-charge'));
  await listSettlementCharges(registryConditions('supplier-expected-cost'));
  await listSettlementStatements(registryConditions('customer-statement'));
  await listSettlementStatements(registryConditions('supplier-bill-reception'));
  await listSettlementOperatingResults(registryConditions('operating-result'));
  await listSettlementOperatingResults(registryConditions('cost-allocation'));
  deepEqual(urls, [
    '/settlement-charges?registry=customer-charge',
    '/settlement-charges?registry=supplier-expected-cost',
    '/settlement-statements?registry=customer-statement',
    '/settlement-statements?registry=supplier-bill-reception',
    '/settlement-operating-results?registry=operating-result',
    '/settlement-operating-results?registry=cost-allocation',
  ]);
});

// Covers: 选择器、排序、筛选维、检索词与游标同进一个查询串——选择器在前、筛选维按名、检索词去首尾空白、游标殿后。
test('册名与排序、筛选维、检索词、游标一并下推', async () => {
  const urls = captureRequests();
  await listSettlementCharges(
    registryConditions('customer-charge', { q: ' SYN-PCL-01 ', filters: { stage: ['CONFIRMED'] }, sort: '-formedAt' }),
    'c1',
  );
  await listSettlementStatements(registryConditions('supplier-bill-reception', { q: 'INV-2610', sort: 'period' }), 'c2');
  await listSettlementOperatingResults(registryConditions('operating-result', { filters: { basis: ['SETTLED', 'ESTIMATED'] } }));
  deepEqual(urls, [
    '/settlement-charges?registry=customer-charge&sort=-formedAt&stage=CONFIRMED&q=SYN-PCL-01&after=c1',
    '/settlement-statements?registry=supplier-bill-reception&sort=period&q=INV-2610&after=c2',
    '/settlement-operating-results?registry=operating-result&basis=ESTIMATED&basis=SETTLED',
  ]);
});

// Covers: 争议队列、资金事实、审核应付三个单册读口不收册名：不带参数即裸路径（首页待办数争议张数就这么调），带条件时一并下推。
test('三个单册读口不带参数即裸路径，带条件与游标时一并下推', async () => {
  const urls = captureRequests();
  await listDisputedCustomerStatements();
  await listSettlementFundsApplications();
  await listAuditedPayables();
  await listDisputedCustomerStatements({ q: 'SYN-ST-2610' }, 'c1');
  await listSettlementFundsApplications({ q: 'SYN-FUNDS', filters: { kind: ['RECEIPT_CONFIRMED'] }, sort: '-occurredAt' });
  await listAuditedPayables({ q: 'AP-202611', sort: 'auditedAt' }, 'c3');
  deepEqual(urls, [
    '/settlement-disputed-statements',
    '/settlement-funds-applications',
    '/settlement-audited-payables',
    '/settlement-disputed-statements?q=SYN-ST-2610&after=c1',
    '/settlement-funds-applications?sort=-occurredAt&kind=RECEIPT_CONFIRMED&q=SYN-FUNDS',
    '/settlement-audited-payables?sort=auditedAt&q=AP-202611&after=c3',
  ]);
});

// Covers: 账单册的精确检索（选择器）与对账单行、账单行两个取行端点不翻页，请求与迁移前一字不变。
test('精确检索与两个取行端点请求一字不变', async () => {
  const urls = captureRequests();
  await findSupplierBillReceptions({ claim: 'SYN-PARTY-CARRIER-TRUNK-01/INV-2610-001' });
  await findSupplierBillReceptions({ supplier: 'SYN-PARTY-CARRIER-TRUNK-01', supplierBillNumber: 'INV-2610-001' });
  await getStatementLines('SYN-ST-2610-01');
  await getSupplierBillLines('SYN-CLAIM-01', '2');
  deepEqual(urls, [
    '/settlement-statements?registry=supplier-bill-reception&claim=SYN-PARTY-CARRIER-TRUNK-01%2FINV-2610-001',
    '/settlement-statements?registry=supplier-bill-reception&supplier=SYN-PARTY-CARRIER-TRUNK-01&supplierBillNumber=INV-2610-001',
    '/settlement-statement-lines?statement=SYN-ST-2610-01',
    '/settlement-supplier-bill-lines?claim=SYN-CLAIM-01&version=2',
  ]);
});

// Covers: 答复的 page 原样交出，翻页条据它算页数与下一页。
test('答复的 page 原样交出', async () => {
  captureRequests();
  const result = await listSettlementOperatingResults(registryConditions('cost-allocation'));
  if (result.kind !== 'outcome') throw new Error(`读口答 ${result.kind}`);
  deepEqual(result.body.page, page);
});

// Covers: 读口对集外的键、坏游标答 400 并带一句理由散文——callerProblem 原样带上 detail；服务端没给字符串时不多出这一格。
test('400 带回的理由散文原样进 detail，没给就没有这一格', async () => {
  globalThis.fetch = (async () =>
    new Response(
      JSON.stringify({ error: { code: 'MALFORMED_REQUEST', detail: '游标与本次的排序或筛选不符，请从第一页重取' } }),
      { status: 400 },
    )) as typeof fetch;
  deepEqual(await listSettlementStatements(registryConditions('customer-statement'), 'stale'), {
    kind: 'callerProblem',
    status: 400,
    code: 'MALFORMED_REQUEST',
    detail: '游标与本次的排序或筛选不符，请从第一页重取',
  });

  globalThis.fetch = (async () => new Response(JSON.stringify({ error: { code: 'MALFORMED_REQUEST' } }), { status: 400 })) as typeof fetch;
  const bare = await listAuditedPayables();
  deepEqual(bare, { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' });
  equal('detail' in bare, false);
});
