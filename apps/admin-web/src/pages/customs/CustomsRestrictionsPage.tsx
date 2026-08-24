import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['customs-restrictions'];

// 三个对象族（内部合规限制、监管核定税费、放行门禁核对）分页签呈现。
// 本页两条 CONTEXT 硬句落进表形：
// ①「外部放行不自动解除内部限制，内部限制解除也不证明监管机构已经放行」——
//   限制表把两类解除分成两列，永不合并为一个「已解除」；
// ②「尚未放行或存在方向性限制时，必须阻断其明确约束的出库、装载出发、跨关务
//   区域移动或交付，但不因此阻止接收、隔离、测量、查验协作或已经授权的处置
//   执行」——门禁核对绑定拟执行动作，判断不复用于其他动作或监管边界。

/**
 * 内部合规限制列表行。字段取 customs-compliance CONTEXT.md「内部合规限制」
 * 「限制责任来源」「限制覆盖项」「限制解除决定」「限制适用性判断」原词。
 */
export interface ComplianceRestrictionRow {
  /** 限制标识。 */
  id: string;
  /** 限制责任来源：只有该来源可以形成有效解除。 */
  responsibleSource: string;
  /** 限制覆盖项：对象、范围、受限动作、监管边界与适用时间逐项保存；未覆盖的对象或动作不因关联自动受限。 */
  coverage: string;
  /** 受限动作。 */
  restrictedActions: string;
  /** 适用时间。 */
  applicablePeriod: string;
  /**
   * 限制解除决定（内部）：责任来源针对明确覆盖项的追加式部分或全部解除，
   * 关联解除条件、满足证据、决定权限和生效时间，不删除原限制历史。
   */
  internalReleaseDecision: string;
  /**
   * 监管放行（外部引用）：监管机构的放行结果，与内部限制具有独立生命周期。
   * 外部放行不自动解除内部限制——与上一列分列即这条硬句的表形，不得合并。
   */
  externalReleaseRef: string;
  /** 限制适用性判断：有效限制、无内部限制、待确认或冲突；供下游重新评估，不是可手工修改的全局状态。 */
  applicabilityJudgment: string;
}

// 「限制解除决定（内部）」与「监管放行（外部引用）」两列的分离是本表的
// 存在理由：任何把两者折成一列「解除状态」的改动都会让读表的人把外部放行
// 读成内部限制已解除，或把内部解除读成监管已放行。
const restrictionColumns: ListColumn<ComplianceRestrictionRow>[] = [
  { id: 'id', header: '限制标识', className: 'font-mono', render: (row) => row.id },
  { id: 'source', header: '限制责任来源', render: (row) => row.responsibleSource },
  { id: 'coverage', header: '限制覆盖项', render: (row) => row.coverage },
  { id: 'actions', header: '受限动作', render: (row) => row.restrictedActions },
  { id: 'period', header: '适用时间', render: (row) => row.applicablePeriod },
  { id: 'internal-release', header: '限制解除决定（内部）', render: (row) => row.internalReleaseDecision },
  { id: 'external-release', header: '监管放行（外部引用）', render: (row) => row.externalReleaseRef },
  { id: 'applicability', header: '限制适用性判断', render: (row) => row.applicabilityJudgment },
];

/**
 * 监管核定税费列表行。字段取 customs-compliance CONTEXT.md「监管核定税费」
 * 「监管补缴要求」「监管税费退回决定」「税费付款协作事项」「税费付款核对」
 * 原词。核对分别判断覆盖状态、差额和事实有效性——三轴各占一列，互斥总状态
 * 是 CONTEXT 明禁的形状。
 */
export interface RegulatoryDutyRow {
  /** 税费结果标识。 */
  id: string;
  /** 申报范围。 */
  declarationScope: string;
  /** 法定义务人：与实际付款方分别记录。 */
  legalObligor: string;
  /** 税费版本。 */
  version: string;
  /** 监管补缴要求：改变或补充义务，但不证明追加付款已经发生。 */
  supplementRequirement: string;
  /** 监管税费退回决定：不证明资金已经实际退回。 */
  refundDecision: string;
  /** 税费付款协作事项：缺少税费结果不能被解释为无需付款。 */
  paymentCollaboration: string;
  /** 税费付款核对·覆盖状态。 */
  coverageStatus: string;
  /** 税费付款核对·差额状态。 */
  differenceStatus: string;
  /** 税费付款核对·有效性状态。 */
  validityStatus: string;
}

const dutyColumns: ListColumn<RegulatoryDutyRow>[] = [
  { id: 'id', header: '税费结果标识', className: 'font-mono', render: (row) => row.id },
  { id: 'scope', header: '申报范围', render: (row) => row.declarationScope },
  { id: 'obligor', header: '法定义务人', render: (row) => row.legalObligor },
  { id: 'version', header: '税费版本', align: 'center', className: 'w-[72px] font-mono', render: (row) => row.version },
  { id: 'supplement', header: '监管补缴要求', render: (row) => row.supplementRequirement },
  { id: 'refund', header: '税费退回决定', render: (row) => row.refundDecision },
  { id: 'collaboration', header: '付款协作事项', render: (row) => row.paymentCollaboration },
  { id: 'coverage', header: '核对·覆盖', align: 'center', render: (row) => row.coverageStatus },
  { id: 'difference', header: '核对·差额', align: 'center', render: (row) => row.differenceStatus },
  { id: 'validity', header: '核对·有效性', align: 'center', render: (row) => row.validityStatus },
];

