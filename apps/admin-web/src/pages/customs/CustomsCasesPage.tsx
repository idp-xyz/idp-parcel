import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['customs-cases'];

// 四个对象族（关务案件、申报单元、正式申报资料快照、提交版本）分页签呈现：
// 案件是稳定业务容器，单元是申报对象集合，快照是版本化资料，提交版本是
// 不可覆盖的对外快照——各自的版本与替代关系独立，折进一张表就会让
// 「案件状态」冒充其余三层的进度。

/**
 * 关务案件列表行。字段取 customs-compliance CONTEXT.md「关务案件」定义与
 * 关闭/重开相关词条原词。关闭核对可以证明可关闭或指出未决项，但不等于
 * 已经形成关闭决定——两列分开。
 */
export interface CustomsCaseRow {
  /** 案件标识：围绕明确监管辖区、方向、程序和法定义务范围建立的稳定案件。 */
  id: string;
  /** 监管辖区。 */
  jurisdiction: string;
  /** 进出口方向：出口、进口及其他独立监管程序分别建立案件。 */
  direction: string;
  /** 监管程序。 */
  procedure: string;
  /** 法定义务范围。 */
  obligation: string;
  /** 关联申报单元（引用；一个案件可以关联多个申报单元和多次提交）。 */
  declarationUnits: string;
  /** 关务案件关闭核对：逐项形成关闭依据的版本化评估。 */
  closureVerification: string;
  /** 关务案件关闭决定：有权责任角色依据仍为当前的关闭核对形成的追加式决定。 */
  closureDecision: string;
  /** 受控重开决定：不撤销原关闭决定，不复活旧提交资格。 */
  reopenDecision: string;
  /** 后续关务案件：关联案件，不覆盖原案件。 */
  followUpCase: string;
}

const caseColumns: ListColumn<CustomsCaseRow>[] = [
  { id: 'id', header: '案件标识', className: 'font-mono', render: (row) => row.id },
  { id: 'jurisdiction', header: '监管辖区', render: (row) => row.jurisdiction },
  { id: 'direction', header: '进出口方向', align: 'center', className: 'w-[88px]', render: (row) => row.direction },
  { id: 'procedure', header: '监管程序', render: (row) => row.procedure },
  { id: 'obligation', header: '法定义务范围', render: (row) => row.obligation },
  { id: 'units', header: '关联申报单元', render: (row) => row.declarationUnits },
  { id: 'closure-verification', header: '关闭核对', render: (row) => row.closureVerification },
  { id: 'closure-decision', header: '关闭决定', render: (row) => row.closureDecision },
  { id: 'reopen', header: '受控重开', render: (row) => row.reopenDecision },
  { id: 'follow-up', header: '后续案件', className: 'font-mono', render: (row) => row.followUpCase },
];

/**
 * 申报单元列表行。字段取 customs-compliance CONTEXT.md「申报单元」「受控跨客户
 * 合报」「申报就绪判断」「申报替代关系」原词。
 */
export interface DeclarationUnitRow {
  /** 申报单元标识。 */
  id: string;
  /** 所属关务案件。 */
  caseRef: string;
  /** 对象范围：一个包裹或明确的一组包裹；不是客户委托、集运单元、总单、舱单或运输班次。 */
  scope: string;
  /** 受控跨客户合报：合报只建立监管归组，不合并各包裹的客户、货物、价值、税费、责任或监管结果。 */
  crossCustomerConsolidation: string;
  /** 当前正式申报资料快照版本。 */
  snapshotVersion: string;
  /** 申报就绪判断：就绪只表示具备申请提交的条件，不等于已授权、已提交、已受理或已放行。 */
  readiness: string;
  /** 申报替代关系：拟替代与有效替代分别成立，原对象及其全部历史永久保留。 */
  substitution: string;
}

