// 「供应商账单与应付」页的判读（UC-SA-004；ADR-0340；票 document-numbering/11）：账单号与主张标识的对应、账单走到哪一步、
// 一行能不能审、两张表单的本地检查与载荷、两个写口答复的那一句。纯函数、只引类型与界面用语词典，node:test 直接跑；取数与
// 表单在 SupplierBillsPage.tsx 与 SupplierBillPanel.tsx。
//
// 话照「说人话」写：答复的业务判别由服务端给（outcome），这里只把它译成一句操作者看得懂的话，不另判业务。走到哪一步是从读口
// 的计数派生的呈现词，不是领域状态：领域里逐行只记匹配分类与有没有审核应付。

import { shortPrincipalRef } from '../../components/principal-ref';
import { documentNumberAbsenceNote, documentTypeWords, numberingCollisionNote } from '../../vocabulary/document-numbering';
import { labelFor, type StatusTone } from '../../vocabulary/status';
import type {
  AuditedPayableRecord,
  SupplierBillAuditResponseBody,
  SupplierBillLinePayableRecord,
  SupplierBillLineRecord,
  SupplierBillReceptionPayload,
  SupplierBillReceptionRecord,
  SupplierBillReceptionResponseBody,
  SupplierExpectedCostRecord,
} from './api';

/** 应付单号的叫法，取单据编号词典那一份（ADR-0355 决定十一），不在各页另写。 */
export const payableNumberName = documentTypeWords.AUDITED_PAYABLE.numberName;

// —— 账单号与主张标识 ——

/**
 * 主张标识由页面在首次提交前生成，不让人填：取「供应商/供应商账单号」。账单号是供应商分配的外部标识（GLOSSARY「外部标识」），
 * 接收口的载荷没有单放它的一格，只能借主张标识带着它——这一借法记在票 document-numbering/11 的完成记录里，等协调方裁。
 *
 * 取成这样而不取随机串：同一家供应商的同一张账单不论谁来录、录几次，都落回同一个主张（服务端答已存在或版本冲突），不会因为
 * 两个人各录一遍就对同一份预期成本对上两次账；前面带上供应商，两家供应商用了同一个账单号也不会撞在一起。
 */
export function claimIdFor(supplier: string, billNumber: string): string {
  return `${supplier}/${billNumber.trim()}`;
}

/** 从主张标识里取回供应商的账单号；不是这一页生成的（接口送入、演示脚本）就取不到，答 undefined。 */
export function billNumberOf(claim: string, supplier: string): string | undefined {
  const prefix = `${supplier}/`;
  if (!claim.startsWith(prefix) || claim.length === prefix.length) return undefined;
  return claim.slice(prefix.length);
}

/** 一份账单在页面上的主标识：取得到账单号显账单号，取不到照旧显主张标识。 */
export function billLabel(row: Pick<SupplierBillReceptionRecord, 'claim' | 'supplier'>): string {
  return billNumberOf(row.claim, row.supplier) ?? row.claim;
}

/** 列表与右栏共用的一份账单的键：主张加版本，换版本另成一行，原版本保留。 */
export function billKey(row: Pick<SupplierBillReceptionRecord, 'claim' | 'claimVersion'>): string {
  return `${row.claim}\u0000${row.claimVersion}`;
}

/**
 * 应付标识由页面在第一次提交前定下：同一行因网络重发，落回的是同一份应付（服务端答已形成过），而不是又形成一份。前缀不与任何
 * 单据号默认格式的前缀相同：应付单号默认是 `AP-` 起头，内部标识也用它，技术详情里两串就难分。
 */
export function newPayableId(now: Date, random: string): string {
  const pad = (value: number) => String(value).padStart(2, '0');
  const stamp = `${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}-${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}`;
  return `PAYABLE-${stamp}-${random.replace(/[^A-Za-z0-9]/g, '').slice(0, 6).toUpperCase()}`;
}

/** 操作证据 `OPERATOR/<发行方>#<sub>` 的短名；缺席即那一份不记提交方。 */
export function operatorWords(evidence: string | undefined): string {
  if (!evidence) return '不记提交方';
  return shortPrincipalRef(evidence.startsWith('OPERATOR/') ? evidence.slice('OPERATOR/'.length) : evidence);
}

