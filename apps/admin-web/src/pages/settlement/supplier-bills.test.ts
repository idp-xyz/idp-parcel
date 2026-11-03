import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  auditAnswerText,
  billLabel,
  billNumberOf,
  billProgress,
  claimIdFor,
  classificationChoices,
  currentExpectedCosts,
  differingClassificationWords,
  hasReceptionProblems,
  lineDisposition,
  matchSummary,
  newPayableId,
  NO_EXPECTED_COST,
  operatorWords,
  pageCountSummary,
  payableLabel,
  payableNumberAbsenceNote,
  payableNumberName,
  receptionAnswerText,
  receptionPayload,
  receptionProblems,
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
  auditAuthorityConfigured: true,
  auditAuthorityConfiguredNow: true,
  receivedAt: '2026-10-31T08:00:00Z',
  recordedAt: '2026-11-01T08:00:00Z',
  ...overrides,
});

test('主张标识由供应商与账单号生成，取得回账单号；别处送入的主张取不到就照旧显主张标识', () => {
  const claim = claimIdFor('SYN-PARTY-CARRIER-TRUNK-01', '  INV/2610-001 ');
  equal(claim, 'SYN-PARTY-CARRIER-TRUNK-01/INV/2610-001');
  equal(billNumberOf(claim, 'SYN-PARTY-CARRIER-TRUNK-01'), 'INV/2610-001');
  equal(billNumberOf('SYN-BILL-DEMO-1', 'SYN-PARTY-CARRIER-TRUNK-01'), undefined);
  equal(billNumberOf('SYN-PARTY-CARRIER-TRUNK-01/', 'SYN-PARTY-CARRIER-TRUNK-01'), undefined);
  equal(billLabel({ claim: 'SYN-BILL-DEMO-1', supplier: 'SYN-PARTY-CARRIER-TRUNK-01' }), 'SYN-BILL-DEMO-1');
  // 同一家供应商同一个账单号不论谁录都落回同一个主张；两家供应商同号不撞。
  equal(claimIdFor('A', 'INV-1'), claimIdFor('A', 'INV-1'));
  ok(claimIdFor('A', 'INV-1') !== claimIdFor('B', 'INV-1'));
});

test('应付标识页面生成，前缀不是应付单号的 AP-', () => {
  const id = newPayableId(new Date('2026-11-02T03:04:05Z'), 'ab-c9z12345');
  equal(id, 'PAYABLE-20261102-030405-ABC9Z1');
  ok(!id.startsWith('AP-'));
});

test('账单走到哪一步：先说要人动手的那件', () => {
  equal(billProgress(bill()).label, '待审核');
  equal(billProgress(bill({ auditAuthorityConfiguredNow: false })).label, '审核人未配置');
  // 审核人登没登看此刻的审核授权册：接收时没登、后来补登了，就是待审核，不再说未配置；接收时登了、后来册上没有了，照此刻说。
  equal(billProgress(bill({ auditAuthorityConfigured: false, auditAuthorityConfiguredNow: true })).label, '待审核');
  equal(billProgress(bill({ auditAuthorityConfigured: true, auditAuthorityConfiguredNow: false })).label, '审核人未配置');
  equal(billProgress(bill({ matchedWithoutPayableCount: 0 })).label, '有差异待核实');
  const done = billProgress(bill({ matchedWithoutPayableCount: 0, classificationCounts: { MATCHED: 3 } }));
  deepEqual(done, { label: '已审完', tone: 'good' });
  // 接收记录上没有匹配的行也算没对上，不当成审完。
  equal(billProgress(bill({ matchedWithoutPayableCount: 0, classificationCounts: { MATCHED: 2 }, matchCount: 2 })).label, '有差异待核实');
});

test('对上情况：分类经用语词典说中文，不出英文码', () => {
  equal(matchSummary(bill({ classificationCounts: { MATCHED: 3 } })), '3 行全对上');
  const summary = matchSummary(bill({ classificationCounts: { MATCHED: 1, PRICE_VARIANCE: 1, NO_MATCHING_OCCURRENCE: 1 } }));
  equal(summary, '对上 1/3 行 · 找不到对应运输 1 · 价格不符 1');
  equal(/[A-Z_]{4,}/.test(summary), false);
  equal(matchSummary(bill({ lineCount: 2, classificationCounts: { MATCHED: 1 } })), '对上 1/2 行 · 没有匹配记录 1');
});

