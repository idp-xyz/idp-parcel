// 供应商账单按金额范围审核的判读（ADR-0357 决定二至五；票 document-numbering/13）：一行上还有哪几段没审、争议还剩哪几段没解、
// 表单里一串金额怎么排成范围、本地拦哪些错、载荷与答复那一句。纯函数，node:test 直接跑；取数与表单在 SupplierBillPanel.tsx。
//
// 范围按最小币单位的整数、左闭右开记位置（服务端的 [rangeFrom, rangeTo)）。人只填金额，页面把它们从这一行（或这段争议）里第一处
// 还空着的地方起依次排开——位置只让「不重叠」「再审只落在原争议之内」在结构上成立，不给人多一件要想的事。金额全程用 bigint，不经
// 浮点；服务端照样判范围、已匹配金额与上限，这里只拦人能当场改的错。

/** 审核结果的三种字面（领域 AuditVerdict）。 */
export type AuditVerdict = 'PASSED' | 'DISPUTED' | 'REJECTED';

export const verdictLabels: Record<AuditVerdict, string> = {
  PASSED: '通过',
  DISPUTED: '争议',
  REJECTED: '拒绝',
};

/** 一行上的一份审核决定，读口逐字给（GET /settlement-supplier-bill-lines 各行的 decisions）。 */
export interface LineAuditDecision {
  decision: string;
  verdict: AuditVerdict;
  rangeFrom: string;
  rangeTo: string;
  resolvesDispute?: string;
}

export interface MinorRange {
  from: bigint;
  to: bigint;
}

const spanOf = (decision: Pick<LineAuditDecision, 'rangeFrom' | 'rangeTo'>): MinorRange => ({
  from: BigInt(decision.rangeFrom),
  to: BigInt(decision.rangeTo),
});

/** outer 里扣掉 taken 之后还空着的各段，按位置排。 */
export function freeRanges(outer: MinorRange, taken: readonly MinorRange[]): MinorRange[] {
  const sorted = [...taken].sort((left, right) => (left.from < right.from ? -1 : left.from > right.from ? 1 : 0));
  const free: MinorRange[] = [];
  let cursor = outer.from;
  for (const range of sorted) {
    const from = range.from > outer.from ? range.from : outer.from;
    const to = range.to < outer.to ? range.to : outer.to;
    if (to <= from) continue;
    if (from > cursor) free.push({ from: cursor, to: from });
    if (to > cursor) cursor = to;
  }
  if (cursor < outer.to) free.push({ from: cursor, to: outer.to });
  return free;
}

export const totalOf = (ranges: readonly MinorRange[]): bigint => ranges.reduce((sum, range) => sum + (range.to - range.from), 0n);

/** 一行上还没有任何结果的各段：主张金额里扣掉直接给的各段（再审落在争议里，不另占）。 */
export function undecidedRanges(claimedAmount: string, decisions: readonly LineAuditDecision[]): MinorRange[] {
  return freeRanges(
    { from: 0n, to: BigInt(claimedAmount) },
    decisions.filter((decision) => !decision.resolvesDispute).map(spanOf),
  );
}

export interface OpenDispute {
  dispute: LineAuditDecision;
  /** 这段争议里还没被再审覆盖的各段。 */
  open: MinorRange[];
}

/** 还没解完的争议：直接给的争议里扣掉点名它的各份再审，剩下的不空就还在争议中。 */
export function openDisputes(decisions: readonly LineAuditDecision[]): OpenDispute[] {
  return decisions
    .filter((decision) => decision.verdict === 'DISPUTED' && !decision.resolvesDispute)
    .map((dispute) => ({
      dispute,
      open: freeRanges(
        spanOf(dispute),
        decisions.filter((decision) => decision.resolvesDispute === dispute.decision).map(spanOf),
      ),
    }))
    .filter((entry) => entry.open.length > 0);
}

/** 这一行还能通过多少：已匹配金额减去已通过的（不小于零）。 */
export function passHeadroom(matchedAmount: string, decisions: readonly LineAuditDecision[]): bigint {
  const passed = decisions
    .filter((decision) => decision.verdict === 'PASSED')
    .reduce((sum, decision) => sum + (BigInt(decision.rangeTo) - BigInt(decision.rangeFrom)), 0n);
  const headroom = BigInt(matchedAmount) - passed;
  return headroom > 0n ? headroom : 0n;
}

/**
 * 把一串金额依次排进还空着的各段：每个金额整段落在一处空当里（一份决定是连续的一段），从最前面一处排得下的空当、紧接着上一段
 * 往后排。排不下（金额比哪一处剩下的都大）答 null，由表单说「超过了还没审的那一段」。
 */
export function layoutRanges(free: readonly MinorRange[], amounts: readonly bigint[]): MinorRange[] | null {
  const remaining = free.map((range) => ({ ...range }));
  const laid: MinorRange[] = [];
  for (const amount of amounts) {
    if (amount <= 0n) return null;
    const slot = remaining.find((range) => range.to - range.from >= amount);
    if (!slot) return null;
    laid.push({ from: slot.from, to: slot.from + amount });
    slot.from += amount;
  }
  return laid;
}

// —— 表单 ——

export interface AuditRowDraft {
  verdict: AuditVerdict;
  /** 最小币单位的正整数串，与本页其余金额同一写法。 */
  amount: string;
  basis: string;
  /** 审核决定标识：这一行草稿第一次提交前定下，重发落回同一份（ADR-0355 决定十）。 */
  decision: string;
}

export interface AuditDraftContext {
  /** 能排的那几段：直接审时是这一行还没审的各段，再审时是这段争议还没解的各段。 */
  free: readonly MinorRange[];
  /** 还能通过多少（passHeadroom）。 */
  headroom: bigint;
  /** 再审：只能通过或拒绝，都要带依据。 */
  resolving: boolean;
}