// —— 账单走到哪一步 ——

export const MATCHED = 'MATCHED';

/** 已对上的行数。 */
export function matchedLineCount(row: Pick<SupplierBillReceptionRecord, 'classificationCounts'>): number {
  return row.classificationCounts[MATCHED] ?? 0;
}

/** 没对上的行数：分类不是已对上，或接收记录上这一行没有匹配。 */
export function differingLineCount(row: Pick<SupplierBillReceptionRecord, 'lineCount' | 'classificationCounts'>): number {
  return Math.max(0, row.lineCount - matchedLineCount(row));
}

export interface BillProgress {
  label: string;
  tone: StatusTone;
}

/** 呈现词：从读口的计数派生，不是领域状态。 */
export const awaitingAuditLabel = '待审核';
export const auditorUnconfiguredLabel = '审核人未配置';
export const differingLabel = '有差异待核实';
export const auditedLabel = '已审完';

/**
 * 一份账单走到哪一步，按先要人动手的那件说：有已对上却还没形成应付的行，就是待审核（这家供应商在这个公司主体下此刻没登审核人
 * 时说审核人未配置——审核会停在未配置，要先去登）；否则有对不上的行，就是有差异待核实；都没有就是已审完。
 *
 * 审核人登没登看 auditAuthorityConfiguredNow，不看接收时的快照 auditAuthorityConfigured：审核在审核那一刻读审核授权册，租户
 * 接收之后补登了审核人，按快照说就会一直说没登。
 */
export function billProgress(
  row: Pick<SupplierBillReceptionRecord, 'lineCount' | 'classificationCounts' | 'matchedWithoutPayableCount' | 'auditAuthorityConfiguredNow'>,
): BillProgress {
  if (row.matchedWithoutPayableCount > 0) {
    return row.auditAuthorityConfiguredNow ? { label: awaitingAuditLabel, tone: 'warn' } : { label: auditorUnconfiguredLabel, tone: 'warn' };
  }
  if (differingLineCount(row) > 0) return { label: differingLabel, tone: 'warn' };
  return { label: auditedLabel, tone: 'good' };
}

/** 列上「对上情况」那一句：已对上几行，其余按分类各几行；分类的词经用语词典。 */
export function matchSummary(row: Pick<SupplierBillReceptionRecord, 'lineCount' | 'classificationCounts'>): string {
  const matched = matchedLineCount(row);
  if (matched === row.lineCount) return `${row.lineCount} 行全对上`;
  const parts = Object.entries(row.classificationCounts)
    .filter(([code]) => code !== MATCHED)
    .sort(([left], [right]) => (left < right ? -1 : left > right ? 1 : 0))
    .map(([code, count]) => `${labelFor(code)} ${count}`);
  const unrecorded = row.lineCount - Object.values(row.classificationCounts).reduce((sum, count) => sum + count, 0);
  if (unrecorded > 0) parts.push(`没有匹配记录 ${unrecorded}`);
  return [`对上 ${matched}/${row.lineCount} 行`, ...parts].join(' · ');
}

// —— 两本册的页与检索 ——

/**
 * 工具条右边那一句。两本册都按时间倒序截页（读口多取一行判页外有没有）：截了就不能说「共几份」，只能说列出了最近的几份——
 * 指标与筛选也只数这些，页外的不在里面。
 */
export function pageCountSummary(listed: number, visible: number, truncated: boolean, unit: string, latest: string): string {
  return truncated
    ? `只列出最近${latest}的 ${listed} ${unit}，更早的没有列出、指标也不算它们；当前显示 ${visible} ${unit}`
    : `共 ${listed} ${unit}，当前显示 ${visible} ${unit}`;
}

/**
 * 应付签的检索池：取回的那一页，加上按应付单号在全册里精确检索到的那几笔（同一笔只留一份）。页外的应付只靠后者才找得到；
 * 前者照旧给账单号、供应商这些字段做页内模糊检索。
 */
export function withExactHits(page: readonly AuditedPayableRecord[], hits: readonly AuditedPayableRecord[]): AuditedPayableRecord[] {
  const listed = new Set(page.map((row) => row.payable));
  return [...page, ...hits.filter((row) => !listed.has(row.payable))];
}

