import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { feedbackFor } from '../../components/action-feedback';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import { formatInstant } from '../catalogue-view';
import { hiddenDecisionHint, statementDisputeRegistrationDecision, statementDisputeResolutionDecision } from '../operator-grants';
import { useMayMakeDecision } from '../use-operator-grants';
import {
  getStatementLines,
  registerStatementDispute,
  resolveStatementDispute,
  type CustomerStatementRecord,
  type DisputeDecisionResponseBody,
  type DisputeResolutionPayload,
  type StatementLinesResponseBody,
} from './api';
import {
  amountProblem,
  chargesOnStatement,
  disputeLabel,
  disputeNumberAbsenceNote,
  disputeNumberName,
  instantToLocalInput,
  localInputToInstant,
  newDisputeId,
  openedAtProblem,
  openDisputesOf,
  registrationAnswerText,
  resolutionAnswerText,
  resolutionOptions,
  textProblem,
} from './dispute-forms';
import { formatMoney, technicalRows } from './ops-ui';

// 对账单页右栏：客服照客户来电或来信代登记一项争议，财务核实后处理（ADR-0354）。两张表单各按自己那一种授予收起（ADR-0315）：
// 只在明确知道没有授予时收起，不知道就摆，服务端照旧每次判。表单只对左边选中的那一张单；单的全貌与已有争议在左边的行与技术详情里。

const fieldClass = 'w-full rounded border border-idpxyz-border bg-idpxyz-editor px-2 py-1 type-body text-idpxyz-text';
const labelClass = 'block type-caption text-idpxyz-textMuted mb-0.5';

function Field({ label, problem, children }: { label: string; problem?: string | null; children: ReactNode }) {
  return (
    <label className="block">
      <span className={labelClass}>{label}</span>
      {children}
      {problem ? <span className="mt-0.5 block type-caption text-[color:var(--ds-status-warning-text,#b45309)]">{problem}</span> : null}
    </label>
  );
}

/** 一次提交的结果：服务端的答复译成一句话；失败照 feedbackFor 原样交出。 */
function AnswerNote({ answer, verb, words }: { answer: ApiResult<DisputeDecisionResponseBody> | null; verb: string; words: (body: DisputeDecisionResponseBody) => string }) {
  if (answer === null) return null;
  const text = answer.kind === 'outcome' ? words(answer.body) : feedbackFor(answer, verb).text;
  return (
    <p role="status" className="mt-2 rounded border border-idpxyz-border px-2 py-1.5 type-body leading-5 text-idpxyz-text">
      {text}
    </p>
  );
}

/** 答复里这几格说明库上已有那一项或刚落了一项：重读这一签，左边的行与右栏的待处理项跟着读口走。 */
const registrationSettled = new Set(['DISPUTE_OPENED', 'EXISTING_DISPUTE']);
const resolutionSettled = new Set(['DISPUTE_RESOLVED', 'DISPUTE_ALREADY_RESOLVED']);

