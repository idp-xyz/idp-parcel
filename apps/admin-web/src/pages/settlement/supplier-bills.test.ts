import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  billKey,
  billLabel,
  billNumberOf,
  billProgress,
  claimIdForBill,
  claimOfBillKey,
  classificationChoices,
  currentExpectedCosts,
  differingClassificationWords,
  hasReceptionProblems,
  legacyBillNumberOf,
  linesForSupplier,
  lineDisposition,
  matchSummary,
  NO_EXPECTED_COST,
  operatorWords,
  pageCountSummary,
  payableLabel,
  payableNumberAbsenceNote,
  payableNumberName,
  receptionAnswerText,
  receptionPayload,
  receptionProblems,
  unrecordedSupplierCount,
  withExactHits,
  type ReceptionDraft,
} from './supplier-bills';
import type { AuditedPayableRecord, SupplierBillReceptionRecord, SupplierExpectedCostRecord } from './api';

const bill = (overrides: Partial<SupplierBillReceptionRecord> = {}): SupplierBillReceptionRecord => ({
  claim: 'SYN-PARTY-CARRIER-TRUNK-01/INV-2610-001',
  claimVersion: '1',
  supplier: 'SYN-PARTY-CARRIER-TRUNK-01',
  legalEntity: 'SYN-LE-01',
  period: '2026-10',
  currency: 'CNY',
  lineCount: 3,
  matchCount: 3,
  totalClaimedAmount: '3300',
  classificationCounts: { MATCHED: 2, PRICE_VARIANCE: 1 },
  payableLineCount: 1,
  payableAmount: '1000',
  matchedWithoutPayableCount: 1,
  rejectedAmount: '0',
  openDisputeAmount: '0',
  undecidedAmount: '2300',
  auditAuthorityConfigured: true,
  auditAuthorityConfiguredNow: true,
  receivedAt: '2026-10-31T08:00:00Z',
  recordedAt: '2026-11-01T08:00:00Z',
  ...overrides,
});

test('账单号：先取服务端账单号册上的；旧行（主张标识形如「供应商/账单号」）从主张标识取回，取不到照旧显主张标识', () => {
  equal(billNumberOf({ claim: 'BILL-20261102-1', supplier: 'S', supplierBillNumber: 'INV-9' }), 'INV-9');
  equal(billNumberOf({ claim: 'SYN-PARTY-CARRIER-TRUNK-01/INV/2610-001', supplier: 'SYN-PARTY-CARRIER-TRUNK-01' }), 'INV/2610-001');
  equal(legacyBillNumberOf('SYN-BILL-DEMO-1', 'SYN-PARTY-CARRIER-TRUNK-01'), undefined);
  equal(legacyBillNumberOf('SYN-PARTY-CARRIER-TRUNK-01/', 'SYN-PARTY-CARRIER-TRUNK-01'), undefined);
  equal(billLabel({ claim: 'SYN-BILL-DEMO-1', supplier: 'SYN-PARTY-CARRIER-TRUNK-01' }), 'SYN-BILL-DEMO-1');
});

test('主张标识：同一家供应商同一个账单号已在册上就沿用那份主张（新号旧号都认），否则用新生成的；两家供应商同号不混', () => {
  const rows = [
    { claim: 'BILL-OLD-1', supplier: 'A', supplierBillNumber: 'INV-1' },
    { claim: 'A/INV-2', supplier: 'A' },
  ];
  equal(claimIdForBill(rows, 'A', ' INV-1 ', 'BILL-NEW'), 'BILL-OLD-1');
  equal(claimIdForBill(rows, 'A', 'INV-2', 'BILL-NEW'), 'A/INV-2');
  equal(claimIdForBill(rows, 'B', 'INV-1', 'BILL-NEW'), 'BILL-NEW');
  equal(claimIdForBill(rows, 'A', 'INV-3', 'BILL-NEW'), 'BILL-NEW');
});

