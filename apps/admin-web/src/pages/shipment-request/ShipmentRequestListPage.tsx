import { useEffect, useState } from 'react';
import {
  WorkbenchPageTemplate,
  decodeHashSegment,
  toggleWorkbenchSort,
  useAddressKeyword,
  useTabReturn,
  type TemplateViewState,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchKpi,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { statusToneText } from '../../components/status-tone';
import { toneFor, type StatusTone } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { problemNote } from './presentation';
import { listShipmentRequestViews, type ApiResult, type ShipmentRequestSummary, type ViewsListResponseBody } from './api';
import { ShipmentRequestDetailPage } from './ShipmentRequestDetailPage';
import { ShipmentRequestDetailPanel } from './ShipmentRequestDetailPanel';
import { useCustomerNames } from './use-customer-names';
import {
  countShipmentsByState,
  customerDisplayOf,
  defaultShipmentSort,
  filterShipments,
  shipmentCountSummary,
  shipmentNoMatchNote,
  sortShipments,
  type CustomerNameIndex,
  type ShipmentStateFilter,
} from './shipment-request-list';
import { formatInstant } from '../moment';

// 页面标题、一句话与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['shipment-request-inquiry'];

// 运单（领域：委托查阅），对 GET /shipment-request-views 真实端点取数；改版第一步换成工作台形态（设计源「五、改版示意 · 运单」）：
// 列只放人要看的——运单号（下一行客户单号）、客户（名称从参与方册转写）、件数、状态、提交时间；来源主体与各种 ID 进右栏
// 详情底部的「技术详情」。
//
// 两条查询语义红线（出处均为 CONTEXT.md）在接线后的落点：「授权查询作用域」——作用域整组由服务端接入面从认证与授权结果
// 裁决，本页不送任何自报身份参数，也不自建跨客户查询层；「统一不可见结果」——列表的空结果是正常业务答案（LISTED + 空数组，
// 不泄露任何存在性），单份的不可见由详情页按同一语义呈现。
//
// 栏目跟真实读模型走：读模型没有的字段（目的地、重量、收件人、预计送达）不虚构，设计源第八节记着要补的读口。
// 检索、筛选、排序只在已取回的这一页行上做，不属于查询协议。

// 详情钻取选中承载在 hash 第二段（#/shipment-request-inquiry/<委托标识>），与外壳的模块级 hash 路由同一约定：刷新回到同一份
// 详情、后退自然收回列表、详情可收藏转发。多标签壳层拿前两段作标签 id，详情因此开成自己的一张标签、列表留在另一张。
function selectedIdFromHash(): string | null {
  const segments = window.location.hash.replace(/^#\/?/, '').split('/');
  return segments[0] === 'shipment-request-inquiry' && segments[1] ? decodeHashSegment(segments[1]) : null;
}

function detailHash(shipmentRequestId: string): string {
  return `#/shipment-request-inquiry/${encodeURIComponent(shipmentRequestId)}`;
}

// 词典五档色调 → 指标数值的字色类（components/status-tone 只有四档语气，中性不着色）。
const kpiToneClass: Partial<Record<StatusTone, string>> = {
  good: statusToneText.success,
  info: statusToneText.info,
  warn: statusToneText.warning,
  bad: statusToneText.danger,
};

function columnsOf(names: CustomerNameIndex): WorkbenchColumn<ShipmentRequestSummary>[] {
  return [
    {
      key: 'shipment-id',
      header: objectNouns.shipmentNumber,
      sortKey: 'shipment-id',
      render: (row) => (
        <div className="min-w-0">
          {/* 标识取主题的链接色 text.link.hover：强调色落在画布与选中行上对比度不够（集团与法人页同一判据）。 */}
          <div className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.shipmentRequestId}>
            {row.shipmentRequestId}
          </div>
          <div className="truncate text-[12px] text-idpxyz-breadcrumb" title={row.sourceRequestKey}>
            {objectNouns.customerReference} <span className="font-mono">{row.sourceRequestKey}</span>
          </div>
        </div>
      ),
    },
    {
      key: 'customer',
      header: objectNouns.customer,
      sortKey: 'customer',
      render: (row) => {
        const customer = customerDisplayOf(row, names);
        return customer.isCode ? (
          <span className="font-mono text-idpxyz-text" title="参与方册上还没有这个账户的名称，显示的是账户编码">
            {customer.text}
          </span>
        ) : (
          <span className="text-idpxyz-textBright">{customer.text}</span>
        );
      },
    },
    {
      key: 'parcel-count',
      header: objectNouns.parcelCount,
      sortKey: 'parcel-count',
      right: true,
      width: '72px',
      render: (row) => <span className="font-mono">{row.declaredParcelCount}</span>,
    },
    {
      key: 'state',
      header: objectNouns.status,
      render: (row) => <StatusPill code={row.state} />,
    },
    {
      key: 'submitted-at',
      header: objectNouns.submittedAt,
      sortKey: 'submitted-at',
      hideWhenDetailOpen: true,
      render: (row) => (
        <time className="font-mono text-idpxyz-breadcrumb" dateTime={row.submittedAt} title={row.submittedAt}>
          {formatInstant(row.submittedAt)}
        </time>
      ),
    },
  ];
}

// 取数答案 → 模板页面态。空列表走空态而不是就绪态的空表格：模板明言不从 rows.length 推断，「还没有运单」这一业务事实要由本页
// 说出来；筛出为空不是空态——viewState 按总数判，表格区另显 noMatchText。
function viewStateOf(answer: ApiResult<ViewsListResponseBody> | null, rowCount: number, retry: () => void): TemplateViewState {
  if (answer === null) return { kind: 'loading', shape: 'table' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0
        ? {
            kind: 'empty',
            title: '还没有运单',
            description: '客户下单或在「新建与撤回」录入后，这里就会出现。',
          }
        : { kind: 'ready' };
    case 'unconfigured':
      return {
        kind: 'unconfigured',
        title: '接入渠道未配置',
        description: '运单查询的接入渠道还没配置好，这不是故障：重试不会改变结果，需要先完成渠道配置。',
        facts: { owner: info.owner, source: info.source, endpoint: 'GET /shipment-request-views' },
      };
    case 'loginRequired':
    case 'notGranted':
      return { kind: answer.kind, facts: { owner: info.owner, source: info.source } };
    case 'callerProblem':
      return {
        kind: 'error',
        title: '这次查询没有被接受',
        description: problemNote(answer.code),
        technical: [
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '问题码', value: answer.code },
        ],
      };
    case 'noAnswer':
      return {
        kind: 'error',
        title: '这次没有取到运单',
        description: problemNote(answer.code),
        onRetry: retry,
        technical: [
          { label: 'HTTP 状态', value: String(answer.status) },
          { label: '问题码', value: answer.code },
        ],
      };
    case 'transport':
      return {
        kind: 'error',
        title: '连不上服务',
        description: '请求没有到达后端；请确认网络与服务在运行后重试。',
        onRetry: retry,
        technical: [{ label: '传输层', value: answer.message }],
      };
  }
}

