import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { technicalRows } from '../settlement/ops-ui';
import { ParcelNo, ShipmentNo } from '../shipment-request/refs';
import { parcelNoOf, shipmentNoOf } from '../shipment-request/ref-no';
import { useCustomerNames } from '../shipment-request/use-customer-names';
import type { CustomerNameIndex } from '../shipment-request/shipment-request-list';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listInitialRoutes,
  listRouteReassessments,
  type InitialRouteListResponseBody,
  type InitialRouteRecord,
  type RouteReassessmentListResponseBody,
  type RouteReassessmentRecord,
} from './api';
import { routePlanRegisters, type RoutePlanRegister } from './presentation';
import { routeSummaryOf, type RouteLeg } from './network-rows';
import { RerouteSuggestionsView } from './RerouteSuggestionsView';
import { InitialRouteTiesView } from './InitialRouteTiesView';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['route-plans'];

// 本页接 GET /route-plans，两册按 ?register= 分派（首次定路线 / 路线复核），签切换。
//
// 列只放运营认的：哪票货（运单号，包裹号挂在下面）、哪个客户（名称从客户账户册转写，读不到显编码）、路线一句话、结论胶囊、
// 时间。计划版本、受理基线、服务目的、复核关联标识、失效依据这些系统引用进每行末尾的「技术详情」，要排查时展开、可复制。
//
// 「无可用路线」与改路判定如实单列（CONTEXT 硬句）：前者是初始判断结论的封闭一格，照词呈现不折进成计划行；改路判定缺席是
// 没评估过改路，与「不许改路」分开说。路线一句话取读口随行带出的各段（network-rows 的 routeSummaryOf），不拆快照另立读法。

/**
 * 成计划行另带各段（读口 plannedLegBody，ADR-0217）。不写进 api.ts 的 InitialRouteRecord：那份类型由后端契约夹具逐键守着，
 * 而夹具今天没有带段的行，写进去契约测试就要求一个夹具里没有的实例。等夹具补上带段的行再并回去。
 */
type InitialRouteRow = InitialRouteRecord & { legs?: RouteLeg[] };

/** 运单号在上、包裹号在下的两行格。 */
function parcelCell(row: { shipmentRequestId: string; declaredParcelId: string }) {
  return (
    <div className="flex min-w-0 flex-col items-start gap-0.5">
      <ShipmentNo shipmentRequestId={row.shipmentRequestId} />
      <span className="text-[11px] text-idpxyz-textMuted">
        包裹 <span className="font-mono">{parcelNoOf(row.declaredParcelId)}</span>
      </span>
    </div>
  );
}

function customerCell(account: string, names: CustomerNameIndex) {
  const name = names.get(account);
  return name ? <span title={account}>{name}</span> : <span className="font-mono text-[12px]">{account}</span>;
}

const techClass = 'max-w-md open:min-w-[18rem]';

