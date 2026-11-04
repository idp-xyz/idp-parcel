import { useState } from 'react';
import { useRegisterList } from './register-list';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState } from '../catalogue-view';
import { entryHint } from '../operator-grants';
import { useMayWriteRegistry } from '../use-operator-grants';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { chipClass } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { Instant } from './detail-primitives';
import { PendingDraftDrawer, type PendingDraftIdentity } from './PendingDraftDrawer';
import {
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
// 「技术详情」——适用范围是册上的范围引用，不是批准者核对的业务内容。
const columns: ListColumn<PendingDraftRow>[] = [
  {
    id: 'kind',
    header: '类别',
    render: (row) => publicationKindLabels[row.source.kind] ?? row.source.kind,
  },
  {
    id: 'object',
    header: '编号',
    className: 'font-mono',
    render: (row) => row.source.objectId,
  },
  {
    id: 'version',
    header: '版本',
    className: 'font-mono',
    render: (row) => row.source.version,
  },
  {
    id: 'status',
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
    id: 'submitter',
    header: '提交人',
    className: 'font-mono text-[12px]',
    // 操作者主体引用是「发行方#主体」的长串，列里显短名，全串悬停与详情里都有。
    render: (row) => <PrincipalRef value={row.source.submitter} />,
  },
  {
    id: 'submitted-at',
    header: '提交时间',
    className: 'font-mono text-[12px]',
    render: (row) => <Instant value={row.source.submittedAt} />,
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
 * 列出来是为了换个人批准、隔天再来、刷新页面都找得回在途的改动：点开一行读回内容，批准与发布就在抽屉里做，带的是
 * 抽屉里看到的那份内容与版本壳的摘要。录入仍在各册自己的页面。
 */
export function PendingPublicationsPage() {
  // 检索词在地址里（`?q=`），刷新与换标签回来也还在。
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<PendingStatusFilter>('ALL');
  const { answer, retry } = useRegisterList(listOpenPublicationDrafts);
  // 抽屉开着的那一份的身份。不从行里反推：发布之后那一行离开列表，抽屉不能跟着关。
  const [selected, setSelected] = useState<PendingDraftIdentity | null>(null);

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

  return (
    <>
      <ListPageTemplate<PendingDraftRow>
        moduleId="pending-publications"
        title={info.title}
        description={`客户合同、服务产品、价格政策等改动先存为待批准，在这里核对内容后批准，再发布生效。点一行查看内容。${entryHint(mayWrite, '新的改动在各自的页面里录入。', '')}`}
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜编号、类别、版本或提交人',
        }}
        filters={
          answer?.kind === 'outcome' && rows.length > 0 ? (
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
        filterSummary={answer?.kind === 'outcome' ? pendingCountSummary(records) : undefined}
        emptyRowsNote="当前筛选条件下没有匹配的改动"
        columns={columns}
        rows={visible}
        rowKey={(row) => row.key}
        onRowClick={(row) => setSelected({ kind: row.source.kind, objectId: row.source.objectId, version: row.source.version })}
        viewState={catalogueViewState(answer, rows.length, retry, {
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
      <PendingDraftDrawer
        target={selected}
        onClose={() => setSelected(null)}
        onChanged={retry}
      />
    </>
  );
}
