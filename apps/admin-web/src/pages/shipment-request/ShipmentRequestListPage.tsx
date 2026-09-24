import { useEffect, useState } from 'react';
import {
  ListPageTemplate,
  decodeHashSegment,
  presentFields,
  useAddressKeyword,
  useTabReturn,
  type InspectorContent,
  type ListColumn,
  type TemplateViewState,
} from '../../templates';

// 详情钻取选中承载在 hash 第二段（#/shipment-request-inquiry/<委托标识>），与外壳
// 的模块级 hash 路由同一约定：刷新回到同一份详情、后退自然收回列表、详情可收藏
// 转发。第二段的**含义**归本页（它是哪份委托）；外壳只把它当地址——多标签壳层拿前两段作
// 标签 id，详情因此开成自己的一张标签、列表留在另一张（shell/workspace-state.ts）。
function selectedIdFromHash(): string | null {
  const segments = window.location.hash.replace(/^#\/?/, '').split('/');
  return segments[0] === 'shipment-request-inquiry' && segments[1] ? decodeHashSegment(segments[1]) : null;
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

// 「导出所选（CSV）」按列取字：取行上已有的字段字面量，不取徽章渲出来的词——委托状态导出的是读模型的状态码，
// 与列里的徽章词是同一事实的两种呈现，CSV 给机器读，码比词稳。
const csvCellText = (row: ShipmentRequestSummary, column: ListColumn<ShipmentRequestSummary>) => {
  switch (column.id) {
    case 'shipment-request-id':
      return row.shipmentRequestId;
    case 'customer-account':
      return row.customerAccountId;
    case 'state':
      return row.state;
    case 'source':
      return row.source;
    case 'source-request-key':
      return row.sourceRequestKey;
    case 'declared-parcel-count':
      return String(row.declaredParcelCount);
    case 'submitted-at':
      return row.submittedAt;
    default:
      return undefined;
  }
};

/** 详情地址：hash 二段，与本页顶部 selectedIdFromHash 读的同一形；壳层会把它开成自己的标签。 */
function detailHash(shipmentRequestId: string): string {
  return `#/shipment-request-inquiry/${encodeURIComponent(shipmentRequestId)}`;
}

// 右侧检查器的内容（票 admin-web-workspace-form/02 第 4 条）：全部取自行里已有的读模型字段，不发第二个请求。
// 概要四格；状态一枚（词表词按词表着色，词表外的原样示码——与列里的徽章同一处置）；快速动作今天只有「打开详情」——
// 撤回 / 取消 / 复核各有自己的页与门（提交与撤回、逐件取消、复核队列），不从检查器发命令；关联对象没有——读模型里的
// 客户账户与来源请求键都不是本管理台里可寻址的对象地址，编一条链接就是死路；审计取提交时刻与提交版本（票面的「修订」）——
// 读模型没有「最近变更」。
function inspectorOf(row: ShipmentRequestSummary): InspectorContent {
  return {
    title: '委托',
    subtitle: row.shipmentRequestId,
    sections: [
      {
        kind: 'summary',
        fields: presentFields([
          { label: '客户账户', value: row.customerAccountId, mono: true },
          { label: '来源', value: row.source, mono: true },
          { label: '来源请求键', value: row.sourceRequestKey, mono: true },
          { label: '声明包裹', value: String(row.declaredParcelCount) },
        ]),
      },
      { kind: 'status', items: [{ label: '委托状态', word: requestStateLabels[row.state] ?? row.state }] },
      {
        kind: 'actions',
        actions: [
          {
            label: '打开详情',
            onRun: () => {
              window.location.hash = detailHash(row.shipmentRequestId);
            },
          },
        ],
      },
      {
        kind: 'audit',
        fields: presentFields([
          { label: '提交时间', value: row.submittedAt, mono: true },
          { label: '提交版本', value: row.submissionVersionId, mono: true },
        ]),
      },
    ],
  };
}

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
  // 检索词在地址里（`?q=`）：壳层把列表与每份详情开成各自的标签、非活动标签卸载，进详情再回来是新实例——检索词靠地址活下来
  // （壳层回程落回列表标签上次停的地址，本页挂载时读回，票 admin-web-workspace-form/06）。
  const [keyword, setKeyword] = useAddressKeyword();
  const tabReturn = useTabReturn();
  // 渲列表还是详情由 hash 二段定：详情地址在壳层是自己的一张标签，本组件在那张标签里渲详情。选中态的唯一来源是
  // hash，开行写 hash、状态经 hashchange 回流，与外壳同一纪律，不双写。
  const [selectedId, setSelectedId] = useState<string | null>(selectedIdFromHash);
  // 多选集（票 admin-web-workspace-form/04）：按委托标识记，翻页 / 改检索词都不清；进详情即丢——它是一次批量动作的暂存，
  // 不跟标签走（票 06）。批量动作只有导出所选——本仓今天没有能对一批委托做的命令端点，不传 extra。
  const [checked, setChecked] = useState<ReadonlySet<string>>(() => new Set());
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
        onBack={() => tabReturn.returnTo('shipment-request-inquiry')}
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
      selection={{ selected: checked, onChange: setChecked }}
      bulkActions={{ csv: { fileName: 'shipment-requests.csv', cellText: csvCellText } }}
      // 单击进检查器、双击（或 Enter）开详情——蓝图母版 B 的姿势：表还在左边，翻行时右栏跟着换。
      inspector={inspectorOf}
      onRowOpen={(row) => {
        window.location.hash = detailHash(row.shipmentRequestId);
      }}
      viewState={viewStateOf(answer, rows.length, retry)}
    />
  );
}
