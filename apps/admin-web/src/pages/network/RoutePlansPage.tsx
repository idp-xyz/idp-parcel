import { useEffect, useState } from 'react';
import type { WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { shipmentNoOf } from '../shipment-request/ref-no';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import type { CustomerNameIndex } from '../shipment-request/shipment-request-list';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { codeStatus } from '../visibility/exception-list';
import { RowStatusPill } from '../visibility/ExceptionDetailPanel';
import { labelFor } from '../../vocabulary/status';
import {
  listInitialRoutes,
  listRouteReassessments,
  type InitialRouteListResponseBody,
  type InitialRouteRecord,
  type RouteReassessmentListResponseBody,
  type RouteReassessmentRecord,
} from './api';
import { routeSummaryOf } from './network-rows';
import { RecordDetailPanel, RouteWorkbench, SectionTabs, techRows } from './route-workbench';
import { customerText, parcelCell, parcelSearchCells, parcelSubtitle, type RoutePlansHeader } from './route-plan-cells';
import { RerouteSuggestionsView } from './RerouteSuggestionsView';
import { InitialRouteTiesView } from './InitialRouteTiesView';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['route-plans'];

// 路线规划，工作台形态：四本册（首次定路线 / 路线复核 / 改路建议 / 成本并列待裁）的签在工具条里切，前两册接
// GET /route-plans 按 ?register= 分派，后两册各走自己的读口与决定口（RerouteSuggestionsView、InitialRouteTiesView）。
//
// 行首是运营认的：哪票货（运单号，包裹号挂在下面）、哪个客户（名称从客户账户册转写，读不到显编码）、结论胶囊（即指标即筛）、
// 路线一句话、时间。计划版本、受理基线、服务目的、复核关联、失效依据与各段明细进右栏「技术详情」。
//
// 「无可用路线」与改路判定如实单列（CONTEXT 硬句）：前者是初始判断结论的封闭一格，照词呈现不折进成计划行；改路判定缺席是
// 没评估过改路，与「不许改路」分开说。路线一句话取读口随行带出的各段（network-rows 的 routeSummaryOf），不拆快照另立读法。


const routeText = (row: InitialRouteRecord) => routeSummaryOf(row.legs) || '—';

function initialRouteColumns(names: CustomerNameIndex): WorkbenchColumn<InitialRouteRecord>[] {
  return [
    { key: 'parcel', header: '运单', render: parcelCell },
    { key: 'customer', header: '客户', render: (row) => customerText(row.customerAccountId, names) },
    { key: 'conclusion', header: '结论', render: (row) => <RowStatusPill status={codeStatus(row.conclusion)} /> },
    { key: 'route', header: '路线', hideWhenDetailOpen: true, render: (row) => <span className="font-mono text-[12px]">{routeText(row)}</span> },
    {
      key: 'recorded-at',
      header: '定路线时间',
      sortKey: 'recorded-at',
      hideWhenDetailOpen: true,
      render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{formatInstant(row.recordedAt)}</span>,
    },
  ];
}

function InitialRouteDetail({ row, names, onClose }: { row: InitialRouteRecord; names: CustomerNameIndex; onClose: () => void }) {
  return (
    <RecordDetailPanel
      title={shipmentNoOf(row.shipmentRequestId)}
      status={codeStatus(row.conclusion)}
      subtitle={parcelSubtitle(row, names)}
      facts={[
        { label: '路线', value: <span className="font-mono">{routeText(row)}</span> },
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
        { label: '运单标识', value: row.shipmentRequestId },
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
              <span className="font-mono text-idpxyz-text">
                {leg.from} → {leg.kind === 'DELIVERY' ? '末端派送' : leg.to}
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

function reassessmentColumns(names: CustomerNameIndex): WorkbenchColumn<RouteReassessmentRecord>[] {
  return [
    { key: 'parcel', header: '运单', render: parcelCell },
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
      sortKey: 'reassessed-at',
      hideWhenDetailOpen: true,
      render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{formatInstant(row.reassessedAt)}</span>,
    },
  ];
}

function ReassessmentDetail({ row, names, onClose }: { row: RouteReassessmentRecord; names: CustomerNameIndex; onClose: () => void }) {
  return (
    <RecordDetailPanel
      title={shipmentNoOf(row.shipmentRequestId)}
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
        { label: '运单标识', value: row.shipmentRequestId },
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

type RoutePlansView = 'initial-route' | 'reassessment' | 'reroute-suggestion' | 'initial-route-tie';

const routePlansViews: { id: RoutePlansView; label: string }[] = [
  { id: 'initial-route', label: '首次定路线' },
  { id: 'reassessment', label: '路线复核' },
  { id: 'reroute-suggestion', label: '改路建议' },
  { id: 'initial-route-tie', label: '成本并列待裁' },
];

// 按册装载的答案：register 与答案体绑定成判别联合，切册期间旧册答案不冒充新册。
type LoadedAnswer =
  | { register: 'initial-route'; answer: ApiResult<InitialRouteListResponseBody> }
  | { register: 'reassessment'; answer: ApiResult<RouteReassessmentListResponseBody> };

const emptyDescription = '运单受理后，系统会为每件包裹定路线并在运输途中复核；有了这样的记录就会出现在这里。';

/**
 * 路线规划：路由判断两册的查阅面，外加改路建议与成本并列两册的决定面。计划以包裹为单位、同一业务时点只有一个当前有效
 * 计划——这句约束由判断库与适用性登记保证，本页照登转写，不维护执行状态、不选版。
 */
export function RoutePlansPage() {
  const [register, setRegister] = useState<RoutePlansView>('initial-route');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<LoadedAnswer | null>(null);
  const names = useCustomerNames();

  useEffect(() => {
    let cancelled = false;
    if (register === 'initial-route') {
      void listInitialRoutes().then((answer) => {
        if (!cancelled) setLoaded({ register: 'initial-route', answer });
      });
    } else if (register === 'reassessment') {
      void listRouteReassessments().then((answer) => {
        if (!cancelled) setLoaded({ register: 'reassessment', answer });
      });
    }
    return () => {
      cancelled = true;
    };
  }, [register, reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const header: RoutePlansHeader = {
    title: info.title,
    subtitle: '每件包裹走哪条路线、复核过几次、要不要改路。定不出路线或不能自动改路时，到「改路建议」「成本并列待裁」里处理。',
    sections: <SectionTabs sections={routePlansViews} active={register} onChange={setRegister} />,
  };

  if (register === 'reroute-suggestion') return <RerouteSuggestionsView key={register} header={header} />;
  if (register === 'initial-route-tie') return <InitialRouteTiesView key={register} header={header} />;

  if (register === 'initial-route') {
    const answer = loaded?.register === 'initial-route' ? loaded.answer : null;
    const judgments: InitialRouteRecord[] = answer?.kind === 'outcome' ? answer.body.judgments : [];
    return (
      <RouteWorkbench<InitialRouteRecord>
        key={register}
        {...header}
        answered={answer?.kind === 'outcome'}
        noun="件"
        rows={judgments}
        // 同一托运申请同一服务目的一行（判断库唯一键），键取五维里够定行的三维。
        rowKey={(row) => `${row.shipmentRequestId}#${row.servicePurpose}#${row.acceptanceBaseline}`}
        statusOf={(row) => codeStatus(row.conclusion)}
        rowAttention={(row) => row.conclusion === 'NO_CURRENT_ROUTE'}
        searchCells={(row) => [...parcelSearchCells(row, names), routeText(row)]}
        searchPlaceholder="搜运单号 / 包裹号 / 客户 / 网点"
        columns={initialRouteColumns(names)}
        sortAccessors={{ 'recorded-at': { value: (row) => row.recordedAt, instant: true } }}
        defaultSort={{ key: 'recorded-at', dir: -1 }}
        renderDetail={(row, close) => <InitialRouteDetail row={row} names={names} onClose={close} />}
        onRefresh={retry}
        viewState={catalogueViewState(answer, judgments.length, retry, {
          module: info,
          endpoint: 'GET /route-plans?register=initial-route',
          emptyTitle: '还没有定过路线',
          emptyDescription,
        })}
      />
    );
  }

  const answer = loaded?.register === 'reassessment' ? loaded.answer : null;
  const reassessments = answer?.kind === 'outcome' ? answer.body.reassessments : [];
  return (
    <RouteWorkbench<RouteReassessmentRecord>
      key={register}
      {...header}
      answered={answer?.kind === 'outcome'}
      noun="次"
      rows={reassessments}
      rowKey={(row) => row.correlationId}
      statusOf={(row) => codeStatus(row.conclusion)}
      rowAttention={(row) => row.conclusion === 'PLAN_LAPSED'}
      searchCells={(row) => parcelSearchCells(row, names)}
      searchPlaceholder="搜运单号 / 包裹号 / 客户"
      columns={reassessmentColumns(names)}
      sortAccessors={{ 'reassessed-at': { value: (row) => row.reassessedAt, instant: true } }}
      defaultSort={{ key: 'reassessed-at', dir: -1 }}
      renderDetail={(row, close) => <ReassessmentDetail row={row} names={names} onClose={close} />}
      onRefresh={retry}
      viewState={catalogueViewState(answer, reassessments.length, retry, {
        module: info,
        endpoint: 'GET /route-plans?register=reassessment',
        emptyTitle: '还没有复核过路线',
        emptyDescription,
      })}
    />
  );
}
