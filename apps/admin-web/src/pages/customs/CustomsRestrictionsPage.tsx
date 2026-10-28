import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  customsRegistrationEndpoints,
  dutyRegistrationEndpoints,
  dutyRegistrationOutcomeLabels,
  dutyUndecidedReasonLabels,
  listDutyCollaborations,
  listDutyVerifications,
  listGateConditions,
  registerCustomsConfiguration,
  registerDutyReconciliation,
  registrationOutcomeLabels,
  type DutyCollaborationListResponseBody,
  type DutyRegistrationKind,
  type DutyVerificationListResponseBody,
  type GateConditionCatalogueRecord,
  type GateConditionListResponseBody,
} from './api';
import {
  dutyRegistrationSnapshotHints,
  dutyRegistrationTitles,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import { collaborationRows, verificationRows, type RegisterRow } from './register-rows';
import { technicalRows } from '../settlement/ops-ui';

// 合规限制与税费页（主责上下文 customs-compliance：关务限制及解除、监管核定税费与放行门禁核对）。
// 界面词与领域词的对照：放行检查 = 放行门禁核对（门禁条件册），税费付款事项 = 税费付款协作事项，税费付款核对同名；
// 内部限制与监管核定税费两族的列表端点未建，如实占位。
const info = moduleInfoById['customs-restrictions'];

// 两条 CONTEXT 硬句落进表形：①「外部放行不自动解除内部限制，内部限制解除也不证明监管机构已经放行」——限制表把两类解除
// 分成两列，永不合并为一个「已解除」；②「尚未放行或存在方向性限制时，必须阻断其明确约束的出库、装载出发、跨关务区域移动
// 或交付」——放行检查绑定具体动作，判断不复用于其他动作或监管边界。

interface GateRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

function gateCol(id: string, header: string, mono = false, nowrap = false): ListColumn<GateRow> {
  return {
    id,
    header,
    className: [mono ? 'font-mono text-xs' : '', nowrap ? 'whitespace-nowrap' : ''].filter(Boolean).join(' ') || undefined,
    render: (row) => row.values[id] ?? '—',
  };
}

function pillCol(id: string, header: string, codeId: string, scope?: string): ListColumn<GateRow> {
  return {
    id,
    header,
    align: 'center',
    render: (row) => (row.values[codeId] ? <StatusPill code={row.values[codeId]} scope={scope} label={row.values[id]} /> : (row.values[id] ?? '—')),
  };
}

function techCol(pairs: [label: string, key: string][]): ListColumn<GateRow> {
  return {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={technicalRows(pairs.map(([label, key]) => ({ label, value: row.values[key] })))} className="max-w-md open:min-w-[18rem]" />,
  };
}

// —— 放行检查（门禁条件册）——

// 三元键（范围·动作·边界）三列并列，是「门禁满足不生成放行，也不能复用于其他动作或监管边界」的表形。刻意不设「门禁
// 判断」列：五值结论是门禁编排的判断语义，查阅面转述登记册本身。
const gateConditionColumns: ListColumn<GateRow>[] = [
  gateCol('scope', '申报范围', true, true),
  gateCol('action', '检查哪一步', false, true),
  gateCol('precondition', '检查项目', true),
  pillCol('state', '认定', 'stateCode'),
  gateCol('registeredAt', '登记时间', true, true),
  techCol([
    ['监管边界', 'boundary'],
  ]),
];

function gateRows(gates: GateConditionCatalogueRecord[]): GateRow[] {
  return gates.flatMap((gate) => {
    const base = {
      scope: gate.scope,
      action: labelFor(gate.action),
      boundary: gate.boundary,
      registeredAt: formatInstant(gate.registeredAt),
    };
    // 空清单是登记方明说「此动作在此边界本就不受门禁」，不是查不到。与关闭核对那边的空清单相反：那边是中性的「无事项」，
    // 这边是放行侧的绿灯。
    if (gate.findings.length === 0) {
      return [
        {
          // 行键循库主键 (tenant, scope_ref, action, boundary_ref)。
          key: `gate:${gate.scope}:${gate.action}:${gate.boundary}`,
          values: { ...base, precondition: '（这一步不用检查）', state: '无需检查', stateCode: 'NOT_GUARDED' },
        },
      ];
    }
    return gate.findings.map((finding) => ({
      key: `finding:${gate.scope}:${gate.action}:${gate.boundary}:${finding.precondition}`,
      values: { ...base, precondition: finding.precondition, state: labelFor(finding.state), stateCode: finding.state },
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
  const visibleRows = needle ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle))) : rows;
  const retry = () => setReloadKey((value) => value + 1);
  const unmet = gates.reduce((sum, gate) => sum + gate.findings.filter((finding) => finding.state !== 'MET').length, 0);

  return (
    <ListPageTemplate<GateRow>
      title="放行检查"
      // 页头说本页最容易读错的那一格：没登记不等于不用检查。
      description="货物出库、装车、跨关区、交付前要过的检查项目和每项的认定；没登记的那一步按未决处理，不是不用检查"
      search={{ value: search, onChange: setSearch, placeholder: '搜申报范围、检查项目' }}
      filterSummary={answer?.kind === 'outcome' ? `${gates.length} 组 · 未满足或冲突 ${unmet} 项` : undefined}
      columns={gateConditionColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的检查项目"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-gate-conditions',
        emptyTitle: '还没有登记放行检查项目',
        emptyDescription: '没登记检查项目的动作一律按未决处理，不会当作不用检查。请在「登记」里按申报范围和动作新增。',
      })}
    />
  );
}

// —— 税费付款事项与税费付款核对两册 ——
//
// 两册分两签而不并成一张「税费付款」表：它们是 UC-CC-009「七层对象必须分离」里的两层，谁都不是「税费状态」——并成一张就得
// 给每份事项配一个「当前核对」，那正是读面替编排下的判。

const collaborationColumns: ListColumn<RegisterRow>[] = [
  gateCol('scope', '申报范围', true, true) as ListColumn<RegisterRow>,
  pillCol('kind', '怎么付', 'kindCode') as ListColumn<RegisterRow>,
  gateCol('basis', '税费 / 无需付款依据', true) as ListColumn<RegisterRow>,
  gateCol('obligor', '法定义务人', true, true) as ListColumn<RegisterRow>,
  gateCol('target', '交接给谁', true, true) as ListColumn<RegisterRow>,
  gateCol('formedAt', '形成时间', true, true) as ListColumn<RegisterRow>,
  techCol([['付款要求来源', 'requirement']]) as ListColumn<RegisterRow>,
];

function DutyCollaborationsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<DutyCollaborationListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listDutyCollaborations().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const collaborations = answer?.kind === 'outcome' ? answer.body.collaborations : [];
  const rows = collaborationRows(collaborations);
  const needle = search.trim().toLowerCase();
  const visibleRows = needle ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle))) : rows;
  const retry = () => setReloadKey((value) => value + 1);
  const notRequired = collaborations.filter((item) => item.kind === 'EXPLICITLY_NOT_REQUIRED').length;

  return (
    <ListPageTemplate<RegisterRow>
      title="税费付款事项"
      description="每个申报范围的税费该不该付、由谁付、交给谁办；没有海关税费结果不等于不用付"
      search={{ value: search, onChange: setSearch, placeholder: '搜申报范围、税费、义务人' }}
      filterSummary={answer?.kind === 'outcome' ? `${collaborations.length} 项 · 明确无需付款 ${notRequired} 项` : undefined}
      columns={collaborationColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的事项"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-duty-collaborations',
        emptyTitle: '还没有税费付款事项',
        emptyDescription: '海关核定税费或登记了明确无需付款的依据之后，在「登记」里登记付款事项；没有事项时放行检查的税费那一道停在未决。',
      })}
    />
  );
}