// —— 一行能不能审 ——

export interface LineDisposition {
  label: string;
  tone: StatusTone;
  /** 这一行在页面上摆不摆「审核通过」：已对上、还没有应付。摆了也照样由服务端判（授权、账户、上限都可能没登）。 */
  auditable: boolean;
}

export function lineDisposition(line: Pick<SupplierBillLineRecord, 'classification' | 'payable'>): LineDisposition {
  if (line.payable) return { label: '已形成应付', tone: 'good', auditable: false };
  if (line.classification === MATCHED) return { label: awaitingAuditLabel, tone: 'warn', auditable: true };
  if (line.classification) return { label: '对不上，先核实', tone: 'warn', auditable: false };
  return { label: '没有匹配记录', tone: 'muted', auditable: false };
}

/** 一笔应付在页面上的主标识：应付单号（ADR-0355 决定十一）；没有号的照旧显应付标识，为什么没号由 payableNumberAbsenceNote 注明。 */
export function payableLabel(payable: Pick<SupplierBillLinePayableRecord, 'payable' | 'documentNumber'>): string {
  return payable.documentNumber ?? payable.payable;
}

/** 没有应付单号时注明的那一句，按应付上记下的原因说，不一律写成「上线前开立」；有号时没有这一句。 */
export function payableNumberAbsenceNote(
  payable: Pick<SupplierBillLinePayableRecord, 'documentNumber' | 'documentNumberAbsence'>,
): string | undefined {
  if (payable.documentNumber || !payable.documentNumberAbsence) return undefined;
  return documentNumberAbsenceNote(payable.documentNumberAbsence);
}

/** 审核应付签的状态胶囊：有应付单号与没有号分开，没有号的原因由技术详情说。 */
export function payableNumberState(payable: Pick<AuditedPayableRecord, 'documentNumber'>): BillProgress {
  return payable.documentNumber ? { label: `有${payableNumberName}`, tone: 'good' } : { label: `无${payableNumberName}`, tone: 'muted' };
}

// —— 登记供应商账单 ——

/** 这一行没有对应的预期成本（「找不到对应运输」）时，预期成本那一格的取值。 */
export const NO_EXPECTED_COST = '__none__';

export interface ReceptionLineDraft {
  /** 预期成本版本；NO_EXPECTED_COST 是找不到对应运输；空串是还没选。 */
  expected: string;
  classification: string;
  claimed: string;
  /** 只在找不到对应运输时由人填；对上了预期成本时取那一版的费用项目。 */
  feeItem: string;
  basis: string;
}

export interface ReceptionDraft {
  supplier: string;
  legalEntity: string;
  billNumber: string;
  version: string;
  period: string;
  currency: string;
  /** RFC 3339；读不出时为 null。 */
  receivedAt: string | null;
  supplementsClaim: string;
  lines: ReceptionLineDraft[];
}

/**
 * 能拿来对账的预期成本：当前那一版——不是失效纠错版本、也没有后继；选了币种时只列同币种的（不同币种没有换算依据，对不上）。
 *
 * 不按供应商筛：预期成本的读口与表上都没有供应商或承运方，只有它依的供应商协议（agreement），那一版协议是哪家供应商的归
 * party-commercial。匹配门也不核供应商，误选就对到别家的预期成本——这一缺口记在 UC-SA-004「给开发的交接」，表单只能如实提醒。
 */
export function currentExpectedCosts(costs: readonly SupplierExpectedCostRecord[], currency: string): SupplierExpectedCostRecord[] {
  const superseded = new Set(costs.map((cost) => cost.priorVersion).filter((version): version is string => !!version));
  return costs.filter(
    (cost) => !cost.withdrawn && !superseded.has(cost.version) && (currency === '' || cost.settlementCurrency === currency),
  );
}

export interface ClassificationChoice {
  value: string;
  label: string;
  hint: string;
}

/**
 * 逐行匹配分类在本页的取值表：键是 domain.MatchClassification 的原名，值是登记表单上那一格的提示（界面词在 vocabulary/status）。
 * 界面上要说「对不上的是哪些」时从这里取，不在句子里另列一遍：领域添一种分类，这里加一行，各句跟着变。
 */
