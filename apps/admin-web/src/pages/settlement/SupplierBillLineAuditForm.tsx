import { useMemo, useState, type ReactNode } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { feedbackFor } from '../../components/action-feedback';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneText } from '../../components/status-tone';
import type { ApiResult } from './api';
import { formatMoney } from './ops-ui';
import {
  auditDraftProblems,
  firstAuditRow,
  hasAuditDraftProblems,
  layoutRanges,
  newAuditDecisionId,
  rangedAuditAnswerText,
  rangedAuditPayloads,
  rangedAuditSettled,
  totalOf,
  verdictLabel,
  type AuditDraftContext,
  type AuditRowDraft,
  type AuditVerdict,
  type RangedAuditAnswer,
  type RangedAuditPayload,
} from './supplier-bill-audit';

// 「审核这一行」与「核实后再审」共用的那张小表单（ADR-0357 决定二至四）：一行拆成几段，每段给通过、争议或拒绝、填金额与依据。
// 人只填金额，页面把各段从这一行（再审时是这段争议）里还空着的地方依次排开；每段一份决定、一次提交，各自答复——一段答不出不连累
// 已经记下的那几段（AT-SA-099）。审核决定标识在每段第一次提交前定下，重发落回同一份。

const fieldClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[12px] text-idpxyz-text focus:border-idpxyz-accent focus:outline-none';

const newId = () => newAuditDecisionId(new Date(), Math.random().toString(36).slice(2));

function Field({ label, problem, children, className }: { label: string; problem?: string | null; children: ReactNode; className?: string }) {
  return (
    <label className={`block ${className ?? ''}`}>
      <span className="mb-0.5 block text-[11px] text-idpxyz-textMuted">{label}</span>
      {children}
      {problem ? <span className={`mt-0.5 block text-[11px] ${statusToneText.warning}`}>{problem}</span> : null}
    </label>
  );
}

export interface LineAuditTarget {
  claim: string;
  version: string;
  line: string;
  /** 再审时是所解的那份争议。 */
  resolvesDispute?: string;
}

