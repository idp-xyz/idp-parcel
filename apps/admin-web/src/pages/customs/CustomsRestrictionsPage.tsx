import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  customsRegistrationEndpoints,
  listGateConditions,
  registerCustomsConfiguration,
  registrationOutcomeLabels,
  type GateConditionCatalogueRecord,
  type GateConditionListResponseBody,
} from './api';
import {
  guardedActionLabels,
  labelOf,
  preconditionStateLabels,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['customs-restrictions'];

// 三个对象族（内部合规限制、监管核定税费、放行门禁核对）分页签呈现。已接线的是
// 门禁核对签，接 GET /customs-gate-conditions（票 admin-web-page-wiring-frontier/06）；
// 另两族列表端点未建，如实占位。
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

const unconfigured = {
  kind: 'unconfigured' as const,
  title: '本对象族的列表端点尚未建立',
  description:
    '本页已接线的是门禁条件册查阅（放行门禁核对签）；内部合规限制与监管核定税费两族的列表端点尚未建立，这两签不发请求、不含未确认参数的默认值。',
  facts: {
    owner: info.owner,
    source: info.source,
    unlock: '限制与税费的查询端点建成并经 ADR-0017 准入闸门放行后接线',
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

// —— 放行门禁核对（接真面，票 admin-web-page-wiring-frontier/06）——

interface GateRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function gateCol(id: string, header: string, mono = false): ListColumn<GateRow> {
  return {
    id,
    header,
    className: mono ? 'font-mono text-xs' : undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

// 三元键（范围·动作·边界）三列并列，是「判断绑定动作与边界、不复用于其他动作」这条
// CONTEXT 硬句的表形：折成一个「核对标识」就会让判断看起来可以跨动作复用。
//
// 刻意不设「门禁判断」列。五值结论（领域 FoldGateConclusion）是门禁编排的判断语义，
// 查阅面转述登记册本身——在这里折一次，页面就成了第二处判断权威，而它读到的还只是
// 登记册的一部分。
const gateConditionColumns: ListColumn<GateRow>[] = [
  gateCol('scope', '申报范围', true),
  gateCol('action', '拟执行动作'),
  gateCol('boundary', '适用监管边界', true),
  gateCol('registeredAt', '登记时间', true),
  gateCol('precondition', '前置条件', true),
  gateCol('state', '认定'),
];

function gateRows(gates: GateConditionCatalogueRecord[]): GateRow[] {
  return gates.flatMap((gate) => {
    const base = {
      scope: gate.scope,
      action: labelOf(guardedActionLabels, gate.action),
      boundary: gate.boundary,
      registeredAt: formatInstant(gate.registeredAt),
    };
    // 空清单是登记方明说「此动作在此边界本就不受门禁」（领域折为不适用），不是查不到。
    // 与关闭义务那边的空清单相反：那边是中性的「无义务项」，这边是放行侧的绿灯，所以
    // 这句话另写，不照抄票 05——照抄会把一句绿灯说成一句中性事实。
    if (gate.findings.length === 0) {
      return [
        {
          // 行键循库主键 (tenant, scope_ref, action, boundary_ref)。
          key: `gate:${gate.scope}:${gate.action}:${gate.boundary}`,
          values: { ...base, precondition: '（未登记任何前置条件）', state: '本就不受门禁' },
        },
      ];
    }
    return gate.findings.map((finding) => ({
      key: `finding:${gate.scope}:${gate.action}:${gate.boundary}:${finding.precondition}`,
      values: {
        ...base,
        precondition: finding.precondition,
        // labelOf 对集外取值原样回显：认定三值没有第四格，坏数据露出来而不是被译顺。
        state: labelOf(preconditionStateLabels, finding.state),
      },
    }));
  });
}

function ReleaseGatesTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<GateConditionListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listGateConditions().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const gates = answer?.kind === 'outcome' ? answer.body.gates : [];
  const rows = gateRows(gates);
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);
  const findingCount = gates.reduce((sum, gate) => sum + gate.findings.length, 0);
  const unguarded = gates.filter((gate) => gate.findings.length === 0).length;

  return (
    <ListPageTemplate<GateRow>
      title="放行门禁核对"
      // 页头携带的是本页最容易读错的那一格：未登记不等于不受门禁。安全作业不受阻断
      // 那条硬句在下面的空册文案里另说——它讲的是「阻断什么」，与「读得出什么」不同题。
      description={`${info.owner}——未登记的（范围·动作·边界）不在本列，其门禁判断读作未决；已登记而无前置条件才是「本就不受门禁」`}
      search={{ value: search, onChange: setSearch, placeholder: '搜索申报范围 / 拟执行动作 / 监管边界 / 前置条件' }}
      filterSummary={
        answer?.kind === 'outcome'
          ? `门禁 ${gates.length} 份 · 前置条件认定 ${findingCount} 项 · 其中不受门禁 ${unguarded} 份`
          : undefined
      }
      columns={gateConditionColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-gate-conditions',
        emptyTitle: '当前租户尚无已登记的门禁条件',
        emptyDescription:
          '上列只含已登记门禁：未登记 ≠ 不受门禁，未登记的动作与边界其门禁判断读作未决、无从复核。页面不会预置门禁。',
      })}
    />
  );
}

// —— 门禁目录登记签（ADR-0085，票 admin-write-faces/02 切片 02b）——
//
// 本页三本册里只有门禁目录有在线登记口：内部限制与监管税费两族连查阅端点都还没有，
// 更没有登记用例可接（票 02「无用例可接就如实跳过，不为凑齐而造用例」）。
//
// 登的是目录在场本身——某（范围·动作·边界）这本前置条件目录存在，没有可比内容，所以
// 它的答案代数里没有内容冲突那一格。目录里的逐项认定（门禁发现）是另一个命令，按票 02
// 的范围裁定属「案上此刻的事实」不进写面，本签因此不收它。

/**
 * 合规限制与监管税费（customs-compliance）。已接线的门禁核对签在前——页面当前能
 * 如实作答的只有它；内部限制与税费两族列表端点未建，如实占位在后；门禁目录的登记签
 * 排在三张读签之后，读写各占各的签。端点建成接线时可回归「动作被放行前要过的层」
 * 那个顺序（内部限制 → 税费义务 → 门禁核对），与 customs-cases 页同一处置。
 *
 * 门禁满足也不生成放行：放行结果始终是监管机构的外部事实，本页四签都不表达它。
 * 登记签同理不表达放行——它只登「这本目录在场」，不登任何一次核对结论。
 */
export function CustomsRestrictionsPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="gates" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="gates">放行门禁核对</TabsTrigger>
          <TabsTrigger value="restrictions">内部合规限制</TabsTrigger>
          <TabsTrigger value="duties">监管核定税费</TabsTrigger>
          <TabsTrigger value="register">登记门禁目录</TabsTrigger>
        </TabsList>
        <TabsContent value="gates" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ReleaseGatesTable />
        </TabsContent>
        <TabsContent value="restrictions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ComplianceRestrictionsTable />
        </TabsContent>
        <TabsContent value="duties" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RegulatoryDutiesTable />
        </TabsContent>
        <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RegistrationPanel
            moduleId="customs-restrictions"
            title={registrationTitles['gate-catalog']}
            endpoint={`POST ${customsRegistrationEndpoints['gate-catalog']}`}
            snapshotHint={registrationSnapshotHints['gate-catalog']}
            submit={(snapshot) => registerCustomsConfiguration('gate-catalog', snapshot)}
            outcomeLabels={registrationOutcomeLabels}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
