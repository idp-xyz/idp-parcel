import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { useAddressKeyword, type WorkbenchColumn } from '../../templates';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import { labelFor } from '../../vocabulary/status';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';
import { codeStatus } from '../visibility/exception-list';
import { RowStatusPill } from '../visibility/ExceptionDetailPanel';
import {
  listNetworkCatalog,
  networkCatalogConditions,
  networkRegistrationEndpoints,
  registerNetworkCatalogVersion,
  registrationOutcomeLabels,
  registrationRefusalReasonLabels,
  type NetworkCatalogFamily,
  type NetworkVersionRecord,
} from './api';
import {
  familyLabels,
  keywordPlaceholders,
  networkCatalogFamilies,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import { adjustmentWindowOf, timeZoneLabel, validityOf, versionWord } from './network-rows';
import { RecordDetailPanel, RouteWorkbench, SectionTabs, techRows, type DetailFact, type RowStatus } from './route-workbench';

const info = moduleInfoById['network-catalog'];

// 工作台形态：各族一本册，签在工具条里切；行首放运营认的对象（网点 / 连接 / 线路的编码，同一对象几版逐版列出），状态胶囊是
// 此刻在不在期（network-rows 的 validityOf，呈现判断不是选版）。版本号、时区原名与各种原码进右栏「技术详情」。
// 读面有的上列，读面没有的不上列（MCP-3 裁决总则）：网点名称、日历正文今天读面不带，不发明。
// 检索词沿地址上的 ?q= 停稳后下推到读口（ADR-0144），翻页走游标；不在答回来的那一页上再筛——截断之后再筛，答出的「没有」可能是假的。
type Row = NetworkVersionRecord & { key: string };

const mono = (value: string) => <span className="truncate font-mono text-[12px]">{value}</span>;

function statusOf(family: NetworkCatalogFamily, row: Row): RowStatus | undefined {
  const code =
    family === 'availability-adjustment'
      ? adjustmentWindowOf(row.effectiveAt, row.liftedAt, new Date())
      : validityOf(row.effectiveFrom, row.effectiveTo, new Date());
  return code ? codeStatus(code) : undefined;
}

/** 行首格：编码在上，第几版在下。日历的行首是它适用的对象。 */
function identityCell(row: Row, family: NetworkCatalogFamily) {
  const code = family === 'service-calendar' || family === 'availability-adjustment' ? row.targetCode : row.code;
  const second =
    family === 'service-calendar'
      ? `${row.targetKind ? labelFor(row.targetKind) : ''} · ${versionWord(row.version)}`
      : family === 'availability-adjustment'
        ? row.targetKind
          ? labelFor(row.targetKind)
          : ''
        : versionWord(row.version);
  return (
    <div className="min-w-0">
      <div className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]">{code ?? '—'}</div>
      <div className="text-[11px] text-idpxyz-breadcrumb">{second}</div>
    </div>
  );
}

const identityHeaders: Record<NetworkCatalogFamily, string> = {
  node: '网点',
  connection: '运输连接',
  line: '线路',
  'service-area': '服务区域',
  'service-calendar': '适用于',
  'availability-adjustment': '哪里',
  'route-strategy': '路线策略',
};

const periodText = (row: Row) => (row.effectiveFrom ? formatRange(row.effectiveFrom, row.effectiveTo) : '—');

