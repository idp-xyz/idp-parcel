// 「供应商账单与应付」页的判读（UC-SA-004；ADR-0340、ADR-0357；票 document-numbering/11、13）：账单号怎么显示、主张标识怎么定、
// 账单与一行走到哪一步、登记表单的本地检查与载荷、接收口答复的那一句。按范围审核的判读在 supplier-bill-audit.ts。纯函数、只引
// 类型与界面用语词典，node:test 直接跑；取数与表单在 SupplierBillsPage.tsx 与 SupplierBillPanel.tsx。
//
// 话照「说人话」写：答复的业务判别由服务端给（outcome），这里只把它译成一句操作者看得懂的话，不另判业务。走到哪一步是从读口
// 的和派生的呈现词，不是领域状态：领域里逐行记的是匹配分类与各段审核决定。

import { shortPrincipalRef } from '../../components/principal-ref';
import { documentNumberAbsenceNote, documentTypeWords } from '../../vocabulary/document-numbering';
import { labelFor, type StatusTone } from '../../vocabulary/status';
import type {
  AuditedPayableRecord,
  SupplierBillLinePayableRecord,
  SupplierBillLineRecord,
  SupplierBillReceptionPayload,
  SupplierBillReceptionRecord,
  SupplierBillReceptionResponseBody,
  SupplierExpectedCostRecord,
} from './api';
import { billNumberConflictText, claimSupplierConflictText } from './supplier-bill-audit';

/** 应付单号的叫法，取单据编号词典那一份（ADR-0355 决定十一），不在各页另写。 */
export const payableNumberName = documentTypeWords.AUDITED_PAYABLE.numberName;

// —— 账单号与主张标识 ——

/**
 * 票 11 期间，接收口还没有放供应商账单号的格，页面把主张标识生成为「供应商/账单号」、再从里面取回账单号显示。这种主张标识永久
 * 留在册上（接收行、审核应付与交接信封都引用它），账单号册不从它拆号回填（ADR-0357 决定一）；这里只为显示取回，不进检索与查重。
 * 不是这种形状的（随机生成的、接口送入的）取不到，答 undefined。
 */
export function legacyBillNumberOf(claim: string, supplier: string): string | undefined {
  const prefix = `${supplier}/`;
  if (!claim.startsWith(prefix) || claim.length === prefix.length) return undefined;
  return claim.slice(prefix.length);
}

/** 一份账单的供应商账单号：服务端账单号册上的那个；没有（旧行）再从主张标识里取回；都没有答 undefined。 */
export function billNumberOf(row: Pick<SupplierBillReceptionRecord, 'claim' | 'supplier' | 'supplierBillNumber'>): string | undefined {
  return row.supplierBillNumber ?? legacyBillNumberOf(row.claim, row.supplier);
}

/**
 * 登记一张账单用哪个主张标识：同一家供应商同一个账单号已在册上（新号旧号一样认），就沿用那份主张的标识——录的是它的新版本，
 * 服务端按版本答重放、版本冲突或接收新版本；不在册上就用页面新生成的那个（fresh）。服务端照样按（供应商，账单号）查重，页面这一步
 * 只是让「更正后重发」落回原主张，不靠它防重。
 */
export function claimIdForBill(
  rows: readonly Pick<SupplierBillReceptionRecord, 'claim' | 'supplier' | 'supplierBillNumber'>[],
  supplier: string,
  billNumber: string,
  fresh: string,
): string {
  const number = billNumber.trim();
  return rows.find((row) => row.supplier === supplier && billNumberOf(row) === number)?.claim ?? fresh;
}

/** 一份账单在页面上的主标识：取得到账单号显账单号，取不到照旧显主张标识。 */
export function billLabel(row: Pick<SupplierBillReceptionRecord, 'claim' | 'supplier' | 'supplierBillNumber'>): string {
  return billNumberOf(row) ?? row.claim;
}

/** 列表与右栏共用的一份账单的键：主张加版本，换版本另成一行，原版本保留。 */
export function billKey(row: Pick<SupplierBillReceptionRecord, 'claim' | 'claimVersion'>): string {
  return `${row.claim}\u0000${row.claimVersion}`;
}