const unitColumns: ListColumn<DeclarationUnitRow>[] = [
  { id: 'id', header: '申报单元标识', className: 'font-mono', render: (row) => row.id },
  { id: 'case', header: '所属关务案件', className: 'font-mono', render: (row) => row.caseRef },
  { id: 'scope', header: '对象范围', render: (row) => row.scope },
  { id: 'consolidation', header: '受控跨客户合报', render: (row) => row.crossCustomerConsolidation },
  { id: 'snapshot', header: '当前资料快照版本', align: 'center', className: 'font-mono', render: (row) => row.snapshotVersion },
  { id: 'readiness', header: '申报就绪判断', render: (row) => row.readiness },
  { id: 'substitution', header: '替代关系', render: (row) => row.substitution },
];

/**
 * 正式申报资料快照列表行。字段取 customs-compliance CONTEXT.md「正式申报资料
 * 快照」「字段级溯源」原词：快照保留客户原始声明、节点实测或观察、关务判断
 * 之间的区别，不覆盖任何来源值。
 */
export interface DeclarationSnapshotRow {
  /** 快照版本标识。 */
  id: string;
  /** 所属申报单元。 */
  unitRef: string;
  /** 归类。 */
  classification: string;
  /** 原产地。 */
  origin: string;
  /** 申报价值。 */
  declaredValue: string;
  /** 监管条件。 */
  conditions: string;
  /** 来源区分：客户原始声明 / 节点实测或观察 / 关务判断，各来源保留不覆盖。 */
  sourceKinds: string;
  /** 字段级溯源：每个字段与来源值、转换规则、判断依据、决定方式、责任角色及适用时间的关系。 */
  fieldLineage: string;
}

const snapshotColumns: ListColumn<DeclarationSnapshotRow>[] = [
  { id: 'id', header: '快照版本', className: 'font-mono', render: (row) => row.id },
  { id: 'unit', header: '所属申报单元', className: 'font-mono', render: (row) => row.unitRef },
  { id: 'classification', header: '归类', render: (row) => row.classification },
  { id: 'origin', header: '原产地', render: (row) => row.origin },
  { id: 'value', header: '申报价值', align: 'right', className: 'font-mono', render: (row) => row.declaredValue },
  { id: 'conditions', header: '监管条件', render: (row) => row.conditions },
  { id: 'sources', header: '来源区分', render: (row) => row.sourceKinds },
  { id: 'lineage', header: '字段级溯源', render: (row) => row.fieldLineage },
];

/**
 * 提交版本列表行。字段取 customs-compliance CONTEXT.md「提交版本」「提交尝试」
 * 「技术回执」「监管接收」「业务受理」「原提交结果仍待确认」原词。技术回执、
 * 监管接收、业务受理是三层外部结果，任一层不推导下一层——三列分开。
 */
export interface SubmissionVersionRow {
  /** 提交版本标识：对外发送前固定的不可覆盖快照。 */
  id: string;
  /** 逻辑申报目标。 */
  target: string;
  /** 所含正式申报资料快照。 */
  snapshotRef: string;
  /** 关务参与方角色快照：任何角色不能由企业类型或另一角色自动推导。 */
  roleSnapshot: string;
  /** 授权依据：自动提交必须同时具有版本化自动提交政策和有效授权依据。 */
  authorization: string;
  /** 提交尝试：只有安全再次发送判断成立时才可关联新的受控尝试；结果未知保持待确认，超时不解释为失败。 */
  attempts: string;
  /** 技术回执：技术成功只说明技术层结果。 */
  techReceipt: string;
  /** 监管接收：监管机构确认已收到，不自动证明进入实质审查。 */
  regulatoryReceived: string;
  /** 业务受理：确认进入适用监管程序，不等于查验结束、税费完成或放行。 */
  businessAccepted: string;
  /** 原提交结果仍待确认：防止原提交被重复发送的业务判断。 */
  pendingConfirmation: string;
}

const submissionColumns: ListColumn<SubmissionVersionRow>[] = [
  { id: 'id', header: '提交版本', className: 'font-mono', render: (row) => row.id },
  { id: 'target', header: '逻辑申报目标', render: (row) => row.target },
  { id: 'snapshot', header: '资料快照', className: 'font-mono', render: (row) => row.snapshotRef },
  { id: 'roles', header: '参与方角色快照', render: (row) => row.roleSnapshot },
  { id: 'authorization', header: '授权依据', render: (row) => row.authorization },
  { id: 'attempts', header: '提交尝试', render: (row) => row.attempts },
  { id: 'tech-receipt', header: '技术回执', align: 'center', render: (row) => row.techReceipt },
  { id: 'received', header: '监管接收', align: 'center', render: (row) => row.regulatoryReceived },
  { id: 'accepted', header: '业务受理', align: 'center', render: (row) => row.businessAccepted },
  { id: 'pending', header: '原结果待确认', align: 'center', render: (row) => row.pendingConfirmation },
];

