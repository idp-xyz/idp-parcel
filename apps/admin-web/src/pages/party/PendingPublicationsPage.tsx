import { useState } from 'react';
import { useRegisterList } from './register-list';
import { WorkbenchPageTemplate, type WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState } from '../catalogue-view';
import { entryHint } from '../operator-grants';
import { useMayWriteRegistry } from '../use-operator-grants';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { chipClass } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { Instant } from './detail-primitives';
import { PendingDraftPanel, type PendingDraftIdentity } from './PendingDraftPanel';
import {
  draftKeyOf,
  draftSearchText,
  matchesPendingStatus,
  pendingCountSummary,
  pendingCountsOf,
  pendingRowsOf,
  unreadableWord,
  type PendingDraftRow,
  type PendingStatusFilter,
} from './pending-publications';
import { listOpenPublicationDrafts, type PublicationDraftListResponseBody } from './publication-draft-api';
import { publicationKindLabels } from './publication-summary';
import { PrincipalRef } from '../../components/PrincipalRef';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['pending-publications'];

// 列只放批准者要看的：改的是什么（类别 + 编号 + 版本）、状态、谁在什么时候提交。适用范围引用、摘要、主体全串进详情的
// 「技术详情」——适用范围是册上的范围引用，不是批准者核对的业务内容。右栏开时只留编号、版本与状态。
const columns: WorkbenchColumn<PendingDraftRow>[] = [
  {
    key: 'kind',
    header: '类别',
    hideWhenDetailOpen: true,
    render: (row) => publicationKindLabels[row.source.kind] ?? row.source.kind,
  },
  {
    key: 'object',
    header: '编号',
    render: (row) => <span className="font-mono">{row.source.objectId}</span>,
  },
  {
    key: 'version',
    header: '版本',
    render: (row) => <span className="font-mono">{row.source.version}</span>,
  },
  {
    key: 'status',
    header: '状态',
    // 读不出内容的改动照样在列表里，状态之外另标一枚；点开说怎么办。
    render: (row) => (
      <span className="inline-flex items-center gap-1.5">
        <StatusPill code={row.source.status} />
        {row.source.readable ? null : <StatusPill label={unreadableWord} tone="bad" />}
      </span>
    ),
  },
  {
    key: 'submitter',
    header: '提交人',
    hideWhenDetailOpen: true,
    // 操作者主体引用是「发行方#主体」的长串，列里显短名，全串悬停与详情里都有。
    render: (row) => (
      <span className="font-mono text-[12px]">
        <PrincipalRef value={row.source.submitter} />
      </span>
    ),
  },
  {
    key: 'submitted-at',
    header: '提交时间',
    hideWhenDetailOpen: true,
    render: (row) => (
      <span className="font-mono text-[12px]">
        <Instant value={row.source.submittedAt} />
      </span>
    ),
  },
];

const statusFilters: ReadonlyArray<{ id: PendingStatusFilter; label: string }> = [
  { id: 'ALL', label: '全部' },
  { id: 'PENDING_APPROVAL', label: '待批准' },
  { id: 'APPROVED', label: '已批准待发布' },
  { id: 'UNREADABLE', label: unreadableWord },
];

/**
 * 审批与发布：客户合同、服务产品、价格政策等改动存为待批准、或已批准而还没发布的那几份（ADR-0314 决定三）。
 *
 * 列出来是为了换个人批准、隔天再来、刷新页面都找得回在途的改动：点开一行在右栏读回内容，批准与发布就在栏底做，带的是
 * 栏里看到的那份内容与版本壳的摘要（容器照票 commercial-registers-edit-containers/06）。录入仍在各册自己的页面。
 */