function initialRouteColumns(names: CustomerNameIndex): ListColumn<InitialRouteRow>[] {
  return [
    { id: 'parcel', header: '运单', render: parcelCell },
    { id: 'customer', header: '客户', render: (row) => customerCell(row.customerAccountId, names) },
    {
      id: 'route',
      header: '路线',
      className: 'min-w-64',
      render: (row) => {
        const summary = routeSummaryOf(row.legs);
        return summary ? <span className="font-mono text-[12px]">{summary}</span> : <span className="text-idpxyz-textMuted">—</span>;
      },
    },
    { id: 'conclusion', header: '结论', align: 'center', render: (row) => <StatusPill code={row.conclusion} /> },
    {
      id: 'applicability',
      header: '这条路线现在',
      // 缺席两解都如实显「—」：无路可走行本就无计划；成计划行缺席即尚无适用性登记。
      render: (row) =>
        row.applicability ? (
          <div className="flex flex-col items-start gap-0.5">
            <StatusPill code={row.applicability.state} />
            <span className="font-mono text-[11px] text-idpxyz-textMuted">{formatInstant(row.applicability.transitionedAt)}</span>
          </div>
        ) : (
          <span className="text-idpxyz-textMuted">—</span>
        ),
    },
    { id: 'recorded-at', header: '定路线时间', className: 'min-w-44 font-mono text-xs', render: (row) => formatInstant(row.recordedAt) },
    {
      id: 'technical',
      header: '',
      className: 'align-top',
      render: (row) => (
        <TechnicalDetails
          className={techClass}
          rows={technicalRows([
            { label: '运单标识', value: row.shipmentRequestId },
            { label: '包裹标识', value: row.declaredParcelId },
            { label: '客户编码', value: row.customerAccountId },
            { label: '计划版本', value: row.planVersion },
            { label: '受理基线', value: row.acceptanceBaseline },
            { label: '服务目的', value: row.servicePurpose },
            { label: '结论码', value: row.conclusion },
            { label: '替代版本', value: row.applicability?.successor },
            { label: '状态依据', value: row.applicability?.basis },
            ...(row.legs ?? []).map((leg) => ({
              label: `第 ${leg.reference.split('#').pop()} 段`,
              value: `${leg.from} → ${leg.to} · ${leg.responsible} · ${formatInstant(leg.earliest)} ~ ${formatInstant(leg.latest)}`,
            })),
          ])}
        />
      ),
    },
  ];
}

function reassessmentColumns(names: CustomerNameIndex): ListColumn<RouteReassessmentRecord>[] {
  return [
    { id: 'parcel', header: '运单', render: parcelCell },
    { id: 'customer', header: '客户', render: (row) => customerCell(row.customerAccountId, names) },
    { id: 'conclusion', header: '复核结果', align: 'center', render: (row) => <StatusPill code={row.conclusion} /> },
    {
      id: 'candidate-state',
      header: '备选路线',
      align: 'center',
      // 缺席即本走向不评估（仅「路线仍可用」走向为空）——如实说，不显空。
      render: (row) =>
        row.candidateState ? <StatusPill code={row.candidateState} /> : <span className="text-xs text-idpxyz-textMuted">不需要</span>,
    },
    {
      id: 'reroute-state',
      header: '改路',
      align: 'center',
      // 缺席即没评估过改路（事实目录未配置或候选未收敛）——与「不许改路」是两回事。
      render: (row) =>
        row.rerouteState ? <StatusPill code={row.rerouteState} /> : <span className="text-xs text-idpxyz-textMuted">未评估</span>,
    },
    { id: 'reassessed-at', header: '复核时间', className: 'min-w-44 font-mono text-xs', render: (row) => formatInstant(row.reassessedAt) },
    {
      id: 'technical',
      header: '',
      className: 'align-top',
      render: (row) => (
        <TechnicalDetails
          className={techClass}
          rows={technicalRows([
            { label: '运单标识', value: row.shipmentRequestId },
            { label: '包裹标识', value: row.declaredParcelId },
            { label: '客户编码', value: row.customerAccountId },
            { label: '复核关联', value: row.correlationId },
            { label: '被复核计划', value: row.reviewedPlan },
            { label: '失效依据', value: row.lapseBasis },
            { label: '受理基线', value: row.acceptanceBaseline },
            { label: '服务目的', value: row.servicePurpose },
            { label: '登记时间', value: formatInstant(row.recordedAt) },
          ])}
        />
      ),
    },
  ];
}

/** 运单号、包裹号（整串与短写法）、客户名称与编码都能搜。 */
function matches(
  row: { shipmentRequestId: string; declaredParcelId: string; customerAccountId: string },
  needle: string,
  names: CustomerNameIndex,
): boolean {
  return [
    row.shipmentRequestId,
    shipmentNoOf(row.shipmentRequestId),
    row.declaredParcelId,
    parcelNoOf(row.declaredParcelId),
    row.customerAccountId,
    names.get(row.customerAccountId) ?? '',
  ].some((value) => value.toLowerCase().includes(needle));
}

