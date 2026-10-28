import { useEffect, useState } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { ReviewFlowTemplate, type DetailField, type ReviewDecisionOption } from '../../templates';
import { ReloginButton } from '../../auth/ReloginButton';
import { TechnicalDetails } from '../../components/states';
import { moduleInfoById } from '../../navigation';
import { Field, inputClass, Section } from './controls';
import { ParcelNo } from './refs';
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
  requestTechnicalRows,
  type OperatorNote,
} from './post-intake-disposition';

// 页面标题取导航登记的那一份;主责上下文(parcel-shipment)只在代码里出现,屏上不说系统的话。
const info = moduleInfoById['post-intake-disposition'];

function NoteLine({ note }: { note: OperatorNote | null }) {
  if (!note) return null;
  return (
    <div className={`text-[12px] ${note.tone === 'outcome' ? 'text-idpxyz-text' : 'text-idpxyz-textMuted'}`}>
      {note.text}
      {note.tone === 'refused' && note.kind === 'loginRequired' ? <ReloginButton size="inline" /> : null}
      <TechnicalDetails rows={note.technical} />
    </div>
  );
}

/**
 * 货物处置(UC-PS-006 的收寄后服务处置决定;ADR-0332、ADR-0341)。上半是客户收寄后申请停止、等运营决定去向的运单
 * (去向五选一);下半是决定留下的待办事项(领域词「待人工处理项」):登记已办 / 办不了,并可按包裹读回各项结果。
 *
 * 写入口按授予收起(ADR-0315):处置决定与待办登记是两种分开授予的运营决定(ADR-0341 决定三)。
 * 摆不摆只是呈现,每个写口照旧由服务端判授予。取消先于收寄的入口在「取消运单」页,不在这里。
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
  // 形成决定会让待办事项多出几项,下半部随之重读。
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
    const label = `处置:${directionLabel(decision)}`;
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
    ? requestDetailRows(selected).map((r) => ({
        label: r.label,
        value: r.mono ? <ParcelNo parcelId={selected.parcelId} /> : r.title ? <span title={r.title}>{r.value}</span> : r.value,
      }))
    : undefined;

  return (
    <div className="flex flex-1 flex-col overflow-auto">
      <div className="flex min-h-[460px] flex-col">
        <ReviewFlowTemplate<PostIntakeDirection>
          title={info.title}
          description={`客户在货物收寄后申请停止运送的运单,在这里决定去向;系统办不了的事项列在下方待办事项。${body ? requestSummaryText(body) : ''}`}
          queueTitle="等待处置的运单"
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
          detailTitle="处置这票货"
          detailFields={detailFields}
          detailExtra={
            <div className="flex flex-col gap-3">
              {mayDecide ? (
                <>
                  <Field label="新目的地" hint="选「改送」时填;其他去向不用填。">
                    <input
                      className={inputClass}
                      value={destination}
                      onChange={(event) => setDestination(event.target.value)}
                      placeholder="改送到哪里"
                    />
                  </Field>
                  <p className="text-[12px] text-idpxyz-textMuted">
                    在下方写明处置理由后选择去向。决定会留下记录,之后要改去向,再做一次新的决定。
                  </p>
                </>
              ) : (
                <p className="text-[12px] text-idpxyz-textMuted">只有获得「货物处置」权限的操作员能在这里做决定。</p>
              )}
              <NoteLine note={note} />
              {selected ? <TechnicalDetails rows={requestTechnicalRows(selected)} /> : null}
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
    const label = `登记${outcome === 'HANDLED' ? '已办' : '办不了'}`;
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
      <Section title="待办事项">
        <p className="mb-3 text-[12px] text-idpxyz-textMuted">
          处置决定里要人去办的事(扣件、安排退运等),办完后在这里登记「已办」或「办不了」。办不了也要登记,这票货仍归当前网点看管。
          之后又做过新决定的事项不会消失,办之前先看最新一次决定怎么说。
        </p>
        <div className="mb-3 flex flex-wrap items-end gap-2">
          <div className="w-[260px]">
            <Field label="按包裹号查">
              <input className={inputClass} value={parcelInput} onChange={(event) => setParcelInput(event.target.value)} placeholder="包裹号" />
            </Field>
          </div>
          <Button
            variant="secondary"
            onClick={() => {
              setParcel(parcelInput.trim() || undefined);
              setSelectedKey(null);
            }}
          >
            查询
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
          <div className="text-[12px] text-idpxyz-textMuted">
            待办事项这次没有读到,不代表没有待办。请稍后刷新。
            <TechnicalDetails
              rows={
                answer.kind === 'callerProblem' || answer.kind === 'noAnswer'
                  ? [
                      { label: 'HTTP 状态', value: String(answer.status) },
                      { label: '问题码', value: answer.code },
                    ]
                  : [{ label: '原因', value: answer.kind === 'transport' ? answer.message : answer.kind }]
              }
            />
          </div>
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
                  <div className="flex flex-wrap items-baseline gap-x-2 text-[12px]">
                    <ParcelNo parcelId={line.parcelId} className="text-idpxyz-textBright" />
                    <span className="text-idpxyz-text">{line.task}</span>
                  </div>
                  <div className="text-[11px] text-idpxyz-textMuted">
                    {line.decisionText} · {line.decidedAt}
                  </div>
                  {line.superseded && <div className="text-[11px] text-idpxyz-accent">{line.supersededText}</div>}
                  <div className="text-[11px] text-idpxyz-textMuted" title={line.resultTitle || undefined}>{line.resultText ? `结果:${line.resultText}` : '还没登记结果'}</div>
                </button>
                {active && <TechnicalDetails rows={line.technical} />}
                {active && !item.result && (
                  mayRegister ? (
                    <div className="mt-3 flex flex-col gap-2">
                      <Field label="办得怎么样" required>
                        <select className={inputClass} value={outcome} onChange={(event) => setOutcome(event.target.value as ManualItemOutcome)}>
                          {manualOutcomeOptions.map((option) => (
                            <option key={option.id} value={option.id}>
                              {option.label}
                            </option>
                          ))}
                        </select>
                      </Field>
                      <Field label="说明" required error={blocked ?? undefined} hint="做了什么,或为什么办不了。">
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
                    <p className="mt-2 text-[12px] text-idpxyz-textMuted">只有获得「待办登记」权限的操作员能登记结果。</p>
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
