import { useEffect, useState, type ReactNode } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn, type TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import { catalogueViewState } from '../catalogue-view';
import { listBusinessParties, type BusinessPartyListResponseBody } from '../party/api';
import {
  confirmDeliveryHandover,
  listDeliveryHandoverNotes,
  readDeliveryHandoverNote,
  type ApiResult,
  type DeliveryHandoverConfirmationResponseBody,
  type DeliveryHandoverNoteFoundResponseBody,
  type DeliveryHandoverNoteListResponseBody,
} from './delivery-scope-api';
import { deliveryScopeListPathOf, handoverNoteFilterKeys, type DeliveryScopeListFilter } from './delivery-pre-advice';
import {
  claimRowsOf,
  confirmationProblemNote,
  confirmationRequestOf,
  deliveringParticipantOptionsOf,
  describeConfirmationAnswer,
  handoverMemberRowOf,
  handoverNoteRowOf,
  type ConfirmationDraft,
  type HandoverNoteReviewRow,
} from './delivery-handover-note';
import { DeliveryScopeFilters, DeliveryScopeReadNote } from './DeliveryScopeFilters';

const info = moduleInfoById['node-operations-review'];
const listBase = '/node-operations-delivery-handover-notes';

function col(id: string, header: string, mono = false): ListColumn<HandoverNoteReviewRow> {
  return {
    id,
    header,
    render: (row) => (mono ? <span className="font-mono text-[12px]">{row.values[id] ?? '—'}</span> : (row.values[id] ?? '—')),
  };
}

const columns: ListColumn<HandoverNoteReviewRow>[] = [
  col('noteId', '交接单', true),
  col('node', '物流节点', true),
  col('deliveringParty', '交付方线索', true),
  col('openedAt', '开立时刻', true),
  col('state', '开放 / 关闭'),
  col('members', '成员', true),
  col('claims', '声称的客户账户', true),
  col('bindings', '绑定'),
  col('scope', '可验证交付范围'),
];

/**
 * 节点作业查阅页的「送站交接单」区（票 ps-external-mark-relations/13；UC-NO-006）：按节点与开立日列节点当场开立的交接单，点一行看声称的
 * 每个账户的结论与途径、名下成员与各自的形成方式。写动作只有一样——对待确认的账户「确认」，是运营决定（送站交接确认）：指名交付方参与方与
 * 依据引用，服务端向参与方与商业问授权交付判断，是即确认，否或未配置不落结论。开立、归入、关闭是一线作业端的作业事实，不在这里。
 */