export const matchClassificationHints: Readonly<Record<string, string>> = {
  MATCHED: '金额与预期成本一样，之后可以审核',
  QUANTITY_VARIANCE: '件数、重量或次数和我们记的不一样',
  PRICE_VARIANCE: '单价或费率和协议不一样',
  NO_MATCHING_OCCURRENCE: '我们这边查不到这笔运输',
  DUPLICATE_BILLING: '这笔之前已经收过',
};

/** 除已对上之外的各个分类的界面词，以顿号连起来，给「对不上的行（…）」那几句用。 */
export function differingClassificationWords(): string {
  return Object.keys(matchClassificationHints)
    .filter((code) => code !== MATCHED)
    .map((code) => labelFor(code))
    .join('、');
}

const choice = (code: string): ClassificationChoice => ({ value: code, label: labelFor(code), hint: matchClassificationHints[code] ?? '' });

/**
 * 一行能选的对上方式，照匹配门的形状（domain.MatchBillLine）：金额与预期成本相等才能选已对上；不等只能是数量或价格不符；
 * 重复收费两种情形都能选；没有对应预期成本就只有找不到对应运输。金额读不出时按不等处理。
 */
export function classificationChoices(expected: string, claimed: string, expectedAmount: string | undefined): ClassificationChoice[] {
  if (expected === NO_EXPECTED_COST) return [choice('NO_MATCHING_OCCURRENCE')];
  if (expected === '') return [];
  if (expectedAmount !== undefined && claimed.trim() === expectedAmount) return [choice('MATCHED'), choice('DUPLICATE_BILLING')];
  return [choice('QUANTITY_VARIANCE'), choice('PRICE_VARIANCE'), choice('DUPLICATE_BILLING')];
}

/** 文字格（依据）的本地检查，上限与对账单争议那几格同。 */
function textProblem(raw: string, what: string): string | null {
  if (raw.trim() === '') return `写一句${what}`;
  if ([...raw].length > 1000) return `${what}写不下就附证据另存，这里至多 1000 字`;
  return null;
}

/** 主张金额的本地检查：与本页金额同一写法的正整数（最小币单位，例如 12.34 元写 1234）。服务端照样会判。 */
export function claimedAmountProblem(raw: string): string | null {
  const value = raw.trim();
  if (value === '') return '填账单上这一行的金额';
  if (!/^[1-9]\d*$/.test(value)) return '金额写成不带小数点、不带符号的正整数，按最小币单位（例如 12.34 元写 1234）';
  return null;
}

export interface ReceptionProblems {
  fields: Record<'supplier' | 'legalEntity' | 'billNumber' | 'version' | 'period' | 'currency' | 'receivedAt' | 'lines', string | null>;
  lines: Record<'expected' | 'classification' | 'claimed' | 'feeItem' | 'basis', string | null>[];
}

/**
 * 表单只拦人能当场改的错；金额对不对得上预期成本、预期成本是不是当前版，服务端照样判。costsByVersion 给出各版预期成本的金额，
 * 用来判这一行选的对上方式是不是这一行能选的那几种（classificationChoices）。
 */
