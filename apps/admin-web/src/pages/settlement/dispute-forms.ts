// 对账单页右栏两张争议表单的判读（ADR-0353）：客服照客户来电或来信代登记一项争议，财务核实后处理。纯函数、只做类型导入，
// node:test 直接跑；取数与表单在 ReconciliationPage.tsx。
//
// 话照「说人话」写：答复的业务判别由服务端给（outcome），这里只把它译成一句操作者看得懂的话，不另判业务。

import type { CustomerChargeRecord, CustomerStatementRecord, DisputeDecisionResponseBody, StatementDisputeRecord } from './api';

/** 一张单上可以被提出异议的一笔费用：费用号、金额（与单上同一写法）与币种。 */
export interface StatementChargeOption {
  charge: string;
  amount: string;
  currency: string;
  feeItem: string;
}

/** 这张单上的费用：按客户费用读口上的「所在对账单」认，读口没透出的费用不猜。 */
export function chargesOnStatement(charges: readonly CustomerChargeRecord[], statementNumber: string): StatementChargeOption[] {
  return charges
    .filter((charge) => charge.billedOn === statementNumber)
    .map((charge) => ({
      charge: charge.charge,
      amount: charge.settlementAmount,
      currency: charge.settlementCurrency,
      feeItem: charge.feeItem,
    }));
}

/** 还在等人处理的争议：没有处理结果，或处理为待复核（待复核可再处理，其余三种是终局）。 */
export function openDisputesOf(statement: Pick<CustomerStatementRecord, 'disputes'>): StatementDisputeRecord[] {
  return statement.disputes.filter((dispute) => !dispute.resolution || dispute.resolution === 'PENDING_REVIEW');
}

/**
 * 争议标识由页面在第一次提交前定下：同一张表单因网络重发，落回的是同一项（服务端答「已登记过」），而不是又开一项。
 * 时间到秒加一段随机串，同一秒两个人各开一项也撞不上。
 */
export function newDisputeId(now: Date, random: string): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  const stamp = `${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}-${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}`;
  return `DSP-${stamp}-${random.replace(/[^A-Za-z0-9]/g, '').slice(0, 6).toUpperCase()}`;
}

/** 争议金额的本地检查：与单上金额同一写法的正整数，且不超过这笔费用。服务端照样会判，这里只让人少跑一趟。 */
export function amountProblem(raw: string, chargeAmount: string | undefined): string | null {
  const value = raw.trim();
  if (value === '') return '填上客户不认的金额';
  if (!/^[1-9]\d*$/.test(value)) return '金额写成不带小数点、不带符号的整数，与单上金额同一写法';
  if (chargeAmount !== undefined && /^\d+$/.test(chargeAmount) && BigInt(value) > BigInt(chargeAmount)) {
    return `不能超过这笔费用的金额 ${chargeAmount}`;
  }
  return null;
}

/** 文字格（理由、处理依据）的本地检查，上限与服务端相同。 */
export function textProblem(raw: string, what: string): string | null {
  if (raw.trim() === '') return `写一句${what}`;
  if ([...raw].length > 1000) return `${what}写不下就附证据另存，这里至多 1000 字`;
  return null;
}

/** datetime-local 输入框的值（本地时间，到分）→ RFC 3339（UTC，到秒）。读不出来答 null。 */
export function localInputToInstant(value: string): string | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return null;
  const at = new Date(value);
  if (Number.isNaN(at.getTime())) return null;
  return at.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

/** 把一个时刻写成 datetime-local 输入框要的本地时间（到分）。 */
export function instantToLocalInput(at: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}T${pad(at.getHours())}:${pad(at.getMinutes())}`;
}

export const resolutionOptions: { value: 'ACCEPTED' | 'PARTIALLY_ACCEPTED' | 'REJECTED' | 'PENDING_REVIEW'; label: string; hint: string }[] = [
  { value: 'ACCEPTED', label: '接受', hint: '客户说得对；金额由费用那边另做调整，这里不改单' },
  { value: 'PARTIALLY_ACCEPTED', label: '部分接受', hint: '客户说对了一部分；调整同样由费用那边另做' },
  { value: 'REJECTED', label: '拒绝', hint: '原费用照收，理由写在依据里' },
  { value: 'PENDING_REVIEW', label: '待复核', hint: '还要再查；之后可以再处理' },
];

const resolutionWord: Record<string, string> = {
  ACCEPTED: '接受',
  PARTIALLY_ACCEPTED: '部分接受',
  REJECTED: '拒绝',
  PENDING_REVIEW: '待复核',
};

/** 登记口答复的那一句。 */
export function registrationAnswerText(body: DisputeDecisionResponseBody): string {
  switch (body.outcome) {
    case 'DISPUTE_OPENED':
      return `已登记争议 ${body.dispute ?? ''}，等财务核实处理。`;
    case 'EXISTING_DISPUTE':
      return `这一项已经登记过了（${body.dispute ?? ''}），没有重复登记。`;
    case 'DISPUTE_CONFLICT':
      return '这个争议编号已经登记过另一份内容；刷新表单重新填一次。';
    case 'STATEMENT_VOIDED_TAKES_NO_DISPUTE':
      return '这张单已经作废，不再收新的争议；请对着替代单登记。';
    case 'SOURCE_NOT_ACCEPTED':
      return '没有登记：对账单、费用、金额或时间对不上——费用要在这张单上，金额不超过这笔费用，客户提出的时间不早于单子发出。';
    default:
      return `服务端答 ${body.outcome}`;
  }
}

/** 处理口答复的那一句。 */
export function resolutionAnswerText(body: DisputeDecisionResponseBody): string {
  const word = body.resolution ? (resolutionWord[body.resolution] ?? body.resolution) : '';
  switch (body.outcome) {
    case 'DISPUTE_RESOLVED':
      return body.resolution === 'PENDING_REVIEW' ? '已记为待复核，查清后可以再处理。' : `已处理：${word}。`;
    case 'DISPUTE_ALREADY_RESOLVED':
      return `这一项已经处理完了（${word}），不能再改。`;
    case 'SOURCE_NOT_ACCEPTED':
      return '没有处理：找不到这一项争议，或处理时间早于客户提出的时间。';
    default:
      return `服务端答 ${body.outcome}`;
  }
}