// 三轴三列并列，是「分别判断覆盖状态、差额和事实有效性……不能实现为一组互斥总状态」这条硬句的表形。刻意不设「付款状态」列。
const verificationColumns: ListColumn<RegisterRow>[] = [
  gateCol('duty', '税费版本', true, true) as ListColumn<RegisterRow>,
  gateCol('funds', '收付款流水', true, true) as ListColumn<RegisterRow>,
  gateCol('scope', '申报范围', true, true) as ListColumn<RegisterRow>,
  pillCol('coverage', '覆盖', 'coverageCode', 'duty-coverage') as ListColumn<RegisterRow>,
  pillCol('delta', '差额', 'deltaCode') as ListColumn<RegisterRow>,
  pillCol('validity', '有效性', 'validityCode') as ListColumn<RegisterRow>,
  gateCol('verifiedAt', '核对时间', true, true) as ListColumn<RegisterRow>,
  techCol([
    ['收付款版本', 'fundsVersion'],
    ['按哪个程序判', 'procedure'],
    ['核对版本', 'version'],
    ['关联依据', 'basis'],
  ]) as ListColumn<RegisterRow>,
];

function DutyVerificationsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<DutyVerificationListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listDutyVerifications().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const verifications = answer?.kind === 'outcome' ? answer.body.verifications : [];
  const rows = verificationRows(verifications);
  const needle = search.trim().toLowerCase();
  const visibleRows = needle ? rows.filter((row) => Object.values(row.values).some((value) => value.toLowerCase().includes(needle))) : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<RegisterRow>
      title="税费付款核对"
      description="每笔税费对上了哪笔收付款：盖住多少、差多少、这笔钱还算不算数，三项分开记"
      search={{ value: search, onChange: setSearch, placeholder: '搜税费版本、收付款流水、申报范围' }}
      // 摘要只报版本数，不数「几版悬着 / 几版可放」：那两个数都要先把三轴折一次，正是本签不做的事。
      filterSummary={answer?.kind === 'outcome' ? `${verifications.length} 版` : undefined}
      columns={verificationColumns}
      rows={visibleRows}
      rowKey={(row) => row.key}
      emptyRowsNote="没有匹配的核对"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /customs-duty-verifications',
        emptyTitle: '还没有税费付款核对',
        emptyDescription: '收付款登记后在「登记」里核对到税费上；没有核对时放行检查的税费那一道停在未决，不会当作已付。',
      })}
    />
  );
}