function columnsOf(family: NetworkCatalogFamily): WorkbenchColumn<Row>[] {
  const identity: WorkbenchColumn<Row> = { key: 'code', header: identityHeaders[family], sortKey: 'code', render: (row) => identityCell(row, family) };
  const status: WorkbenchColumn<Row> = {
    key: 'status',
    header: '状态',
    render: (row) => {
      const value = statusOf(family, row);
      return value ? <RowStatusPill status={value} /> : '—';
    },
  };
  const period: WorkbenchColumn<Row> = { key: 'period', header: '有效期', hideWhenDetailOpen: true, render: (row) => mono(periodText(row)) };
  const zone: WorkbenchColumn<Row> = {
    key: 'zone',
    header: '时区',
    hideWhenDetailOpen: true,
    render: (row) => <span title={row.businessTimezone}>{timeZoneLabel(row.businessTimezone)}</span>,
  };
  const scope: WorkbenchColumn<Row> = { key: 'scope', header: '适用服务', render: (row) => (row.applicableScope ? labelFor(row.applicableScope) : '—') };
  switch (family) {
    case 'connection':
      return [identity, { key: 'endpoints', header: '从 → 到', render: (row) => mono(`${row.fromNode ?? '—'} → ${row.toNode ?? '—'}`) }, status, zone, period];
    case 'line':
      return [identity, { key: 'segments', header: '途经的运输连接', render: (row) => mono((row.segments ?? []).join(' → ') || '—') }, status, scope, period];
    case 'availability-adjustment':
      return [
        identity,
        { key: 'kind', header: '调整', render: (row) => (row.kind ? labelFor(row.kind) : '—') },
        status,
        { key: 'source', header: '消息来源', hideWhenDetailOpen: true, render: (row) => mono(row.source ?? '—') },
        {
          key: 'window',
          header: '起止',
          hideWhenDetailOpen: true,
          render: (row) => mono(`${row.effectiveAt ? formatInstant(row.effectiveAt) : '—'} → ${row.liftedAt ? formatInstant(row.liftedAt) : '未解除'}`),
        },
      ];
    case 'route-strategy':
      return [
        identity,
        status,
        scope,
        // 排序方式缺席是登记方没声明，不是「成本最低」；如实说未声明。
        { key: 'ranking', header: '怎么选路线', render: (row) => (row.rankingForm ? labelFor(row.rankingForm) : <span className="text-idpxyz-breadcrumb">未声明</span>) },
        period,
      ];
    case 'service-calendar':
      return [identity, status, period];
    default:
      return [identity, status, zone, period];
  }
}

function factsOf(family: NetworkCatalogFamily, row: Row): DetailFact[] {
  const period: DetailFact = { label: '有效期', value: <span className="font-mono">{periodText(row)}</span> };
  switch (family) {
    case 'connection':
      return [
        { label: '出发网点', value: <span className="font-mono">{row.fromNode ?? '—'}</span> },
        { label: '到达网点', value: <span className="font-mono">{row.toNode ?? '—'}</span> },
        { label: '时区', value: timeZoneLabel(row.businessTimezone) },
        period,
      ];
    case 'line':
      return [
        { label: '途经', value: <span className="font-mono">{(row.segments ?? []).join(' → ') || '—'}</span> },
        { label: '适用服务', value: row.applicableScope ? labelFor(row.applicableScope) : '—' },
        { label: '时区', value: timeZoneLabel(row.businessTimezone) },
        period,
      ];
    case 'availability-adjustment':
      return [
        { label: '调整', value: row.kind ? labelFor(row.kind) : '—' },
        { label: '消息来源', value: <span className="font-mono">{row.source ?? '—'}</span> },
        { label: '开始', value: row.effectiveAt ? formatInstant(row.effectiveAt) : '—' },
        { label: '解除', value: row.liftedAt ? formatInstant(row.liftedAt) : '未解除' },
      ];
    case 'route-strategy':
      return [
        { label: '适用服务', value: row.applicableScope ? labelFor(row.applicableScope) : '—' },
        { label: '怎么选路线', value: row.rankingForm ? labelFor(row.rankingForm) : '未声明' },
        period,
      ];
    case 'service-calendar':
      return [{ label: '适用对象', value: row.targetKind ? labelFor(row.targetKind) : '—' }, period];
    default:
      return [{ label: '时区', value: timeZoneLabel(row.businessTimezone) }, period];
  }
}

function NetworkDetail({ family, row, onClose }: { family: NetworkCatalogFamily; row: Row; onClose: () => void }) {
  const title = (family === 'service-calendar' || family === 'availability-adjustment' ? row.targetCode : row.code) ?? '—';
  return (
    <RecordDetailPanel
      title={title}
      status={statusOf(family, row)}
      subtitle={`${familyLabels[family]} · ${versionWord(row.version)}`}
      facts={factsOf(family, row)}
      technical={techRows([
        { label: '编码', value: row.code },
        { label: '版本号', value: String(row.version) },
        { label: '时区', value: row.businessTimezone },
        { label: '适用服务码', value: row.applicableScope },
        { label: '排序方式码', value: row.rankingForm },
        { label: '对象类别码', value: row.targetKind },
        { label: '调整种类码', value: row.kind },
        { label: '生效起点', value: row.effectiveFrom },
        { label: '截止', value: row.effectiveTo },
      ])}
      onClose={onClose}
    >
      <p>
        同一对象改过几次就有几版，逐版列出、不覆盖；要改就登一个更大的版本号，旧版到新版生效时自动截止。哪一版被路线规划用上，由定路线时的那一刻决定。
      </p>
    </RecordDetailPanel>
  );
}