export function ShipmentRequestListPage() {
  // 检索词在地址里（`?q=`）：壳层把列表与每份详情开成各自的标签、非活动标签卸载，进详情再回来是新实例——检索词靠地址活下来。
  const [search, setSearch] = useAddressKeyword();
  const tabReturn = useTabReturn();
  // 渲列表还是详情由 hash 二段定：详情地址在壳层是自己的一张标签，本组件在那张标签里渲详情。选中态的唯一来源是 hash。
  const [detailId, setDetailId] = useState<string | null>(selectedIdFromHash);
  const [state, setState] = useState<ShipmentStateFilter>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultShipmentSort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  // null 表示取数中；答案（含各种未形成）一律进 answer，页面不吞任何一格。
  const [answer, setAnswer] = useState<ApiResult<ViewsListResponseBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  const names = useCustomerNames();

  useEffect(() => {
    const onHashChange = () => setDetailId(selectedIdFromHash());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  useEffect(() => {
    let cancelled = false;
    // 刷新不清上一份业务答案：旧表在新答案到来前继续可用；首取与非业务答案的重试仍回到加载中。
    setAnswer((previous) => (previous?.kind === 'outcome' ? previous : null));
    void listShipmentRequestViews().then((result) => {
      if (cancelled) return;
      setAnswer(result);
      setRefreshing(false);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  if (detailId !== null) {
    return <ShipmentRequestDetailPage shipmentRequestId={detailId} onBack={() => tabReturn.returnTo('shipment-request-inquiry')} />;
  }

  const retry = () => {
    setRefreshing(true);
    setReloadToken((token) => token + 1);
  };
  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];
  const answered = answer?.kind === 'outcome';
  const visible = sortShipments(filterShipments(rows, { search, state }, names), sort, names);
  const counts = countShipmentsByState(rows);
  const toggleState = (code: string) => setState(state === code ? 'ALL' : code);

  // 指标即筛：只列这一页上真出现过的状态，按生命周期顺序；词与色调经用语词典。
  const kpis: WorkbenchKpi[] | undefined = answered
    ? counts.map((item) => ({
        label: item.label,
        value: item.count,
        tone: kpiToneClass[toneFor(item.code)],
        active: state === item.code,
        onClick: () => toggleState(item.code),
      }))
    : undefined;

  const statusChips: WorkbenchChip[] | undefined = answered
    ? [
        { id: 'ALL', label: '全部', count: rows.length, active: state === 'ALL', onClick: () => setState('ALL') },
        ...counts.map((item) => ({
          id: item.code,
          label: item.label,
          count: item.count,
          active: state === item.code,
          onClick: () => toggleState(item.code),
        })),
      ]
    : undefined;

  return (
    <WorkbenchPageTemplate<ShipmentRequestSummary>
      title={info.title}
      subtitle={info.summary}
      kpis={kpis}
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: '搜运单号、客户单号、客户…' }}
      statusChips={statusChips}
      toolbarRight={answered ? shipmentCountSummary(rows.length, visible.length) : undefined}
      columns={columnsOf(names)}
      rows={visible}
      rowKey={(row) => row.shipmentRequestId}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText={shipmentNoMatchNote}
      onClearFilters={() => {
        setSearch('');
        setState('ALL');
      }}
      renderDetail={(key) => {
        const row = visible.find((item) => item.shipmentRequestId === key);
        return row ? (
          <ShipmentRequestDetailPanel
            row={row}
            names={names}
            onClose={() => setSelectedKey(null)}
            onOpenDetail={() => {
              window.location.hash = detailHash(row.shipmentRequestId);
            }}
          />
        ) : null;
      }}
      viewState={viewStateOf(answer, rows.length, retry)}
    />
  );
}