export function receptionProblems(
  draft: ReceptionDraft,
  costsByVersion: ReadonlyMap<string, Pick<SupplierExpectedCostRecord, 'feeItem' | 'settlementAmount'>>,
  now: number = Date.now(),
): ReceptionProblems {
  const receivedAt = draft.receivedAt === null ? Number.NaN : Date.parse(draft.receivedAt);
  return {
    fields: {
      supplier: draft.supplier === '' ? '选是哪家供应商的账单' : null,
      legalEntity: draft.legalEntity === '' ? '选账单开给本公司哪个主体' : null,
      billNumber: draft.billNumber.trim() === '' ? '照账单填供应商的账单号' : null,
      version: draft.version.trim() === '' ? '填版本；第一次来件填 1' : null,
      period: draft.period.trim() === '' ? '照账单填账期' : null,
      currency: draft.currency === '' ? '选账单的币种' : null,
      receivedAt: Number.isNaN(receivedAt) ? '填账单收到的时间' : receivedAt > now ? '收到的时间不能晚于现在' : null,
      lines: draft.lines.length === 0 ? '账单至少一行' : null,
    },
    lines: draft.lines.map((line) => {
      const choices = classificationChoices(line.expected, line.claimed, costsByVersion.get(line.expected)?.settlementAmount);
      return {
        expected: line.expected === '' ? '选这一行对的是哪一笔预期成本，或选「找不到对应运输」' : null,
        classification:
          line.expected === ''
            ? null
            : !choices.some((option) => option.value === line.classification)
              ? '选这一行对上了没有'
              : null,
        claimed: claimedAmountProblem(line.claimed),
        feeItem: line.expected === NO_EXPECTED_COST && line.feeItem.trim() === '' ? '填这一行的费用项目' : null,
        basis: line.classification !== '' && line.classification !== MATCHED ? textProblem(line.basis, '依据：怎么核的') : null,
      };
    }),
  };
}

export function hasReceptionProblems(problems: ReceptionProblems): boolean {
  return (
    Object.values(problems.fields).some((problem) => problem !== null) ||
    problems.lines.some((line) => Object.values(line).some((problem) => problem !== null))
  );
}

/**
 * 接收口的载荷。行号按表单里的顺序从 1 起；对上了预期成本的行，费用项目取那一版的；已对上不带依据（带了匹配门会拒），其余带。
 * 可缺的几格缺就不写——载荷按未知字段严格解码，空串也是一个值。
 */
export function receptionPayload(
  draft: ReceptionDraft,
  costsByVersion: ReadonlyMap<string, Pick<SupplierExpectedCostRecord, 'feeItem'>>,
): SupplierBillReceptionPayload {
  const refOf = (index: number) => String(index + 1);
  return {
    claim: claimIdFor(draft.supplier, draft.billNumber),
    version: draft.version.trim(),
    supplier: draft.supplier,
    legalEntity: draft.legalEntity,
    period: draft.period.trim(),
    currency: draft.currency,
    lines: draft.lines.map((line, index) => ({
      line: refOf(index),
      feeItem: line.expected === NO_EXPECTED_COST ? line.feeItem.trim() : (costsByVersion.get(line.expected)?.feeItem ?? ''),
      claimed: line.claimed.trim(),
    })),
    receivedAt: draft.receivedAt ?? '',
    ...(draft.supplementsClaim ? { supplementsClaim: draft.supplementsClaim } : {}),
    directives: draft.lines.map((line, index) => ({
      line: refOf(index),
      classification: line.classification,
      ...(line.expected !== NO_EXPECTED_COST ? { expectedVersion: line.expected } : {}),
      ...(line.classification !== MATCHED ? { basis: line.basis.trim() } : {}),
    })),
  };
}

/** 接收答复里这几格说明库上已有这份账单或刚落了一份：重读账单册。 */
export const receptionSettled = new Set(['BILL_RECEIVED', 'EXISTING_RESULT']);

/** 接收口答复的那一句。 */
export function receptionAnswerText(body: SupplierBillReceptionResponseBody, billNumber: string): string {
  const bill = billNumber.trim() === '' ? '这张账单' : `账单 ${billNumber.trim()}`;
  switch (body.outcome) {
    case 'BILL_RECEIVED':
      return body.auditAuthority === 'UNCONFIGURED'
        ? `已登记${bill}。这家供应商在这个公司主体下还没有登记审核人，审核会停在「未配置」，登了审核人再审。`
        : `已登记${bill}，对上的行可以在右栏审核。`;
    case 'EXISTING_RESULT':
      return `${bill}已经登记过了，内容一样，没有重复登记。`;
    case 'VERSION_CONFLICT':
      return `没有登记：同一家供应商的${bill}、同一个版本已经登记过另一份内容。先核对已登记的那份；供应商更正后重发的，把版本加一再登记。`;
    case 'SOURCE_NOT_ACCEPTED':
      return '没有登记：账单内容立不起来——选的预期成本不在册上，或金额与所选的对上方式不符（已对上要金额一样，数量或价格不符要金额不一样）。';
    case 'BILL_UNDECIDED':
      return body.undecidedReason === 'CONVERSION_UNCONFIGURED'
        ? '没有登记：账单币种与所选预期成本的结算币种不同，本公司没有登记换算依据（未配置），不按当前汇率猜。'
        : `没有登记：服务端这一次没有答出结果（${body.undecidedReason ?? '原因未说'}），稍后重试。`;
    case 'EXPECTED_COST_NOT_CURRENT': {
      const stale = body.notCurrent;
      if (!stale) return '没有登记：所选的预期成本已经不是当前那一版，改选当前那一版再登记。';
      const why = stale.withdrawn ? '已经撤下（它依的运输已失效）' : `已经被更正过（现在是 ${stale.successor ?? '新的一版'}）`;
      return `没有登记：第 ${stale.line} 行选的预期成本 ${stale.version} ${why}，改选当前那一版再登记。`;
    }
    default:
      return `服务端答 ${body.outcome}`;
  }
}

