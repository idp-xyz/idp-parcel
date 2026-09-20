import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
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
import {
  applicabilityStateLabels,
  candidateStateLabels,
  initialRouteConclusionLabels,
  labelOf,
  reassessmentConclusionLabels,
  rerouteStateLabels,
  routePlanRegisterLabels,
  routePlanRegisters,
  type RoutePlanRegister,
} from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['route-plans'];

// 本页接 GET /route-plans（票 admin-skeleton-closure-batch/03 阶段二），两册按
// ?register= 分派（初始路由判断 / 路由复核），chip 切换——与网络目录页同一取数形。
//
// 旧骨架各栏随对栏裁定处置（判据同结算申请页：目录事实只有本册登了什么）：
//   计划履约段数、路由策略版本、生效边界——**全撤**。三者住在计划本体（jsonb）内，
//     检索列面册级缺席；权威读法归判断口，列面不拆快照自造第二读法。
//   替代关系——**换形**。不再是行上一列，而是适用性组的 successor 原词（0005 迁移
//     的逐态矩阵：仅已被替代态在场），在计划适用性一格内呈现。
//   包裹标识——**换词**。册上存的是受理那一刻的五维脱敏引用（客户账户、托运申请、
//     受理基线、申报包裹、服务目的），照登上列，不折成单一「包裹」列。
//   「无当前有效路由」与改路决定如实单列（CONTEXT 硬句）：前者是初始判断结论的
//     封闭一格，照词呈现不折进成计划行；后者透出改路判定三态词，改路建议本体住
//     jsonb 属详情读法。

const initialRouteColumns: ListColumn<InitialRouteRecord>[] = [
  {
    id: 'shipment-request',
    header: '托运申请',
    className: 'font-mono text-xs',
    render: (row) => <span className="text-idpxyz-accent">{row.shipmentRequestId}</span>,
  },
  {
    id: 'declared-parcel',
    header: '申报包裹',
    className: 'font-mono text-xs',
    render: (row) => row.declaredParcelId,
  },
  {
    id: 'customer',
    header: '客户账户',
    className: 'font-mono text-xs',
    render: (row) => row.customerAccountId,
  },
  { id: 'service-purpose', header: '服务目的', className: 'font-mono text-xs', render: (row) => row.servicePurpose },
  {
    id: 'acceptance-baseline',
    header: '受理基线',
    className: 'font-mono text-xs',
    render: (row) => row.acceptanceBaseline,
  },
  {
    id: 'conclusion',
    header: '判断结论',
    align: 'center',
    // 封闭两格词表转写；「无当前有效路由」占完整一格，不显成空行。
    render: (row) => labelOf(initialRouteConclusionLabels, row.conclusion),
  },
  {
    id: 'plan-version',
    header: '计划版本',
    className: 'font-mono text-xs',
    // 缺席即本行是无路可走判断——无版本是真话，不编造占位版本号。
    render: (row) => row.planVersion ?? '—',
  },
  {
    id: 'applicability',
    header: '计划适用性',
    className: 'min-w-44 text-xs',
    // 缺席两解都如实显「—」：无路可走行本就无计划；成计划行缺席即尚无适用性登记。
    // 组在场时逐态呈现：状态词 + 迁移时点，替代版本与依据按逐态矩阵在场才显示。
    render: (row) =>
      row.applicability ? (
        <div className="min-w-44">
          <p>
            {labelOf(applicabilityStateLabels, row.applicability.state)}（
            {formatInstant(row.applicability.transitionedAt)}）
          </p>
          {row.applicability.successor && (
            <p className="font-mono">替代版本 {row.applicability.successor}</p>
          )}
          {row.applicability.basis && <p>依据：{row.applicability.basis}</p>}
        </div>
      ) : (
        '—'
      ),
  },
  {
    id: 'recorded-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.recordedAt),
  },
];

const reassessmentColumns: ListColumn<RouteReassessmentRecord>[] = [
  {
    id: 'correlation',
    header: '关联标识',
    className: 'font-mono text-xs',
    render: (row) => <span className="text-idpxyz-accent">{row.correlationId}</span>,
  },
  {
    id: 'shipment-request',
    header: '托运申请',
    className: 'font-mono text-xs',
    render: (row) => row.shipmentRequestId,
  },
  {
    id: 'declared-parcel',
    header: '申报包裹',
    className: 'font-mono text-xs',
    render: (row) => row.declaredParcelId,
  },
  {
    id: 'customer',
    header: '客户账户',
    className: 'font-mono text-xs',
    render: (row) => row.customerAccountId,
  },
  { id: 'service-purpose', header: '服务目的', className: 'font-mono text-xs', render: (row) => row.servicePurpose },
  {
    id: 'conclusion',
    header: '复核走向',
    align: 'center',
    render: (row) => labelOf(reassessmentConclusionLabels, row.conclusion),
  },
  {
    id: 'reviewed-plan',
    header: '被复核计划',
    className: 'font-mono text-xs',
    render: (row) => row.reviewedPlan ?? '—',
  },
  {
    id: 'lapse-basis',
    header: '失效依据',
    className: 'font-mono text-xs',
    render: (row) => row.lapseBasis ?? '—',
  },
  {
    id: 'candidate-state',
    header: '候选评估',
    align: 'center',
    // 缺席即本走向不评估（0002 迁移：仅「仍适用」走向为 NULL）——如实说，不显空。
    render: (row) =>
      row.candidateState ? (
        labelOf(candidateStateLabels, row.candidateState)
      ) : (
        <span className="text-idpxyz-textMuted">不评估</span>
      ),
  },
  {
    id: 'reroute-state',
    header: '改路判定',
    align: 'center',
    // 缺席即没评估过改路（事实目录未配置或候选未收敛）——与「禁止改路」是两回事。
    render: (row) =>
      row.rerouteState ? (
        labelOf(rerouteStateLabels, row.rerouteState)
      ) : (
        <span className="text-idpxyz-textMuted">未评估</span>
      ),
  },
  {
    id: 'reassessed-at',
    header: '复核时点',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.reassessedAt),
  },
  {
    id: 'recorded-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.recordedAt),
  },
];