/** billKey 里的主张身份：选中的账单不在页上时，按它在全册里读回。 */
export function claimOfBillKey(key: string): string {
  return key.split('\u0000')[0];
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

export interface BillProgress {
  label: string;
  tone: StatusTone;
}

/** 呈现词：从读口的和派生，不是领域状态。 */
export const awaitingAuditLabel = '待审核';
export const auditorUnconfiguredLabel = '审核人未配置';
export const disputedLabel = '有争议待再审';
export const auditedLabel = '已审完';

/**
 * 一份账单走到哪一步，按先要人动手的那件说：还有没给任何审核结果的金额，就是待审核（这家供应商在这个公司主体下此刻没登审核人
 * 时说审核人未配置——审核会停在未配置，要先去登）；都给过了但还有争议没解，就是有争议待再审；都没有就是已审完。对不上的行也在
 * 「待审核」里：它们要审核人给争议或拒绝（ADR-0357 决定二），不再是只能核实、没有入口的那一格。
 *
 * 审核人登没登看 auditAuthorityConfiguredNow，不看接收时的快照 auditAuthorityConfigured：审核在审核那一刻读审核授权册，租户
 * 接收之后补登了审核人，按快照说就会一直说没登。
 */
export function billProgress(
  row: Pick<SupplierBillReceptionRecord, 'undecidedAmount' | 'openDisputeAmount' | 'auditAuthorityConfiguredNow'>,
): BillProgress {
  if (BigInt(row.undecidedAmount) > 0n) {
    return row.auditAuthorityConfiguredNow ? { label: awaitingAuditLabel, tone: 'warn' } : { label: auditorUnconfiguredLabel, tone: 'warn' };
  }
  if (BigInt(row.openDisputeAmount) > 0n) return { label: disputedLabel, tone: 'warn' };
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

// —— 一行走到哪一步 ——

export interface LineDisposition {
  label: string;
  tone: StatusTone;
  /** 这一行还有没给结果的金额：页面摆「审核这一行」。摆了也照样由服务端判（授权、账户、上限都可能没登）。 */
  auditable: boolean;
}

/** 一行走到哪一步，判法同 billProgress，只看这一行的和。 */
export function lineDisposition(line: Pick<SupplierBillLineRecord, 'undecidedAmount' | 'openDisputeAmount'>): LineDisposition {
  if (BigInt(line.undecidedAmount) > 0n) return { label: awaitingAuditLabel, tone: 'warn', auditable: true };
  if (BigInt(line.openDisputeAmount) > 0n) return { label: disputedLabel, tone: 'warn', auditable: false };
  return { label: auditedLabel, tone: 'good', auditable: false };
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

/** 当前那一版——不是失效纠错版本、也没有后继；选了币种时只留同币种的（不同币种没有换算依据，对不上）。 */
function currentOfCurrency(costs: readonly SupplierExpectedCostRecord[], currency: string): SupplierExpectedCostRecord[] {
  const superseded = new Set(costs.map((cost) => cost.priorVersion).filter((version): version is string => !!version));
  return costs.filter(
    (cost) => !cost.withdrawn && !superseded.has(cost.version) && (currency === '' || cost.settlementCurrency === currency),
  );
}

/**
 * 能拿来对账的预期成本：所选供应商的、当前那一版、同币种的。预期成本记下了它所依协议的供应商，匹配门核账单的供应商与之相同
 * （ADR-0358），所以别家的不列；没选供应商时一份都不列。没记供应商的旧行也不列——匹配门对它们答未记，补记之后才能选。
 * 这里的筛选只省人的事，挡人的是匹配门。
 */
export function currentExpectedCosts(
  costs: readonly SupplierExpectedCostRecord[],
  currency: string,
  supplier: string,
): SupplierExpectedCostRecord[] {
  if (supplier === '') return [];
  return currentOfCurrency(costs, currency).filter((cost) => cost.supplier === supplier);
}

/** 当前、同币种而还没记供应商的预期成本有几份：表单如实说一句，补记之后它们才列得出来。 */
export function unrecordedSupplierCount(costs: readonly SupplierExpectedCostRecord[], currency: string): number {
  return currentOfCurrency(costs, currency).filter((cost) => !cost.supplier).length;
}

/**
 * 换了供应商之后，各行已选的预期成本不在这家的可选之列就清掉，连同对上方式——不留一个匹配门会拒的选择。「找不到对应运输」
 * 不点名预期成本，留着。
 */
export function linesForSupplier(lines: readonly ReceptionLineDraft[], selectable: readonly SupplierExpectedCostRecord[]): ReceptionLineDraft[] {
  const versions = new Set(selectable.map((cost) => cost.version));
  return lines.map((line) =>
    line.expected === '' || line.expected === NO_EXPECTED_COST || versions.has(line.expected) ? line : { ...line, expected: '', classification: '' },
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
  claim: string,
): SupplierBillReceptionPayload {
  const refOf = (index: number) => String(index + 1);
  return {
    claim,
    version: draft.version.trim(),
    supplier: draft.supplier,
    supplierBillNumber: draft.billNumber.trim(),
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
export function receptionAnswerText(
  body: SupplierBillReceptionResponseBody,
  billNumber: string,
  supplierName: (supplier: string) => string = (supplier) => supplier,
): string {
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
    case 'SUPPLIER_BILL_NUMBER_CONFLICT':
      return billNumberConflictText(body, billNumber);
    case 'CLAIM_SUPPLIER_CONFLICT':
      return claimSupplierConflictText(body, supplierName);
    case 'EXPECTED_COST_SUPPLIER_MISMATCH': {
      const refused = body.expectedCostSupplier;
      if (!refused) return '没有登记：所选的预期成本是别家供应商的，改选这家供应商的预期成本再登记。';
      // 那一版的供应商说名称（supplierName 取不到名称时自己退回标识），与账单号冲突那一句同一种说法。
      const owner = refused.supplier ? supplierName(refused.supplier) : '别家供应商';
      return `没有登记：第 ${refused.line} 行选的预期成本 ${refused.version} 是${owner}的，不是这张账单的供应商的。改选这家的预期成本，或改成「找不到对应运输」。`;
    }
    case 'EXPECTED_COST_SUPPLIER_UNRECORDED': {
      const refused = body.expectedCostSupplier;
      const which = refused ? `第 ${refused.line} 行选的预期成本 ${refused.version} ` : '所选的预期成本';
      return `没有登记：${which}还没记是哪家供应商的（较早形成的那一批），核不了是不是这家的。请运营先补记供应商再登记。`;
    }
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
