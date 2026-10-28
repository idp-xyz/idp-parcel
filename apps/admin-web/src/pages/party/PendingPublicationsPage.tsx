import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { entryHint } from '../operator-grants';
import { useMayWriteRegistry } from '../use-operator-grants';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { Instant, statusWordBadge } from './detail-primitives';
import { PendingDraftDrawer, type PendingDraftIdentity } from './PendingDraftDrawer';
import { draftSearchText, pendingCountSummary, pendingRowsOf, type PendingDraftRow } from './pending-publications';
import { labelOf } from './presentation';
import { draftStatusLabels, listOpenPublicationDrafts, type PublicationDraftListResponseBody } from './publication-draft-api';
import { publicationKindLabels } from './publication-summary';
import { PrincipalRef } from '../../components/PrincipalRef';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['pending-publications'];

const columns: ListColumn<PendingDraftRow>[] = [
  {
    id: 'kind',
    header: '类别',
    render: (row) => publicationKindLabels[row.source.kind] ?? row.source.kind,
  },
  {
    id: 'object',
    header: '对象',
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
    id: 'scope',
    header: '适用范围',
    className: 'font-mono',
    render: (row) => row.source.scope,
  },
  {
    id: 'status',
    header: '状态',
    // 读不回的载体（本构建折不回它的正文）照样在列表里，状态之外另标一句，悬停是成因；点开抽屉说恢复动作。
    render: (row) => (
      <span className="inline-flex items-center gap-1.5">
        {statusWordBadge(labelOf(draftStatusLabels, row.source.status))}
        {row.source.readable ? null : (
          <span className="whitespace-nowrap text-[12px] text-idpxyz-textMuted" title={row.source.unreadableCause}>
            本构建读不回
          </span>
        )}
      </span>
    ),
  },
  {
    id: 'submitter',
    header: '录入者',
    className: 'font-mono text-[12px]',
    // 操作者主体引用是「发行方#主体」的长串，列里显短名，全串悬停与详情里都有。
    render: (row) => (
      <PrincipalRef value={row.source.submitter} />
    ),
  },
  {
    id: 'submitted-at',
    header: '录入时刻',
    className: 'font-mono text-[12px]',
    render: (row) => <Instant value={row.source.submittedAt} />,
  },
];

/**
 * 待批准发布：租户名下已经存为待批准、或已批准而还没发布的载体（ADR-0314 决定三）。
 *
 * 列出来是为了换个人批准、隔天再来、刷新页面都找得回在途的载体：点开一行读回内容与摘要，批准与发布就在抽屉里做，带的是
 * 抽屉里看到的那份内容与版本壳的摘要。录入仍在各册的发布签里。
 */
export function PendingPublicationsPage() {
  // 检索词在地址里（`?q=`），刷新与换标签回来也还在。
  const [search, setSearch] = useAddressKeyword();
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<PublicationDraftListResponseBody> | null>(null);
  // 抽屉开着的那一份载体的身份。不从行里反推：批准或发布之后列表重取，行会暂时清空，抽屉不能跟着关。
  const [selected, setSelected] = useState<PendingDraftIdentity | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listOpenPublicationDrafts().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const rows = answer?.kind === 'outcome' ? pendingRowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visible = needle
    ? rows.filter((row) =>
        draftSearchText(row.source, publicationKindLabels[row.source.kind] ?? row.source.kind).includes(needle),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);
  // 录入入口在各册的发布签，没有登记册配置写授予的操作者看不到它们（ADR-0315）；本页自己没有那个入口，说明文案不往那里指，
  // 也不解释一个本页没有的入口。
  const mayWrite = useMayWriteRegistry();

  return (
    <>
      <ListPageTemplate<PendingDraftRow>
        moduleId="pending-publications"
        title={info.title}
        description={`已经存为待批准、或已批准而还没发布的发布稿。点一行读回内容，批准与发布在详情里做。${entryHint(mayWrite, '录入在各册的发布页签。', '')}`}
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索类别、对象、版本、范围、状态或录入者',
        }}
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区矛盾。
        filterSummary={answer?.kind === 'outcome' ? pendingCountSummary(rows.map((row) => row.source)) : undefined}
        emptyRowsNote="这组条件下没有匹配的发布稿"
        columns={columns}
        rows={visible}
        rowKey={(row) => row.key}
        onRowClick={(row) => setSelected({ kind: row.source.kind, objectId: row.source.objectId, version: row.source.version })}
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /commercial-open-publication-drafts',
          emptyTitle: '当前没有待批准的发布稿',
          emptyDescription: `没有尚未发布的载体。${entryHint(mayWrite, '各册发布页签里「存为待批准」之后，它们会出现在这里。', '有人在各册存为待批准之后，它们会出现在这里。')}`,
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
