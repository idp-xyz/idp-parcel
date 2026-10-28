import { useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { ReviewFlowTemplate, type DetailField, type ReviewDecisionOption } from '../../templates';
import { ReloginButton } from '../../auth/ReloginButton';
import { moduleInfoById } from '../../navigation';
import { Field, inputClass, Section } from './controls';
import { postIntakeDispositionDecision, postIntakeManualItemDecision } from '../operator-grants';
import { useMayMakeDecision } from '../use-operator-grants';
import {
  decidePostIntakeDisposition,
  listPostIntakeDispositionRequests,
  listPostIntakeManualItems,
  registerPostIntakeManualItemResult,
  type ApiResult,
  type ManualItemOutcome,
  type PostIntakeDirection,
  type PostIntakeDispositionRequestRow,
  type PostIntakeDispositionRequestsResponseBody,
  type PostIntakeManualItemRow,
  type PostIntakeManualItemsResponseBody,
} from './api';
import {
  decisionClearsRequest,
  decisionDraftOf,
  decisionNoteOf,
  directionBlockedReason,
  directionLabel,
  directionOptions,
  manualItemLineOf,
  manualItemKeyOf,
  manualItemsEmptyState,
  manualOutcomeOptions,
  manualResultBlockedReason,
  manualResultDraftOf,
  manualResultNoteOf,
  manualSummaryText,
  requestDetailRows,
  requestKeyOf,
  requestQueueRowOf,
  requestQueueViewState,
  requestSummaryText,
  type OperatorNote,
} from './post-intake-disposition';

const info = moduleInfoById['post-intake-disposition'];

function NoteLine({ note }: { note: OperatorNote | null }) {
  if (!note) return null;
  return (
    <p className={`text-[12px] ${note.tone === 'outcome' ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>
      {note.text}
      {note.tone === 'refused' && note.kind === 'loginRequired' ? <ReloginButton size="inline" /> : null}
    </p>
  );
}

/**
 * 收寄后处置(UC-PS-006;ADR-0332、ADR-0341)。上半是等回应的停止请求与收寄后服务处置决定(去向五选一);
 * 下半是决定留下的待人工处理项:登记已处理 / 无法处理,并可按包裹读回各项结果。
 *
 * 写入口按授予收起(ADR-0315):处置决定与待人工处理项登记是两种分开授予的运营决定(ADR-0341 决定三)。
 * 摆不摆只是呈现,每个写口照旧由服务端判授予。取消先于收寄的入口在「取消与收寄后处置」页,不在这里。
 */
export function PostIntakeDispositionPage() {
  const mayDecide = useMayMakeDecision(postIntakeDispositionDecision);
  const mayRegister = useMayMakeDecision(postIntakeManualItemDecision);

  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<PostIntakeDispositionRequestsResponseBody> | null>(null);
  const [pending, setPending] = useState(false);
  const [note, setNote] = useState<OperatorNote | null>(null);
  const [destination, setDestination] = useState('');

  useEffect(() => {
    let cancelled = false;
    void listPostIntakeDispositionRequests().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const requests = body?.requests ?? [];
  const selected: PostIntakeDispositionRequestRow | null = requests.find((row) => requestKeyOf(row) === selectedId) ?? null;
  const retry = () => setReloadKey((value) => value + 1);
  // 形成决定会让待人工处理项多出几项,下半部随之重读。
  const [itemsReloadKey, setItemsReloadKey] = useState(0);

  const decisions: ReviewDecisionOption<PostIntakeDirection>[] = mayDecide
    ? directionOptions.map((option) => {
        const blocked = directionBlockedReason(option.id, destination);
        return { id: option.id, label: option.label, variant: option.variant, disabled: blocked !== undefined, disabledReason: blocked };
      })
    : [];

  const onDecide = ({ itemId, decision, reason }: { itemId: string; decision: PostIntakeDirection; reason: string }) => {
    const row = requests.find((candidate) => requestKeyOf(candidate) === itemId);
    if (!row) return;
    const label = `收寄后服务处置决定:${directionLabel(decision)}`;
    setPending(true);
    setNote(null);
    void decidePostIntakeDisposition(decisionDraftOf(row, decision, reason, destination)).then((result) => {
      setPending(false);
      setNote(decisionNoteOf(label, result));
      if (result.kind === 'outcome') {
        if (decisionClearsRequest(result)) setSelectedId(null);
        setDestination('');
        retry();
        setItemsReloadKey((value) => value + 1);
      }
    });
  };

  const detailFields: DetailField[] | undefined = selected
    ? requestDetailRows(selected).map((r) => ({ label: r.label, value: r.mono || r.title ? <span className={r.mono ? 'font-mono' : undefined} title={r.title}>{r.value}</span> : r.value }))
    : undefined;

  return (
    <div className="flex flex-1 flex-col overflow-auto">
      <div className="flex min-h-[460px] flex-col">
        <ReviewFlowTemplate<PostIntakeDirection>
          title={info.title}
          description={`客户在取消边界之后提出的停止请求,等有权主体形成收寄后服务处置决定。${body ? requestSummaryText(body) : ''}`}
          queueTitle="等回应的请求"
          queue={requests.map((row) => {
            const line = requestQueueRowOf(row);
            return {
              id: line.id,
              title: line.title,
              subtitle: line.subtitle,
              status: <span className="shrink-0 text-[11px] text-idpxyz-textMuted">{line.statusText}</span>,
              meta: line.meta,
            };
          })}
          selectedId={selectedId}
          onSelect={(id) => {
            setSelectedId(id);
            setDestination('');
          }}
          detailTitle="请求详情"
          detailFields={detailFields}
          detailExtra={
            <div className="flex flex-col gap-3">
              {mayDecide ? (
                <>
                  <Field label="改送目的地" hint="只有「改送」带目的地,别的去向不带。">
                    <input
                      className={inputClass}
                      value={destination}
                      onChange={(event) => setDestination(event.target.value)}
                      placeholder="改送的目的地"
                    />
                  </Field>
                  <p className="text-[12px] text-idpxyz-textMuted">
                    「复核理由」就是决定的依据,必填。决定只追加、不可撤销;之后要改走向,请基于链尾再形成一份新的收寄后服务处置决定。
                  </p>
                </>
              ) : (
                <p className="text-[12px] text-idpxyz-textMuted">收寄后服务处置决定的入口只对持该运营决定授予的操作者显示。</p>
              )}
              <NoteLine note={note} />
            </div>
          }
          decisions={decisions}
          decisionPending={pending}
          onDecide={onDecide}
          viewState={requestQueueViewState(answer, retry, info)}
        />
      </div>
      <ManualItemsSection mayRegister={mayRegister} reloadKey={itemsReloadKey} />
    </div>
  );
}

function ManualItemsSection({ mayRegister, reloadKey }: { mayRegister: boolean; reloadKey: number }) {
  const [localReload, setLocalReload] = useState(0);
  const [parcelInput, setParcelInput] = useState('');
  const [parcel, setParcel] = useState<string | undefined>(undefined);
  const [answer, setAnswer] = useState<ApiResult<PostIntakeManualItemsResponseBody> | null>(null);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [outcome, setOutcome] = useState<ManualItemOutcome>('HANDLED');
  const [noteText, setNoteText] = useState('');
  const [blocked, setBlocked] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [note, setNote] = useState<OperatorNote | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listPostIntakeManualItems(parcel).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [parcel, reloadKey, localReload]);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const items: PostIntakeManualItemRow[] = body?.items ?? [];
  const selected = items.find((item) => manualItemKeyOf(item) === selectedKey) ?? null;

  const register = () => {
    if (!selected) return;
    const reason = manualResultBlockedReason(noteText);
    setBlocked(reason ?? null);
    if (reason) return;
    const label = `登记${outcome === 'HANDLED' ? '已处理' : '无法处理'}`;
    setSending(true);
    setNote(null);
    void registerPostIntakeManualItemResult(manualResultDraftOf(selected, outcome, noteText)).then((result) => {
      setSending(false);
      setNote(manualResultNoteOf(label, result));
      if (result.kind === 'outcome' || (result.kind === 'callerProblem' && result.status === 409)) {
        setNoteText('');
        setLocalReload((value) => value + 1);
      }
    });
  };

  return (
    <div className="px-4 pb-6">
      <Section title="待人工处理项">
        <p className="mb-3 text-[12px] text-idpxyz-textMuted">
          收寄后服务处置决定里系统不自动执行的请求,要运营人员在系统外办完后在这里登记结果。「已处理」与「无法处理」都是承接结果;无法处理时当前控制责任不因此消失。
          被后来的决定取代的项不会消失,办之前先看链尾决定说了什么。
        </p>
        <div className="mb-3 flex flex-wrap items-end gap-2">
          <div className="w-[260px]">
            <Field label="按包裹读回">
              <input className={inputClass} value={parcelInput} onChange={(event) => setParcelInput(event.target.value)} placeholder="包裹标识" />
            </Field>
          </div>
          <Button
            variant="secondary"
            onClick={() => {
              setParcel(parcelInput.trim() || undefined);
              setSelectedKey(null);
            }}
          >
            读回
          </Button>
          {parcel && (
            <Button
              variant="ghost"
              onClick={() => {
                setParcelInput('');
                setParcel(undefined);
                setSelectedKey(null);
              }}
            >
              回到全部待办
            </Button>
          )}
        </div>
        {!answer && <p className="text-[12px] text-idpxyz-textMuted">读取中…</p>}
        {answer && answer.kind !== 'outcome' && (
          <p className="text-[12px] text-idpxyz-textMuted">
            待人工处理项读不到({answer.kind === 'callerProblem' || answer.kind === 'noAnswer' ? `HTTP ${answer.status} ${answer.code}` : answer.kind === 'transport' ? answer.message : answer.kind})。这不是「没有待人工处理项」。
          </p>
        )}
        {body && <p className="mb-2 text-[12px] text-idpxyz-text">{manualSummaryText(body, parcel)}</p>}
        {body && items.length === 0 && (
          <p className="text-[12px] text-idpxyz-textMuted">{parcel ? manualItemsEmptyState.readback : manualItemsEmptyState.pending}</p>
        )}
        <ul className="flex flex-col gap-2">
          {items.map((item) => {
            const line = manualItemLineOf(item);
            const active = line.id === selectedKey;
            return (
              <li key={line.id} className={`rounded border p-3 ${active ? 'border-idpxyz-accent' : 'border-idpxyz-border'}`}>
                <button type="button" className="block w-full text-left" onClick={() => setSelectedKey(line.id)}>
                  <div className="font-mono text-[12px] text-idpxyz-textBright">{line.title}</div>
                  <div className="text-[12px] text-idpxyz-text">{line.detail}</div>
                  {line.superseded && <div className="text-[11px] text-idpxyz-accent">{line.supersededText}</div>}
                  <div className="text-[11px] text-idpxyz-textMuted" title={line.resultTitle || undefined}>{line.resultText ? `结果:${line.resultText}` : '尚未登记结果'}</div>
                </button>
                {active && !item.result && (
                  mayRegister ? (
                    <div className="mt-3 flex flex-col gap-2">
                      <Field label="处理结果" required>
                        <select className={inputClass} value={outcome} onChange={(event) => setOutcome(event.target.value as ManualItemOutcome)}>
                          {manualOutcomeOptions.map((option) => (
                            <option key={option.id} value={option.id}>
                              {option.label}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <Field label="说明" required error={blocked ?? undefined} hint="做了什么,或为什么办不了;随结果留痕。">
                        <textarea className={inputClass} rows={3} value={noteText} onChange={(event) => setNoteText(event.target.value)} />
                      </Field>
                      <div>
                        <Button variant="default" disabled={sending} onClick={register}>
                          登记结果
                        </Button>
                        {sending && <span className="ml-2 text-[11px] text-idpxyz-textMuted">提交中…</span>}
                      </div>
                    </div>
                  ) : (
                    <p className="mt-2 text-[12px] text-idpxyz-textMuted">登记结果的入口只对持待人工处理项登记授予的操作者显示。</p>
                  )
                )}
              </li>
            );
          })}
        </ul>
        <div className="mt-2">
          <NoteLine note={note} />
        </div>
      </Section>
    </div>
  );
}
