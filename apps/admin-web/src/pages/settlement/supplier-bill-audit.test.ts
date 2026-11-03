import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  auditDraftProblems,
  billNumberConflictText,
  claimSupplierConflictText,
  rangedAuditAnswerText,
  firstAuditRow,
  freeRanges,
  hasAuditDraftProblems,
  layoutRanges,
  newAuditDecisionId,
  newClaimId,
  openDisputes,
  passHeadroom,
  rangedAuditPayloads,
  totalOf,
  undecidedRanges,
  type AuditRowDraft,
  type LineAuditDecision,
} from './supplier-bill-audit';

const decision = (id: string, verdict: LineAuditDecision['verdict'], from: number, to: number, resolvesDispute?: string): LineAuditDecision => ({
  decision: id,
  verdict,
  rangeFrom: String(from),
  rangeTo: String(to),
  ...(resolvesDispute ? { resolvesDispute } : {}),
});

const range = (from: number, to: number) => ({ from: BigInt(from), to: BigInt(to) });

test('还没审的各段：主张金额扣掉直接给的各段，再审不另占', () => {
  const decisions = [decision('p', 'PASSED', 0, 4000), decision('d', 'DISPUTED', 6000, 8000), decision('r', 'PASSED', 6000, 7000, 'd')];
  deepEqual(undecidedRanges('8500', decisions), [range(4000, 6000), range(8000, 8500)]);
  equal(totalOf(undecidedRanges('8500', decisions)), 2500n);
  deepEqual(undecidedRanges('8500', []), [range(0, 8500)]);
  deepEqual(freeRanges(range(0, 10), [range(0, 10)]), []);
});

test('还没解完的争议：扣掉点名它的再审，覆盖完的不再列', () => {
  const decisions = [
    decision('d1', 'DISPUTED', 0, 3000),
    decision('d2', 'DISPUTED', 3000, 5000),
    decision('re1', 'PASSED', 0, 1000, 'd1'),
    decision('re2', 'REJECTED', 3000, 5000, 'd2'),
  ];
  const open = openDisputes(decisions);
  equal(open.length, 1);
  equal(open[0].dispute.decision, 'd1');
  deepEqual(open[0].open, [range(1000, 3000)]);
});

test('还能通过多少：已匹配金额减已通过，不小于零', () => {
  equal(passHeadroom('8000', [decision('p', 'PASSED', 0, 5000), decision('d', 'DISPUTED', 5000, 8500)]), 3000n);
  equal(passHeadroom('0', []), 0n);
  equal(passHeadroom('100', [decision('p', 'PASSED', 0, 200)]), 0n);
});

test('一串金额依次排进空当，每段连续，排不下答 null', () => {
  deepEqual(layoutRanges([range(0, 8500)], [5000n, 3000n, 500n]), [range(0, 5000), range(5000, 8000), range(8000, 8500)]);
  deepEqual(layoutRanges([range(100, 200), range(500, 1000)], [300n, 50n]), [range(500, 800), range(100, 150)]);
  equal(layoutRanges([range(0, 100), range(200, 300)], [150n]), null);
  equal(layoutRanges([range(0, 100)], [0n]), null);
});

const row = (verdict: AuditRowDraft['verdict'], amount: string, basis = ''): AuditRowDraft => ({ verdict, amount, basis, decision: `AUDIT-${amount}` });

test('表单本地检查：争议与拒绝要依据，合计不超还没审的，通过不超还能通过的；再审只能通过或拒绝且都要依据', () => {
  const direct = { free: [range(0, 8500)], headroom: 8000n, resolving: false };
  ok(!hasAuditDraftProblems(auditDraftProblems([row('PASSED', '8000'), row('DISPUTED', '500', '超出协议价')], direct)));

  const noBasis = auditDraftProblems([row('REJECTED', '500')], direct);
  match(noBasis.rows[0].basis ?? '', /为什么拒绝/);
  match(auditDraftProblems([row('PASSED', '12.5')], direct).rows[0].amount ?? '', /正整数/);
  match(auditDraftProblems([row('PASSED', '8000'), row('DISPUTED', '600', 'x')], direct).overall ?? '', /还没审的 8500/);
  match(auditDraftProblems([row('PASSED', '8500')], direct).overall ?? '', /最多还能通过 8000/);
  match(auditDraftProblems([], direct).overall ?? '', /至少/);

  const resolving = { free: [range(5000, 8000)], headroom: 3000n, resolving: true };
  match(auditDraftProblems([row('DISPUTED', '100', 'x')], resolving).rows[0].verdict ?? '', /只能通过或拒绝/);
  match(auditDraftProblems([row('PASSED', '100')], resolving).rows[0].basis ?? '', /解除争议/);
  ok(!hasAuditDraftProblems(auditDraftProblems([row('PASSED', '2000', '补来了过磅单'), row('REJECTED', '1000', '超重不认')], resolving)));
});

