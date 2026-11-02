import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import type { WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  customsRegistrationEndpoints,
  listPortsPaths,
  registerCustomsConfiguration,
  registrationOutcomeLabels,
  type CandidatePortListResponseBody,
  type DeclarationPathListResponseBody,
  type PortsPathsRegistry,
} from './api';
import { portsPathsRegistryLabels, problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { labelFor } from '../../vocabulary/status';
import { useMayWriteRegistry } from '../use-operator-grants';
import { unregisteredNameWord, validityOf } from '../network/network-rows';
import { codeStatus } from '../visibility/exception-list';
import { RowStatusPill } from '../visibility/ExceptionDetailPanel';
import { RecordDetailPanel, RouteWorkbench, SectionTabs, notConnectedOverride, techRows, type RowStatus } from '../network/route-workbench';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['customs-ports-paths'];

// 本页三本册（票 admin-remainder-mechanism-batch/03），工作台形态，册签在工具条里切：
//
// 「可用口岸」（口岸 + 可用期间）与「申报路径」（路径 + 经哪个口岸、方向、申报模式 + 适用期间）接
// GET /customs-ports-paths 按 registry 分派。两册按（键，生效起点）版本化，全部版本连同区间原样上列；状态胶囊是此刻在不在期
// （network-rows 的 validityOf，呈现判断不是选版），区间与原码进右栏。
//
// 「可用区域」如实占位：区域维未建模（等自己的票），不为它虚构列表，也不发请求。口岸行没有「所属区域」「关务适用性判断」
// 「限制及解除结果」：目录事实只有键与区间，适用性是判断链的产物、限制归 customs-restrictions 页，都不是本册的列。

type View = 'ports' | 'paths' | 'regions';

const views: { id: View; label: string }[] = [
  { id: 'ports', label: '可用口岸' },
  { id: 'paths', label: '申报路径' },
  { id: 'regions', label: '可用区域' },
];

interface CatalogueRow {
  key: string;
  /** 行首：口岸或路径。 */
  name: string;
  port: string;
  direction?: string;
  mode?: string;
  appliesFrom: string;
  appliesUntil?: string;
}

function statusOf(row: CatalogueRow): RowStatus | undefined {
  const code = validityOf(row.appliesFrom, row.appliesUntil, new Date());
  return code ? codeStatus(code) : undefined;
}

const nameColumn = (header: string, unnamed = false): WorkbenchColumn<CatalogueRow> => ({
  key: 'name',
  header,
  sortKey: 'name',
  render: (row) => (
    <div className="min-w-0">
      <div className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]">{row.name}</div>
      {/* 口岸在领域里只有编码（network-rows 的 unregisteredNameWord 写着为什么），名称一格如实说没登记。 */}
      {unnamed ? <div className="text-[11px] text-idpxyz-breadcrumb">{unregisteredNameWord}</div> : null}
    </div>
  ),
});
const statusColumn: WorkbenchColumn<CatalogueRow> = {
  key: 'status',
  header: '状态',
  render: (row) => {
    const status = statusOf(row);
    return status ? <RowStatusPill status={status} /> : '—';
  },
};
const periodColumn = (header: string): WorkbenchColumn<CatalogueRow> => ({
  key: 'period',
  header,
  hideWhenDetailOpen: true,
  render: (row) => <span className="font-mono text-[12px]">{formatRange(row.appliesFrom, row.appliesUntil)}</span>,
});

const columnsOf: Record<View, WorkbenchColumn<CatalogueRow>[]> = {
  ports: [nameColumn('口岸', true), statusColumn, periodColumn('可用期间')],
  paths: [
    nameColumn('申报路径'),
    statusColumn,
    { key: 'port', header: '经哪个口岸', render: (row) => <span className="font-mono text-[12px]">{row.port}</span> },
    { key: 'direction', header: '方向', render: (row) => (row.direction ? labelFor(row.direction) : '—') },
    { key: 'mode', header: '申报模式', hideWhenDetailOpen: true, render: (row) => <span className="font-mono text-[12px]">{row.mode ?? '—'}</span> },
  ],
  regions: [nameColumn('区域'), statusColumn],
};