export function SupplierBillLineAuditForm({
  target,
  context,
  verdicts,
  currency,
  submit,
  onSettled,
  onDone,
  onCancel,
}: {
  target: LineAuditTarget;
  context: AuditDraftContext;
  /** 这个人能给的结果（按授予，再审只有通过与拒绝）；第一段不在其中时换成其中第一种。 */
  verdicts: readonly AuditVerdict[];
  currency: string;
  submit: (payload: RangedAuditPayload) => Promise<ApiResult<RangedAuditAnswer>>;
  /** 至少一段记下了（或本来就有）：重读这张账单。 */
  onSettled: () => void;
  /** 每一段都记下了：表单收起，答复交给这一行接着说。 */
  onDone: (answers: string[]) => void;
  onCancel: () => void;
}) {
  const [rows, setRows] = useState<AuditRowDraft[]>(() => {
    const first = firstAuditRow(context, newId());
    return [verdicts.includes(first.verdict) ? first : { ...first, verdict: verdicts[0] ?? first.verdict }];
  });
  const [touched, setTouched] = useState(false);
  const [pending, setPending] = useState(false);
  const [answers, setAnswers] = useState<string[]>([]);
  const problems = auditDraftProblems(rows, context);
  const amounts = rows.map((row) => (/^[1-9]\d*$/.test(row.amount.trim()) ? BigInt(row.amount.trim()) : 0n));
  const laid = useMemo(() => (amounts.every((amount) => amount > 0n) ? layoutRanges(context.free, amounts) : null), [context.free, amounts.join(',')]);
  const available = totalOf(context.free);

  const setRow = (index: number, patch: Partial<AuditRowDraft>) =>
    setRows((current) => current.map((row, at) => (at === index ? { ...row, ...patch } : row)));

  const addRow = () => {
    const used = amounts.reduce((sum, amount) => sum + amount, 0n);
    const rest = available > used ? available - used : 0n;
    const next: AuditVerdict = context.resolving ? 'REJECTED' : verdicts.includes('DISPUTED') ? 'DISPUTED' : verdicts[verdicts.length - 1];
    setRows((current) => [...current, { verdict: next, amount: rest > 0n ? String(rest) : '', basis: '', decision: newId() }]);
  };

  const segmentOf = (index: number) => {
    const range = laid?.[index];
    const amount = rows[index]?.amount.trim();
    return `第 ${target.line} 行${amount ? ` ${formatMoney(amount, currency)}` : ''}${range ? `（区间 ${range.from}–${range.to}）` : ''}的${verdictLabel(rows[index].verdict)}：`;
  };

  const onSubmit = async () => {
    setTouched(true);
    if (hasAuditDraftProblems(problems) || laid === null) return;
    setPending(true);
    const payloads = rangedAuditPayloads(target, rows, laid);
    const said: string[] = [];
    const left: AuditRowDraft[] = [];
    let settled = false;
    for (const [index, payload] of payloads.entries()) {
      const result = await submit(payload);
      const segment = segmentOf(index);
      if (result.kind === 'outcome') {
        said.push(rangedAuditAnswerText(result.body, segment, currency, formatMoney));
        if (rangedAuditSettled.has(result.body.outcome)) {
          settled = true;
          continue;
        }
        // 这个标识已经用在别的内容上：这一段换一个新标识留在表单里，人改好再交。
        const conflicted = result.body.outcome === 'DECISION_CONFLICT' || result.body.outcome === 'PAYABLE_CONFLICT';
        left.push(conflicted ? { ...rows[index], decision: newId() } : rows[index]);
      } else {
        said.push(`${segment}${feedbackFor(result, '审核').text}`);
        left.push(rows[index]);
      }
    }
    setPending(false);
    if (settled) onSettled();
    if (left.length === 0) {
      onDone(said);
      return;
    }
    setAnswers(said);
    setRows(left);
  };

  return (
    <form
      className="mt-1.5 space-y-2 rounded border border-idpxyz-border p-2.5"
      aria-label={context.resolving ? `第 ${target.line} 行争议再审` : `审核第 ${target.line} 行`}
      onSubmit={(event) => {
        event.preventDefault();
        void onSubmit();
      }}
    >
      <p className="text-[11px] leading-4 text-idpxyz-breadcrumb">
        {context.resolving
          ? `这段争议还没解的有 ${formatMoney(String(available), currency)}。核实之后逐段给通过或拒绝，都要写依据；没覆盖到的部分仍在争议中。`
          : `这一行还没审的有 ${formatMoney(String(available), currency)}，其中最多还能通过 ${formatMoney(String(context.headroom), currency)}。可以拆成几段分别给结果；超出能通过的部分只能争议或拒绝。`}
        金额按最小币单位写（例如 12.34 元写 1234）。
      </p>
      {rows.map((row, index) => {
        const rowProblems = problems.rows[index];
        const range = laid?.[index];
        return (
          <div key={row.decision} className="grid grid-cols-1 gap-2 sm:grid-cols-[8rem_9rem_1fr_auto] sm:items-start">
            <Field label="结果" problem={touched ? rowProblems.verdict : null}>
              <select className={fieldClass} value={row.verdict} onChange={(event) => setRow(index, { verdict: event.target.value as AuditVerdict })}>
                {verdicts.map((verdict) => (
                  <option key={verdict} value={verdict}>
                    {verdictLabel(verdict)}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="金额" problem={touched ? rowProblems.amount : null}>
              <input className={`${fieldClass} font-mono`} inputMode="numeric" value={row.amount} onChange={(event) => setRow(index, { amount: event.target.value })} />
              {range ? <span className="mt-0.5 block text-[11px] text-idpxyz-breadcrumb">{formatMoney(row.amount.trim(), currency)}</span> : null}
            </Field>
            <Field label={row.verdict === 'PASSED' && !context.resolving ? '依据（可不写）' : '依据'} problem={touched ? rowProblems.basis : null}>
              <input
                className={fieldClass}
                value={row.basis}
                placeholder={row.verdict === 'DISPUTED' ? '争的是什么、凭什么' : row.verdict === 'REJECTED' ? '为什么拒绝' : context.resolving ? '凭什么解除争议' : ''}
                onChange={(event) => setRow(index, { basis: event.target.value })}
              />
            </Field>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              className="mt-4 h-7 px-1.5"
              aria-label={`删去这一段`}
              disabled={rows.length === 1}
              onClick={() => setRows((current) => current.filter((_, at) => at !== index))}
            >
              <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
            </Button>
          </div>
        );
      })}
      {touched && problems.overall ? <p className={`text-[11px] ${statusToneText.warning}`}>{problems.overall}</p> : null}
      {answers.map((answer, index) => (
        <p key={index} role="status" className="rounded border border-idpxyz-border px-2 py-1.5 text-[12px] leading-5 text-idpxyz-text">
          {answer}
        </p>
      ))}
      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" size="sm" variant="ghost" className="h-7 text-[12px]" onClick={addRow} disabled={pending}>
          <Plus className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
          再拆一段
        </Button>
        <Button type="submit" size="sm" className={`${primaryActionClass} h-7 text-[12px]`} disabled={pending}>
          {pending ? '提交中…' : context.resolving ? '提交再审' : '提交审核'}
        </Button>
        <Button type="button" size="sm" variant="ghost" className="h-7 text-[12px]" onClick={onCancel} disabled={pending}>
          取消
        </Button>
      </div>
    </form>
  );
}
