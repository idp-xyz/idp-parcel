// 对账单页右栏两张争议表单的判读（ADR-0354）：客服照客户来电或来信代登记一项争议，财务核实后处理。纯函数、只引类型与界面用语
// 词典，node:test 直接跑；取数与表单在 ReconciliationPage.tsx。
//
// 话照「说人话」写：答复的业务判别由服务端给（outcome），这里只把它译成一句操作者看得懂的话，不另判业务。

import { documentNumberAbsenceNote, documentTypeWords, numberingCollisionNote } from '../../vocabulary/document-numbering';
import type { CustomerStatementRecord, DisputeDecisionResponseBody, StatementDisputeRecord, StatementLinesResponseBody } from './api';

/**
 * 一项争议在页面上的主标识：争议号（ADR-0355 决定十一）。没有争议号的（上线前开立，或开立时单据编号时区未配置）照旧显示争议标识，
 * 为什么没有由 disputeNumberAbsenceNote 在技术详情里注明。
 */
export function disputeLabel(dispute: Pick<StatementDisputeRecord, 'dispute' | 'documentNumber'>): string {
  return dispute.documentNumber ?? dispute.dispute;
}

/** 没有争议号时技术详情里注明的那一句，按争议上记下的原因说，不一律写成「上线前开立」；有号时没有这一句。 */
export function disputeNumberAbsenceNote(dispute: Pick<StatementDisputeRecord, 'documentNumber' | 'documentNumberAbsence'>): string | undefined {
  if (dispute.documentNumber || !dispute.documentNumberAbsence) return undefined;
  return documentNumberAbsenceNote(dispute.documentNumberAbsence);
}

/** 争议号在界面上的叫法，取单据编号词典那一份（ADR-0355 决定十一），不在各页另写。 */
export const disputeNumberName = documentTypeWords.STATEMENT_DISPUTE.numberName;

/** 一张单上可以被提出异议的一笔费用：费用号、金额（与单上同一写法）与币种。 */
export interface StatementChargeOption {
  charge: string;
  amount: string;
  currency: string;
}

/** 这张单上的费用：取单自己的费用行（不可覆盖快照），不从按页上列的费用册反查——页外的费用与作废单上的行在那里都找不到。 */
export function chargesOnStatement(lines: StatementLinesResponseBody): StatementChargeOption[] {
  if (lines.outcome !== 'STATEMENT_LINES_LISTED') return [];
  return (lines.lines ?? []).map((line) => ({ charge: line.charge, amount: line.amount, currency: lines.currency ?? '' }));
}

/**
 * 提出时刻的本地检查：不晚于现在（登记时刻）、不早于发布、不晚于作废（ADR-0354 决定一、三）。作废之前提出、作废之后才登记的照收；
 * 服务端照样会判，登记时刻以服务端此刻为准。几个时刻都是 RFC 3339；读不出的不在这里拦。
 */
export function openedAtProblem(
  openedAt: string,
  publishedAt: string | undefined,
  voidedAt: string | undefined,
  now: number = Date.now(),
): string | null {
  const at = Date.parse(openedAt);
  if (Number.isNaN(at)) return null;
  if (at > now) return '客户提出的时间不能晚于现在：先有客户提出，才有这次登记';
  if (publishedAt && at < Date.parse(publishedAt)) return '客户提出的时间早于这张单发出，说的不是这张单';
  if (voidedAt && at > Date.parse(voidedAt)) return '这张单已作废，作废之后提出的异议不再登记在这张单上';
  return null;
}

/** 还在等人处理的争议：没有处理结果，或处理为待复核（待复核可再处理，其余三种是终局）。 */
export function openDisputesOf(statement: Pick<CustomerStatementRecord, 'disputes'>): StatementDisputeRecord[] {
  return statement.disputes.filter((dispute) => !dispute.resolution || dispute.resolution === 'PENDING_REVIEW');
}

/**
 * 争议标识由页面在第一次提交前定下：同一张表单因网络重发，落回的是同一项（服务端答「已登记过」），而不是又开一项。
 * 时间到秒加一段随机串，同一秒两个人各开一项也撞不上。前缀不与任何单据号默认格式的前缀相同：争议号默认是 `DSP-` 起头，
 * 内部标识也用它，技术详情里两串就难分。已登记的旧标识不动。
 */
export function newDisputeId(now: Date, random: string): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  const stamp = `${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}-${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}`;
  return `DISPUTE-${stamp}-${random.replace(/[^A-Za-z0-9]/g, '').slice(0, 6).toUpperCase()}`;
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

/** 答复里那一项争议怎么称呼：有争议号说号；没有的照旧说争议标识，并带上为什么没号。 */
function answeredDispute(body: DisputeDecisionResponseBody): string {
  const label = body.documentNumber ?? body.dispute ?? '';
  const note = disputeNumberAbsenceNote(body);
  return note ? `${label}（${note}）` : label;
}

/** 登记口答复的那一句。 */
export function registrationAnswerText(body: DisputeDecisionResponseBody): string {
  switch (body.outcome) {
    case 'DISPUTE_OPENED':
      return `已登记争议 ${answeredDispute(body)}，等财务核实处理。`;
    case 'EXISTING_DISPUTE':
      return `这一项已经登记过了（${answeredDispute(body)}），没有重复登记。`;
    case 'DISPUTE_CONFLICT':
      return '这个争议编号已经登记过另一份内容；刷新表单重新填一次。';
    case 'STATEMENT_VOIDED_TAKES_NO_DISPUTE':
      return '没有登记：这张单已作废，作废之后提出的异议不再登记在这张单上。';
    case 'SOURCE_NOT_ACCEPTED':
      return '没有登记：对账单、费用、金额或时间对不上——费用要在这张单上，金额不超过这笔费用，客户提出的时间不早于单子发出、也不晚于现在。';
    case 'NUMBERING_RULE_COLLISION':
      return numberingCollisionNote(body.documentType);
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