// 按册装载的答案：register 与答案体绑定成判别联合，切册期间旧册答案不冒充新册。
type LoadedAnswer =
  | { register: 'initial-route'; answer: ApiResult<InitialRouteListResponseBody> }
  | { register: 'reassessment'; answer: ApiResult<RouteReassessmentListResponseBody> };

/**
 * 路由计划：路由判断两册的查阅面。计划以包裹为单位、同一业务时点只有一个当前有效
 * 计划——这句约束由判断库与适用性登记保证，本页照登转写，不维护执行状态、不选版。
 */
export function RoutePlansPage() {
  const [register, setRegister] = useState<RoutePlanRegister>('initial-route');
  const [keyword, setKeyword] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<LoadedAnswer | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (register === 'initial-route') {
      void listInitialRoutes().then((answer) => {
        if (!cancelled) setLoaded({ register: 'initial-route', answer });
      });
    } else {
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
      {routePlanRegisters.map((candidate) => (
        <button
          key={candidate}
          type="button"
          className={chipClass(candidate === register)}
          onClick={() => setRegister(candidate)}
        >
          {routePlanRegisterLabels[candidate]}
        </button>
      ))}
    </>
  );
  const shared = {
    title: info.title,
    description: `${info.owner}——两册照登转写；计划本体与改路决定住在判断快照内，本页上列检索列面`,
    search: {
      value: keyword,
      onChange: setKeyword,
      placeholder: '按托运申请 / 申报包裹 / 客户账户检索',
    },
    filters: chips,
  };
  // 空册文案两册同句：写入方是渠道墙后的路由编排，尚未接入任何进程。
  const emptyDescription =
    '读取入口已配置，但登记册为空；两册的写入方是渠道墙后的路由编排，尚未接入任何进程——册空是墙拦不是缺陷，本页不预置数据。';

  if (register === 'initial-route') {
    const answer = loaded?.register === 'initial-route' ? loaded.answer : null;
    const judgments = answer?.kind === 'outcome' ? answer.body.judgments : [];
    const visibleJudgments = needle
      ? judgments.filter((row) =>
          [row.shipmentRequestId, row.declaredParcelId, row.customerAccountId, row.planVersion ?? ''].some(
            (value) => value.toLowerCase().includes(needle),
          ),
        )
      : judgments;
    return (
      <ListPageTemplate<InitialRouteRecord>
        {...shared}
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 行」会与状态区
          // 「这不是登记册为空」直接矛盾（README 列表页上列通则第六条）。
          answer?.kind === 'outcome' ? `初始路由判断 ${judgments.length} 行` : undefined
        }
        columns={initialRouteColumns}
        rows={visibleJudgments}
        // 同一托运申请同一服务目的一行（判断库唯一键），键取五维里够定行的三维。
        rowKey={(row) => `${row.shipmentRequestId}#${row.servicePurpose}#${row.acceptanceBaseline}`}
        viewState={catalogueViewState(answer, judgments.length, retry, {
          module: info,
          endpoint: 'GET /route-plans?register=initial-route',
          emptyTitle: '当前租户尚无初始路由判断登记',
          emptyDescription,
        })}
      />
    );
  }

  const answer = loaded?.register === 'reassessment' ? loaded.answer : null;
  const reassessments = answer?.kind === 'outcome' ? answer.body.reassessments : [];
  const visibleReassessments = needle
    ? reassessments.filter((row) =>
        [
          row.correlationId,
          row.shipmentRequestId,
          row.declaredParcelId,
          row.customerAccountId,
          row.reviewedPlan ?? '',
        ].some((value) => value.toLowerCase().includes(needle)),
      )
    : reassessments;
  return (
    <ListPageTemplate<RouteReassessmentRecord>
      {...shared}
      filterSummary={
        answer?.kind === 'outcome' ? `路由复核 ${reassessments.length} 行` : undefined
      }
      columns={reassessmentColumns}
      rows={visibleReassessments}
      rowKey={(row) => row.correlationId}
      viewState={catalogueViewState(answer, reassessments.length, retry, {
        module: info,
        endpoint: 'GET /route-plans?register=reassessment',
        emptyTitle: '当前租户尚无路由复核登记',
        emptyDescription,
      })}
    />
  );
}