const unconfigured = {
  kind: 'unconfigured' as const,
  title: '关务合规模块尚未接线',
  description: '查阅读口尚未建立（已接线的关务端点是接收面：外部结果接收等），本页不发请求、不含未确认参数的默认值。',
  facts: {
    owner: info.owner,
    source: info.source,
    unlock: '案件/单元/快照/提交版本的查询端点建成并经 ADR-0017 准入闸门放行后接线',
  },
};

function CustomsCasesTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<CustomsCaseRow>
      title="关务案件"
      description={`${info.owner}——出口、进口及其他独立监管程序分别建立案件`}
      // 筛选维度（接线时实装进 filters 槽）：监管辖区、进出口方向（出口/进口，封闭
      // 二向）、监管程序。案件标识经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索案件标识 / 监管辖区' }}
      columns={caseColumns}
      // 接线前无实例：行数据与总数届时由 customs-compliance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

function DeclarationUnitsTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<DeclarationUnitRow>
      title="申报单元"
      description={`${info.owner}——申报单元不是客户委托、集运单元、总单、舱单或运输班次`}
      // 筛选维度（接线时实装进 filters 槽）：申报就绪判断、受控跨客户合报在场与否、
      // 替代关系（拟替代/有效替代分别成立）。单元标识与所属案件经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索申报单元标识 / 所属案件' }}
      columns={unitColumns}
      // 接线前无实例：行数据与总数届时由 customs-compliance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

function DeclarationSnapshotsTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<DeclarationSnapshotRow>
      title="正式申报资料快照"
      description={`${info.owner}——快照保留客户声明、节点实测与关务判断的区别，不覆盖任何来源值`}
      // 筛选维度（接线时实装进 filters 槽）：来源区分（客户原始声明/节点实测或观察/
      // 关务判断，封闭三源）。快照版本与所属单元经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索快照版本 / 所属申报单元' }}
      columns={snapshotColumns}
      // 接线前无实例：行数据与总数届时由 customs-compliance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

function SubmissionVersionsTable() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<SubmissionVersionRow>
      title="提交版本"
      description={`${info.owner}——提交版本是不可覆盖快照，其存在不证明技术传输、监管接收、业务受理或放行成功`}
      // 筛选维度（接线时实装进 filters 槽）：技术回执/监管接收/业务受理三层结果态
      //（分层保存，任一层不推导下一层）、原结果待确认。版本与申报目标经搜索。
      search={{ value: search, onChange: setSearch, placeholder: '搜索提交版本 / 逻辑申报目标' }}
      columns={submissionColumns}
      // 接线前无实例：行数据与总数届时由 customs-compliance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{ page, pageSize, total: 0, onPageChange: setPage, onPageSizeChange: setPageSize }}
      viewState={unconfigured}
    />
  );
}

/**
 * 关务案件与申报（customs-compliance）。页签顺序按对象层级：案件容器 →
 * 申报单元 → 资料快照 → 提交版本，逐层向外，越靠后越接近对外发送。
 */
export function CustomsCasesPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="cases" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="cases">关务案件</TabsTrigger>
          <TabsTrigger value="units">申报单元</TabsTrigger>
          <TabsTrigger value="snapshots">资料快照</TabsTrigger>
          <TabsTrigger value="submissions">提交版本</TabsTrigger>
        </TabsList>
        <TabsContent value="cases" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CustomsCasesTable />
        </TabsContent>
        <TabsContent value="units" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DeclarationUnitsTable />
        </TabsContent>
        <TabsContent value="snapshots" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DeclarationSnapshotsTable />
        </TabsContent>
        <TabsContent value="submissions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <SubmissionVersionsTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
