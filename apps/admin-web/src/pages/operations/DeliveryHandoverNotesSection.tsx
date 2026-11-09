import { useEffect, useState, type ReactNode } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import type { ListColumn, TemplateViewState } from '../../templates';
import { RecordDetailPanel, RouteWorkbench, techRows } from '../network/route-workbench';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import { withPlaceNames, workbenchColumnsOf } from './review-workbench';
import { usePlaceNames } from '../network/use-place-names';
import { placeLabel } from '../network/place-names';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { OperatorRefusalNote } from '../../components/states';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { catalogueConditionsNarrow, type CatalogueConditions } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword, type CatalogueCursor } from '../use-catalogue-cursor';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
import { PageOnlySearch } from '../visibility/page-only';
import { documentTypeWords } from '../../vocabulary/document-numbering';
import { useMayMakeDecision } from '../use-operator-grants';
import { deliveryHandoverConfirmationDecision, hiddenDecisionHint } from '../operator-grants';
import { labelFor } from '../../vocabulary/status';
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
import {
  deliveryScopeConditions,
  deliveryScopeFilterNarrow,
  documentNumberDetailOf,
  handoverNoteFilterKeys,
  type DeliveryScopeListFilter,
} from './delivery-pre-advice';
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
import { DeliveryScopeFilters, DeliveryScopeReadNote, type DeliveryScopeControls } from './DeliveryScopeFilters';
import { allStatuses, handoverNotePushedStatus, pushedStatusChips, pushedStatusFilters, rowsOnPage } from './list-conditions';

const info = moduleInfoById['node-operations-review'];
// 号名取单据编号词典（设置 → 单据编号同一份），不在本页另起叫法。
const numberName = documentTypeWords.DELIVERY_HANDOVER_NOTE.numberName;
const listBase = '/node-operations-delivery-handover-notes';

function col(id: string, header: string, mono = false): ListColumn<HandoverNoteReviewRow> {
  return {
    id,
    header,
    render: (row) => (mono ? <span className="font-mono type-body">{row.values[id] ?? '—'}</span> : (row.values[id] ?? '—')),
  };
}

// 主列是交接单的单据号（服务端收下开立时取的，ADR-0355 决定二、十一）；没有单据号的照旧显设备签发的交接单标识，原因在技术详情。
const columns: ListColumn<HandoverNoteReviewRow>[] = [
  col('documentNumber', numberName, true),
  col('node', '网点'),
  col('deliveringParty', '谁来送', true),
  col('openedAt', '开单时间', true),
  col('state', '状态'),
  col('members', '件数', true),
  col('claims', '自称的客户', true),
  col('bindings', '客户确认进度'),
  col('scope', '送货范围成立没有'),
];

/**
 * 节点作业查阅页的「送站交接单」区（票 ps-external-mark-relations/13；UC-NO-006）：按节点与开立日列节点当场开立的交接单，点一行看声称的
 * 每个账户的结论与途径、名下成员与各自的形成方式。写动作只有一样——对待确认的账户「确认」，是运营决定（送站交接确认）：指名交付方参与方与
 * 依据引用，服务端向参与方与商业问授权交付判断，是即确认，否或未配置不落结论。开立、归入、关闭是一线作业端的作业事实，不在这里。
 *
 * 列表接读口分页（ADR-0144，票 catalogue-read-pagination-rollout/14）：检索框下推成 `q`；节点、日子与单据号是读口的选择器，照旧交服务端；
 * 开放与否的胶囊下推成 closed 一维，按的是整本册，换胶囊回第一页。
 */
export function DeliveryHandoverNotesSection({ sections }: { sections: ReactNode }) {
  const [filter, setFilter] = useState<DeliveryScopeListFilter>({ node: '', day: '', documentNumber: '' });
  const [status, setStatus] = useState(allStatuses);
  const [keyword, setKeyword] = useState('');
  const conditions = deliveryScopeConditions(
    handoverNoteFilterKeys,
    filter,
    useSettledKeyword(keyword),
    pushedStatusFilters(handoverNotePushedStatus, status),
  );
  const controls: DeliveryScopeControls = {
    sections,
    filter,
    onFilter: setFilter,
    status,
    onStatus: setStatus,
    keyword,
    onKeyword: setKeyword,
  };
  // 日子认不出时不发请求（deliveryScopeConditions 答 null）：取数的那一件只在条件成立时挂上。
  return conditions === null ? (
    <HandoverNoteWorkbench controls={controls} cursor={null} />
  ) : (
    <HandoverNoteCursor controls={controls} conditions={conditions} />
  );
}

