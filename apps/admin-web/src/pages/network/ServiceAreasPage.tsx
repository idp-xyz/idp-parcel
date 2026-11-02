import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { useAddressKeyword, type WorkbenchColumn } from '../../templates';
import { RegistrationPanel } from '../../components/registration';
import { CountryLabel } from '../refdata-labels';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatRange } from '../catalogue-view';
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
  type NetworkVersionRecord,
} from './api';
import { keywordPlaceholders, problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { validityOf, versionWord } from './network-rows';
import { placeLabel, type PlaceNameIndex } from './place-names';
import { usePlaceNames } from './use-place-names';
import { RecordDetailPanel, RouteWorkbench, techRows, type RowStatus } from './route-workbench';

const info = moduleInfoById['service-areas'];

// 工作台形态：行首是区域编码（第几版挂在下面），状态胶囊是此刻在不在期，覆盖一格说国家与邮编段；揽收 / 派送网点、有效期
// 与版本号进右栏。覆盖缺席是这一版没登覆盖（老版本或只登了骨架），如实说「没登覆盖」，不当成「覆盖全部」。
// 揽收 / 派送网点按此刻生效那一版查名（use-place-names），没有名称的显编码；编码原值在右栏技术详情。
const nodesText = (nodes: string[] | undefined, names: PlaceNameIndex) =>
  nodes && nodes.length > 0 ? nodes.map((node) => placeLabel(names, node)).join('、') : '—';
const periodText = (row: NetworkVersionRecord) => (row.effectiveFrom ? formatRange(row.effectiveFrom, row.effectiveTo) : '—');
const prefixText = (row: NetworkVersionRecord) =>
  row.coverage?.postalPrefixes && row.coverage.postalPrefixes.length > 0 ? `邮编以 ${row.coverage.postalPrefixes.join('、')} 开头` : '全境';

function statusOf(row: NetworkVersionRecord): RowStatus | undefined {
  const code = validityOf(row.effectiveFrom, row.effectiveTo, new Date());
  return code ? codeStatus(code) : undefined;
}

const coverageCell = (row: NetworkVersionRecord) =>
  row.coverage ? (
    <span>
      <CountryLabel code={row.coverage.country} />
      <span className="text-idpxyz-breadcrumb"> · {prefixText(row)}</span>
    </span>
  ) : (
    <span className="text-idpxyz-breadcrumb">没登覆盖</span>
  );

const columnsOf = (names: PlaceNameIndex): WorkbenchColumn<NetworkVersionRecord>[] => [
  {
    key: 'area',
    header: '服务区域',
    sortKey: 'area',
    render: (row) => (
      <div className="min-w-0">
        <div className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]">{row.code ?? '—'}</div>
        <div className="text-[11px] text-idpxyz-breadcrumb">{versionWord(row.version)}</div>
      </div>
    ),
  },
  { key: 'status', header: '状态', render: (row) => { const status = statusOf(row); return status ? <RowStatusPill status={status} /> : '—'; } },
  { key: 'coverage', header: '覆盖', render: coverageCell },
  { key: 'origin', header: '揽收网点', hideWhenDetailOpen: true, render: (row) => nodesText(row.coverage?.originNodes, names) },
  { key: 'destination', header: '派送网点', hideWhenDetailOpen: true, render: (row) => nodesText(row.coverage?.destinationNodes, names) },
];

function AreaDetail({ row, names, onClose }: { row: NetworkVersionRecord; names: PlaceNameIndex; onClose: () => void }) {
  return (
    <RecordDetailPanel
      title={row.code ?? '—'}
      status={statusOf(row)}
      subtitle={`服务区域 · ${versionWord(row.version)}`}
      facts={[
        { label: '覆盖', value: coverageCell(row) },
        { label: '有效期', value: <span className="font-mono">{periodText(row)}</span> },
        { label: '揽收网点', value: nodesText(row.coverage?.originNodes, names) },
        { label: '派送网点', value: nodesText(row.coverage?.destinationNodes, names) },
      ]}
      technical={techRows([
        { label: '揽收网点编码', value: row.coverage?.originNodes?.join(', ') },
        { label: '派送网点编码', value: row.coverage?.destinationNodes?.join(', ') },
        { label: '区域编码', value: row.code },
        { label: '版本号', value: String(row.version) },
        { label: '国家代码', value: row.coverage?.country },
        { label: '邮编前缀', value: row.coverage?.postalPrefixes?.join(', ') },
        { label: '生效起点', value: row.effectiveFrom },
        { label: '截止', value: row.effectiveTo },
      ])}
      onClose={onClose}
    >
      <p>下单时按收件地址落到哪个服务区域，决定这票货能不能走、由哪个网点派送。区域改过几次就有几版，逐版列出。</p>
    </RecordDetailPanel>
  );
}

// 与网络与网点页同一读口(family=service-area),同一种接法:检索词沿地址上的 ?q= 停稳后下推,翻页走游标(ADR-0144)。
function ServiceAreasTable() {
  const [keyword, setKeyword] = useAddressKeyword();
  const conditions = networkCatalogConditions('service-area', { q: useSettledKeyword(keyword) });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listNetworkCatalog);
  const areas = answer?.kind === 'outcome' ? answer.body.versions : [];
  const mayWrite = useMayWriteRegistry();
  const names = usePlaceNames();

  return (
    <RouteWorkbench<NetworkVersionRecord>
      title={info.title}
      subtitle="每个服务区域覆盖哪个国家、哪些邮编段，由哪些网点揽收和派送。"
      answered={answer?.kind === 'outcome'}
      rows={areas}
      rowKey={(row) => `${row.code}@${row.version}`}
      statusOf={statusOf}
      searchCells={() => []}
      search={{ value: keyword, onChange: setKeyword, pushedDown: true }}
      searchPlaceholder={keywordPlaceholders['service-area']}
      columns={columnsOf(names)}
      sortAccessors={{ area: { value: (row) => row.code } }}
      defaultSort={{ key: 'area', dir: 1 }}
      renderDetail={(row, close) => <AreaDetail row={row} names={names} onClose={close} />}
      onRefresh={retry}
      pagination={pagination}
      viewState={catalogueViewState(answer, areas.length, retry, {
        module: info,
        endpoint: 'GET /network-catalog?family=service-area',
        emptyTitle: '还没有登记服务区域',
        emptyDescription: entryHint(mayWrite, '在「新增或更新」签里登记服务区域后，会在这里出现；没有服务区域之前，路线规划找不到派送范围。'),
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

/**
 * 服务区域：逐版查阅，外加登记签（ADR-0085）。服务区域族的登记签落在本页而不在网络与网点页（MCP-3 裁决③）：该族由专页
 * 承担，写签跟着读签走，登进去的结果才看得见。写入口按授予收起（ADR-0315）。
 */
export function ServiceAreasPage() {
  const mayWrite = useMayWriteRegistry();
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">服务区域</TabsTrigger>
          {mayWrite ? <TabsTrigger value="register">新增或更新</TabsTrigger> : null}
        </TabsList>
        <TabsContent value="catalog" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ServiceAreasTable />
        </TabsContent>
        {mayWrite ? (
          <TabsContent value="register" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
            <RegistrationPanel
              moduleId="service-areas"
              title={registrationTitles['service-area']}
              endpoint={`POST ${networkRegistrationEndpoints['service-area']}`}
              snapshotHint={registrationSnapshotHints['service-area']}
              submit={(snapshot) => registerNetworkCatalogVersion('service-area', snapshot)}
              outcomeLabels={registrationOutcomeLabels}
              refusalReasonLabels={registrationRefusalReasonLabels}
              problemNote={problemNote}
            />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