function RegisterDisputeForm({
  statement,
  onAnswered,
}: {
  statement: CustomerStatementRecord;
  onAnswered: (text: string) => void;
}) {
  // 费用取这张单自己的行：目录行只带行数，按页上列的费用册又会漏掉页外的费用、认不出作废单上的行。
  const [lines, setLines] = useState<ApiResult<StatementLinesResponseBody> | null>(null);
  useEffect(() => {
    let cancelled = false;
    setLines(null);
    void getStatementLines(statement.statementNumber).then((next) => {
      if (!cancelled) setLines(next);
    });
    return () => {
      cancelled = true;
    };
  }, [statement.statementNumber]);
  const options = useMemo(() => (lines?.kind === 'outcome' ? chargesOnStatement(lines.body) : []), [lines]);
  const [charge, setCharge] = useState('');
  const [amount, setAmount] = useState('');
  const [reason, setReason] = useState('');
  const [openedAt, setOpenedAt] = useState(() => instantToLocalInput(new Date()));
  // 编号在第一次提交前定下、提交后不换：网络重发落回同一项。登记成了或换了单才换一个新的。
  const [disputeId, setDisputeId] = useState(() => newDisputeId(new Date(), Math.random().toString(36).slice(2)));
  const [touched, setTouched] = useState(false);
  const [pending, setPending] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<DisputeDecisionResponseBody> | null>(null);

  useEffect(() => {
    setCharge('');
    setAmount('');
    setAnswer(null);
    setTouched(false);
    setDisputeId(newDisputeId(new Date(), Math.random().toString(36).slice(2)));
  }, [statement.statementNumber]);

  const selected = options.find((option) => option.charge === charge);
  const openedInstant = localInputToInstant(openedAt);
  const problems = {
    charge: charge === '' ? '选一笔客户不认的费用' : null,
    amount: amountProblem(amount, selected?.amount),
    reason: textProblem(reason, '客户怎么说'),
    openedAt:
      openedInstant === null ? '填客户提出异议的时间' : openedAtProblem(openedInstant, statement.publishedAt, statement.voidedAt),
  };
  const blocked = Object.values(problems).some((problem) => problem !== null);

  const submit = async () => {
    setTouched(true);
    const at = localInputToInstant(openedAt);
    if (blocked || at === null) return;
    setPending(true);
    const result = await registerStatementDispute({
      dispute: disputeId,
      statement: statement.statementNumber,
      charge,
      disputedAmount: amount.trim(),
      reason: reason.trim(),
      openedAt: at,
    });
    setPending(false);
    setAnswer(result);
    if (result.kind === 'outcome' && registrationSettled.has(result.body.outcome)) {
      if (result.body.outcome === 'DISPUTE_OPENED') {
        setDisputeId(newDisputeId(new Date(), Math.random().toString(36).slice(2)));
        setAmount('');
        setReason('');
        setTouched(false);
      }
      onAnswered(registrationAnswerText(result.body));
    }
  };

  if (lines === null) return <p className="type-body text-idpxyz-textMuted">正在读这张单上的费用…</p>;
  if (lines.kind !== 'outcome' || options.length === 0) {
    return (
      <p className="type-body leading-5 text-idpxyz-textMuted">
        {lines.kind === 'outcome' ? '这张单上没有能选的费用行。' : `${feedbackFor(lines, '读取').text}；读不到这张单上的费用，没法选是哪一笔。`}
      </p>
    );
  }
  return (
    <form
      className="space-y-2"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <Field label="哪一笔费用" problem={touched ? problems.charge : null}>
        <select className={fieldClass} value={charge} onChange={(event) => setCharge(event.target.value)}>
          <option value="">选一笔</option>
          {options.map((option) => (
            <option key={option.charge} value={option.charge}>
              {option.charge} · {formatMoney(option.amount, option.currency)}
            </option>
          ))}
        </select>
      </Field>
      <Field label={`客户不认的金额（与单上金额同一写法${selected ? `，这笔共 ${selected.amount}` : ''}）`} problem={touched ? problems.amount : null}>
        <input className={`${fieldClass} font-mono`} inputMode="numeric" value={amount} onChange={(event) => setAmount(event.target.value)} />
      </Field>
      <Field label="客户怎么说" problem={touched ? problems.reason : null}>
        <textarea className={fieldClass} rows={3} value={reason} onChange={(event) => setReason(event.target.value)} placeholder="例：客户来电说这票实重 1.2 kg，单上按 1.5 kg 计" />
      </Field>
      {statement.voidedAt ? (
        <p className="type-caption leading-4 text-idpxyz-textMuted">
          这张单在 {formatInstant(statement.voidedAt)} 作废；作废之前客户提出的异议照常登记，作废之后提出的不再登记在这张单上。
        </p>
      ) : null}
      <Field label="客户提出的时间" problem={touched ? problems.openedAt : null}>
        <input className={fieldClass} type="datetime-local" value={openedAt} onChange={(event) => setOpenedAt(event.target.value)} />
      </Field>
      <Button type="submit" size="sm" disabled={pending}>
        {pending ? '登记中…' : '登记争议'}
      </Button>
      <AnswerNote answer={answer} verb="登记" words={registrationAnswerText} />
    </form>
  );
}