function portRows(body: CandidatePortListResponseBody): CatalogueRow[] {
  // 行键循库主键 (tenant, port_ref, applies_from)：同口岸各版本起点唯一。版本序由服务端定（键升序、版本新在前）。
  return body.ports.map((record) => ({
    key: `port:${record.port}:${record.appliesFrom}`,
    name: record.port,
    port: record.port,
    appliesFrom: record.appliesFrom,
    appliesUntil: record.appliesUntil,
  }));
}

function pathRows(body: DeclarationPathListResponseBody): CatalogueRow[] {
  // 口岸是标识引用——「引用的口岸此刻是否在册」是读者拿两册对照的判断，本行不代答。
  return body.paths.map((record) => ({
    key: `path:${record.path}:${record.appliesFrom}`,
    name: record.path,
    port: record.port,
    direction: record.direction,
    mode: record.declarationMode,
    appliesFrom: record.appliesFrom,
    appliesUntil: record.appliesUntil,
  }));
}

function CatalogueDetail({ view, row, onClose }: { view: View; row: CatalogueRow; onClose: () => void }) {
  const period = { label: view === 'ports' ? '可用期间' : '适用期间', value: <span className="font-mono">{formatRange(row.appliesFrom, row.appliesUntil)}</span> };
  return (
    <RecordDetailPanel
      title={row.name}
      status={statusOf(row)}
      subtitle={view === 'ports' ? '可用口岸' : '申报路径'}
      facts={
        view === 'ports'
          ? [{ label: '名称', value: <span className="font-normal text-idpxyz-breadcrumb">{unregisteredNameWord}</span> }, period]
          : [
              { label: '经哪个口岸', value: <span className="font-mono">{row.port}</span> },
              { label: '方向', value: row.direction ? labelFor(row.direction) : '—' },
              { label: '申报模式', value: <span className="font-mono">{row.mode ?? '—'}</span> },
              period,
            ]
      }
      technical={techRows([
        { label: view === 'ports' ? '口岸标识' : '路径标识', value: row.name },
        { label: '口岸标识', value: view === 'paths' ? row.port : undefined },
        { label: '方向码', value: row.direction },
        { label: '生效起点', value: row.appliesFrom },
        { label: '截止', value: row.appliesUntil },
      ])}
      onClose={onClose}
    >
      <p>
        {view === 'ports'
          ? '路线规划只在可用口岸里选。要停用一个口岸，就登一版更晚起用的新版本，旧版到那时自动截止；已登的版本不会被覆盖。'
          : '路线规划只在这些申报路径里选。拟改路要换口岸、方向或报关服务方时，必须先重新做一次关务适用性判断。'}
      </p>
    </RecordDetailPanel>
  );
}

type Loaded =
  | { view: 'ports'; answer: ApiResult<CandidatePortListResponseBody> }
  | { view: 'paths'; answer: ApiResult<DeclarationPathListResponseBody> };