// —— 内部限制与监管核定税费：列表端点未建，如实占位 ——

const unconfigured = {
  kind: 'unconfigured' as const,
  title: '这一签的数据还没有接通',
  description: '内部限制与海关核定税费的查询入口还在建设中；本签不发请求，也不显示任何示意数据。',
  facts: {
    owner: info.owner,
    source: info.source,
    unlock: '限制与税费的查询端点建成并经 ADR-0017 准入闸门放行后接线',
  },
};

interface PlaceholderRow {
  id: string;
}

// 「限制解除决定（内部）」与「海关放行（外部引用）」两列的分离是本表的存在理由：任何把两者折成一列「解除状态」的改动都会让
// 读表的人把外部放行读成内部限制已解除，或把内部解除读成海关已放行。
const restrictionColumns: ListColumn<PlaceholderRow>[] = [
  { id: 'id', header: '限制编号', render: (row) => row.id },
  { id: 'source', header: '谁设的限制', render: () => '—' },
  { id: 'coverage', header: '限制什么', render: () => '—' },
  { id: 'period', header: '适用时间', render: () => '—' },
  { id: 'internal-release', header: '内部解除', render: () => '—' },
  { id: 'external-release', header: '海关放行', render: () => '—' },
];

// 核对三轴各占一列，互斥总状态是 CONTEXT 明禁的形状。
const dutyColumns: ListColumn<PlaceholderRow>[] = [
  { id: 'id', header: '税费编号', render: (row) => row.id },
  { id: 'scope', header: '申报范围', render: () => '—' },
  { id: 'obligor', header: '法定义务人', render: () => '—' },
  { id: 'supplement', header: '补缴要求', render: () => '—' },
  { id: 'refund', header: '退税决定', render: () => '—' },
  { id: 'coverage', header: '覆盖', render: () => '—' },
  { id: 'difference', header: '差额', render: () => '—' },
  { id: 'validity', header: '有效性', render: () => '—' },
];