function HandoverNoteCursor({ controls, conditions }: { controls: DeliveryScopeControls; conditions: CatalogueConditions }) {
  return (
    <HandoverNoteWorkbench controls={controls} conditions={conditions} cursor={useCatalogueCursor(conditions, listDeliveryHandoverNotes)} />
  );
}

function HandoverNoteWorkbench({
  controls,
  conditions,
  cursor,
}: {
  controls: DeliveryScopeControls;
  conditions?: CatalogueConditions;
  cursor: CatalogueCursor<DeliveryHandoverNoteListResponseBody> | null;
}) {
  const { sections, filter, onFilter, status, onStatus, keyword, onKeyword } = controls;
  const [refine, setRefine] = useState('');
  const places = usePlaceNames();
  const answer = cursor?.answer ?? null;
  const body = answer?.kind === 'outcome' ? answer.body : null;
  const pageRows = body ? withPlaceNames(body.handoverNotes.map(handoverNoteRowOf), ['node'], places) : [];
  const rows = rowsOnPage(pageRows, refine);
  const retry = () => cursor?.retry();
  const viewState: TemplateViewState =
    cursor === null
      ? { kind: 'error', title: '开单日期写得不对', description: '这一天在日历上不存在；改正日期后再查。' }
      : catalogueViewState(answer, pageRows.length, cursor.retry, {
          module: info,
          endpoint: `GET ${listBase}`,
          emptyTitle: '还没有送货交接单',
          emptyDescription: '客户或司机到网点送货时，网点在作业终端上当场开交接单，开了之后会出现在这里。',
          narrowed:
            deliveryScopeFilterNarrow(filter) ||
            (conditions !== undefined && catalogueConditionsNarrow(conditions)) ||
            pastFirstPage(cursor.pagination),
        });

  return (
    <RouteWorkbench<HandoverNoteReviewRow>
      title={info.title}
      subtitle="网点收货时当场开的交接单。单上自称的客户全部确认后，这批货的送货范围才算成立；待确认的客户可以在右栏里确认。"
      sections={sections}
      extraFilters={
        <>
          <DeliveryScopeFilters filter={filter} dayLabel="开单日" numberLabel={numberName} onChange={onFilter} />
          <PageOnlySearch value={refine} onChange={setRefine} subject="自称的客户、网点名称等其余各格" />
        </>
      }
      answered={body !== null}
      noun="张"
      rows={rows}
      rowKey={(row) => row.key}
      statusChips={pushedStatusChips(handoverNotePushedStatus, status, onStatus)}
      onClearFilters={() => {
        onStatus(allStatuses);
        setRefine('');
        onFilter({ node: '', day: '', documentNumber: '' });
      }}
      searchCells={() => []}
      search={{ value: keyword, onChange: onKeyword, pushedDown: true }}
      searchPlaceholder={`搜${numberName}、交接单标识、网点编码、谁来送`}
      columns={workbenchColumnsOf(columns, { labelKey: 'state' })}
      renderDetail={(row, close) => <HandoverNoteDetail key={row.noteId} noteId={row.noteId} onClose={close} onLanded={retry} />}
      onRefresh={cursor?.retry}
      toolbarRight={cursor?.pagination ? <CursorPager pagination={cursor.pagination} /> : undefined}
      viewState={viewState}
    />
  );
}

/**
 * 一张交接单的右栏，形状同本区其余几页（事实格 + 正文 + 技术详情）：单子开放还是关闭、送货范围成立没有、几件、客户确认进度是事实格；
 * 单上自称的每个客户各一行（结论、途径与确认按钮），名下成员一列在正文；单据号（没有时按词典注明为什么没有）、设备签发的交接单标识、网点编码、
 * 关闭事实、确认人与来源单据、成员的事实身份进技术详情。标题是单据号，没有单据号的照旧用交接单标识。确认是运营决定（送站交接确认，
 * UC-NO-006 步骤 5B），表单在栏内展开。
 */
