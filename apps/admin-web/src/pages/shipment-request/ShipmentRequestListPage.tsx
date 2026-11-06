import { useState } from 'react';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import {
  WorkbenchPageTemplate,
  decodeHashSegment,
  toggleWorkbenchSort,
  useAddressKeyword,
  useTabReturn,
  type TemplateViewState,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { labelFor } from '../../vocabulary/status';
import { objectNouns } from '../../vocabulary/objects';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { problemNote } from './presentation';
import { listShipmentRequestViews, type ApiResult, type ShipmentRequestSummary, type ViewsListResponseBody } from './api';
import { CursorPager } from './cursor-pager';
import { ShipmentRequestDetailPage } from './ShipmentRequestDetailPage';
import { ShipmentRequestDetailPanel } from './ShipmentRequestDetailPanel';
import { useCustomerNames } from './use-customer-names';
import {
  customerDisplayOf,
  defaultShipmentSort,
  shipmentListConditions,
  shipmentNoMatchNote,
  shipmentStates,
  type CustomerNameIndex,
  type ShipmentStateFilter,
} from './shipment-request-list';
import { formatInstant } from '../moment';
import { shipmentDisplayNoOf } from './ref-no';

// 页面标题、一句话与主责出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['shipment-request-inquiry'];

// 运单（领域：委托查阅），对 GET /shipment-request-views 真实端点取数；改版第一步换成工作台形态（设计源「五、改版示意 · 运单」）：
// 列只放人要看的——运单号（下一行客户单号）、客户（名称从参与方册转写）、件数、状态、提交时间；来源主体与各种 ID 进右栏
// 详情底部的「技术详情」。运单号是委托的单据号（ADR-0355 决定十一），委托标识进技术详情；没有单据号的委托照旧显委托标识。
//
// 两条查询语义红线（出处均为 CONTEXT.md）在接线后的落点：「授权查询作用域」——作用域整组由服务端接入面从认证与授权结果
// 裁决，本页不送任何自报身份参数，也不自建跨客户查询层；「统一不可见结果」——列表的空结果是正常业务答案（LISTED + 空数组，
// 不泄露任何存在性），单份的不可见由详情页按同一语义呈现。
//
// 栏目跟真实读模型走：读模型没有的字段（目的地、重量、收件人、预计送达）不虚构，设计源第八节记着要补的读口。
//
// 读口已迁到 ADR-0144，按游标翻页（工具条右端）。检索词（地址上的 `?q=`，停稳后）、状态胶囊与提交时间的先后下推成查询条件
// （shipment-request-list 的 shipmentListConditions），不在答回来的那一页上再筛再排。客户名称不在读口检索的列里，检索框的
// 提示因此不写客户；读口另有客户账户一维，本页没有对应的筛选条，不另加。状态胶囊不带计数、命令头不摆指标：一页之内数出来的
// 不是各状态的总数，按状态的总数读口不给；当前条件下共几张由翻页条说。

// 详情钻取选中承载在 hash 第二段（#/shipment-request-inquiry/<运单号>），与外壳的模块级 hash 路由同一约定：刷新回到同一份
// 详情、后退自然收回列表、详情可收藏转发。多标签壳层拿前两段作标签 id，详情因此开成自己的一张标签、列表留在另一张。
//
// 第二段放屏上那个号（shipmentDisplayNoOf）而不是委托标识整串：壳层的标签名、状态栏位置与最近打开都按地址第二段写字
// （shell/workspace-state 的 tabForHash、pages/my-work/recent-objects 的 recentObjectTitle），不为补名字发请求，写出来就与列表上
// 那一格同字。有运单号是运单号——开立后不变（ADR-0355 决定三），作地址收藏转发照样稳；老运单是去前缀的委托标识。详情页按
// 运单号与这个号可能对应的委托标识一起认（shipment-ref 的 shipmentDetailSourceOf），从前收藏的委托标识整串地址照样打得开。
function selectedRefFromHash(): string | null {
  const segments = window.location.hash.replace(/^#\/?/, '').split('/');
  return segments[0] === 'shipment-request-inquiry' && segments[1] ? decodeHashSegment(segments[1]) : null;
}

function detailHash(row: Pick<ShipmentRequestSummary, 'shipmentRequestId' | 'documentNumber'>): string {
  return `#/shipment-request-inquiry/${encodeURIComponent(shipmentDisplayNoOf(row))}`;
}

// 表头只有提交时间一列可排：别的列在读口上没有对应的排序维，在答回来的一页里排只排得动这一页。
function columnsOf(names: CustomerNameIndex): WorkbenchColumn<ShipmentRequestSummary>[] {
  return [
    {
      key: 'shipment-id',
      header: objectNouns.shipmentNumber,
      render: (row) => (
        <div className="min-w-0">
          {/* 标识取主题的链接色 text.link.hover：强调色落在画布与选中行上对比度不够（集团与法人页同一判据）。 */}
          <div className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={shipmentDisplayNoOf(row)}>
            {shipmentDisplayNoOf(row)}
          </div>
          <div className="truncate type-body text-idpxyz-breadcrumb" title={row.sourceRequestKey}>
            {objectNouns.customerReference} <span className="font-mono">{row.sourceRequestKey}</span>
          </div>
        </div>
      ),
    },
    {
      key: 'customer',
      header: objectNouns.customer,
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
// 说出来；带检索词或状态答出零行不是空态（narrowed，同 catalogue-view 的那一格）——留在就绪态，表格区另显 noMatchText。
function viewStateOf(
  answer: ApiResult<ViewsListResponseBody> | null,
  rowCount: number,
  retry: () => void,
  narrowed: boolean,
): TemplateViewState {
  if (answer === null) return { kind: 'loading', shape: 'table' };
  switch (answer.kind) {
    case 'outcome':
      return rowCount === 0 && !narrowed
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
        // 读口按 ADR-0144 拒掉的查询（集外的键、坏游标）随答一句理由散文，原样接在后面。
        description: answer.detail === undefined ? problemNote(answer.code) : `${problemNote(answer.code)} ${answer.detail}`,
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
  // 检索词在地址里（`?q=`）：壳层把列表与每份详情开成各自的标签；离开满新鲜期回来、刷新、收藏打开都是新实例——检索词靠地址活下来。
  const [search, setSearch] = useAddressKeyword();
  const tabReturn = useTabReturn();
  // 渲列表还是详情由 hash 二段定：详情地址在壳层是自己的一张标签，本组件在那张标签里渲详情。选中态的唯一来源是 hash。
  const [detailRef, setDetailRef] = useState<string | null>(selectedRefFromHash);
  const [state, setState] = useState<ShipmentStateFilter>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultShipmentSort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const conditions = shipmentListConditions(useSettledKeyword(search), state, sort);
  // null 表示取数中；答案（含各种未形成）一律进 answer，页面不吞任何一格。换条件回第一页、翻页与重取都在 useCatalogueCursor 里。
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listShipmentRequestViews);
  const names = useCustomerNames();

  // 只认本标签名下的地址：列表与详情是两张标签，藏着的列表若照读详情标签的地址会自己翻成详情。
  useOwnAddressChange(() => setDetailRef(selectedRefFromHash()));

  if (detailRef !== null) {
    return <ShipmentRequestDetailPage shipmentRef={detailRef} onBack={() => tabReturn.returnTo('shipment-request-inquiry')} />;
  }

  const rows = answer?.kind === 'outcome' ? answer.body.requests : [];
  const toggleState = (code: ShipmentStateFilter) => setState(state === code ? 'ALL' : code);
  const statusChips: WorkbenchChip[] = [
    { id: 'ALL', label: '全部', active: state === 'ALL', onClick: () => setState('ALL') },
    ...shipmentStates.map((code) => ({ id: code, label: labelFor(code), active: state === code, onClick: () => toggleState(code) })),
  ];

  return (
    <WorkbenchPageTemplate<ShipmentRequestSummary>
      title={info.title}
      subtitle={info.summary}
      onRefresh={retry}
      refreshing={answer === null}
      search={{ value: search, onChange: setSearch, placeholder: '搜运单号、客户单号…' }}
      statusChips={statusChips}
      toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
      columns={columnsOf(names)}
      rows={rows}
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
        const row = rows.find((item) => item.shipmentRequestId === key);
        return row ? (
          <ShipmentRequestDetailPanel
            row={row}
            names={names}
            onClose={() => setSelectedKey(null)}
            onOpenDetail={() => {
              window.location.hash = detailHash(row);
            }}
          />
        ) : null;
      }}
      viewState={viewStateOf(answer, rows.length, retry, catalogueConditionsNarrow(conditions))}
    />
  );
}
