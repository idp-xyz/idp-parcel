import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { Plus, Trash2, X } from 'lucide-react';
import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@idpxyz/ui-primitives';
import { feedbackFor } from '../../components/action-feedback';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneText } from '../../components/status-tone';
import { formatInstant } from '../moment';
import { listBusinessParties, type BusinessPartyRecord } from '../party/api';
import { listCurrencies, type CurrencyRecord } from '../reference-data/api';
import {
  listSettlementCharges,
  receiveSupplierBill,
  type ApiResult,
  type SupplierBillReceptionRecord,
  type SupplierBillReceptionResponseBody,
  type SupplierExpectedCostRecord,
} from './api';
import { instantToLocalInput, localInputToInstant } from './dispute-forms';
import { formatMoney } from './ops-ui';
import {
  billLabel,
  classificationChoices,
  currentExpectedCosts,
  hasReceptionProblems,
  NO_EXPECTED_COST,
  receptionAnswerText,
  receptionPayload,
  receptionProblems,
  receptionSettled,
  type ReceptionDraft,
  type ReceptionLineDraft,
} from './supplier-bills';

// 「登记供应商账单」任务式表单（ADR-0340 决定一；票 document-numbering/11）：照供应商来件逐格录一份账单，并逐行说清对的是哪一笔
// 预期成本、对上了没有。按 UC-SA-004 输入语义收：供应商、开给本公司哪个主体、供应商的账单号与版本、账期、币种、收到的时间、
// 各行金额与依据；被追加的原账单可缺。主张标识由页面从供应商与账单号生成（supplier-bills.ts 的 claimIdFor），不让人填。
// 入口按「供应商账单接收」授予收起，在页面里判（ADR-0315）。

const fieldClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[12px] text-idpxyz-text focus:border-idpxyz-accent focus:outline-none';

function Field({ label, problem, children, className }: { label: string; problem?: string | null; children: ReactNode; className?: string }) {
  return (
    <label className={`block ${className ?? ''}`}>
      <span className="mb-0.5 block text-[11px] text-idpxyz-textMuted">{label}</span>
      {children}
      {problem ? <span className={`mt-0.5 block text-[11px] ${statusToneText.warning}`}>{problem}</span> : null}
    </label>
  );
}

const emptyLine = (): ReceptionLineDraft => ({ expected: '', classification: '', claimed: '', feeItem: '', basis: '' });

const emptyDraft = (): Omit<ReceptionDraft, 'receivedAt'> & { receivedLocal: string } => ({
  supplier: '',
  legalEntity: '',
  billNumber: '',
  version: '1',
  period: '',
  currency: '',
  receivedLocal: instantToLocalInput(new Date()),
  supplementsClaim: '',
  lines: [emptyLine()],
});

interface Catalogues {
  parties: BusinessPartyRecord[];
  currencies: CurrencyRecord[];
  costs: SupplierExpectedCostRecord[];
  problem: string | null;
}