function ResolveDisputeForm({ statement, onAnswered }: { statement: CustomerStatementRecord; onAnswered: (text: string) => void }) {
  const open = openDisputesOf(statement);
  const [dispute, setDispute] = useState('');
  const [resolution, setResolution] = useState<DisputeResolutionPayload['resolution'] | ''>('');
  const [basis, setBasis] = useState('');
  const [touched, setTouched] = useState(false);
  const [pending, setPending] = useState(false);
  const [answer, setAnswer] = useState<ApiResult<DisputeDecisionResponseBody> | null>(null);

  useEffect(() => {
    setDispute('');
    setResolution('');
    setBasis('');
    setAnswer(null);
    setTouched(false);
  }, [statement.statementNumber]);

  const problems = {
    dispute: dispute === '' ? '选一项要处理的争议' : null,
    resolution: resolution === '' ? '选怎么处理' : null,
    basis: textProblem(basis, '核实了什么'),
  };
  const blocked = Object.values(problems).some((problem) => problem !== null);

  const submit = async () => {
    setTouched(true);
    if (blocked || resolution === '') return;
    setPending(true);
    const result = await resolveStatementDispute({
      dispute,
      resolution,
      basis: basis.trim(),
      resolvedAt: new Date().toISOString().replace(/\.\d{3}Z$/, 'Z'),
    });
    setPending(false);
    setAnswer(result);
    if (result.kind === 'outcome' && resolutionSettled.has(result.body.outcome)) {
      setDispute('');
      setResolution('');
      setBasis('');
      setTouched(false);
      onAnswered(resolutionAnswerText(result.body));
    }
  };

  if (open.length === 0) {
    return (
      <>
        <p className="type-body leading-5 text-idpxyz-textMuted">这张单上没有还在等处理的争议。</p>
        <AnswerNote answer={answer} verb="处理" words={resolutionAnswerText} />
      </>
    );
  }
  return (
    <form
      className="space-y-2"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <Field label="哪一项争议" problem={touched ? problems.dispute : null}>
        <select className={fieldClass} value={dispute} onChange={(event) => setDispute(event.target.value)}>
          <option value="">选一项</option>
          {open.map((item) => (
            <option key={item.dispute} value={item.dispute}>
              {disputeLabel(item)} · {item.charge} · 不认 {item.disputedAmount} · {formatInstant(item.openedAt)}
              {item.resolution === 'PENDING_REVIEW' ? ' · 待复核' : ''}
            </option>
          ))}
        </select>
      </Field>
      {dispute ? (
        <p className="type-caption leading-4 text-idpxyz-textMuted">客户说：{open.find((item) => item.dispute === dispute)?.reason}</p>
      ) : null}
      <fieldset>
        <legend className={labelClass}>怎么处理</legend>
        <div className="space-y-1">
          {resolutionOptions.map((option) => (
            <label key={option.value} className="flex items-start gap-1.5 type-body text-idpxyz-text">
              <input type="radio" name="dispute-resolution" checked={resolution === option.value} onChange={() => setResolution(option.value)} />
              <span>
                {option.label}
                <span className="block type-caption text-idpxyz-textMuted">{option.hint}</span>
              </span>
            </label>
          ))}
        </div>
        {touched && problems.resolution ? <span className="type-caption text-[color:var(--ds-status-warning-text,#b45309)]">{problems.resolution}</span> : null}
      </fieldset>
      <Field label="核实了什么" problem={touched ? problems.basis : null}>
        <textarea className={fieldClass} rows={3} value={basis} onChange={(event) => setBasis(event.target.value)} placeholder="例：调了磅单，实重 1.5 kg，原费用无误" />
      </Field>
      <Button type="submit" size="sm" disabled={pending}>
        {pending ? '处理中…' : '提交处理'}
      </Button>
      <AnswerNote answer={answer} verb="处理" words={resolutionAnswerText} />
    </form>
  );
}

/**
 * 这张单上已登记的争议：以争议号为主标识（ADR-0355 决定十一），争议标识与没有争议号的原因进技术详情。没有争议号的照旧显示争议标识，
 * 原因按争议上记下的那一种注明，不一律写成「上线前开立」。
 */
