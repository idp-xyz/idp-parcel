import { useEffect, useState } from 'react';
import {
  ListPageTemplate,
  type ListColumn,
  type TemplateViewState,
} from '../../templates';

// 详情钻取选中承载在 hash 第二段（#/shipment-request-inquiry/<委托标识>），与外壳
// 的模块级 hash 路由同一约定：刷新回到同一份详情、后退自然收回列表、详情可收藏
// 转发。外壳只认第一段，本段归本页所有。
function selectedIdFromHash(): string | null {
  const segments = window.location.hash.replace(/^#\/?/, '').split('/');
  return segments[0] === 'shipment-request-inquiry' && segments[1]
    ? decodeURIComponent(segments[1])
    : null;
}
import { StatusBadgeFor, type DomainStatus } from '../../domain/status';
import { moduleInfoById } from '../../navigation';
import { requestStateLabels, problemNote } from './presentation';
import {
  listShipmentRequestViews,
  type ApiResult,
  type ShipmentRequestSummary,
  type ViewsListResponseBody,
} from './api';
import { ShipmentRequestDetailPage } from './ShipmentRequestDetailPage';

// 页面标题的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['shipment-request-inquiry'];

// 委托查阅列表，对 GET /shipment-request-views 真实端点取数。
//
// 两条查询语义红线（出处均为 CONTEXT.md）在接线后的落点：「授权查询作用域」——
// 作用域整组由服务端接入面从认证与授权结果裁决，本页不送任何自报身份参数，也不
// 自建跨客户查询层；「统一不可见结果」——列表的空结果是正常业务答案（LISTED +
// 空数组，不泄露任何存在性），单份的不可见由详情页按同一语义呈现。
//
// 栏目跟真实读模型走：读模型没有的字段（客户委托参考、服务产品、目的范围）不虚构。
// 检索是页面侧对已取回行的便利过滤，不属于查询协议。

// 状态徽章：词表词取自 domain/status 的领域状态词表（两处词同源于 CONTEXT 原词，
// 断言只桥接两个模块的类型边界）；词表没收录的状态原样示码，不猜色调。
function requestStateBadge(state: string) {
  const word = requestStateLabels[state];
  return word !== undefined ? (
    <StatusBadgeFor status={word as DomainStatus} />
  ) : (
    <span className="font-mono text-[12px] text-idpxyz-textMuted">{state}</span>
  );
}

const columns: ListColumn<ShipmentRequestSummary>[] = [
  {
    id: 'shipment-request-id',
    header: '委托标识',
    className: 'w-[180px]',
    render: (row) => (
      <span className="font-mono text-[12px] text-idpxyz-accent">
        {row.shipmentRequestId}
      </span>
    ),
  },
  {
    id: 'customer-account',
    header: '客户账户',
    render: (row) => <span className="font-mono text-[12px]">{row.customerAccountId}</span>,
  },
  {
    id: 'state',
    header: '委托状态',
    className: 'w-[112px]',
    render: (row) => requestStateBadge(row.state),
  },
  {
    id: 'source',
    header: '来源',
    className: 'w-[132px]',
    render: (row) => <span className="font-mono text-[12px]">{row.source}</span>,
  },
  {
    id: 'source-request-key',
    header: '来源请求键',
    render: (row) => <span className="font-mono text-[12px]">{row.sourceRequestKey}</span>,
  },
  {
    id: 'declared-parcel-count',
    header: '声明包裹',
    align: 'right',
    className: 'w-[88px]',
    render: (row) => row.declaredParcelCount,
  },
  {
    id: 'submitted-at',
    header: '提交时间',
    className: 'w-[210px]',
    render: (row) => <span className="font-mono text-[12px]">{row.submittedAt}</span>,
  },
];

// 取数答案 → 模板四态。空列表走空态而不是就绪态的空表格：模板明言不从 rows.length
// 推断，「暂无数据」这一业务事实要由本页说出来。
function viewStateOf(
  answer: ApiResult<ViewsListResponseBody> | null,
  rowCount: number,
  retry: () => void,
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0
        ? {
            kind: 'empty',
            title: '当前作用域内没有可见委托',
            description:
              '空列表是正常业务答案（LISTED），不是故障；作用域内出现委托后本页即可见。',
          }
        : { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description:
          '查询端点已建立，但接入渠道未配置（403 ACCESS_CHANNEL_NOT_CONFIGURED）。' +
          '恢复动作是提供渠道参数（PAR-INT-01），改请求或重试不会改变结果。',
      };
    case 'callerProblem':
      return {
        kind: 'error',
        title: `调用方式问题（HTTP ${answer.status}）`,
        description: problemNote(answer.code),
      };
    case 'noAnswer':
      return {
        kind: 'error',
        title: `服务端未形成答案（HTTP ${answer.status}）`,
        description: problemNote(answer.code),
        onRetry: retry,
      };
    case 'transport':
      return {
        kind: 'error',
        title: '请求未到达 parcel-api',
        description: `${answer.message}；请确认代理与服务端在运行（约定走 /api 经代理转发）。`,
        onRetry: retry,
      };
  }
}

export function ShipmentRequestListPage() {
  const [keyword, setKeyword] = useState('');
  // 钻取选中：列表与详情共用一个导航位，选中后整区切详情。选中态的唯一来源是
  // hash，点行写 hash、状态经 hashchange 回流，与外壳同一纪律，不双写。
  const [selectedId, setSelectedId] = useState<string | null>(selectedIdFromHash);
  // null 表示取数中；答案（含各种未形成）一律进 answer，页面不吞任何一格。
  const [answer, setAnswer] = useState<ApiResult<ViewsListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    const onHashChange = () => setSelectedId(selectedIdFromHash());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listShipmentRequestViews().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  if (selectedId !== null) {
    return (
      <ShipmentRequestDetailPage
        shipmentRequestId={selectedId}
        onBack={() => {
          window.location.hash = '#/shipment-request-inquiry';
        }}
      />
    );
  }

  const retry = () => setReloadToken((token) => token + 1);
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];
  const needle = keyword.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        [row.shipmentRequestId, row.sourceRequestKey, row.customerAccountId].some(
          (field) => field.toLowerCase().includes(needle),
        ),
      )
    : rows;

  return (
    <ListPageTemplate<ShipmentRequestSummary>
      title={info.title}
      description="小包托运（parcel-shipment）的委托与其生命周期结果，查询按授权查询作用域过滤。"
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按委托标识、来源请求键或客户账户检索',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `共 ${visibleRows.length} 条` : undefined
      }
      columns={columns}
      rows={visibleRows}
      rowKey={(row) => row.shipmentRequestId}
      onRowClick={(row) => {
        window.location.hash = `#/shipment-request-inquiry/${encodeURIComponent(row.shipmentRequestId)}`;
      }}
      viewState={viewStateOf(answer, rows.length, retry)}
    />
  );
}
