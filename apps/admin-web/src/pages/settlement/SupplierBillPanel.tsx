import { useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { feedbackFor } from '../../components/action-feedback';
import { StatusPill } from '../../components/StatusPill';
import { statusToneBanner } from '../../components/status-tone';
import { labelFor, type StatusTone } from '../../vocabulary/status';
import { formatInstant } from '../moment';
import { hiddenDecisionHint, supplierBillAuditDecision } from '../operator-grants';
import { useMayMakeDecision } from '../use-operator-grants';
import { ExceptionDetailPanel } from '../visibility/ExceptionDetailPanel';
import {
  auditSupplierBillLine,
  getSupplierBillLines,
  type ApiResult,
  type SupplierBillLineDecisionRecord,
  type SupplierBillLineRecord,
  type SupplierBillLinesResponseBody,
  type SupplierBillReceptionRecord,
} from './api';
import { formatMoney, technicalRows } from './ops-ui';
import { namedRef, type PartyNames } from './party-names';
import { SupplierBillLineAuditForm, type LineAuditTarget } from './SupplierBillLineAuditForm';
import { openDisputes, passHeadroom, totalOf, undecidedRanges, verdictLabels, type AuditDraftContext } from './supplier-bill-audit';
import {
  billLabel,
  billNumberOf,
  billProgress,
  legacyBillNumberOf,
  lineDisposition,
  matchSummary,
  operatorWords,
  payableLabel,
  payableNumberAbsenceNote,
  payableNumberName,
} from './supplier-bills';

// 供应商账单与应付页「供应商账单」签的右栏：这张账单的全貌、各行与各段审核结果，以及「审核这一行」与「核实后再审」——一行拆成
// 几段分别给通过、争议或拒绝，通过的那一段形成审核应付并交回应付单号，争议的那一段核实后再审（ADR-0357）。审核入口按「供应商账单
// 审核」授予收起（ADR-0315）：只在明确知道没有授予时收起，不知道就摆，服务端照旧每次判。
//
// 审核不改数字：每段的金额是人从这一行主张金额里划出来的，通过的各段之和不超过这一行的已匹配金额（价差行只到预期成本），超出的
// 那一段只能争议或拒绝——这一上限服务端照样判，页面先说清楚。

/** 差额是主张减预期：正为账单多收，负为少收；没有差额不说。 */
function varianceWords(variance: string | undefined, currency: string): string {
  if (!variance || variance === '0') return '';
  return variance.startsWith('-') ? `，账单少收 ${formatMoney(variance.slice(1), currency)}` : `，账单多收 ${formatMoney(variance, currency)}`;
}

const verdictTone: Record<SupplierBillLineDecisionRecord['verdict'], StatusTone> = {
  PASSED: 'good',
  DISPUTED: 'warn',
  REJECTED: 'muted',
};

/** 一行的和那一句：只说不为零的几样。 */
function tallyWords(line: SupplierBillLineRecord, currency: string): string {
  const parts: string[] = [];
  if (line.passedAmount !== '0') parts.push(`已通过 ${formatMoney(line.passedAmount, currency)}`);
  if (line.openDisputeAmount !== '0') parts.push(`争议中 ${formatMoney(line.openDisputeAmount, currency)}`);
  if (line.rejectedAmount !== '0') parts.push(`已拒绝 ${formatMoney(line.rejectedAmount, currency)}`);
  if (line.undecidedAmount !== '0') parts.push(`还没审 ${formatMoney(line.undecidedAmount, currency)}`);
  return parts.join(' · ');
}

function DecisionRow({
  decision,
  currency,
  onShowPayable,
}: {
  decision: SupplierBillLineDecisionRecord;
  currency: string;
  onShowPayable: (number: string) => void;
}) {
  const note = decision.payable ? payableNumberAbsenceNote(decision.payable) : undefined;
  return (
    <li className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[12px]">
      <StatusPill label={decision.resolvesDispute ? `再审${verdictLabels[decision.verdict]}` : verdictLabels[decision.verdict]} tone={verdictTone[decision.verdict]} />
      <span className="font-semibold text-idpxyz-textBright">{formatMoney(decision.amount, currency)}</span>
      <span className="text-[11px] text-idpxyz-breadcrumb">
        区间 {decision.rangeFrom}–{decision.rangeTo} · {formatInstant(decision.auditedAt)} 由 {operatorWords(decision.operationEvidence)} 审
      </span>
      {decision.payable ? (
        <span>
          应付 <span className="font-mono font-semibold text-idpxyz-textBright">{payableLabel(decision.payable)}</span>
          {note ? <span className="text-idpxyz-breadcrumb">（{note}）</span> : null}
          <Button
            type="button"
            size="sm"
            variant="ghost"
            className="ml-1 h-6 px-1.5 text-[12px] text-idpxyz-link"
            onClick={() => onShowPayable(payableLabel(decision.payable!))}
          >
            在「审核应付」里看
          </Button>
        </span>
      ) : null}
      {decision.basis ? <span className="basis-full text-[11px] leading-4 text-idpxyz-breadcrumb">依据：{decision.basis}</span> : null}
    </li>
  );
}

/** 这一行上正开着的那张表单：直接审，或对某一段争议再审。 */
type OpenForm = { kind: 'audit' } | { kind: 'reaudit'; dispute: string };

function LineCard({
  row,
  line,
  mayAudit,
  submit,
  onSettled,
  onShowPayable,
}: {
  row: SupplierBillReceptionRecord;
  line: SupplierBillLineRecord;
  mayAudit: boolean;
  submit: (payload: Parameters<typeof auditSupplierBillLine>[0]) => ReturnType<typeof auditSupplierBillLine>;
  onSettled: () => void;
  onShowPayable: (number: string) => void;
}) {
  const [form, setForm] = useState<OpenForm | null>(null);
  const currency = row.currency;
  const disposition = lineDisposition(line);
  const headroom = passHeadroom(line.matchedAmount, line.decisions);
  const disputes = openDisputes(line.decisions);
  const target: LineAuditTarget = { claim: row.claim, version: row.claimVersion, line: line.line };
  let context: AuditDraftContext | null = null;
  if (form?.kind === 'audit') {
    context = { free: undecidedRanges(line.claimedAmount, line.decisions), headroom, resolving: false };
  } else if (form?.kind === 'reaudit') {
    const open = disputes.find((entry) => entry.dispute.decision === form.dispute);
    if (open) {
      context = { free: open.open, headroom, resolving: true };
      target.resolvesDispute = form.dispute;
    }
  }
  return (
    <li className="rounded border border-idpxyz-border px-3 py-2 text-idpxyz-text">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="font-semibold text-idpxyz-textBright">
          第 {line.line} 行 · {formatMoney(line.claimedAmount, currency)}
        </span>
        <span className="flex items-center gap-1.5">
          {line.classification ? <StatusPill code={line.classification} /> : null}
          <StatusPill label={disposition.label} tone={disposition.tone} />
        </span>
      </div>
      <p className="mt-1 text-[11px] leading-4 text-idpxyz-breadcrumb">
        费用项目 <span className="font-mono">{line.feeItem}</span>
        {line.expectedVersion
          ? ` · 对的预期成本 ${formatMoney(line.expectedAmount, currency)}${varianceWords(line.varianceAmount, currency)}`
          : line.classification
            ? ' · 我们这边查不到对应的运输'
            : ''}
        {` · 最多能通过 ${formatMoney(line.matchedAmount, currency)}`}
      </p>
      {line.basis ? <p className="text-[11px] leading-4 text-idpxyz-breadcrumb">对账依据：{line.basis}</p> : null}
      {line.decisions.length > 0 ? (
        <>
          <ul className="mt-1.5 space-y-1">
            {line.decisions.map((decision) => (
              <DecisionRow key={decision.decision} decision={decision} currency={currency} onShowPayable={onShowPayable} />
            ))}
          </ul>
          <p className="mt-1 text-[11px] text-idpxyz-breadcrumb">{tallyWords(line, currency)}</p>
        </>
      ) : null}
      {mayAudit && form === null ? (
        <div className="mt-1.5 flex flex-wrap items-center gap-2">
          {disposition.auditable ? (
            <Button type="button" size="sm" variant="outline" className="h-7 text-[12px]" onClick={() => setForm({ kind: 'audit' })}>
              审核这一行
            </Button>
          ) : null}
          {disputes.map((entry) => (
            <Button
              key={entry.dispute.decision}
              type="button"
              size="sm"
              variant="outline"
              className="h-7 text-[12px]"
              onClick={() => setForm({ kind: 'reaudit', dispute: entry.dispute.decision })}
            >
              核实后再审争议中的 {formatMoney(String(totalOf(entry.open)), currency)}
            </Button>
          ))}
        </div>
      ) : null}
      {mayAudit && form !== null && context !== null ? (
        <SupplierBillLineAuditForm
          key={form.kind === 'reaudit' ? form.dispute : 'audit'}
          target={target}
          context={context}
          currency={currency}
          submit={submit}
          onSettled={onSettled}
          onCancel={() => setForm(null)}
        />
      ) : null}
    </li>
  );
}

export function SupplierBillPanel({
  row,
  names,
  legalEntityName,
  onClose,
  onAudited,
  onShowPayable,
}: {
  row: SupplierBillReceptionRecord;
  names: PartyNames;
  legalEntityName: string | undefined;
  onClose: () => void;
  onAudited: () => void;
  onShowPayable: (number: string) => void;
}) {
  const mayAudit = useMayMakeDecision(supplierBillAuditDecision);
  const [reload, setReload] = useState(0);
  const [lines, setLines] = useState<ApiResult<SupplierBillLinesResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLines((previous) => (reload === 0 ? null : previous));
    void getSupplierBillLines(row.claim, row.claimVersion).then((next) => {
      if (!cancelled) setLines(next);
    });
    return () => {
      cancelled = true;
    };
  }, [row.claim, row.claimVersion, reload]);

  useEffect(() => {
    setReload(0);
  }, [row.claim, row.claimVersion]);

  // 表单逐段提交、逐段说答复；至少一段记下了就重读这张账单与左边的册，表单里没记下的那几段留着改。
  const onSettled = () => {
    setReload((value) => value + 1);
    onAudited();
  };

  const supplier = namedRef(names.party(row.supplier), row.supplier);
  const supplements = row.supplementsClaim ? (legacyBillNumberOf(row.supplementsClaim, row.supplier) ?? row.supplementsClaim) : undefined;
  const billNumber = billNumberOf(row);
  const body = lines?.kind === 'outcome' ? lines.body : null;

  return (
    <ExceptionDetailPanel
      ariaLabel={`供应商账单 ${billLabel(row)}`}
      title={billLabel(row)}
      status={billProgress(row)}
      subtitle={`${supplier.primary} · 账期 ${row.period}${row.claimVersion !== '1' ? ` · 版本 ${row.claimVersion}` : ''}`}
      facts={[
        { label: '供应商', value: supplier.primary },
        { label: '开给本公司', value: legalEntityName ?? row.legalEntity },
        { label: '账单金额', value: formatMoney(row.totalClaimedAmount, row.currency) },
        { label: '对上情况', value: matchSummary(row) },
        { label: '已形成应付', value: row.payableLineCount > 0 ? `${row.payableLineCount} 行 · ${formatMoney(row.payableAmount, row.currency)}` : '还没有' },
        { label: '还没审', value: row.undecidedAmount !== '0' ? formatMoney(row.undecidedAmount, row.currency) : '没有' },
        { label: '争议中', value: row.openDisputeAmount !== '0' ? formatMoney(row.openDisputeAmount, row.currency) : '没有' },
        { label: '已拒绝', value: row.rejectedAmount !== '0' ? formatMoney(row.rejectedAmount, row.currency) : '没有' },
        { label: '收到时间', value: formatInstant(row.receivedAt) },
        { label: '登记人', value: operatorWords(row.operationEvidence) },
        { label: '追加于', value: supplements ?? '不是追加的账单' },
      ]}
      technical={technicalRows([
        { label: '供应商账单号', value: billNumber ? (row.supplierBillNumber ? billNumber : `${billNumber}（从旧主张标识取回，未进账单号册）`) : '未记' },
        { label: '主张标识', value: row.claim },
        { label: '主张版本', value: row.claimVersion },
        { label: '供应商编码', value: row.supplier },
        { label: '公司主体编码', value: row.legalEntity },
        { label: '被追加的主张', value: row.supplementsClaim },
        { label: '接收时审核授权', value: row.auditAuthorityConfigured ? '已配置' : '未配置' },
        { label: '接收操作证据', value: row.operationEvidence },
        { label: '登记时间', value: formatInstant(row.recordedAt) },
      ])}
      onClose={onClose}
    >
      {/* 看的是此刻的审核授权册（auditAuthorityConfiguredNow），审核那一刻读的也是它；接收时的快照只进技术详情。 */}
      {!row.auditAuthorityConfiguredNow ? (
        <p className={`rounded border px-3 py-2 text-idpxyz-text ${statusToneBanner.warning}`}>
          这家供应商在这个公司主体下现在还没有登记审核人（审核授权未配置）：审核会停在未配置，不形成应付。登了审核人再审。
        </p>
      ) : null}
      <section className="space-y-1.5">
        <h3 className="text-[12px] font-bold text-idpxyz-textBright">各行与去向</h3>
        {lines === null ? (
          <p>正在读这张账单的各行…</p>
        ) : lines.kind !== 'outcome' ? (
          <p>{feedbackFor(lines, '读取').text}；读不到这张账单的各行，没法审核。</p>
        ) : body?.outcome !== 'SUPPLIER_BILL_LINES_LISTED' ? (
          <p>这张账单已不在册上；刷新列表后重新点它。</p>
        ) : (
          <ul className="space-y-2">
            {(body.lines ?? []).map((line) => (
              <LineCard
                key={line.line}
                row={row}
                line={line}
                mayAudit={mayAudit}
                submit={auditSupplierBillLine}
                onSettled={onSettled}
                onShowPayable={onShowPayable}
              />
            ))}
          </ul>
        )}
      </section>
      <section className="space-y-1.5">
        <h3 className="text-[12px] font-bold text-idpxyz-textBright">审核这张账单</h3>
        {mayAudit ? (
          <p>
            逐行审核，一行可以拆成几段分别给通过、争议或拒绝：通过的那一段形成审核应付、交回{payableNumberName}，一行通过的不超过它能通过的
            金额；对不上的那部分记争议或拒绝，争议核实后在这一行再审。拒绝了的那一段这份账单上不再审，供应商要再收得另开追加账单。
          </p>
        ) : (
          <p>{hiddenDecisionHint('审核', labelFor(supplierBillAuditDecision))}</p>
        )}
      </section>
    </ExceptionDetailPanel>
  );
}
