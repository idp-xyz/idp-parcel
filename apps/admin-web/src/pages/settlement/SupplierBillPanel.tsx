import { useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { feedbackFor } from '../../components/action-feedback';
import { primaryActionClass } from '../../components/primary-action';
import { StatusPill } from '../../components/StatusPill';
import { statusToneBanner } from '../../components/status-tone';
import { labelFor } from '../../vocabulary/status';
import { formatInstant } from '../moment';
import { hiddenDecisionHint, supplierBillAuditDecision } from '../operator-grants';
import { useMayMakeDecision } from '../use-operator-grants';
import { ExceptionDetailPanel } from '../visibility/ExceptionDetailPanel';
import {
  auditSupplierBillLine,
  getSupplierBillLines,
  type ApiResult,
  type SupplierBillAuditResponseBody,
  type SupplierBillLineRecord,
  type SupplierBillLinesResponseBody,
  type SupplierBillReceptionRecord,
} from './api';
import { formatMoney, technicalRows } from './ops-ui';
import { namedRef, type PartyNames } from './party-names';
import {
  auditAnswerText,
  auditSettled,
  billLabel,
  billNumberOf,
  billProgress,
  differingClassificationWords,
  lineDisposition,
  matchSummary,
  newPayableId,
  operatorWords,
  payableLabel,
  payableNumberAbsenceNote,
  payableNumberName,
} from './supplier-bills';

// 供应商账单与应付页「供应商账单」签的右栏：这张账单的全貌、各行与它们的去向，以及「审核这张账单」——对已对上、还没有应付的
// 一行点审核通过，形成审核应付（ADR-0340 决定二）。审核入口按「供应商账单审核」授予收起（ADR-0315）：只在明确知道没有授予
// 时收起，不知道就摆，服务端照旧每次判。
//
// 审核只摆「通过」：领域里供应商账单行的争议与拒绝没有口（ADR-0340 越权风险点一），对不上的行这里只说要先核实，不另造
// 一个按钮。审核也不改数字：应付金额就是这一行的主张金额（domain.FormAuditedPayable）。

/** 差额是主张减预期：正为账单多收，负为少收；没有差额不说。 */
function varianceWords(variance: string | undefined, currency: string): string {
  if (!variance || variance === '0') return '';
  return variance.startsWith('-') ? `，账单少收 ${formatMoney(variance.slice(1), currency)}` : `，账单多收 ${formatMoney(variance, currency)}`;
}

function LineCard({
  line,
  currency,
  mayAudit,
  pending,
  onAudit,
  onShowPayable,
}: {
  line: SupplierBillLineRecord;
  currency: string;
  mayAudit: boolean;
  pending: boolean;
  onAudit: () => void;
  onShowPayable: (number: string) => void;
}) {
  const disposition = lineDisposition(line);
  const note = line.payable ? payableNumberAbsenceNote(line.payable) : undefined;
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
      </p>
      {line.basis ? <p className="text-[11px] leading-4 text-idpxyz-breadcrumb">依据：{line.basis}</p> : null}
      {line.payable ? (
        <div className="mt-1.5 flex flex-wrap items-center gap-2 text-[12px]">
          <span>
            应付 <span className="font-mono font-semibold text-idpxyz-textBright">{payableLabel(line.payable)}</span>
            {note ? <span className="text-idpxyz-breadcrumb">（{note}）</span> : null}
            <span className="text-idpxyz-breadcrumb"> · {formatInstant(line.payable.auditedAt)} 由 {operatorWords(line.payable.operationEvidence)} 审核</span>
          </span>
          <Button type="button" size="sm" variant="ghost" className="h-6 px-1.5 text-[12px] text-idpxyz-link" onClick={() => onShowPayable(payableLabel(line.payable!))}>
            在「审核应付」里看
          </Button>
        </div>
      ) : disposition.auditable ? (
        mayAudit ? (
          <div className="mt-1.5 flex flex-wrap items-center gap-2">
            <Button type="button" size="sm" className={`${primaryActionClass} h-7 text-[12px]`} disabled={pending} onClick={onAudit}>
              {pending ? '审核中…' : '审核通过，形成应付'}
            </Button>
            <span className="text-[11px] text-idpxyz-breadcrumb">应付金额就是这一行的金额，审核不改数字。</span>
          </div>
        ) : null
      ) : line.classification ? (
        <p className="mt-1 text-[11px] leading-4 text-idpxyz-breadcrumb">
          这一行和预期成本对不上，不能直接审核；先和供应商核实。系统里还没有登记供应商账单争议或拒绝的入口。
        </p>
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
  // 每一行的应付标识在第一次提交前定下、提交后不换：网络重发落回同一份应付。形成了（或答已形成过）那一行就不再摆审核。
  const [payableIds, setPayableIds] = useState<Record<string, string>>({});
  const [pendingLine, setPendingLine] = useState<string | null>(null);
  const [answer, setAnswer] = useState<{ line: string; result: ApiResult<SupplierBillAuditResponseBody> } | null>(null);

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
    setPayableIds({});
    setAnswer(null);
    setReload(0);
  }, [row.claim, row.claimVersion]);

  const payableIdOf = (line: string) => payableIds[line] ?? newPayableId(new Date(), Math.random().toString(36).slice(2));

  const audit = async (line: string) => {
    const payable = payableIdOf(line);
    setPayableIds((current) => ({ ...current, [line]: payable }));
    setPendingLine(line);
    const result = await auditSupplierBillLine({ claim: row.claim, version: row.claimVersion, line, payable });
    setPendingLine(null);
    setAnswer({ line, result });
    if (result.kind === 'outcome' && auditSettled.has(result.body.outcome)) {
      setReload((value) => value + 1);
      onAudited();
    }
    // 这个应付标识已经用在另一行上：换一个新的再审。
    if (result.kind === 'outcome' && result.body.outcome === 'PAYABLE_CONFLICT') {
      setPayableIds((current) => ({ ...current, [line]: newPayableId(new Date(), Math.random().toString(36).slice(2)) }));
    }
  };

  const supplier = namedRef(names.party(row.supplier), row.supplier);
  const supplements = row.supplementsClaim ? (billNumberOf(row.supplementsClaim, row.supplier) ?? row.supplementsClaim) : undefined;
  const body = lines?.kind === 'outcome' ? lines.body : null;
  const answerText =
    answer === null ? null : answer.result.kind === 'outcome' ? auditAnswerText(answer.result.body, answer.line, formatMoney) : feedbackFor(answer.result, '审核').text;

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
        { label: '收到时间', value: formatInstant(row.receivedAt) },
        { label: '登记人', value: operatorWords(row.operationEvidence) },
        { label: '追加于', value: supplements ?? '不是追加的账单' },
      ]}
      technical={technicalRows([
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
                line={line}
                currency={row.currency}
                mayAudit={mayAudit}
                pending={pendingLine === line.line}
                onAudit={() => void audit(line.line)}
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
            对上了的行逐行审核：通过就形成审核应付，交回{payableNumberName}；对不上的行（{differingClassificationWords()}）要先核实，不能在这里审核。
          </p>
        ) : (
          <p>{hiddenDecisionHint('审核', labelFor(supplierBillAuditDecision))}</p>
        )}
        {answerText ? (
          <p role="status" className="rounded border border-idpxyz-border px-2 py-1.5 text-[12px] leading-5 text-idpxyz-text">
            {answerText}
          </p>
        ) : null}
      </section>
    </ExceptionDetailPanel>
  );
}
