import { useState, type ReactNode } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, useAddressKeyword, type ListColumn } from '../../templates';
import { MultiRegistrationPanel, chipClass, type RegistrationTarget } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { useMayWriteRegistry } from '../use-operator-grants';
import { technicalRows } from '../settlement/ops-ui';
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

const info = moduleInfoById['network-catalog'];

// 列只放运营认的：哪个网点 / 连接 / 线路、从哪到哪、什么时区、有效期、此刻在不在期。版本号与原码进每行末尾的「技术详情」。
// 读面有的上列，读面没有的不上列（MCP-3 裁决总则）：网点名称、日历正文今天读面不带，不发明列。
type Row = NetworkVersionRecord & { key: string };

const mono = (value: ReactNode) => <span className="font-mono text-[12px]">{value}</span>;

function codeCol(header: string): ListColumn<Row> {
  return {
    id: 'code',
    header,
    render: (row) => (
      <div className="min-w-0">
        <div className="truncate font-mono text-[12px] font-medium text-idpxyz-textBright">{row.code ?? '—'}</div>
        <div className="text-[11px] text-idpxyz-textMuted">{versionWord(row.version)}</div>
      </div>
    ),
  };
}

const zoneCol: ListColumn<Row> = {
  id: 'zone',
  header: '时区',
  render: (row) => <span title={row.businessTimezone}>{timeZoneLabel(row.businessTimezone)}</span>,
};

const periodCol: ListColumn<Row> = {
  id: 'period',
  header: '有效期',
  className: 'min-w-64',
  render: (row) => mono(row.effectiveFrom ? formatRange(row.effectiveFrom, row.effectiveTo) : '—'),
};

const validityCol: ListColumn<Row> = {
  id: 'validity',
  header: '状态',
  align: 'center',
  className: 'w-[96px]',
  render: (row) => {
    const code = validityOf(row.effectiveFrom, row.effectiveTo, new Date());
    return code ? <StatusPill code={code} /> : '—';
  },
};

function techCol(rowsOf: (row: Row) => { label: string; value: string | undefined }[]): ListColumn<Row> {
  return {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={technicalRows(rowsOf(row))} className="max-w-md open:min-w-[18rem]" />,
  };
}

const versionTech = (row: Row) => [
  { label: '版本号', value: String(row.version) },
  { label: '时区', value: row.businessTimezone },
];

const familyColumns: Record<NetworkCatalogFamily, ListColumn<Row>[]> = {
  node: [codeCol('网点'), zoneCol, periodCol, validityCol, techCol(versionTech)],
  connection: [
    codeCol('运输连接'),
    { id: 'endpoints', header: '从 → 到', render: (row) => mono(`${row.fromNode ?? '—'} → ${row.toNode ?? '—'}`) },
    zoneCol,
    periodCol,
    validityCol,
    techCol(versionTech),
  ],
  line: [
    codeCol('线路'),
    { id: 'segments', header: '途经的运输连接', className: 'min-w-64', render: (row) => mono((row.segments ?? []).join(' → ') || '—') },
    { id: 'scope', header: '适用服务', render: (row) => (row.applicableScope ? labelFor(row.applicableScope) : '—') },
    periodCol,
    validityCol,
    techCol((row) => [...versionTech(row), { label: '适用服务码', value: row.applicableScope }]),
  ],
  // 服务区域由专页承担，本页签里不含它；封闭集类型要求键在场，列表留空。
  'service-area': [],
  'service-calendar': [
    {
      id: 'target',
      header: '适用于',
      render: (row) => (
        <div className="min-w-0">
          <div className="truncate font-mono text-[12px] font-medium text-idpxyz-textBright">{row.targetCode ?? '—'}</div>
          <div className="text-[11px] text-idpxyz-textMuted">
            {row.targetKind ? labelFor(row.targetKind) : ''} · {versionWord(row.version)}
          </div>
        </div>
      ),
    },
    periodCol,
    validityCol,
    techCol((row) => [
      { label: '版本号', value: String(row.version) },
      { label: '对象类别码', value: row.targetKind },
    ]),
  ],
  'availability-adjustment': [
    {
      id: 'target',
      header: '哪里',
      render: (row) => (
        <div className="min-w-0">
          <div className="truncate font-mono text-[12px] font-medium text-idpxyz-textBright">{row.targetCode ?? '—'}</div>
          <div className="text-[11px] text-idpxyz-textMuted">{row.targetKind ? labelFor(row.targetKind) : ''}</div>
        </div>
      ),
    },
    { id: 'kind', header: '调整', align: 'center', render: (row) => (row.kind ? <StatusPill code={row.kind} /> : '—') },
    { id: 'source', header: '消息来源', render: (row) => mono(row.source ?? '—') },
    {
      id: 'window',
      header: '起止',
      className: 'min-w-64',
      render: (row) => mono(`${row.effectiveAt ? formatInstant(row.effectiveAt) : '—'} → ${row.liftedAt ? formatInstant(row.liftedAt) : '未解除'}`),
    },
    {
      id: 'validity',
      header: '状态',
      align: 'center',
      className: 'w-[96px]',
      render: (row) => {
        const code = adjustmentWindowOf(row.effectiveAt, row.liftedAt, new Date());
        return code ? <StatusPill code={code} /> : '—';
      },
    },
    techCol((row) => [
      { label: '调整编码', value: row.code },
      { label: '版本号', value: String(row.version) },
      { label: '调整种类码', value: row.kind },
      { label: '对象类别码', value: row.targetKind },
    ]),
  ],
  'route-strategy': [
    codeCol('路线策略'),
    { id: 'scope', header: '适用服务', render: (row) => (row.applicableScope ? labelFor(row.applicableScope) : '—') },
    // 排序方式缺席是登记方没声明，不是「成本最低」；如实说未声明。
    { id: 'ranking', header: '怎么选路线', render: (row) => (row.rankingForm ? labelFor(row.rankingForm) : <span className="text-idpxyz-textMuted">未声明</span>) },
    periodCol,
    validityCol,
    techCol((row) => [
      { label: '版本号', value: String(row.version) },
      { label: '适用服务码', value: row.applicableScope },
      { label: '排序方式码', value: row.rankingForm },
    ]),
  ],
};