test('一行能不能审：已对上且还没有应付才摆审核；差异行不摆', () => {
  equal(lineDisposition({ classification: 'MATCHED' }).auditable, true);
  equal(lineDisposition({ classification: 'MATCHED', payable: { payable: 'P', amount: '1', currency: 'CNY', auditor: 'A', auditedAt: 'x' } }).auditable, false);
  equal(lineDisposition({ classification: 'PRICE_VARIANCE' }).auditable, false);
  equal(lineDisposition({}).label, '没有匹配记录');
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

test('能拿来对账的预期成本只列当前那一版、同币种的', () => {
  const costs = [
    cost({ version: 'V1' }),
    cost({ version: 'V2', priorVersion: 'V1', correctionReason: 'R' }),
    cost({ version: 'W1', withdrawn: true }),
    cost({ version: 'U1', settlementCurrency: 'USD' }),
  ];
  deepEqual(
    currentExpectedCosts(costs, 'CNY').map((item) => item.version),
    ['V2'],
  );
  deepEqual(
    currentExpectedCosts(costs, '').map((item) => item.version),
    ['V2', 'U1'],
  );
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

test('接收载荷：主张标识由页面生成、行号从 1 起、已对上不带依据、可缺的格缺就不写', () => {
  const payload = receptionPayload(draft(), costs);
  equal(payload.claim, 'SYN-PARTY-CARRIER-TRUNK-01/INV-2610-001');
  deepEqual(payload.lines, [
    { line: '1', feeItem: 'FEE-TRUNK', claimed: '8500' },
    { line: '2', feeItem: 'FEE-OTHER', claimed: '300' },
  ]);
  deepEqual(payload.directives, [
    { line: '1', classification: 'MATCHED', expectedVersion: 'V1' },
    { line: '2', classification: 'NO_MATCHING_OCCURRENCE', basis: '查过 10 月干线' },
  ]);
  equal('supplementsClaim' in payload, false);
  equal(receptionPayload(draft({ supplementsClaim: 'X/INV-0' }), costs).supplementsClaim, 'X/INV-0');
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
});

test('审核答复说人话：交回应付单号；越权上限、撞号、三种未配置各说各的', () => {
  equal(
    auditAnswerText({ outcome: 'PAYABLE_FORMED', payable: 'PAYABLE-1', documentNumber: 'AP-202611-00001', amount: '8500', currency: 'CNY' }, '1'),
    '第 1 行审核通过，形成应付 AP-202611-00001，金额 8500 CNY。',
  );
  match(
    auditAnswerText({ outcome: 'PAYABLE_FORMED', payable: 'PAYABLE-1', documentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' }, '1'),
    /PAYABLE-1（开立时单据编号时区未配置，无单据号）/,
  );
  match(auditAnswerText({ outcome: 'EXISTING_PAYABLE', documentNumber: 'AP-202611-00001' }, '1'), /已经形成过应付 AP-202611-00001/);
  match(auditAnswerText({ outcome: 'AUDIT_MUST_ESCALATE' }, '2'), /越权审核上限/);
  match(auditAnswerText({ outcome: 'NUMBERING_RULE_COLLISION', documentType: 'AUDITED_PAYABLE' }, '1'), /应付单号与已有的号重复/);
  match(auditAnswerText({ outcome: 'AUDIT_UNDECIDED', undecidedReason: 'AUDIT_AUTHORITY_UNCONFIGURED' }, '1'), /审核授权未配置/);
  match(auditAnswerText({ outcome: 'AUDIT_UNDECIDED', undecidedReason: 'PAYABLE_ACCOUNT_UNCONFIGURED' }, '1'), /应付归集账户/);
  match(auditAnswerText({ outcome: 'AUDIT_UNDECIDED', undecidedReason: 'AUDIT_ESCALATION_UNCONFIGURED' }, '1'), /越权审核上限（未配置）/);
  match(auditAnswerText({ outcome: 'LINE_NOT_AUDITABLE' }, '3'), /先核实差异/);
  match(auditAnswerText({ outcome: 'LINE_NOT_AUDITABLE' }, '3'), new RegExp(differingClassificationWords()));
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