// 本页的三册：路由判断两册之外，另有复核形成的改路建议一册（ADR-0284 决定六），它走自己的读口与决定口。
type RoutePlansView = RoutePlanRegister | 'reroute-suggestion' | 'initial-route-tie';

const routePlansViews: RoutePlansView[] = [...routePlanRegisters, 'reroute-suggestion', 'initial-route-tie'];

const routePlansViewLabels: Record<RoutePlansView, string> = {
  'initial-route': '首次定路线',
  reassessment: '路线复核',
  'reroute-suggestion': '改路建议',
  'initial-route-tie': '成本并列待裁',
};

// 按册装载的答案：register 与答案体绑定成判别联合，切册期间旧册答案不冒充新册。
type LoadedAnswer =
  | { register: 'initial-route'; answer: ApiResult<InitialRouteListResponseBody> }
  | { register: 'reassessment'; answer: ApiResult<RouteReassessmentListResponseBody> };

/**
 * 路线规划：路由判断两册的查阅面，外加改路建议与成本并列两册的决定面。计划以包裹为单位、同一业务时点只有一个当前有效
 * 计划——这句约束由判断库与适用性登记保证，本页照登转写，不维护执行状态、不选版。
 */
export function RoutePlansPage() {
  const [register, setRegister] = useState<RoutePlansView>('initial-route');
  const [keyword, setKeyword] = useState('');
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
  const needle = keyword.trim().toLowerCase();
  const chips = (
    <>
      {routePlansViews.map((candidate) => (
        <button
          key={candidate}
          type="button"
          className={chipClass(candidate === register)}
          onClick={() => setRegister(candidate)}
        >
          {routePlansViewLabels[candidate]}
        </button>
      ))}
    </>
  );
  const shared = {
    title: info.title,
    description: '每件包裹走哪条路线、之后复核过几次、要不要改路。定不出路线或路线失效又不能自动改路时，到「改路建议」「成本并列待裁」里处理。',
    search: {
      value: keyword,
      onChange: setKeyword,
      placeholder: '搜运单号 / 包裹号 / 客户',
    },
    filters: chips,
  };
  const emptyDescription = '运单受理后，系统会为每件包裹定路线并在运输途中复核；有了这样的记录就会出现在这里。';

  if (register === 'reroute-suggestion') {
    return <RerouteSuggestionsView header={shared} keyword={keyword} />;
  }

  if (register === 'initial-route-tie') {
    return <InitialRouteTiesView header={shared} keyword={keyword} />;
  }

  if (register === 'initial-route') {
    const answer = loaded?.register === 'initial-route' ? loaded.answer : null;
    const judgments: InitialRouteRow[] = answer?.kind === 'outcome' ? answer.body.judgments : [];
    const visibleJudgments = needle
      ? judgments.filter((row) => matches(row, needle, names))
      : judgments;
    return (
      <ListPageTemplate<InitialRouteRow>
        {...shared}
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 行」会与状态区
          // 「这不是登记册为空」直接矛盾（README 列表页上列通则第六条）。
          answer?.kind === 'outcome' ? `共 ${judgments.length} 件` : undefined
        }
        columns={initialRouteColumns(names)}
        rows={visibleJudgments}
        // 同一托运申请同一服务目的一行（判断库唯一键），键取五维里够定行的三维。
        rowKey={(row) => `${row.shipmentRequestId}#${row.servicePurpose}#${row.acceptanceBaseline}`}
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
  const visibleReassessments = needle
    ? reassessments.filter((row) => matches(row, needle, names))
    : reassessments;
  return (
    <ListPageTemplate<RouteReassessmentRecord>
      {...shared}
      filterSummary={
        answer?.kind === 'outcome' ? `共 ${reassessments.length} 次复核` : undefined
      }
      columns={reassessmentColumns(names)}
      rows={visibleReassessments}
      rowKey={(row) => row.correlationId}
      viewState={catalogueViewState(answer, reassessments.length, retry, {
        module: info,
        endpoint: 'GET /route-plans?register=reassessment',
        emptyTitle: '还没有复核过路线',
        emptyDescription,
      })}
    />
  );
}
