import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['route-plans'];

/**
 * 包裹级路由计划与受控改路的查阅面。栏目取 network-routing CONTEXT.md 原词：
 * 路由以包裹为计划单位，同一包裹同一业务时点只有一个当前有效路由计划；旧版本、
 * 候选与淘汰原因保留但不是并行有效计划。受控改路以版本与替代关系呈现——改路
 * 决定保留原因、策略版本、输入依据和与原计划的替代关系；改路建议不改变当前
 * 有效计划，「无当前有效路由」是明确判断而非空行，接线时这两类结果要如实
 * 单列呈现，不得折进计划版本行。
 *
 * 查询端点未建，数据区如实呈现「未配置」态，不发请求、不含合成数据。
 */

/**
 * 计划版本一行。查询契约未建，这是页面侧暂定；真契约落地时以它为准重谈，
 * 不得反过来把这里当已发布的查询 Schema。
 */
export interface RoutePlanRow {
  /** 包裹（计划单位，身份归 parcel-shipment）。 */
  parcelId: string;
  /** 路由计划版本。 */
  planVersion: string;
  /** 计划适用性：当前有效 / 已被替代 / 已失效 / 已结束（CONTEXT 生命周期原词）。 */
  applicability: string;
  /** 明确服务目的（路由计划针对一个明确包裹和一次明确服务目的形成）。 */
  servicePurpose: string;
  /** 计划履约段数（段的所有权仍在计划内，具体班次与装载归 transport-fulfillment）。 */
  plannedSegmentCount: number;
  /** 采用的路由策略版本。 */
  strategyVersion: string;
  /** 业务生效边界。 */
  effectiveBoundary: string;
  /** 替代关系：受控改路时指向被替代的原计划版本；首版缺席。 */
  replacesVersion?: string;
}

// 计划适用性是 CONTEXT 生命周期词（当前有效/已被替代/已失效/已结束），尚未收入
// domain/status 的共享词表，先以原词文本呈现；色调判断属词表所有者的决定，
// 本页不自造徽章档位。
const columns: ListColumn<RoutePlanRow>[] = [
  {
    id: 'parcel-id',
    header: '包裹',
    className: 'w-[160px]',
    render: (row) => <span className="font-mono text-[12px] text-idpxyz-accent">{row.parcelId}</span>,
  },
  {
    id: 'plan-version',
    header: '计划版本',
    className: 'w-[104px]',
    render: (row) => <span className="font-mono text-[12px]">{row.planVersion}</span>,
  },
  {
    id: 'applicability',
    header: '计划适用性',
    className: 'w-[104px]',
    render: (row) => row.applicability,
  },
  {
    id: 'service-purpose',
    header: '服务目的',
    render: (row) => row.servicePurpose,
  },
  {
    id: 'planned-segments',
    header: '计划履约段',
    align: 'right',
    className: 'w-[96px]',
    render: (row) => row.plannedSegmentCount,
  },
  {
    id: 'strategy-version',
    header: '路由策略版本',
    render: (row) => <span className="font-mono text-[12px]">{row.strategyVersion}</span>,
  },
  {
    id: 'effective-boundary',
    header: '生效边界',
    render: (row) => <span className="font-mono text-[12px]">{row.effectiveBoundary}</span>,
  },
  {
    id: 'replaces-version',
    header: '替代关系',
    render: (row) =>
      row.replacesVersion ? (
        <span className="font-mono text-[12px]">替代自 {row.replacesVersion}</span>
      ) : (
        '—'
      ),
  },
];

export function RoutePlansPage() {
  const [keyword, setKeyword] = useState('');

  return (
    <ListPageTemplate<RoutePlanRow>
      title={info.title}
      description={`${info.owner} · 计划表达意图，不维护已装载、运输中等执行状态。`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按包裹标识检索',
      }}
      columns={columns}
      rows={[]}
      rowKey={(row) => `${row.parcelId}#${row.planVersion}`}
      viewState={{
        kind: 'unconfigured',
        title: '路由计划查询端点尚未建立',
        description: `查询契约待建；本页不发请求、不含合成数据。场景出处：${info.source}`,
      }}
    />
  );
}
