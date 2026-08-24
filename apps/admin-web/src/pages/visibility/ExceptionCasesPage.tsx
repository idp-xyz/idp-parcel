import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['exception-cases'];

/**
 * 异常案件列表行。字段取 visibility-exception CONTEXT.md「案件身份、范围与隔离」
 * 「案件责任、响应与闭环」的原词；接线前没有任何实例数据。分诊队列另有专页
 * （governance 的 ExceptionTriagePage），本页是案件本体的查阅面。
 */
export interface ExceptionCaseRow {
  /** 案件标识。 */
  id: string;
  /** 根对象：每个案件必须具有一个根对象。 */
  rootObject: string;
  /**
   * 影响范围（版本化）：明确覆盖的受影响对象集合。范围扩大、缩小、排除或拆出
   * 都保留依据与历史；成员、装载或同批关系不使对象动态、隐式继承案件。
   */
  impactScope: string;
  /** 主状态：待响应、处理中、已关闭的精简主生命周期。 */
  mainStatus: string;
  /**
   * 当前工作条件：等待客户、合作伙伴、监管或内部团队，以及监控中、已升级等。
   * 它们作为工作条件和下一行动管理，不扩展为互斥主状态——因此与主状态分列。
   */
  workCondition: string;
  /** 严重度：描述已经造成或可能造成的影响，不表示处置顺序，也不产生执行限制。 */
  severity: string;
  /** 处置优先级：综合严重度、干预窗口、客户承诺、影响范围和可恢复性，驱动队列、时限和升级。 */
  priority: string;
  /** 案件责任团队与当前处理人：每个开放案件始终必须有一个内部责任团队；团队不是原因方或赔偿责任方。 */
  responsibleTeam: string;
  /** 响应周期：首次响应、下一行动、客户更新与解决目标的当前时限版本；转派与归并不重置原周期。 */
  responseCycle: string;
  /**
   * 关闭结论：含受控归并的「已归并」（关联主案件，原编号与绩效历史不删除）；
   * 受控重开形成新的响应周期并保留原周期。
   */
  closureConclusion: string;
}

// 主状态与当前工作条件分列、严重度与处置优先级分列，都是 CONTEXT 明文的
// 「分别」关系；折成一列就把工作条件升格成了主状态、把影响混进了紧迫性。
const columns: ListColumn<ExceptionCaseRow>[] = [
  { id: 'id', header: '案件标识', className: 'font-mono', render: (row) => row.id },
  { id: 'root', header: '根对象', render: (row) => row.rootObject },
  { id: 'scope', header: '影响范围（版本化）', render: (row) => row.impactScope },
  { id: 'status', header: '主状态', align: 'center', className: 'w-[80px]', render: (row) => row.mainStatus },
  { id: 'condition', header: '当前工作条件', render: (row) => row.workCondition },
  { id: 'severity', header: '严重度', align: 'center', className: 'w-[72px]', render: (row) => row.severity },
  { id: 'priority', header: '处置优先级', align: 'center', className: 'w-[88px]', render: (row) => row.priority },
  { id: 'team', header: '责任团队 / 处理人', render: (row) => row.responsibleTeam },
  { id: 'cycle', header: '响应周期', render: (row) => row.responseCycle },
  { id: 'closure', header: '关闭结论', render: (row) => row.closureConclusion },
];

/**
 * 异常案件（visibility-exception）。行对象是围绕同一因果链和处置范围建立的
 * 业务案件。案件关闭不修改源事实、不解除来源限制；重开不自动重开委托、
 * 路由计划、关务案件、限制、面单交易或财务事项。
 */
export function ExceptionCasesPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ExceptionCaseRow>
      title={info.title}
      // 页头携带归并与重开的共同底线：两者都以追加表达，不删除历史。
      description={`${info.owner}——受控归并以「已归并」关闭并关联主案件，受控重开形成新响应周期，均不删除历史`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索案件标识 / 根对象',
      }}
      columns={columns}
      // 接线前无实例：行数据与总数届时由 visibility-exception 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
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
        description: `业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