test('表单开出来的第一段：能通过先放通过，通过不了放争议，再审放通过', () => {
  deepEqual(firstAuditRow({ free: [range(0, 8500)], headroom: 8000n, resolving: false }, 'A'), { verdict: 'PASSED', amount: '8000', basis: '', decision: 'A' });
  deepEqual(firstAuditRow({ free: [range(0, 300)], headroom: 0n, resolving: false }, 'B'), { verdict: 'DISPUTED', amount: '300', basis: '', decision: 'B' });
  deepEqual(firstAuditRow({ free: [range(5000, 8000)], headroom: 9000n, resolving: true }, 'C'), { verdict: 'PASSED', amount: '3000', basis: '', decision: 'C' });
});

test('载荷：一段一份，依据只在写了时带，再审带所解的争议', () => {
  const payloads = rangedAuditPayloads(
    { claim: 'BILL-1', version: '1', line: '2', resolvesDispute: 'AUDIT-D' },
    [row('PASSED', '2000', ' 补来了过磅单 '), row('REJECTED', '1000', '超重不认')],
    [range(5000, 7000), range(7000, 8000)],
  );
  deepEqual(payloads, [
    { claim: 'BILL-1', version: '1', line: '2', decision: 'AUDIT-2000', verdict: 'PASSED', rangeFrom: '5000', rangeTo: '7000', basis: '补来了过磅单', resolvesDispute: 'AUDIT-D' },
    { claim: 'BILL-1', version: '1', line: '2', decision: 'AUDIT-1000', verdict: 'REJECTED', rangeFrom: '7000', rangeTo: '8000', basis: '超重不认', resolvesDispute: 'AUDIT-D' },
  ]);
  const direct = rangedAuditPayloads({ claim: 'BILL-1', version: '1', line: '1' }, [row('PASSED', '10')], [range(0, 10)]);
  ok(!('basis' in direct[0]) && !('resolvesDispute' in direct[0]));
});

test('标识：主张标识与审核决定标识都由页面生成，不含账单号，前缀避开应付单号', () => {
  const now = new Date(Date.UTC(2026, 10, 2, 3, 4, 5));
  equal(newClaimId(now, 'a1-b2c3d4e5'), 'BILL-20261102-030405-A1B2C3');
  equal(newAuditDecisionId(now, 'zz'), 'AUDIT-20261102-030405-ZZ');
});

test('审核答复说人话：三种结果、超出已匹配、范围已裁决、未配置各说各的', () => {
  const money = (amount: string, currency: string) => `${amount}/${currency}`;
  match(
    rangedAuditAnswerText({ outcome: 'PAYABLE_FORMED', documentNumber: 'AP-202611-00001', amount: '5000', currency: 'CNY' }, '第 1 行 ', 'CNY', money),
    /第 1 行 审核通过，形成应付 AP-202611-00001，金额 5000\/CNY/,
  );
  match(rangedAuditAnswerText({ outcome: 'PAYABLE_FORMED', payable: 'AUDIT-1', documentNumberAbsence: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED' }, '', 'CNY'), /AUDIT-1（/);
  match(rangedAuditAnswerText({ outcome: 'DISPUTE_RECORDED' }, '', 'CNY'), /不形成应付、也不算拒绝/);
  match(rangedAuditAnswerText({ outcome: 'REJECTION_RECORDED' }, '', 'CNY'), /追加账单/);
  match(rangedAuditAnswerText({ outcome: 'LINE_NOT_AUDITABLE', matchedAmount: '8000', passedAmount: '5000' }, '', 'CNY', money), /最多只能通过 8000\/CNY，已通过 5000\/CNY/);
  match(rangedAuditAnswerText({ outcome: 'RANGE_ALREADY_DECIDED' }, '', 'CNY'), /已经有审核结果/);
  match(rangedAuditAnswerText({ outcome: 'AUDIT_UNDECIDED', undecidedReason: 'AUDIT_AUTHORITY_UNCONFIGURED' }, '', 'CNY'), /审核授权未配置/);
  match(rangedAuditAnswerText({ outcome: 'NUMBERING_RULE_COLLISION', documentType: 'AUDITED_PAYABLE' }, '', 'CNY'), /应付单号/);
});

test('账单号冲突说清撞的是哪一边', () => {
  match(billNumberConflictText({ registeredClaim: 'BILL-1' }, ' INV-1 ', (claim) => `主张 ${claim}`), /账单号 INV-1 已经登记过（主张 BILL-1）/);
  match(billNumberConflictText({ registeredSupplierBillNumber: 'INV-0' }, 'INV-1'), /已经以账单号 INV-0 登记过/);
  // 新版本换了供应商：说出原先那家供应商，带不带号都是这一句。
  match(claimSupplierConflictText({ registeredSupplier: 'SUP-A' }, (id) => `${id}公司`), /之前登记的版本是SUP-A公司的，同一份账单的新版本不能换供应商/);
});