test('账单走到哪一步：先说要人动手的那件', () => {
  equal(billProgress(bill()).label, '待审核');
  equal(billProgress(bill({ auditAuthorityConfiguredNow: false })).label, '审核人未配置');
  // 审核人登没登看此刻的审核授权册：接收时没登、后来补登了，就是待审核，不再说未配置；接收时登了、后来册上没有了，照此刻说。
  equal(billProgress(bill({ auditAuthorityConfigured: false, auditAuthorityConfiguredNow: true })).label, '待审核');
  equal(billProgress(bill({ auditAuthorityConfigured: true, auditAuthorityConfiguredNow: false })).label, '审核人未配置');
  // 都给过结果但还有争议没解；争议解完（或一段争议都没有）即已审完，拒绝的那几段也算审完。
  equal(billProgress(bill({ undecidedAmount: '0', openDisputeAmount: '500' })).label, '有争议待再审');
  deepEqual(billProgress(bill({ undecidedAmount: '0', openDisputeAmount: '0', rejectedAmount: '300' })), { label: '已审完', tone: 'good' });
});

test('对上情况：分类经用语词典说中文，不出英文码', () => {
  equal(matchSummary(bill({ classificationCounts: { MATCHED: 3 } })), '3 行全对上');
  const summary = matchSummary(bill({ classificationCounts: { MATCHED: 1, PRICE_VARIANCE: 1, NO_MATCHING_OCCURRENCE: 1 } }));
  equal(summary, '对上 1/3 行 · 找不到对应运输 1 · 价格不符 1');
  equal(/[A-Z_]{4,}/.test(summary), false);
  equal(matchSummary(bill({ lineCount: 2, classificationCounts: { MATCHED: 1 } })), '对上 1/2 行 · 没有匹配记录 1');
});

test('一行走到哪一步：还有没审的金额就摆审核；只剩争议说待再审；都给过结果说已审完', () => {
  deepEqual(lineDisposition({ undecidedAmount: '300', openDisputeAmount: '0' }), { label: '待审核', tone: 'warn', auditable: true });
  deepEqual(lineDisposition({ undecidedAmount: '0', openDisputeAmount: '200' }), { label: '有争议待再审', tone: 'warn', auditable: false });
  equal(lineDisposition({ undecidedAmount: '0', openDisputeAmount: '0' }).label, '已审完');
});