export function DeliveryHandoverNotesSection({ chips }: { chips: ReactNode }) {
  const [filter, setFilter] = useState<DeliveryScopeListFilter>({ node: '', day: '' });
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{ path: string; answer: ApiResult<DeliveryHandoverNoteListResponseBody> } | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const path = deliveryScopeListPathOf(listBase, handoverNoteFilterKeys, filter);

  useEffect(() => {
    if (path === null) return undefined;
    let cancelled = false;
    void listDeliveryHandoverNotes(path).then((answer) => {
      if (!cancelled) setLoaded({ path, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [path, reloadKey]);

  const answer = path !== null && loaded?.path === path ? loaded.answer : null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? body.handoverNotes.map(handoverNoteRowOf) : [];
  const retry = () => setReloadKey((value) => value + 1);
  const narrowed = filter.node.trim() !== '' || filter.day !== '';
  const viewState: TemplateViewState =
    path === null
      ? { kind: 'error', title: '开立日认不出', description: '日期格不是一个日历上存在的日子；改正后才发请求，页面不发半截条件。' }
      : catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: `GET ${listBase}`,
          emptyTitle: '当前租户尚无送站交接单',
          emptyDescription: '读取入口已配置，交接单册为空——交接单由一线作业端在节点当场开立；页面不含合成数据。',
          narrowed,
        });

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <ListPageTemplate<HandoverNoteReviewRow>
        title={info.title}
        description={`${info.owner} · 送站交接单由节点当场开立，声称的客户账户全部确认才成立可验证交付范围。点一行看各账户的结论与名下成员；待确认的账户可由持「送站交接确认」授予的运营操作者依授权交付关系确认。`}
        filters={<DeliveryScopeFilters chips={chips} filter={filter} dayLabel="开立日" onChange={setFilter} />}
        filterSummary={body ? `送站交接单 ${rows.length} 行` : undefined}
        emptyRowsNote={narrowed ? '当前筛选条件下没有交接单' : undefined}
        columns={columns}
        rows={rows}
        rowKey={(row) => row.key}
        onRowClick={(row) => setSelected(row.noteId)}
        viewState={viewState}
      />
      {selected ? (
        <HandoverNoteDetail key={selected} noteId={selected} onClose={() => setSelected(null)} onLanded={retry} />
      ) : null}
    </div>
  );
}

function HandoverNoteDetail({ noteId, onClose, onLanded }: { noteId: string; onClose: () => void; onLanded: () => void }) {
  const [answer, setAnswer] = useState<ApiResult<DeliveryHandoverNoteFoundResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [draft, setDraft] = useState<ConfirmationDraft | null>(null);
  useEffect(() => {
    let cancelled = false;
    void readDeliveryHandoverNote(noteId).then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [noteId, reloadKey]);

  const note = answer?.kind === 'outcome' ? answer.body : null;
  const claims = note ? claimRowsOf(note.handoverNote) : [];
  const members = note ? note.members.map(handoverMemberRowOf) : [];

  return (
    <Card className="mx-4 mb-4">
      <CardHeader>
        <CardTitle>
          送站交接单 <span className="font-mono">{noteId}</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 text-xs">
        {answer === null ? <p className="text-idpxyz-textMuted">读取中…</p> : null}
        {answer !== null && answer.kind !== 'outcome' ? <DeliveryScopeReadNote answer={answer} what="查阅送站交接单" /> : null}
        {note ? (
          <>
            <p className="text-idpxyz-textMuted">
              {handoverNoteRowOf(note.handoverNote).values.state} · 交付方线索{' '}
              <span className="font-mono">{note.handoverNote.deliveringParty}</span>（只是线索，不参与范围判断）· 可验证交付范围：
              {handoverNoteRowOf(note.handoverNote).values.scope}
            </p>
            <table className="w-full">
              <thead>
                <tr className="text-left text-idpxyz-textMuted">
                  <th className="py-1 pr-2 font-normal">声称的客户账户</th>
                  <th className="py-1 pr-2 font-normal">结论</th>
                  <th className="py-1 pr-2 font-normal">途径</th>
                  <th className="py-1 pr-2 font-normal">作结论方 · 时刻</th>
                  <th className="py-1 pr-2 font-normal">交付方参与方 · 依据引用</th>
                  <th className="py-1 font-normal">动作</th>
                </tr>
              </thead>
              <tbody>
                {claims.map((claim) => (
                  <tr key={claim.key} className="align-top border-t border-idpxyz-border">
                    <td className="py-1 pr-2 font-mono">{claim.values.customerAccount}</td>
                    <td className="py-1 pr-2">{claim.values.conclusion}</td>
                    <td className="py-1 pr-2">{claim.values.route || '—'}</td>
                    <td className="py-1 pr-2 font-mono">{claim.values.concluded || '—'}</td>
                    <td className="py-1 pr-2 font-mono">{claim.values.evidence || '—'}</td>
                    <td className="py-1">
                      {claim.confirm.kind === 'available' ? (
                        <Button
                          variant="outline"
                          disabled={draft !== null}
                          onClick={() => {
                            if (claim.confirm.kind !== 'available') return;
                            setDraft({
                              noteId: claim.confirm.noteId,
                              customerAccount: claim.confirm.customerAccount,
                              deliveringParticipant: '',
                              basis: '',
                            });
                          }}
                        >
                          确认
                        </Button>
                      ) : (
                        <span className="text-idpxyz-textMuted">{claim.confirm.reason}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {draft ? (
              <ConfirmationForm
                draft={draft}
                onChange={setDraft}
                onClose={() => setDraft(null)}
                onLanded={() => {
                  setReloadKey((value) => value + 1);
                  onLanded();
                }}
              />
            ) : null}
            <div>
              <p className="text-idpxyz-textMuted">名下成员（{members.length}）：收寄带来的与归入的都算；关闭冻结成员</p>
              {members.length === 0 ? (
                <p className="text-idpxyz-textMuted">名下还没有作业实物。</p>
              ) : (
                <table className="w-full">
                  <thead>
                    <tr className="text-left text-idpxyz-textMuted">
                      <th className="py-1 pr-2 font-normal">作业实物</th>
                      <th className="py-1 pr-2 font-normal">形成方式</th>
                      <th className="py-1 pr-2 font-normal">事实身份</th>
                      <th className="py-1 font-normal">业务时间</th>
                    </tr>
                  </thead>
                  <tbody>
                    {members.map((member) => (
                      <tr key={member.key} className="border-t border-idpxyz-border">
                        <td className="py-1 pr-2 font-mono">{member.values.unit}</td>
                        <td className="py-1 pr-2">{member.values.membership}</td>
                        <td className="py-1 pr-2 font-mono">{member.values.factId}</td>
                        <td className="py-1 font-mono">{member.values.formedAt}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          </>
        ) : null}
        <div>
          <Button variant="outline" onClick={onClose}>
            收起
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

type SubmissionState =
  | { kind: 'idle' }
  | { kind: 'incomplete'; reason: string }
  | { kind: 'submitting' }
  | { kind: 'answered'; answer: ApiResult<DeliveryHandoverConfirmationResponseBody> };

function ConfirmationForm({
  draft,
  onChange,
  onClose,
  onLanded,
}: {
  draft: ConfirmationDraft;
  onChange: (draft: ConfirmationDraft) => void;
  onClose: () => void;
  onLanded: () => void;
}) {
  const [state, setState] = useState<SubmissionState>({ kind: 'idle' });
  const [parties, setParties] = useState<ApiResult<BusinessPartyListResponseBody> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void listBusinessParties().then((answer) => {
      if (!cancelled) setParties(answer);
    });
    return () => {
      cancelled = true;
    };
  }, []);
  const options = parties?.kind === 'outcome' ? deliveringParticipantOptionsOf(parties.body.parties) : [];
  const submit = () => {
    const built = confirmationRequestOf(draft);
    if (built.kind === 'incomplete') {
      setState({ kind: 'incomplete', reason: built.reason });
      return;
    }
    setState({ kind: 'submitting' });
    void confirmDeliveryHandover(built.request).then((answer) => {
      setState({ kind: 'answered', answer });
      if (answer.kind === 'outcome' && describeConfirmationAnswer(answer.body).landed) onLanded();
    });
  };

  return (
    <div className="flex flex-col gap-2 border-t border-idpxyz-border pt-2">
      <p className="text-idpxyz-textMuted">
        确认 <span className="font-mono">{draft.noteId}</span> 对客户账户 <span className="font-mono">{draft.customerAccount}</span> 的绑定：
        指名交付方参与方与依据引用，服务端向参与方与商业问授权交付判断。操作者身份由接入渠道给，本表单不收也不送；运营决定只能确认，不能否认。
      </p>
      <div className="block">
        <span className="text-idpxyz-textMuted">交付方参与方 *（参与方目录）</span>
        {parties?.kind === 'outcome' ? (
          <select
            className="ml-2 font-mono"
            aria-label="交付方参与方"
            value={draft.deliveringParticipant}
            onChange={(event) => onChange({ ...draft, deliveringParticipant: event.target.value })}
          >
            <option value="">— 选一位 —</option>
            {options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        ) : (
          <>
            {parties === null ? <span className="ml-2 text-idpxyz-textMuted">读取参与方目录中…</span> : null}
            {parties !== null ? (
              <div className="ml-2">
                <DeliveryScopeReadNote answer={parties} what="查阅参与方目录" />
                <span className="text-idpxyz-textMuted">目录读不到时可逐字填参与方标识，判断照旧由参与方与商业作答：</span>
                <Input
                  value={draft.deliveringParticipant}
                  className="font-mono text-[13px]"
                  placeholder="参与方标识"
                  onChange={(event) => onChange({ ...draft, deliveringParticipant: event.target.value })}
                />
              </div>
            ) : null}
          </>
        )}
      </div>
      <label className="block">
        <span className="text-idpxyz-textMuted">依据引用 *</span>
        <Input
          value={draft.basis}
          className="font-mono text-[13px]"
          placeholder="柜台核验记录、委托书或工单的引用，逐字保存、不判真伪"
          onChange={(event) => onChange({ ...draft, basis: event.target.value })}
        />
      </label>
      <div className="flex items-center gap-3">
        <Button variant="default" className={primaryActionClass} onClick={submit} disabled={state.kind === 'submitting'}>
          {state.kind === 'submitting' ? '提交中…' : '提交确认'}
        </Button>
        <Button variant="outline" onClick={onClose}>
          收起
        </Button>
      </div>
      <ConfirmationAnswerNote state={state} />
    </div>
  );
}

function ConfirmationAnswerNote({ state }: { state: SubmissionState }) {
  switch (state.kind) {
    case 'idle':
    case 'submitting':
      return null;
    case 'incomplete':
      return <p className="text-idpxyz-danger">未提交：{state.reason}</p>;
  }
  const answer = state.answer;
  switch (answer.kind) {
    case 'outcome': {
      const note = describeConfirmationAnswer(answer.body);
      return (
        <div className={note.tone === 'answered' ? 'text-idpxyz-textMuted' : 'text-idpxyz-danger'}>
          <p>{note.headline}</p>
          {note.details.length > 0 ? (
            <ul className="mt-0.5 ml-4 list-disc">
              {note.details.map((line) => (
                <li key={line}>{line}</li>
              ))}
            </ul>
          ) : null}
        </div>
      );
    }
    case 'unconfigured':
      return (
        <p className="text-idpxyz-textMuted">
          接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。确认端点已建立并装配，操作者渠道的发行方参数未设，服务端如实拒绝。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="送站交接确认" className="text-idpxyz-textMuted" />;
    case 'callerProblem':
      return (
        <p className="text-idpxyz-danger">
          调用方式问题（HTTP {answer.status}）：{confirmationProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-idpxyz-danger">
          服务端未形成答案（HTTP {answer.status}）：{confirmationProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className="text-idpxyz-danger">请求未到达 parcel-api：{answer.message}</p>;
  }
}