export function PendingPublicationsPage() {
  // 检索词在地址里（`?q=`），刷新与换标签回来也还在。
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<PendingStatusFilter>('ALL');
  const { answer, retry, pending } = useRegisterList(listOpenPublicationDrafts);
  // 右栏开着的那一行。记整行而不只记键：发布之后那一行离开列表，栏不能跟着关——人要看完发布的答复；行留在表里直到换行或关栏。
  const [selected, setSelected] = useState<PendingDraftRow | null>(null);

  const rows = answer?.kind === 'outcome' ? pendingRowsOf(answer.body) : [];
  const records = rows.map((row) => row.source);
  const counts = pendingCountsOf(records);
  const needle = search.trim().toLowerCase();
  const visible = rows.filter(
    (row) =>
      matchesPendingStatus(row.source, status) &&
      (needle === '' ||
        draftSearchText(row.source, publicationKindLabels[row.source.kind] ?? row.source.kind).includes(needle)),
  );
  // 选中的那一行已不在（发布后离开列表、或被筛掉）时钉在表尾，栏才开得住；它显示的仍是最后一次列表里的样子，栏里的读回是新的。
  const shown = selected !== null && !visible.some((row) => row.key === selected.key) ? [...visible, selected] : visible;
  // 录入入口在各册自己的页面，没有登记册配置写授予的操作者看不到它们（ADR-0315）；本页自己没有那个入口，说明文案不往那里指，
  // 也不解释一个本页没有的入口。
  const mayWrite = useMayWriteRegistry();
  const countOf: Record<PendingStatusFilter, number> = {
    ALL: counts.total,
    PENDING_APPROVAL: counts.pending,
    APPROVED: counts.approved,
    UNREADABLE: counts.unreadable,
  };
  // 读不出的一格只在真有这样的行时才摆：平常摆一个恒为 0 的「内容读不出」只会让人以为出过事。
  const shownFilters = statusFilters.filter((filter) => filter.id !== 'UNREADABLE' || counts.unreadable > 0);
  const answered = answer?.kind === 'outcome';

  return (
    <WorkbenchPageTemplate<PendingDraftRow>
      title={info.title}
      viewStateKey="pending-publications"
      subtitle={`客户合同、服务产品、价格政策等改动先存为待批准，在这里核对内容后批准，再发布生效。点一行查看内容。${entryHint(mayWrite, '新的改动在各自的页面里录入。', '')}`}
      onRefresh={retry}
      refreshing={pending}
      search={{ value: search, onChange: setSearch, placeholder: '搜编号、类别、版本或提交人' }}
      extraFilters={
        answered && rows.length > 0 ? (
          <>
            {shownFilters.map((filter) => (
              <button
                key={filter.id}
                type="button"
                className={chipClass(filter.id === status)}
                aria-pressed={filter.id === status}
                onClick={() => setStatus(filter.id === status ? 'ALL' : filter.id)}
              >
                {filter.label} {countOf[filter.id]}
              </button>
            ))}
          </>
        ) : undefined
      }
      // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区矛盾。
      toolbarRight={answered ? pendingCountSummary(records) : undefined}
      noMatchText="当前筛选条件下没有匹配的改动"
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
      }}
      columns={columns}
      rows={shown}
      rowKey={(row) => row.key}
      selectedKey={selected?.key ?? null}
      onSelect={(key) => setSelected(key === null ? null : (shown.find((row) => row.key === key) ?? null))}
      renderDetail={(key) => {
        const row = shown.find((candidate) => candidate.key === key);
        if (!row) return null;
        const identity: PendingDraftIdentity = { kind: row.source.kind, objectId: row.source.objectId, version: row.source.version };
        // 按载体身份给 key：换一行整个重挂，上一份的批准与发布答复不留。
        return <PendingDraftPanel key={draftKeyOf(identity)} target={identity} onClose={() => setSelected(null)} onChanged={retry} />;
      }}
      // 空态按摆出来的行数判：列表空了而钉着的那一行还在，表就还在，栏才开得住。
      viewState={catalogueViewState(answer, shown.length, retry, {
        module: info,
        endpoint: 'GET /commercial-open-publication-drafts',
        emptyTitle: '没有等待批准或发布的改动',
        emptyDescription: entryHint(
          mayWrite,
          '在客户合同、服务产品、价格政策等页面录入新版本并「存为待批准」后，会出现在这里等人批准。',
          '有同事录入新版本并存为待批准后，会出现在这里。',
        ),
      })}
    />
  );
}