test('应付以应付单号为主标识；没号照旧显应付标识，按记下的原因说', () => {
  equal(payableNumberName, '应付单号');
  equal(payableLabel({ payable: 'PAYABLE-1', documentNumber: 'AP-202611-00001' }), 'AP-202611-00001');
  equal(payableLabel({ payable: 'PAYABLE-1' }), 'PAYABLE-1');
  equal(payableNumberAbsenceNote({ documentNumber: 'AP-202611-00001' }), undefined);
  equal(payableNumberAbsenceNote({ documentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' }), '开立时单据编号时区未配置，无单据号');
  equal(payableNumberAbsenceNote({ documentNumberAbsence: 'ISSUED_BEFORE_NUMBERING' }), '上线前开立，无单据号');
});

test('操作证据说短名；缺席说不记提交方', () => {
  equal(operatorWords('OPERATOR/http://127.0.0.1:5556/dex#syn-operator-config-01'), 'syn-operator-con…');
  equal(operatorWords(undefined), '不记提交方');
});

const cost = (overrides: Partial<SupplierExpectedCostRecord>): SupplierExpectedCostRecord => ({
  version: 'V1',
  occurrence: 'TCO-1',
  occurrenceReason: 'ACTUAL_FULFILLMENT',
  occurrenceVersion: 'V1',
  occurredAt: '2026-10-30T00:00:00Z',
  feeItem: 'FEE-TRUNK',
  purchaseRuleVersion: 'PLAN@v1',
  agreement: 'AGR/v3',
  evaluation: 'EVAL-1',
  originalCurrency: 'CNY',
  originalAmount: '8500',
  settlementCurrency: 'CNY',
  settlementAmount: '8500',
  recordedAt: '2026-10-30T00:00:00Z',
  ...overrides,
});

test('能拿来对账的预期成本只列所选供应商的、当前那一版、同币种的', () => {
  const costs = [
    cost({ version: 'V1', supplier: 'TRUNK' }),
    cost({ version: 'V2', priorVersion: 'V1', correctionReason: 'R', supplier: 'TRUNK' }),
    cost({ version: 'W1', withdrawn: true, supplier: 'TRUNK' }),
    cost({ version: 'U1', settlementCurrency: 'USD', supplier: 'TRUNK' }),
    cost({ version: 'L1', supplier: 'LM' }),
    cost({ version: 'OLD' }),
  ];
  deepEqual(
    currentExpectedCosts(costs, 'CNY', 'TRUNK').map((item) => item.version),
    ['V2'],
  );
  deepEqual(
    currentExpectedCosts(costs, '', 'TRUNK').map((item) => item.version),
    ['V2', 'U1'],
  );
  deepEqual(
    currentExpectedCosts(costs, 'CNY', 'LM').map((item) => item.version),
    ['L1'],
  );
  // 没选供应商一份都不列；没记供应商的旧行哪一家都不列，只计数。
  deepEqual(currentExpectedCosts(costs, 'CNY', ''), []);
  equal(unrecordedSupplierCount(costs, 'CNY'), 1);
  equal(unrecordedSupplierCount(costs, 'USD'), 0);
});

test('换了供应商，各行选的别家预期成本连同对上方式清掉，找不到对应运输与本家的留着', () => {
  const line = (expected: string, classification: string) => ({ expected, classification, claimed: '8500', feeItem: '', basis: '' });
  const lines = [line('V2', 'MATCHED'), line('L1', 'MATCHED'), line(NO_EXPECTED_COST, 'NO_MATCHING_OCCURRENCE'), line('', '')];
  deepEqual(linesForSupplier(lines, [cost({ version: 'V2', supplier: 'TRUNK' })]), [
    line('V2', 'MATCHED'),
    line('', ''),
    line(NO_EXPECTED_COST, 'NO_MATCHING_OCCURRENCE'),
    line('', ''),
  ]);
});

test('对上方式照匹配门的形状给：金额相等才能选已对上，没有预期成本只能选找不到对应运输', () => {
  deepEqual(classificationChoices('V1', '8500', '8500').map((option) => option.value), ['MATCHED', 'DUPLICATE_BILLING']);
  deepEqual(classificationChoices('V1', '9000', '8500').map((option) => option.value), ['QUANTITY_VARIANCE', 'PRICE_VARIANCE', 'DUPLICATE_BILLING']);
  deepEqual(classificationChoices(NO_EXPECTED_COST, '9000', undefined).map((option) => option.value), ['NO_MATCHING_OCCURRENCE']);
  deepEqual(classificationChoices('', '9000', undefined), []);
  equal(classificationChoices('V1', '8500', '8500')[0].label, '已对上');
});

const draft = (overrides: Partial<ReceptionDraft> = {}): ReceptionDraft => ({
  supplier: 'SYN-PARTY-CARRIER-TRUNK-01',
  legalEntity: 'SYN-LE-01',
  billNumber: 'INV-2610-001',
  version: '1',
  period: '2026-10',
  currency: 'CNY',
  receivedAt: '2026-11-01T08:00:00Z',
  supplementsClaim: '',
  lines: [
    { expected: 'V1', classification: 'MATCHED', claimed: '8500', feeItem: '', basis: '' },
    { expected: NO_EXPECTED_COST, classification: 'NO_MATCHING_OCCURRENCE', claimed: '300', feeItem: 'FEE-OTHER', basis: '查过 10 月干线' },
  ],
  ...overrides,
});

const costs = new Map([['V1', { feeItem: 'FEE-TRUNK', settlementAmount: '8500' }]]);

test('登记表单的本地检查：缺格、金额写法、对上方式与依据', () => {
  const now = Date.parse('2026-11-02T00:00:00Z');
  equal(hasReceptionProblems(receptionProblems(draft(), costs, now)), false);
  const blank = receptionProblems(draft({ supplier: '', billNumber: ' ', receivedAt: '2026-11-03T00:00:00Z', lines: [] }), costs, now);
  ok(blank.fields.supplier && blank.fields.billNumber && blank.fields.receivedAt && blank.fields.lines);
  const lines = receptionProblems(
    draft({
      lines: [
        { expected: 'V1', classification: 'MATCHED', claimed: '9000', feeItem: '', basis: '' },
        { expected: 'V1', classification: 'PRICE_VARIANCE', claimed: '12.5', feeItem: '', basis: ' ' },
        { expected: '', classification: '', claimed: '1', feeItem: '', basis: '' },
        { expected: NO_EXPECTED_COST, classification: 'NO_MATCHING_OCCURRENCE', claimed: '1', feeItem: '', basis: '查过' },
      ],
    }),
    costs,
    now,
  ).lines;
  ok(lines[0].classification, '金额不等却选已对上要拦');
  ok(lines[1].claimed && lines[1].basis, '金额写坏、差异不带依据要拦');
  ok(lines[2].expected);
  ok(lines[3].feeItem, '找不到对应运输要人填费用项目');
});

test('接收载荷：主张标识由调用方交进来、账单号单放一格、行号从 1 起、已对上不带依据、可缺的格缺就不写', () => {
  const payload = receptionPayload(draft(), costs, 'BILL-20261102-030405-ABC');
  equal(payload.claim, 'BILL-20261102-030405-ABC');
  equal(payload.supplierBillNumber, 'INV-2610-001');
  deepEqual(payload.lines, [
    { line: '1', feeItem: 'FEE-TRUNK', claimed: '8500' },
    { line: '2', feeItem: 'FEE-OTHER', claimed: '300' },
  ]);
  deepEqual(payload.directives, [
    { line: '1', classification: 'MATCHED', expectedVersion: 'V1' },
    { line: '2', classification: 'NO_MATCHING_OCCURRENCE', basis: '查过 10 月干线' },
  ]);
  equal('supplementsClaim' in payload, false);
  equal(receptionPayload(draft({ supplementsClaim: 'X/INV-0' }), costs, 'BILL-1').supplementsClaim, 'X/INV-0');
});

test('接收答复说人话：受理、审核人未配置、重复、版本冲突、未受理、未配置、预期成本不是当前版', () => {
  equal(receptionAnswerText({ outcome: 'BILL_RECEIVED' }, 'INV-1'), '已登记账单 INV-1，对上的行可以在右栏审核。');
  match(receptionAnswerText({ outcome: 'BILL_RECEIVED', auditAuthority: 'UNCONFIGURED' }, 'INV-1'), /还没有登记审核人/);
  match(receptionAnswerText({ outcome: 'EXISTING_RESULT' }, 'INV-1'), /已经登记过了/);
  match(receptionAnswerText({ outcome: 'VERSION_CONFLICT' }, 'INV-1'), /版本加一/);
  match(receptionAnswerText({ outcome: 'SOURCE_NOT_ACCEPTED' }, 'INV-1'), /没有登记/);
  match(receptionAnswerText({ outcome: 'BILL_UNDECIDED', undecidedReason: 'CONVERSION_UNCONFIGURED' }, 'INV-1'), /换算依据.*未配置/);
  equal(
    receptionAnswerText({ outcome: 'EXPECTED_COST_NOT_CURRENT', notCurrent: { line: '2', version: 'V1', withdrawn: false, successor: 'V2' } }, 'INV-1'),
    '没有登记：第 2 行选的预期成本 V1 已经被更正过（现在是 V2），改选当前那一版再登记。',
  );
  match(receptionAnswerText({ outcome: 'SUPPLIER_BILL_NUMBER_CONFLICT', registeredClaim: 'BILL-OLD' }, 'INV-1'), /账单号 INV-1 已经登记过/);
  match(
    receptionAnswerText({ outcome: 'SUPPLIER_BILL_NUMBER_CONFLICT', registeredSupplierBillNumber: 'INV-1', registeredSupplier: 'SUP-A' }, 'INV-1', () => '甲公司'),
    /以甲公司的账单号 INV-1 登记过/,
  );
});

test('接收答复说人话：选了别家供应商的预期成本、选了没记供应商的预期成本（ADR-0358）', () => {
  const names: Record<string, string> = { 'SYN-PARTY-CARRIER-LM-01': '合成末端承运方' };
  const supplierName = (id: string) => names[id] ?? id;
  equal(
    receptionAnswerText(
      { outcome: 'EXPECTED_COST_SUPPLIER_MISMATCH', expectedCostSupplier: { line: '1', version: 'L1', supplier: 'SYN-PARTY-CARRIER-LM-01' } },
      'INV-1',
      supplierName,
    ),
    '没有登记：第 1 行选的预期成本 L1 是合成末端承运方的，不是这张账单的供应商的。改选这家的预期成本，或改成「找不到对应运输」。',
  );
  // 取不到名称时退回标识。
  match(
    receptionAnswerText(
      { outcome: 'EXPECTED_COST_SUPPLIER_MISMATCH', expectedCostSupplier: { line: '1', version: 'L1', supplier: 'SYN-PARTY-UNKNOWN' } },
      'INV-1',
      supplierName,
    ),
    /是SYN-PARTY-UNKNOWN的/,
  );
  match(receptionAnswerText({ outcome: 'EXPECTED_COST_SUPPLIER_MISMATCH' }, 'INV-1'), /别家供应商/);
  match(
    receptionAnswerText({ outcome: 'EXPECTED_COST_SUPPLIER_UNRECORDED', expectedCostSupplier: { line: '2', version: 'OLD' } }, 'INV-1'),
    /第 2 行选的预期成本 OLD 还没记是哪家供应商的.*补记/,
  );
  equal(/[A-Z_]{8,}/.test(receptionAnswerText({ outcome: 'EXPECTED_COST_SUPPLIER_UNRECORDED' }, 'INV-1')), false);
});

test('对不上的分类从取值表取界面词：不含已对上，不出英文码', () => {
  const words = differingClassificationWords();
  ok(words.includes('价格不符') && words.includes('找不到对应运输'));
  equal(words.includes('已对上'), false);
  equal(/[A-Z_]{4,}/.test(words), false);
});

test('两本册截了页就不说「共几份」，只说列出了最近的几份、指标不算页外的', () => {
  equal(pageCountSummary(3, 2, false, '份', '收到'), '共 3 份，当前显示 2 份');
  const cut = pageCountSummary(200, 12, true, '份', '收到');
  match(cut, /只列出最近收到的 200 份/);
  match(cut, /指标也不算它们/);
  equal(cut.includes('共 '), false);
});

test('应付签的检索池：页上那些加上按应付单号在全册里检索到的页外那一笔，同一笔不重复', () => {
  const payable = (id: string, number?: string): AuditedPayableRecord => ({
    payable: id,
    ...(number ? { documentNumber: number } : {}),
    claim: 'C',
    line: '1',
    expectedVersion: 'V1',
    supplier: 'S',
    period: '2026-10',
    legalEntity: 'LE',
    account: 'ACC',
    currency: 'CNY',
    amount: '1',
    auditor: 'A',
    auditedAt: '2026-10-31T08:00:00Z',
    recordedAt: '2026-10-31T08:00:00Z',
  });
  const page = [payable('P-1', 'AP-202611-00002'), payable('P-2', 'AP-202611-00001')];
  deepEqual(
    withExactHits(page, [payable('P-OLD', 'AP-202501-00001')]).map((row) => row.payable),
    ['P-1', 'P-2', 'P-OLD'],
  );
  deepEqual(
    withExactHits(page, [payable('P-1', 'AP-202611-00002')]).map((row) => row.payable),
    ['P-1', 'P-2'],
  );
});

test('选中的账单不在页上时按主张读回：从 billKey 取回主张身份，主张标识里带斜杠也取得对', () => {
  const key = billKey({ claim: 'SYN-PARTY-1/INV/2610-001', claimVersion: '2' });
  equal(claimOfBillKey(key), 'SYN-PARTY-1/INV/2610-001');
});