// 行键：各族的唯一键不同（日历按对象，其余按编码），版本号一起进键。
function rowOf(family: NetworkCatalogFamily, record: NetworkVersionRecord): Row {
  const identity = family === 'service-calendar' ? `${record.targetKind}:${record.targetCode}` : record.code;
  return { ...record, key: `${family}:${identity}@${record.version}` };
}

function NetworkCatalogTable() {
  const [familyId, setFamilyId] = useState<NetworkCatalogFamily>('node');
  const [keyword, setKeyword] = useAddressKeyword();
  const conditions = networkCatalogConditions(familyId, { q: useSettledKeyword(keyword) });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listNetworkCatalog);
  const mayWrite = useMayWriteRegistry();
  // 手上的答复认的是此刻这一问的条件签名(族在其中),答的一定是当前族,不会把上一族的行套进这一族的列。
  const rows = answer?.kind === 'outcome' ? answer.body.versions.map((record) => rowOf(familyId, record)) : [];

  return (
    <RouteWorkbench<Row>
      // 换族重挂：上一族的状态筛选与选中行不带进下一族。
      key={familyId}
      title={info.title}
      // 网络目录是册级不登名称（network-routing 的网点只有编码这一身份，CONTEXT 不定义名称），所以不设名称栏、行首显编码，
      // 只在页头用册级说法交代一句——行级的「未登记」会让人去催一个登不进的字段。
      subtitle="公司的网点、网点之间的运输连接、由连接串成的线路，以及服务日历、停运与恢复、路线策略。网点目录只登编码，不登名称。"
      sections={
        <SectionTabs
          sections={networkCatalogFamilies.map((family) => ({ id: family, label: familyLabels[family] }))}
          active={familyId}
          onChange={setFamilyId}
        />
      }
      answered={answer?.kind === 'outcome'}
      rows={rows}
      rowKey={(row) => row.key}
      statusOf={(row) => statusOf(familyId, row)}
      searchCells={() => []}
      search={{ value: keyword, onChange: setKeyword, pushedDown: true }}
      searchPlaceholder={keywordPlaceholders[familyId]}
      columns={columnsOf(familyId)}
      sortAccessors={{ code: { value: (row) => row.code ?? row.targetCode } }}
      defaultSort={{ key: 'code', dir: 1 }}
      renderDetail={(row, close) => <NetworkDetail family={familyId} row={row} onClose={close} />}
      onRefresh={retry}
      pagination={pagination}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /network-catalog?family=${familyId}`,
        emptyTitle: `还没有登记${familyLabels[familyId]}`,
        emptyDescription: entryHint(mayWrite, `在「新增或更新」签里登记${familyLabels[familyId]}后，会在这里出现。`),
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

// 登记签装本页读签的同六族——服务区域的登记面随它的读面归专页,写签不比读签多铺一族:
// 那会让同一本册在两处都能登,而其中一处的页面上根本看不到登进去的结果。
const registrationTargets: RegistrationTarget[] = networkCatalogFamilies.map((family) => ({
  id: family,
  label: familyLabels[family],
  title: registrationTitles[family],
  endpoint: `POST ${networkRegistrationEndpoints[family]}`,
  snapshotHint: registrationSnapshotHints[family],
  submit: (snapshot) => registerNetworkCatalogVersion(family, snapshot),
  outcomeLabels: registrationOutcomeLabels,
  refusalReasonLabels: registrationRefusalReasonLabels,
}));

/**
 * 网络与网点：逐族查阅版本，外加登记签（ADR-0085）。
 *
 * 登记签不是「编辑」表单：目录按版推进，新版翻旧插新、不覆盖行，停用由停运与恢复另族陈述——所以只有登记一个动作，
 * 没有行级编辑或删除。写入口按授予收起（ADR-0315）：明确没有登记册写授予时不摆这一签。
 */
export function NetworkCatalogPage() {
  const mayWrite = useMayWriteRegistry();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">网络与网点</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">新增或更新</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="catalog" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <NetworkCatalogTable />
        </TabsContent>
        {mayWrite ? (
          <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <MultiRegistrationPanel moduleId="network-catalog" targets={registrationTargets} problemNote={problemNote} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