// —— 审核这张账单 ——

/** 审核答复里这几格说明库上已有这一行的应付或刚形成一份：重读账单与应付册。 */
export const auditSettled = new Set(['PAYABLE_FORMED', 'EXISTING_PAYABLE']);

/** 答复里那一笔应付怎么称呼：有应付单号说号；没有的照旧说应付标识，并带上为什么没号。 */
function answeredPayable(body: SupplierBillAuditResponseBody): string {
  const label = body.documentNumber ?? body.payable ?? '';
  const note = body.documentNumber ? undefined : body.documentNumberAbsence ? documentNumberAbsenceNote(body.documentNumberAbsence) : undefined;
  return note ? `${label}（${note}）` : label;
}

const undecidedAuditWords: Record<string, string> = {
  AUDIT_AUTHORITY_UNCONFIGURED: '这家供应商在这个公司主体下还没有登记审核人（审核授权未配置）',
  PAYABLE_ACCOUNT_UNCONFIGURED: '这家供应商在这个公司主体下还没有登记应付归集账户（未配置）',
  AUDIT_ESCALATION_UNCONFIGURED: '本公司还没有登记越权审核上限（未配置），不默认放行',
};

/** 审核口答复的那一句。line 是表单上的行号；money 是页面的金额写法（ops-ui 的 formatMoney），由调用方交进来，本文件不引 tsx。 */
export function auditAnswerText(
  body: SupplierBillAuditResponseBody,
  line: string,
  money: (amount: string, currency: string) => string = (amount, currency) => `${amount} ${currency}`,
): string {
  const amount = body.amount && body.currency ? `，金额 ${money(body.amount, body.currency)}` : '';
  switch (body.outcome) {
    case 'PAYABLE_FORMED':
      return `第 ${line} 行审核通过，形成应付 ${answeredPayable(body)}${amount}。`;
    case 'EXISTING_PAYABLE':
      return `第 ${line} 行已经形成过应付 ${answeredPayable(body)}，没有重复形成。`;
    case 'PAYABLE_CONFLICT':
      return '没有形成应付：这个应付标识已经用在另一行上；刷新这张账单后重试。';
    case 'LINE_NOT_AUDITABLE':
      return `不能审核第 ${line} 行：它和预期成本对不上（${differingClassificationWords()}之一），要先核实差异。`;
    case 'AUDIT_MUST_ESCALATE':
      return `第 ${line} 行没有形成应付：金额超过本公司登记的越权审核上限，要升级审批。升级后由谁终审还没有定，这一行先停着。`;
    case 'SOURCE_NOT_ACCEPTED':
      return '没有审核：找不到这张账单或这一行；刷新后重试。';
    case 'AUDIT_UNDECIDED': {
      const reason = body.undecidedReason ? undecidedAuditWords[body.undecidedReason] : undefined;
      return reason
        ? `第 ${line} 行没有形成应付：${reason}。登好之后再审。`
        : `第 ${line} 行没有形成应付：服务端这一次没有答出结果（${body.undecidedReason ?? '原因未说'}），稍后重试。`;
    }
    case 'NUMBERING_RULE_COLLISION':
      return numberingCollisionNote(body.documentType);
    default:
      return `服务端答 ${body.outcome}`;
  }
}