export interface AuditDraftProblems {
  rows: Record<'verdict' | 'amount' | 'basis', string | null>[];
  /** 整张表单的那一句：一行都没有、合计超出、通过超出。 */
  overall: string | null;
}

const positiveMinor = /^[1-9]\d*$/;

export function auditDraftProblems(rows: readonly AuditRowDraft[], context: AuditDraftContext): AuditDraftProblems {
  const problems: AuditDraftProblems = {
    rows: rows.map((row) => ({
      verdict: context.resolving && row.verdict === 'DISPUTED' ? '再审只能通过或拒绝' : null,
      amount:
        row.amount.trim() === ''
          ? '填这一段的金额'
          : !positiveMinor.test(row.amount.trim())
            ? '金额写成不带小数点、不带符号的正整数，按最小币单位（例如 12.34 元写 1234）'
            : null,
      basis:
        (row.verdict !== 'PASSED' || context.resolving) && row.basis.trim() === ''
          ? row.verdict === 'PASSED'
            ? '写一句为什么解除争议、凭什么通过'
            : row.verdict === 'DISPUTED'
              ? '写一句争的是什么、凭什么'
              : '写一句为什么拒绝'
          : [...row.basis].length > 1000
            ? '依据写不下就附证据另存，这里至多 1000 字'
            : null,
    })),
    overall: null,
  };
  if (rows.length === 0) {
    problems.overall = '至少给一段结果';
    return problems;
  }
  if (problems.rows.some((row) => row.amount !== null)) return problems;
  const amounts = rows.map((row) => BigInt(row.amount.trim()));
  const total = amounts.reduce((sum, amount) => sum + amount, 0n);
  const available = totalOf(context.free);
  const passing = rows.reduce((sum, row, index) => (row.verdict === 'PASSED' ? sum + amounts[index] : sum), 0n);
  if (total > available) {
    problems.overall = context.resolving
      ? `几段合计 ${total}，超过了这段争议还没解的 ${available}`
      : `几段合计 ${total}，超过了这一行还没审的 ${available}`;
  } else if (passing > context.headroom) {
    problems.overall = `通过的合计 ${passing}，这一行最多还能通过 ${context.headroom}；超出的部分和预期成本对不上，只能争议或拒绝`;
  } else if (layoutRanges(context.free, amounts) === null) {
    problems.overall = '有一段排不进还空着的地方（它比哪一处空当都大），拆小一点';
  }
  return problems;
}

export function hasAuditDraftProblems(problems: AuditDraftProblems): boolean {
  return problems.overall !== null || problems.rows.some((row) => Object.values(row).some((problem) => problem !== null));
}

/** 审核口新形载荷的一份（ADR-0357 决定五）。 */
export interface RangedAuditPayload {
  claim: string;
  version: string;
  line: string;
  decision: string;
  verdict: AuditVerdict;
  rangeFrom: string;
  rangeTo: string;
  basis?: string;
  resolvesDispute?: string;
}

/** 一张表单排好的各段，一段一份载荷；依据只在写了时带（载荷严格解码，空串也是一个值），再审带上所解的争议。 */
export function rangedAuditPayloads(
  target: { claim: string; version: string; line: string; resolvesDispute?: string },
  rows: readonly AuditRowDraft[],
  ranges: readonly MinorRange[],
): RangedAuditPayload[] {
  return rows.map((row, index) => ({
    claim: target.claim,
    version: target.version,
    line: target.line,
    decision: row.decision,
    verdict: row.verdict,
    rangeFrom: String(ranges[index].from),
    rangeTo: String(ranges[index].to),
    ...(row.basis.trim() !== '' ? { basis: row.basis.trim() } : {}),
    ...(target.resolvesDispute ? { resolvesDispute: target.resolvesDispute } : {}),
  }));
}

/**
 * 一张审核表单开出来时的第一段：直接审时，能通过就先放「通过」、金额取还能通过的与还没审的两者之小；一分也通过不了（差异那一段、
 * 找不到对应运输、重复收费）就放「争议」、金额取还没审的全部。再审时放「通过」、金额取这段争议还没解的全部，人可以改成拒绝。
 */
export function firstAuditRow(context: AuditDraftContext, decision: string): AuditRowDraft {
  const available = totalOf(context.free);
  if (context.resolving) return { verdict: 'PASSED', amount: String(available), basis: '', decision };
  if (context.headroom > 0n) {
    return { verdict: 'PASSED', amount: String(context.headroom < available ? context.headroom : available), basis: '', decision };
  }
  return { verdict: 'DISPUTED', amount: String(available), basis: '', decision };
}

// —— 标识 ——

function stampOf(now: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}-${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}`;
}

const tail = (random: string) => random.replace(/[^A-Za-z0-9]/g, '').slice(0, 6).toUpperCase();

/**
 * 审核决定标识，页面在这一段第一次提交前定下：网络重发落回同一份决定。通过时它就是所形成审核应付的标识（ADR-0357 决定三），前缀
 * 不与应付单号默认的 `AP-` 相同，技术详情里两串好分。
 */
export function newAuditDecisionId(now: Date, random: string): string {
  return `AUDIT-${stampOf(now)}-${tail(random)}`;
}

/**
 * 主张标识，页面在第一次提交前定下：内部标识与幂等键，不再编进供应商账单号（ADR-0357 决定一）。同一家供应商的同一张账单录两次
 * 由服务端按账单号查重，答「账单号冲突」并带出已登记的那份。
 */
export function newClaimId(now: Date, random: string): string {
  return `BILL-${stampOf(now)}-${tail(random)}`;
}