function DisputeList({ statement }: { statement: CustomerStatementRecord }) {
  if (statement.disputes.length === 0) {
    return <p className="type-body text-idpxyz-textMuted">这张单上还没有登记过争议。</p>;
  }
  return (
    <ul className="space-y-2">
      {statement.disputes.map((item) => (
        <li key={item.dispute} className="rounded border border-idpxyz-border px-2 py-1.5">
          <p className="font-mono type-body text-idpxyz-textBright">{disputeLabel(item)}</p>
          <p className="type-caption leading-4 text-idpxyz-textMuted">
            {item.charge} · 不认 {item.disputedAmount} · {item.resolution ? labelFor(item.resolution, 'dispute') : '还没有处理结果'}
          </p>
          <TechnicalDetails
            className="mt-1"
            rows={technicalRows([
              { label: '争议标识', value: item.dispute },
              { label: disputeNumberName, value: disputeNumberAbsenceNote(item) },
            ])}
          />
        </li>
      ))}
    </ul>
  );
}

/**
 * 对账单页右栏。statement 为空时只说先选一张单。gone 为真：最近一次重读的列表里已经没有这张单（比如列表只列还有争议在等的单，
 * 刚处理完的那张就出列了）——手上那一份是旧的，表单不再按它摆，只留刚才那一次的答复。
 */
export function StatementDisputePanel({
  statement,
  gone = false,
  onAnswered,
}: {
  statement: CustomerStatementRecord | undefined;
  gone?: boolean;
  onAnswered: () => void;
}) {
  const mayRegister = useMayMakeDecision(statementDisputeRegistrationDecision);
  const mayResolve = useMayMakeDecision(statementDisputeResolutionDecision);
  const [lastAnswer, setLastAnswer] = useState<{ statement: string; text: string } | null>(null);
  const answered = (text: string) => {
    if (statement) setLastAnswer({ statement: statement.statementNumber, text });
    onAnswered();
  };
  return (
    <aside className="w-full shrink-0 overflow-y-auto border-t border-idpxyz-border p-4 lg:w-[360px] lg:border-l lg:border-t-0">
      <h2 className="type-value font-bold text-idpxyz-textBright">客户对账单争议</h2>
      {statement ? (
        <p className="mt-0.5 font-mono type-caption text-idpxyz-textMuted">{statement.statementNumber}</p>
      ) : (
        <p className="mt-1 type-body leading-5 text-idpxyz-textMuted">先在左边点一张对账单，再登记客户提出的争议或处理已有的争议。</p>
      )}
      {statement && gone ? (
        <>
          {lastAnswer?.statement === statement.statementNumber ? (
            <p role="status" className="mt-3 rounded border border-idpxyz-border px-2 py-1.5 type-body leading-5 text-idpxyz-text">
              {lastAnswer.text}
            </p>
          ) : null}
          <p className="mt-3 type-body leading-5 text-idpxyz-textMuted">这张单已不在当前列表里；要再登记或处理，换个筛选后重新点它。</p>
        </>
      ) : statement ? (
        <>
          <section className="mt-4">
            <h3 className="mb-1.5 type-body font-bold text-idpxyz-textBright">这张单上的争议</h3>
            <DisputeList statement={statement} />
          </section>
          <section className="mt-5">
            <h3 className="mb-1.5 type-body font-bold text-idpxyz-textBright">登记争议</h3>
            <p className="mb-2 type-caption leading-4 text-idpxyz-textMuted">客户来电或来信说某笔费用不对，照他的话登记下来，等财务核实。</p>
            {mayRegister ? (
              <RegisterDisputeForm statement={statement} onAnswered={answered} />
            ) : (
              <p className="type-body text-idpxyz-textMuted">{hiddenDecisionHint('登记争议', labelFor(statementDisputeRegistrationDecision))}</p>
            )}
          </section>
          <section className="mt-5">
            <h3 className="mb-1.5 type-body font-bold text-idpxyz-textBright">处理争议</h3>
            <p className="mb-2 type-caption leading-4 text-idpxyz-textMuted">核实之后给出结论。接受与部分接受不在这里改金额，费用调整另做。</p>
            {mayResolve ? (
              <ResolveDisputeForm statement={statement} onAnswered={answered} />
            ) : (
              <p className="type-body text-idpxyz-textMuted">{hiddenDecisionHint('处理争议', labelFor(statementDisputeResolutionDecision))}</p>
            )}
          </section>
        </>
      ) : null}
    </aside>
  );
}