function HandoverNoteDetail({ noteId, onClose, onLanded }: { noteId: string; onClose: () => void; onLanded: () => void }) {
  const [answer, setAnswer] = useState<ApiResult<DeliveryHandoverNoteFoundResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [draft, setDraft] = useState<ConfirmationDraft | null>(null);
  const names = useCustomerNames();
  // 确认是运营决定「送站交接确认」：明确知道没有这一种授予时不摆确认按钮，答不出授予时照摆、让服务端去答（ADR-0315）。
  const mayConfirm = useMayMakeDecision(deliveryHandoverConfirmationDecision);
  const places = usePlaceNames();
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
  const row = note ? handoverNoteRowOf(note.handoverNote) : null;
  const customer = (account: string) => names.get(account) ?? <span className="font-mono">{account}</span>;

  return (
    <RecordDetailPanel
      title={note?.handoverNote.documentNumber ?? noteId}
      status={note ? { label: note.handoverNote.closed ? '已关闭' : '开放', tone: note.handoverNote.closed ? 'muted' : 'info' } : undefined}
      subtitle="送货交接单"
      facts={
        note && row
          ? [
              { label: '网点', value: placeLabel(places, note.handoverNote.node) },
              { label: '开单时间', value: row.values.openedAt },
              // 状态胶囊只说开放 / 已关闭，关闭时刻放这一格；没关的单子这一格说还开着。
              { label: '关闭时间', value: note.handoverNote.closedAt ? formatInstant(note.handoverNote.closedAt) : '还开着' },
              { label: '送货范围', value: row.values.scope },
              { label: '件数', value: row.values.members },
              { label: '客户确认进度', value: row.values.bindings },
              { label: '谁来送（只是线索）', value: <span className="font-mono">{note.handoverNote.deliveringParty}</span> },
            ]
          : []
      }
      technical={techRows([
        { label: numberName, value: note ? documentNumberDetailOf(note.handoverNote) : undefined },
        // 设备签发的标识（ADR-0023）：作业端与客户确认都按它寻址，单据号不替代它。
        { label: '交接单标识（设备签发）', value: noteId },
        { label: '网点编码', value: note?.handoverNote.node },
        { label: '关闭事实', value: note?.handoverNote.closureFactId },
        ...claims.map((claim) => ({ label: `${claim.values.customerAccount} 确认`, value: [claim.values.concluded, claim.values.evidence].filter(Boolean).join(' · ') })),
        ...members.map((member) => ({ label: `${member.values.unit}`, value: member.values.factId })),
      ])}
      onClose={onClose}
    >
      {answer === null ? <p>读取中…</p> : null}
      {answer !== null && answer.kind !== 'outcome' ? <DeliveryScopeReadNote answer={answer} what="查看送货交接单" /> : null}
      {note ? (
        <>
          <p className="font-medium text-idpxyz-text">自称的客户</p>
          <ul className="space-y-1.5">
            {claims.map((claim) => (
              <li key={claim.key} className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span className="text-idpxyz-text">{customer(claim.values.customerAccount)}</span>
                <span>{claim.values.conclusion}</span>
                {claim.values.route ? <span>· {claim.values.route}</span> : null}
                {claim.confirm.kind === 'available' && !mayConfirm ? (
                  <span className="basis-full type-caption">{hiddenDecisionHint('确认', labelFor(deliveryHandoverConfirmationDecision))}</span>
                ) : claim.confirm.kind === 'available' ? (
                  <Button
                    variant="outline"
                    size="sm"
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
                  <span className="basis-full type-caption">{claim.confirm.reason}</span>
                )}
              </li>
            ))}
          </ul>
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
          <p className="font-medium text-idpxyz-text">名下的货（{members.length}）</p>
          <p>收货带来的与后来归入的都算；单子关闭后不再变。</p>
          {members.length === 0 ? (
            <p>名下还没有货。</p>
          ) : (
            <ul className="space-y-0.5">
              {members.map((member) => (
                <li key={member.key}>
                  <span className="font-mono text-idpxyz-text">{member.values.unit}</span> · {member.values.membership} ·{' '}
                  <span className="font-mono">{member.values.formedAt}</span>
                </li>
              ))}
            </ul>
          )}
        </>
      ) : null}
    </RecordDetailPanel>
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
                  className="font-mono type-value"
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
          className="font-mono type-value"
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
          操作入口还没有配置好，这次什么都没有保存；重试也不会改变结果，请联系系统管理员完成配置。
        </p>
      );
    case 'loginRequired':
    case 'notGranted':
      return <OperatorRefusalNote kind={answer.kind} what="送站交接确认" className="text-idpxyz-textMuted" />;
    case 'callerProblem':
      return (
        <p className="text-idpxyz-danger">
          没有保存：{confirmationProblemNote(answer.code)}
        </p>
      );
    case 'noAnswer':
      return (
        <p className="text-idpxyz-danger">
          结果还不确定：{confirmationProblemNote(answer.code)}
        </p>
      );
    case 'transport':
      return <p className="text-idpxyz-danger">连不上服务，这次没有提交；请检查网络后重试（{answer.message}）</p>;
  }
}