function PortsPathsWorkbench() {
  const [view, setView] = useState<View>('ports');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<Loaded | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (view === 'ports') {
      void listPortsPaths('candidate-port').then((answer) => {
        if (!cancelled) setLoaded({ view: 'ports', answer });
      });
    } else if (view === 'paths') {
      void listPortsPaths('declaration-path').then((answer) => {
        if (!cancelled) setLoaded({ view: 'paths', answer });
      });
    }
    return () => {
      cancelled = true;
    };
  }, [view, reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  // 两册答复体不同形，判读只用它的格（kind），所以在这里放宽成 unknown 体。
  const answer: ApiResult<unknown> | null = loaded && loaded.view === view ? loaded.answer : null;
  const rows: CatalogueRow[] =
    loaded?.view === view && loaded.answer.kind === 'outcome'
      ? loaded.view === 'ports'
        ? portRows(loaded.answer.body as CandidatePortListResponseBody)
        : pathRows(loaded.answer.body as DeclarationPathListResponseBody)
      : [];

  const viewState =
    view === 'regions'
      ? ({ kind: 'unconfigured' } as const)
      : catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: `GET /customs-ports-paths?registry=${view === 'ports' ? 'candidate-port' : 'declaration-path'}`,
          emptyTitle: view === 'ports' ? '还没有登记可用口岸' : '还没有登记申报路径',
          emptyDescription:
            view === 'ports'
              ? '在「登记」里新增口岸后在这里出现；没登记口岸之前路线规划选不出口岸。'
              : '在「登记」里新增申报路径后在这里出现；没登记路径之前路线规划选不出申报路径。',
        });

  return (
    <RouteWorkbench<CatalogueRow>
      key={view}
      title={info.title}
      subtitle="公司可以走的口岸，以及从哪个口岸、按哪个方向、用哪种申报模式报关；路线规划只在这些里面选。"
      sections={<SectionTabs sections={views} active={view} onChange={setView} />}
      answered={view !== 'regions' && answer?.kind === 'outcome'}
      rows={rows}
      rowKey={(row) => row.key}
      statusOf={statusOf}
      searchCells={(row) => [row.name, row.port, row.mode, row.direction ? labelFor(row.direction) : undefined]}
      searchPlaceholder={view === 'paths' ? '搜路径、口岸、申报模式' : '搜口岸'}
      columns={columnsOf[view]}
      sortAccessors={{ name: { value: (row) => row.name } }}
      defaultSort={{ key: 'name', dir: 1 }}
      renderDetail={(row, close) => <CatalogueDetail view={view} row={row} onClose={close} />}
      onRefresh={view === 'regions' ? undefined : retry}
      viewState={viewState}
      // 区域维未建模：说「还不能在这里查看」，与「没有记录」「接入渠道未配置」都分开。
      stateOverride={
        view === 'regions'
          ? notConnectedOverride('可用区域还不能在这里查看', '可用区域的登记与查询还在建设中，这不是「没有区域」；口岸与申报路径两签已经可用。')
          : undefined
      }
    />
  );
}

// —— 登记签（ADR-0085，票 admin-write-faces/02 切片 02b）——

// 两册的读口 registry 词与登记口的类别词逐字相同，所以选册按钮直接取读面已有的词表，
// 不为写签另造一套说法。区域不在此列：它连读面都还没有册（维未建模），登记口更无从谈起。
const registrationKinds: PortsPathsRegistry[] = ['candidate-port', 'declaration-path'];

const registrationTargets: RegistrationTarget[] = registrationKinds.map((kind) => ({
  id: kind,
  label: portsPathsRegistryLabels[kind],
  title: registrationTitles[kind],
  endpoint: `POST ${customsRegistrationEndpoints[kind]}`,
  snapshotHint: registrationSnapshotHints[kind],
  submit: (snapshot: unknown) => registerCustomsConfiguration(kind, snapshot),
  outcomeLabels: registrationOutcomeLabels,
}));

/**
 * 口岸与申报路径（customs-compliance）。词取两处原句：CONTEXT-MAP「customs-compliance ↔ network-routing」——关务提供合规候选区域、口岸、申报路径、限制及解除结果，路由
 * 只在合格候选中选择；customs-compliance CONTEXT.md 所有权句——本上下文拥有合规候选
 * 区域、口岸、申报路径和关务适用性判断。
 *
 * 页面不做任何选择动作：目录只登路径三维事实，路由在合格候选中选择是 network-routing
 * 的事；拟改路改变关务区域、口岸或报关服务方时，必须先请求新的关务适用性判断
 * （CONTEXT-MAP 原句）——那次判断的查阅口随判断链自己的票，不在本页两册之内。
 *
 * 读的三本册在工作台里切（未建模的区域如实占位），登记签在后：读写各占各的
 * 签，形同网络两页。登记签只有登记一个动作——登记册不可覆盖，更正是登一个更晚生效
 * 起点的新版（ADR-0070），所以没有行级编辑或删除面。墙降之前它必然答 403「接入渠道
 * 未配置」，那是诚实答案；墙降当天在装配点换真 Intake 即点亮，本页一行不用改。
 */
export function CustomsPortsPathsPage() {
  // 写入口按授予收起（ADR-0315）：明确没有登记册写授予时不摆登记签。
  const mayWrite = useMayWriteRegistry();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">口岸与申报路径</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">登记</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="catalog" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <PortsPathsWorkbench />
        </TabsContent>
        {mayWrite ? (
          <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <MultiRegistrationPanel moduleId="customs-ports-paths" targets={registrationTargets} problemNote={problemNote} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