/**
 * 放行门禁核对列表行。字段取 customs-compliance CONTEXT.md「放行门禁核对」
 * 原词：判断绑定申报范围、拟执行动作与适用监管边界，不能复用于其他动作或
 * 边界；门禁满足不生成放行，门禁未满足也不能删除已经接收的放行结果。
 */
export interface ReleaseGateRow {
  /** 核对标识。 */
  id: string;
  /** 申报范围。 */
  declarationScope: string;
  /** 拟执行动作：门禁按动作逐一核对，出库、装载出发、跨关务区域移动、交付分别判断。 */
  intendedAction: string;
  /** 适用监管边界。 */
  regulatoryBoundary: string;
  /** 前置条件：税费付款核对、限制、处置及其他已接受监管事实。 */
  prerequisites: string;
  /** 门禁判断：待满足、部分满足、满足、冲突或不适用。 */
  verdict: string;
  /** 放行结果（外部引用）：仍由监管机构形成，UC-CC-006 接收和解释；门禁满足只允许进入监管结果等待或放行复核。 */
  releaseResultRef: string;
}

const gateColumns: ListColumn<ReleaseGateRow>[] = [
  { id: 'id', header: '核对标识', className: 'font-mono', render: (row) => row.id },
  { id: 'scope', header: '申报范围', render: (row) => row.declarationScope },
  { id: 'action', header: '拟执行动作', render: (row) => row.intendedAction },
  { id: 'boundary', header: '适用监管边界', render: (row) => row.regulatoryBoundary },
  { id: 'prerequisites', header: '前置条件', render: (row) => row.prerequisites },
  { id: 'verdict', header: '门禁判断', align: 'center', render: (row) => row.verdict },
  { id: 'release', header: '放行结果（外部引用）', render: (row) => row.releaseResultRef },
];

const unconfigured = {
  kind: 'unconfigured' as const,
  title: '关务合规模块尚未接线',
  description: '查阅读口尚未建立（已接线的关务端点是接收面：外部结果接收等），本页不发请求、不含未确认参数的默认值。',
  facts: {
    owner: info.owner,
    source: info.source,
    unlock: '限制/税费/门禁核对的查询端点建成并经 ADR-0017 准入闸门放行后接线',
  },
};

function ComplianceRestrictionsTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ComplianceRestrictionRow>
      title="内部合规限制"
      // 页头携带两类解除独立生命周期的硬句原词。
      description={`${info.owner}——外部放行不自动解除内部限制，内部限制解除也不证明监管机构已经放行`}
      // 筛选维度（接线时实装进 filters 槽）：限制责任来源、限制适用性判断（有效限制/
      // 无内部限制/待确认/冲突，封闭四格）、受限动作。限制标识经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索限制标识 / 责任来源' }}
      columns={restrictionColumns}
      // 接线前无实例：行数据与总数届时由 customs-compliance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

function RegulatoryDutiesTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<RegulatoryDutyRow>
      title="监管核定税费"
      description={`${info.owner}——监管核定与实际付款、资金退回、代垫回收分别存在；代垫回收归 settlement-accounting`}
      // 筛选维度（接线时实装进 filters 槽）：核对三轴各自的状态（覆盖/差额/有效性，
      // 互斥总状态是 CONTEXT 明禁形状，筛选也按轴分立）、法定义务人。标识与范围经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索税费结果标识 / 申报范围' }}
      columns={dutyColumns}
      // 接线前无实例：行数据与总数届时由 customs-compliance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

function ReleaseGatesTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ReleaseGateRow>
      title="放行门禁核对"
      // 页头携带安全作业不受阻断的硬句：未放行阻断的只是方向性动作。
      description={`${info.owner}——尚未放行只阻断出库、装载出发、跨关务区域移动或交付，不阻止接收、隔离、测量、查验协作或已授权处置执行`}
      // 筛选维度（接线时实装进 filters 槽）：拟执行动作（出库/装载出发/跨关务区域
      // 移动/交付，封闭四动作）、门禁判断（待满足/部分满足/满足/冲突/不适用，封闭
      // 五格）、适用监管边界。核对标识与申报范围经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索核对标识 / 申报范围 / 拟执行动作' }}
      columns={gateColumns}
      // 接线前无实例：行数据与总数届时由 customs-compliance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

/**
 * 合规限制与监管税费（customs-compliance）。页签顺序按动作被放行前要过的层：
 * 内部限制 → 税费义务 → 门禁核对；门禁满足也不生成放行，放行结果始终是
 * 监管机构的外部事实。
 */
export function CustomsRestrictionsPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="restrictions" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="restrictions">内部合规限制</TabsTrigger>
          <TabsTrigger value="duties">监管核定税费</TabsTrigger>
          <TabsTrigger value="gates">放行门禁核对</TabsTrigger>
        </TabsList>
        <TabsContent value="restrictions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ComplianceRestrictionsTable />
        </TabsContent>
        <TabsContent value="duties" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RegulatoryDutiesTable />
        </TabsContent>
        <TabsContent value="gates" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ReleaseGatesTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