// 行键：各族的唯一键不同（日历按对象，其余按编码），版本号一起进键。
function rowOf(family: NetworkCatalogFamily, record: NetworkVersionRecord): Row {
  const identity = family === 'service-calendar' ? `${record.targetKind}:${record.targetCode}` : record.code;
  return { ...record, key: `${family}:${identity}@${record.version}` };
}

// 逐族查阅版本原文;chip 六族不含服务区域(MCP-3 裁决③,服务区域由专页承担)。
// 读口已迁到 ADR-0144(票 catalogue-read-pagination/03、05):检索词沿地址上的 ?q= 停稳后下推,翻页走游标,换族或换检索词
// 回第一页;次序是读口各族的缺省序,本页不另排。不在答回来的那一页上再筛——截断之后再筛,答出的「没有」可能是假的。
function NetworkCatalogTable() {
  const [familyId, setFamilyId] = useState<NetworkCatalogFamily>('node');
  const [keyword, setKeyword] = useAddressKeyword();
  const conditions = networkCatalogConditions(familyId, { q: useSettledKeyword(keyword) });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listNetworkCatalog);
  // 手上的答复认的是此刻这一问的条件签名(族在其中),答的一定是当前族,不会把上一族的行套进这一族的列。
  const rows = answer?.kind === 'outcome' ? answer.body.versions.map((record) => rowOf(familyId, record)) : [];

  return (
    <ListPageTemplate<Row>
      title={info.title}
      description="公司的网点、网点之间的运输连接、由连接串成的线路，以及服务日历、停运与恢复、路线策略。同一对象改过几次就有几版，逐版列出。"
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: keywordPlaceholders[familyId],
      }}
      filters={
        <>
          {networkCatalogFamilies.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === familyId)}
              onClick={() => setFamilyId(candidate)}
            >
              {familyLabels[candidate]}
            </button>
          ))}
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示:未配置/错误态下「0 个版本」会与状态区
        // 「这不是目录为空」的说明自相矛盾(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome'
          ? `${familyLabels[familyId]} 本页 ${rows.length} 条`
          : undefined
      }
      columns={familyColumns[familyId]}
      rows={rows}
      rowKey={(row) => row.key}
      emptyRowsNote={`没有符合条件的${familyLabels[familyId]}；换个关键词试试`}
      pagination={pagination}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /network-catalog?family=${familyId}`,
        emptyTitle: `还没有登记${familyLabels[familyId]}`,
        emptyDescription: `在「新增或更新」签里登记${familyLabels[familyId]}后，会在这里出现。`,
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
