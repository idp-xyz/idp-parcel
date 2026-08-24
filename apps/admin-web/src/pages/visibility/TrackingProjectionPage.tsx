import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['tracking-projection'];

/**
 * 全程追踪投影列表行。字段取 visibility-exception CONTEXT.md「追踪投影与里程碑」
 * 的并行维度原词；接线前没有任何实例数据。投影是只读旅程视图——「任何运营
 * 人员和集成来源都不能直接编辑当前追踪摘要或标准追踪里程碑」，故本页零动作。
 */
export interface TrackingProjectionRow {
  /** 包裹标识。身份与谱系归 parcel-shipment；真实拆合后各当前有效包裹分别投影。 */
  parcelId: string;
  /** 追踪摘要：由并行维度派生的当前简要说明，只用于阅读和查询。 */
  summary: string;
  /** 物流进展（最后确认）。 */
  logisticsProgress: string;
  /** 位置或控制范围（最后确认）。 */
  controlScope: string;
  /** 关务进展（最后确认）。 */
  customsProgress: string;
  /** 交付或退运进展（最后确认）。 */
  deliveryProgress: string;
  /** 异常影响。 */
  exceptionImpact: string;
  /** 最新标准追踪里程碑；无法可靠映射时保持未归类，不为凑完整时间线强行映射。 */
  latestMilestone: string;
  /** 当前 ETA：版本化预测，含预计时间范围与可信程度；信息不足时允许不形成，不得以计划时间或客户承诺填充。 */
  eta: string;
  /** 可见性缺口：只证明预期数据尚未获得，不证明停止移动、运输延误或已经遗失。 */
  visibilityGap: string;
  /** 投影版本：只增不改写，原版本连同条目与所用映射版本可按版本读回。 */
  projectionVersion: string;
}

// 物流、位置或控制、关务、交付退运、异常影响、预测是并行维度，各占一列；
// 追踪摘要另占一列只作阅读入口。把这些维度折成一列「包裹状态」正是
// GLOSSARY「追踪摘要」禁止的统一状态机——本表的列形状就是那条规则的落点。
const columns: ListColumn<TrackingProjectionRow>[] = [
  { id: 'parcel', header: '包裹标识', className: 'font-mono', render: (row) => row.parcelId },
  { id: 'summary', header: '追踪摘要', render: (row) => row.summary },
  { id: 'logistics', header: '物流进展', render: (row) => row.logisticsProgress },
  { id: 'control', header: '位置或控制范围', render: (row) => row.controlScope },
  { id: 'customs', header: '关务进展', render: (row) => row.customsProgress },
  { id: 'delivery', header: '交付或退运进展', render: (row) => row.deliveryProgress },
  { id: 'exception', header: '异常影响', render: (row) => row.exceptionImpact },
  { id: 'milestone', header: '最新标准里程碑', render: (row) => row.latestMilestone },
  { id: 'eta', header: '当前 ETA', render: (row) => row.eta },
  { id: 'gap', header: '可见性缺口', render: (row) => row.visibilityGap },
  { id: 'version', header: '投影版本', align: 'center', className: 'w-[72px] font-mono', render: (row) => row.projectionVersion },
];

/**
 * 全程追踪（visibility-exception）。行对象是按包裹的全程追踪投影：只消费各源
 * 上下文已接受的事实，引用来源不复制第二套源事实；有效事实冲突且无法裁决时
 * 投影保持信息待确认并形成适用异常信号，本上下文不自行使源事实失效。
 */
export function TrackingProjectionPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<TrackingProjectionRow>
      title={info.title}
      // 页头携带 GLOSSARY「追踪摘要」的硬句原词，与并行维度分列的表形互为呼应。
      description={`${info.owner}——追踪摘要用于阅读和查询，不是一条覆盖各源状态的统一状态机`}
      // 筛选维度（接线时实装进 filters 槽）：最新标准里程碑（无法可靠映射的保持
      // 未归类，未归类本身是可筛的一格）、异常影响在场、可见性缺口在场、ETA 可信
      // 程度。包裹标识与外部标识经搜索。
      search={{
        value: search,
        onChange: setSearch,
        // 外部标识只用于定位候选对象，不证明查询权限（CONTEXT「客户可见性与通知」）。
        placeholder: '搜索包裹标识 / 外部标识',
      }}
      columns={columns}
      // 接线前无实例：行数据与总数届时由 visibility-exception 应用端口供给。
      rows={[]}
      rowKey={(row) => row.parcelId}
      pagination={{
        page,
        pageSize,
        total: 0,
        onPageChange: setPage,
        onPageSizeChange: setPageSize,
      }}
      viewState={{
        kind: 'unconfigured',
        title: '追踪与异常模块尚未接线',
        // 客户追踪视图读口（GET /customer-tracking-view）已在 parcel-api 建立，但那是
        // 客户隔离作用域的视图；本页的运营查阅作用域是否复用该读口尚未裁决，裁决前
        // 不接线——接错作用域比不接更糟（把客户隔离视图当运营全景会漏报）。
        description: '客户隔离的追踪视图读口已建立，但本页的运营查阅作用域是否复用它尚未裁决；裁决前本页不发请求、不含未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '「运营查阅作用域是否复用客户隔离读口」经 visibility-exception 所有权裁决后按裁决接线（裁决问题记录在 admin-web-uiux-20260824 票 03/04）',
        },
      }}
    />
  );
}