function ComplianceRestrictionsTable() {
  const [search, setSearch] = useState('');
  return (
    <ListPageTemplate<PlaceholderRow>
      title="内部限制"
      description="公司自己给货物设的限制（如扣货、暂停派送）；海关放行不会自动解除它，解除它也不等于海关放行"
      search={{ value: search, onChange: setSearch, placeholder: '搜限制编号、设限来源' }}
      columns={restrictionColumns}
      rows={[]}
      rowKey={(row) => row.id}
      viewState={unconfigured}
    />
  );
}

function RegulatoryDutiesTable() {
  const [search, setSearch] = useState('');
  return (
    <ListPageTemplate<PlaceholderRow>
      title="海关核定税费"
      description="海关核定的每笔税费、补缴要求与退税决定；核定了不等于付了，退税决定了不等于钱回来了"
      search={{ value: search, onChange: setSearch, placeholder: '搜税费编号、申报范围' }}
      columns={dutyColumns}
      rows={[]}
      rowKey={(row) => row.id}
      viewState={unconfigured}
    />
  );
}

// —— 登记签：放行检查项目 + 税费付款事项与核对 ——
//
// 本页有在线登记口的是这三本；内部限制与海关核定税费两族连查阅端点都还没有，更没有登记用例可接。三本一页多册，按
// MultiRegistrationPanel 选册——换册即换草稿，各册输入形状互不相容。协作与核对两口的答案代数是另一族，词表另给一张。
const dutyRegistrationTarget = (kind: DutyRegistrationKind): RegistrationTarget => ({
  id: kind,
  label: dutyRegistrationTitles[kind],
  title: dutyRegistrationTitles[kind],
  endpoint: `POST ${dutyRegistrationEndpoints[kind]}`,
  snapshotHint: dutyRegistrationSnapshotHints[kind],
  submit: (snapshot: unknown) => registerDutyReconciliation(kind, snapshot),
  outcomeLabels: dutyRegistrationOutcomeLabels,
  undecidedReasonLabels: dutyUndecidedReasonLabels,
});

const registrationTargets: RegistrationTarget[] = [
  {
    id: 'gate-catalog',
    label: registrationTitles['gate-catalog'],
    title: registrationTitles['gate-catalog'],
    endpoint: `POST ${customsRegistrationEndpoints['gate-catalog']}`,
    snapshotHint: registrationSnapshotHints['gate-catalog'],
    submit: (snapshot: unknown) => registerCustomsConfiguration('gate-catalog', snapshot),
    outcomeLabels: registrationOutcomeLabels,
  },
  dutyRegistrationTarget('duty-collaboration'),
  dutyRegistrationTarget('duty-payment-verification'),
];

/**
 * 合规限制与税费。已接线的三张读签在前（放行检查、税费付款事项、税费付款核对；事项与核对紧挨放行检查，因为核对是放行检查
 * 「税费付款」那一道读的东西）；内部限制与海关核定税费两族列表端点未建，如实占位在后；登记签排在所有读签之后。
 * 放行检查通过也不生成放行：放行结果始终是海关的外部事实，本页各签都不表达它。
 */
export function CustomsRestrictionsPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="gates" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="gates">放行检查</TabsTrigger>
          <TabsTrigger value="duty-collaborations">税费付款事项</TabsTrigger>
          <TabsTrigger value="duty-verifications">税费付款核对</TabsTrigger>
          <TabsTrigger value="restrictions">内部限制</TabsTrigger>
          <TabsTrigger value="duties">海关核定税费</TabsTrigger>
          <TabsTrigger value="register">登记</TabsTrigger>
        </TabsList>
        <TabsContent value="gates" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ReleaseGatesTable />
        </TabsContent>
        <TabsContent value="duty-collaborations" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DutyCollaborationsTable />
        </TabsContent>
        <TabsContent value="duty-verifications" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DutyVerificationsTable />
        </TabsContent>
        <TabsContent value="restrictions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ComplianceRestrictionsTable />
        </TabsContent>
        <TabsContent value="duties" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RegulatoryDutiesTable />
        </TabsContent>
        <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <MultiRegistrationPanel moduleId="customs-restrictions" targets={registrationTargets} problemNote={problemNote} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