/** 表单要的几本册：往来单位、币种、供应商预期成本（公司主体由页面读好传进来）。哪本读不到就如实说，不拿空表冒充「没有」。 */
function useReceptionCatalogues(open: boolean): Catalogues | null {
  const [catalogues, setCatalogues] = useState<Catalogues | null>(null);
  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setCatalogues(null);
    void Promise.all([listBusinessParties(), listCurrencies(), listSettlementCharges('supplier-expected-cost')]).then(
      ([parties, currencies, costs]) => {
        if (cancelled) return;
        const failed = [
          parties.kind === 'outcome' ? null : `往来单位（${feedbackFor(parties, '读取').text}）`,
          currencies.kind === 'outcome' ? null : `币种（${feedbackFor(currencies, '读取').text}）`,
          costs.kind === 'outcome' ? null : `供应商预期成本（${feedbackFor(costs, '读取').text}）`,
        ].filter((item): item is string => item !== null);
        setCatalogues({
          parties: parties.kind === 'outcome' ? parties.body.parties.filter((party) => !party.deactivatedAt) : [],
          currencies: currencies.kind === 'outcome' ? currencies.body.currencies : [],
          costs: costs.kind === 'outcome' ? costs.body.costs : [],
          problem: failed.length > 0 ? `读不到${failed.join('、')}，这几格没法选。` : null,
        });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [open]);
  return catalogues;
}

function ReceptionForm({
  catalogues,
  legalEntities,
  bills,
  onReceived,
}: {
  catalogues: Catalogues;
  legalEntities: readonly { id: string; name: string }[];
  bills: readonly SupplierBillReceptionRecord[];
  onReceived: (claim: string, version: string) => void;
}) {
  const [draft, setDraft] = useState(emptyDraft);
  const [touched, setTouched] = useState(false);
  const [pending, setPending] = useState(false);
  const [answer, setAnswer] = useState<{ result: ApiResult<SupplierBillReceptionResponseBody>; billNumber: string } | null>(null);

  const costsByVersion = useMemo(() => new Map(catalogues.costs.map((cost) => [cost.version, cost])), [catalogues.costs]);
  const options = useMemo(() => currentExpectedCosts(catalogues.costs, draft.currency), [catalogues.costs, draft.currency]);
  const supplementOptions = bills.filter((bill) => bill.supplier === draft.supplier);
  const fullDraft: ReceptionDraft = { ...draft, receivedAt: localInputToInstant(draft.receivedLocal) };
  const problems = receptionProblems(fullDraft, costsByVersion);
  const blocked = hasReceptionProblems(problems);
  const total = draft.lines.reduce((sum, line) => (/^\d+$/.test(line.claimed.trim()) ? sum + BigInt(line.claimed.trim()) : sum), 0n);

  const set = <Key extends keyof typeof draft>(key: Key, value: (typeof draft)[Key]) => {
    setDraft((current) => ({ ...current, [key]: value }));
    setAnswer(null);
  };
  // 一行改了预期成本或金额，对上方式跟着这一行能选的那几种走：只剩一种能选（没有预期成本）或金额正好相等时替人选上，原来选的
  // 不在能选的里就清掉，不留一个匹配门会拒的选择。
  const setLine = (index: number, patch: Partial<ReceptionLineDraft>) => {
    setDraft((current) => {
      const lines = current.lines.map((line, at) => {
        if (at !== index) return line;
        const next = { ...line, ...patch };
        const choices = classificationChoices(next.expected, next.claimed, costsByVersion.get(next.expected)?.settlementAmount);
        if (!choices.some((option) => option.value === next.classification)) next.classification = '';
        if (next.classification === '' && (choices.length === 1 || choices[0]?.value === 'MATCHED')) next.classification = choices[0].value;
        return next;
      });
      return { ...current, lines };
    });
    setAnswer(null);
  };

  const submit = async () => {
    setTouched(true);
    if (blocked) return;
    setPending(true);
    const payload = receptionPayload(fullDraft, costsByVersion);
    const result = await receiveSupplierBill(payload);
    setPending(false);
    setAnswer({ result, billNumber: draft.billNumber });
    if (result.kind === 'outcome' && receptionSettled.has(result.body.outcome)) {
      onReceived(payload.claim, payload.version);
    }
  };

  const settled = answer?.result.kind === 'outcome' && receptionSettled.has(answer.result.body.outcome);
  const answerText =
    answer === null
      ? null
      : answer.result.kind === 'outcome'
        ? receptionAnswerText(answer.result.body, answer.billNumber)
        : feedbackFor(answer.result, '登记').text;

  if (settled) {
    return (
      <div className="space-y-3">
        <p role="status" className="rounded border border-idpxyz-border px-3 py-2 text-[12px] leading-5 text-idpxyz-text">
          {answerText}
        </p>
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="text-[12px] text-idpxyz-link"
          onClick={() => {
            setDraft(emptyDraft());
            setAnswer(null);
            setTouched(false);
          }}
        >
          再登一张
        </Button>
      </div>
    );
  }

  return (
    <form
      className="space-y-3"
      aria-label="登记供应商账单"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      {catalogues.problem ? <p className={`text-[12px] ${statusToneText.warning}`}>{catalogues.problem}</p> : null}
      <div className="grid grid-cols-1 gap-2.5 sm:grid-cols-2">
        <Field label="供应商" problem={touched ? problems.fields.supplier : null}>
          <select className={fieldClass} value={draft.supplier} onChange={(event) => set('supplier', event.target.value)}>
            <option value="">选一家</option>
            {catalogues.parties.map((party) => (
              <option key={party.partyId} value={party.partyId}>
                {party.partyName}（{party.partyId}）
              </option>
            ))}
          </select>
        </Field>
        <Field label="开给本公司哪个主体" problem={touched ? problems.fields.legalEntity : null}>
          <select className={fieldClass} value={draft.legalEntity} onChange={(event) => set('legalEntity', event.target.value)}>
            <option value="">选一个</option>
            {legalEntities.map((entity) => (
              <option key={entity.id} value={entity.id}>
                {entity.name}（{entity.id}）
              </option>
            ))}
          </select>
        </Field>
        <Field label="供应商的账单号（照账单上的号填）" problem={touched ? problems.fields.billNumber : null}>
          <input className={`${fieldClass} font-mono`} value={draft.billNumber} onChange={(event) => set('billNumber', event.target.value)} />
        </Field>
        <Field label="版本（第一次来件填 1；供应商更正后重发同一个号填 2、3…）" problem={touched ? problems.fields.version : null}>
          <input className={`${fieldClass} font-mono`} value={draft.version} onChange={(event) => set('version', event.target.value)} />
        </Field>
        <Field label="账期（照账单上写的）" problem={touched ? problems.fields.period : null}>
          <input className={fieldClass} value={draft.period} placeholder="例：2026-10" onChange={(event) => set('period', event.target.value)} />
        </Field>
        <Field label="币种" problem={touched ? problems.fields.currency : null}>
          <select className={fieldClass} value={draft.currency} onChange={(event) => set('currency', event.target.value)}>
            <option value="">选一种</option>
            {catalogues.currencies.map((currency) => (
              <option key={currency.code} value={currency.code}>
                {currency.nameZh}（{currency.code}）
              </option>
            ))}
          </select>
        </Field>
        <Field label="收到账单的时间" problem={touched ? problems.fields.receivedAt : null}>
          <input className={fieldClass} type="datetime-local" value={draft.receivedLocal} onChange={(event) => set('receivedLocal', event.target.value)} />
        </Field>
        <Field label="是对哪张账单的追加或更正（没有就不选）">
          <select
            className={fieldClass}
            value={draft.supplementsClaim}
            disabled={draft.supplier === ''}
            onChange={(event) => set('supplementsClaim', event.target.value)}
          >
            <option value="">不是追加</option>
            {supplementOptions.map((bill) => (
              <option key={`${bill.claim}:${bill.claimVersion}`} value={bill.claim}>
                {billLabel(bill)} · {bill.period}
              </option>
            ))}
          </select>
        </Field>
      </div>

      <fieldset className="space-y-2">
        <legend className="mb-1 text-[12px] font-semibold text-idpxyz-textBright">账单各行</legend>
        <p className="text-[11px] leading-4 text-idpxyz-breadcrumb">
          每一行说清对的是我们记的哪一笔预期成本、对上了没有。金额按最小币单位写整数（例如 12.34 元写 1234），与本页其他金额同一写法。
          {draft.currency === '' ? '先选币种，预期成本只列同币种的。' : `只列 ${draft.currency} 的当前预期成本；不同币种没有换算依据，对不上。`}
        </p>
        {touched && problems.fields.lines ? <p className={`text-[11px] ${statusToneText.warning}`}>{problems.fields.lines}</p> : null}
        {draft.lines.map((line, index) => {
          const lineProblems = problems.lines[index];
          const expected = costsByVersion.get(line.expected);
          const choices = classificationChoices(line.expected, line.claimed, expected?.settlementAmount);
          return (
            <div key={index} className="space-y-2 rounded border border-idpxyz-border p-2.5">
              <div className="flex items-center justify-between">
                <span className="text-[12px] font-semibold text-idpxyz-textBright">第 {index + 1} 行</span>
                {draft.lines.length > 1 ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon"
                    aria-label={`删掉第 ${index + 1} 行`}
                    onClick={() => set('lines', draft.lines.filter((_, at) => at !== index))}
                  >
                    <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
                  </Button>
                ) : null}
              </div>
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                <Field label="对的是哪一笔预期成本" problem={touched ? lineProblems.expected : null} className="sm:col-span-2">
                  <select className={fieldClass} value={line.expected} onChange={(event) => setLine(index, { expected: event.target.value })}>
                    <option value="">选一笔</option>
                    {options.map((cost) => (
                      <option key={cost.version} value={cost.version}>
                        {formatMoney(cost.settlementAmount, cost.settlementCurrency)} · {cost.feeItem} · {formatInstant(cost.occurredAt)} · {cost.agreement}
                      </option>
                    ))}
                    <option value={NO_EXPECTED_COST}>找不到对应运输（我们这边没有这笔）</option>
                  </select>
                </Field>
                <Field label="账单上这一行的金额" problem={touched ? lineProblems.claimed : null}>
                  <input
                    className={`${fieldClass} font-mono`}
                    inputMode="numeric"
                    value={line.claimed}
                    onChange={(event) => setLine(index, { claimed: event.target.value })}
                  />
                </Field>
              </div>
              {expected ? (
                <p className="text-[11px] text-idpxyz-breadcrumb">
                  我们记的是 {formatMoney(expected.settlementAmount, expected.settlementCurrency)}（费用项目 {expected.feeItem}）。
                </p>
              ) : null}
              {line.expected === NO_EXPECTED_COST ? (
                <Field label="费用项目（照账单上这一行的收费项填）" problem={touched ? lineProblems.feeItem : null}>
                  <input className={`${fieldClass} font-mono`} value={line.feeItem} onChange={(event) => setLine(index, { feeItem: event.target.value })} />
                </Field>
              ) : null}
              {choices.length > 0 ? (
                <fieldset>
                  <legend className="mb-0.5 text-[11px] text-idpxyz-textMuted">对上了没有</legend>
                  <div className="flex flex-wrap gap-x-4 gap-y-1">
                    {choices.map((option) => (
                      <label key={option.value} className="flex items-start gap-1.5 text-[12px] text-idpxyz-text" title={option.hint}>
                        <input
                          type="radio"
                          name={`line-${index}-classification`}
                          checked={line.classification === option.value}
                          onChange={() => setLine(index, { classification: option.value })}
                        />
                        <span>
                          {option.label}
                          <span className="block text-[11px] text-idpxyz-textMuted">{option.hint}</span>
                        </span>
                      </label>
                    ))}
                  </div>
                  {touched && lineProblems.classification ? (
                    <span className={`text-[11px] ${statusToneText.warning}`}>{lineProblems.classification}</span>
                  ) : null}
                </fieldset>
              ) : null}
              {line.classification !== '' && line.classification !== 'MATCHED' ? (
                <Field label="依据：怎么核的" problem={touched ? lineProblems.basis : null}>
                  <input
                    className={fieldClass}
                    value={line.basis}
                    placeholder="例：协议单价 18 元，账单按 20 元计"
                    onChange={(event) => setLine(index, { basis: event.target.value })}
                  />
                </Field>
              ) : null}
            </div>
          );
        })}
        <Button type="button" size="sm" variant="ghost" className="gap-1 text-[12px] text-idpxyz-link" onClick={() => set('lines', [...draft.lines, emptyLine()])}>
          <Plus className="h-3.5 w-3.5" aria-hidden="true" />
          加一行
        </Button>
      </fieldset>

      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" size="sm" className={primaryActionClass} disabled={pending}>
          {pending ? '登记中…' : '登记账单'}
        </Button>
        <span className="text-[12px] text-idpxyz-breadcrumb">
          {draft.lines.length} 行 · 合计 {formatMoney(total.toString(), draft.currency)}
        </span>
      </div>
      {answerText ? (
        <p role="status" className="rounded border border-idpxyz-border px-3 py-2 text-[12px] leading-5 text-idpxyz-text">
          {answerText}
        </p>
      ) : null}
    </form>
  );
}

export function SupplierBillReceptionDialog({
  open,
  onOpenChange,
  bills,
  legalEntities,
  onReceived,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  bills: readonly SupplierBillReceptionRecord[];
  legalEntities: readonly { id: string; name: string }[];
  onReceived: (claim: string, version: string) => void;
}) {
  const catalogues = useReceptionCatalogues(open);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="max-h-[90vh] overflow-y-auto">
        <DialogHeader className="items-start">
          <div>
            <DialogTitle>登记供应商账单</DialogTitle>
            <DialogDescription className="mt-1">
              照供应商发来的账单逐格录入。登记只是收下账单、记下每一行对上了没有，不形成应付；对上的行登记后在右栏审核。
            </DialogDescription>
          </div>
          <Button type="button" variant="ghost" size="icon" aria-label="关闭" onClick={() => onOpenChange(false)}>
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </DialogHeader>
        <DialogBody className="text-[12px] leading-5 text-idpxyz-text">
          {catalogues === null ? <p>正在读供应商、公司主体、币种与预期成本…</p> : <ReceptionForm catalogues={catalogues} legalEntities={legalEntities} bills={bills} onReceived={onReceived} />}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            关闭
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
