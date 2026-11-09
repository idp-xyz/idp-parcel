import { useState } from 'react';
import type { WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import { useShipmentNumbers } from '../shipment-request/use-shipment-numbers';
import type { CustomerNameIndex } from '../shipment-request/shipment-request-list';
import { CursorPager } from '../shipment-request/cursor-pager';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { matchesSearch } from '../party/list-order';
import { codeStatus } from '../visibility/exception-list';
import { RowStatusPill } from '../visibility/ExceptionDetailPanel';
import { PageOnlySearch } from '../visibility/page-only';
import { labelFor } from '../../vocabulary/status';
import { listInitialRoutes, listRouteReassessments, type InitialRouteRecord, type RouteReassessmentRecord } from './api';
import { routeSummaryOf } from './network-rows';
import { placeLabel, type PlaceNameIndex } from './place-names';
import { usePlaceNames } from './use-place-names';
import { routeKeywordPlaceholders } from './presentation';
import type { RouteRegister } from './route-plan-conditions';
import { useRouteRegisterList } from './use-route-register-list';
import { RecordDetailPanel, RouteWorkbench, SectionTabs, techRows } from './route-workbench';
import {
  customerText,
  parcelCell,
  parcelColumnHeader,
  parcelPageOnlyCells,
  parcelSubtitle,
  parcelTitle,
  shipmentTechnicalRows,
  type RoutePlansHeader,
  type ShipmentNumberOf,
} from './route-plan-cells';
import { RerouteSuggestionsView } from './RerouteSuggestionsView';
import { InitialRouteTiesView } from './InitialRouteTiesView';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['route-plans'];

// 路线规划，工作台形态：四本册（首次定路线 / 路线复核 / 改路建议 / 成本并列待裁）的签在工具条里切，前两册接
// GET /route-plans 按 ?register= 分派，后两册各走自己的读口与决定口（RerouteSuggestionsView、InitialRouteTiesView）。
//
// 行首是运营认的：哪票货（运单号，包裹号挂在下面）、哪个客户（名称从客户账户册转写，读不到显编码）、结论胶囊、
// 路线一句话、时间。运单号按页上的委托标识经委托查阅取回（route-plan-cells 文件头）。委托标识、计划版本、受理基线、服务目的、
// 复核关联、失效依据与各段明细进右栏「技术详情」。
//
// 「无可用路线」与改路判定如实单列（CONTEXT 硬句）：前者是初始判断结论的封闭一格，照词呈现不折进成计划行；改路判定缺席是
// 没评估过改路，与「不许改路」分开说。路线一句话取读口随行带出的各段（network-rows 的 routeSummaryOf），不拆快照另立读法。
//
// 各册接读口分页（ADR-0144，票 catalogue-read-pagination-rollout/14；条件怎样拼在 route-plan-conditions.ts）：检索框下推成 `q`，
// 结论、状态胶囊下推成各册的筛选维，胶囊不带计数；次序是读口的缺省序（最新的在前）。列上显的正是读口可排那一维时（定路线时间、
// 发现并列的时间）表头先后下推；复核时间、建议时间不是可排维，那两列不给排。读口检索只认标识，运单号、客户名称与路线上的网点
// 名称在「只筛本页」的框里再找。


// 一句话路线里的网点按此刻生效那一版查名（use-place-names），没有名称的显编码；编码在右栏技术详情的逐段里。
const routeText = (row: InitialRouteRecord, places: PlaceNameIndex) => routeSummaryOf(row.legs, (node) => placeLabel(places, node)) || '—';

function initialRouteColumns(
  names: CustomerNameIndex,
  places: PlaceNameIndex,
  numberOf: ShipmentNumberOf,
): WorkbenchColumn<InitialRouteRecord>[] {
  return [
    { key: 'parcel', header: parcelColumnHeader, render: parcelCell(numberOf) },
    { key: 'customer', header: '客户', render: (row) => customerText(row.customerAccountId, names) },
    { key: 'conclusion', header: '结论', render: (row) => <RowStatusPill status={codeStatus(row.conclusion)} /> },
    { key: 'route', header: '路线', hideWhenDetailOpen: true, render: (row) => <span className="type-body">{routeText(row, places)}</span> },
    {
      key: 'recorded-at',
      header: '定路线时间',
      sortKey: 'recorded-at',
      hideWhenDetailOpen: true,
      render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{formatInstant(row.recordedAt)}</span>,
    },
  ];
}

function InitialRouteDetail({
  row,
  names,
  places,
  numberOf,
  onClose,
}: {
  row: InitialRouteRecord;
  names: CustomerNameIndex;
  places: PlaceNameIndex;
  numberOf: ShipmentNumberOf;
  onClose: () => void;
}) {
  return (
    <RecordDetailPanel
      title={parcelTitle(row, numberOf)}
      status={codeStatus(row.conclusion)}
      subtitle={parcelSubtitle(row, names)}
      facts={[
        { label: '路线', value: routeText(row, places) },
        {
          label: '这条路线现在',
          // 缺席两解都如实显：无路可走行本就无计划；成计划行缺席即尚无适用性登记。
          value: row.applicability
            ? `${labelFor(row.applicability.state)}（${formatInstant(row.applicability.transitionedAt)}）`
            : row.conclusion === 'NO_CURRENT_ROUTE'
              ? '没有路线'
              : '还没有变化',
        },
        { label: '定路线时间', value: formatInstant(row.recordedAt) },
        { label: '服务', value: labelFor(row.servicePurpose) },
      ]}
      technical={techRows([
        ...shipmentTechnicalRows(row, numberOf),
        { label: '包裹标识', value: row.declaredParcelId },
        { label: '客户编码', value: row.customerAccountId },
        { label: '计划版本', value: row.planVersion },
        { label: '受理基线', value: row.acceptanceBaseline },
        { label: '服务目的码', value: row.servicePurpose },
        { label: '结论码', value: row.conclusion },
        { label: '替代版本', value: row.applicability?.successor },
        { label: '状态依据', value: row.applicability?.basis },
        // 段引用单列一行、原样可复制：末端交接声明计划段时照抄它（demo-chain-gaps/72）。
        ...(row.legs ?? []).flatMap((leg, index) => [
          { label: `第 ${index + 1} 段引用`, value: leg.reference },
          { label: `第 ${index + 1} 段`, value: `${leg.kind} · ${leg.from} → ${leg.to} · ${leg.responsible} · ${leg.windowBasis}` },
        ]),
      ])}
      onClose={onClose}
    >
      {row.legs && row.legs.length > 0 ? (
        <ol className="space-y-1">
          {row.legs.map((leg) => (
            <li key={leg.reference} className="flex flex-wrap items-baseline gap-x-2">
              <span className="text-idpxyz-text">
                {placeLabel(places, leg.from)} → {leg.kind === 'DELIVERY' ? '末端派送' : placeLabel(places, leg.to)}
              </span>
              <span className="font-mono">
                {formatInstant(leg.earliest)} ~ {formatInstant(leg.latest)}
              </span>
            </li>
          ))}
        </ol>
      ) : null}
      {row.conclusion === 'NO_CURRENT_ROUTE' ? (
        <p>定路线时没有找到可用的路线：网点、连接或服务区域此刻都不满足。补登网络资料后，系统会在复核时重新判断。</p>
      ) : null}
    </RecordDetailPanel>
  );
}

function reassessmentColumns(names: CustomerNameIndex, numberOf: ShipmentNumberOf): WorkbenchColumn<RouteReassessmentRecord>[] {
  return [
    { key: 'parcel', header: parcelColumnHeader, render: parcelCell(numberOf) },
    { key: 'customer', header: '客户', render: (row) => customerText(row.customerAccountId, names) },
    { key: 'conclusion', header: '复核结果', render: (row) => <RowStatusPill status={codeStatus(row.conclusion)} /> },
    {
      key: 'reroute-state',
      header: '改路',
      hideWhenDetailOpen: true,
      // 缺席即没评估过改路（事实目录未配置或候选未收敛）——与「不许改路」是两回事。
      render: (row) =>
        row.rerouteState ? <RowStatusPill status={codeStatus(row.rerouteState)} /> : <span className="text-idpxyz-breadcrumb">未评估</span>,
    },
    {
      key: 'reassessed-at',
      header: '复核时间',
      hideWhenDetailOpen: true,
      render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{formatInstant(row.reassessedAt)}</span>,
    },
  ];
}

function ReassessmentDetail({
  row,
  names,
  numberOf,
  onClose,
}: {
  row: RouteReassessmentRecord;
  names: CustomerNameIndex;
  numberOf: ShipmentNumberOf;
  onClose: () => void;
}) {
  return (
    <RecordDetailPanel
      title={parcelTitle(row, numberOf)}
      status={codeStatus(row.conclusion)}
      subtitle={parcelSubtitle(row, names)}
      facts={[
        { label: '复核时间', value: formatInstant(row.reassessedAt) },
        // 缺席即本走向不评估备选（仅「路线仍可用」走向为空）。
        { label: '备选路线', value: row.candidateState ? labelFor(row.candidateState) : '不需要' },
        { label: '改路', value: row.rerouteState ? labelFor(row.rerouteState) : '未评估' },
        { label: '服务', value: labelFor(row.servicePurpose) },
      ]}
      technical={techRows([
        ...shipmentTechnicalRows(row, numberOf),
        { label: '包裹标识', value: row.declaredParcelId },
        { label: '客户编码', value: row.customerAccountId },
        { label: '复核关联', value: row.correlationId },
        { label: '被复核计划', value: row.reviewedPlan },
        { label: '失效依据', value: row.lapseBasis },
        { label: '受理基线', value: row.acceptanceBaseline },
        { label: '结论码', value: row.conclusion },
        { label: '登记时间', value: formatInstant(row.recordedAt) },
      ])}
      onClose={onClose}
    >
      {row.conclusion === 'PLAN_LAPSED' ? (
        <p>原来的路线已经失效。能自动改路时系统直接改；不能时会在「改路建议」里等人选一条备选路线。</p>
      ) : (
        <p>运输途中每过一次交接，系统都会复核一次这件包裹的路线还能不能用。</p>
      )}
    </RecordDetailPanel>
  );
}

const routePlansViews: { id: RouteRegister; label: string }[] = [
  { id: 'initial-route', label: '首次定路线' },
  { id: 'reassessment', label: '路线复核' },
  { id: 'reroute-suggestion', label: '改路建议' },
  { id: 'initial-route-tie', label: '成本并列待裁' },
];

const emptyDescription = '运单受理后，系统会为每件包裹定路线并在运输途中复核；有了这样的记录就会出现在这里。';

// 列上的定路线时间就是读口的可排维 recordedAt（ports.InitialRoutePages）。
const initialRouteSort = { key: 'recorded-at', field: 'recordedAt' };

// 判断键去掉租户后余下的各段（客户账户随委托定，不必进键）：同一委托的几件包裹各是一行。
const initialRouteKey = (row: InitialRouteRecord) =>
  `${row.shipmentRequestId}/${row.acceptanceBaseline}/${row.declaredParcelId}/${row.servicePurpose}`;

/**
 * 路线规划：路由判断两册的查阅面，外加改路建议与成本并列两册的决定面。计划以包裹为单位、同一业务时点只有一个当前有效
 * 计划——这句约束由判断库与适用性登记保证，本页照登转写，不维护执行状态、不选版。
 */
export function RoutePlansPage() {
  const [register, setRegister] = useState<RouteRegister>('initial-route');
  const header: RoutePlansHeader = {
    title: info.title,
    subtitle: '每件包裹走哪条路线、复核过几次、要不要改路。定不出路线或不能自动改路时，到「改路建议」「成本并列待裁」里处理。',
    sections: <SectionTabs sections={routePlansViews} active={register} onChange={setRegister} />,
  };

  // 换册重挂：上一册的胶囊、只筛本页的框、表头先后、翻页位置与选中行不带进下一册；检索词在地址上，跟着留下。
  if (register === 'reroute-suggestion') return <RerouteSuggestionsView key={register} header={header} />;
  if (register === 'initial-route-tie') return <InitialRouteTiesView key={register} header={header} />;
  if (register === 'initial-route') return <InitialRoutesView key={register} header={header} />;
  return <ReassessmentsView key={register} header={header} />;
}

function InitialRoutesView({ header }: { header: RoutePlansHeader }) {
  const names = useCustomerNames();
  const places = usePlaceNames();
  const list = useRouteRegisterList('initial-route', listInitialRoutes, initialRouteSort);
  const judgments = list.answer?.kind === 'outcome' ? list.answer.body.judgments : [];
  const numberOf = useShipmentNumbers(judgments.map((row) => row.shipmentRequestId), list.refreshes);
  const rows = judgments.filter((row) =>
    matchesSearch(list.refine, [...parcelPageOnlyCells(row, names, numberOf), routeText(row, places)]),
  );
  return (
    <RouteWorkbench<InitialRouteRecord>
      {...header}
      extraFilters={<PageOnlySearch value={list.refine} onChange={list.setRefine} subject="运单号、客户名称、网点" />}
      answered={list.answer?.kind === 'outcome'}
      noun="件"
      rows={rows}
      rowKey={initialRouteKey}
      statusChips={list.statusChips}
      onClearFilters={list.clearFilters}
      rowAttention={(row) => row.conclusion === 'NO_CURRENT_ROUTE'}
      searchCells={() => []}
      search={{ value: list.keyword, onChange: list.setKeyword, pushedDown: true }}
      searchPlaceholder={routeKeywordPlaceholders['initial-route']}
      columns={initialRouteColumns(names, places, numberOf)}
      pushedSort={list.pushedSort}
      renderDetail={(row, close) => <InitialRouteDetail row={row} names={names} places={places} numberOf={numberOf} onClose={close} />}
      onRefresh={list.refresh}
      toolbarRight={list.pagination ? <CursorPager pagination={list.pagination} /> : undefined}
      viewState={catalogueViewState(list.answer, judgments.length, list.refresh, {
        module: info,
        endpoint: 'GET /route-plans?register=initial-route',
        emptyTitle: '还没有定过路线',
        emptyDescription,
        narrowed: list.narrowed,
      })}
    />
  );
}

function ReassessmentsView({ header }: { header: RoutePlansHeader }) {
  const names = useCustomerNames();
  const list = useRouteRegisterList('reassessment', listRouteReassessments);
  const reassessments = list.answer?.kind === 'outcome' ? list.answer.body.reassessments : [];
  const numberOf = useShipmentNumbers(reassessments.map((row) => row.shipmentRequestId), list.refreshes);
  const rows = reassessments.filter((row) => matchesSearch(list.refine, parcelPageOnlyCells(row, names, numberOf)));
  return (
    <RouteWorkbench<RouteReassessmentRecord>
      {...header}
      extraFilters={<PageOnlySearch value={list.refine} onChange={list.setRefine} subject="运单号、客户名称" />}
      answered={list.answer?.kind === 'outcome'}
      noun="次"
      rows={rows}
      rowKey={(row) => row.correlationId}
      statusChips={list.statusChips}
      onClearFilters={list.clearFilters}
      rowAttention={(row) => row.conclusion === 'PLAN_LAPSED'}
      searchCells={() => []}
      search={{ value: list.keyword, onChange: list.setKeyword, pushedDown: true }}
      searchPlaceholder={routeKeywordPlaceholders.reassessment}
      columns={reassessmentColumns(names, numberOf)}
      renderDetail={(row, close) => <ReassessmentDetail row={row} names={names} numberOf={numberOf} onClose={close} />}
      onRefresh={list.refresh}
      toolbarRight={list.pagination ? <CursorPager pagination={list.pagination} /> : undefined}
      viewState={catalogueViewState(list.answer, reassessments.length, list.refresh, {
        module: info,
        endpoint: 'GET /route-plans?register=reassessment',
        emptyTitle: '还没有复核过路线',
        emptyDescription,
        narrowed: list.narrowed,
      })}
    />
  );
}
