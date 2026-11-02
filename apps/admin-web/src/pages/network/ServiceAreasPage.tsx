import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, useAddressKeyword, type ListColumn } from '../../templates';
import { RegistrationPanel } from '../../components/registration';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { CountryLabel } from '../refdata-labels';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatRange } from '../catalogue-view';
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
  type NetworkVersionRecord,
} from './api';
import { keywordPlaceholders, problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { validityOf, versionWord } from './network-rows';

const info = moduleInfoById['service-areas'];

// 列只放运营认的：哪个区域、覆盖哪个国家与邮编段、由哪些网点揽收与派送、有效期、此刻在不在期。版本号进技术详情。
// 覆盖缺席是这一版没登覆盖（老版本或只登了骨架），如实说「没登覆盖」，不当成「覆盖全部」。
const nodesText = (nodes: string[] | undefined) => (nodes && nodes.length > 0 ? nodes.join('、') : '—');

const columns: ListColumn<NetworkVersionRecord>[] = [
  {
    id: 'area',
    header: '服务区域',
    render: (row) => (
      <div className="min-w-0">
        <div className="truncate font-mono text-[12px] font-medium text-idpxyz-textBright">{row.code ?? '—'}</div>
        <div className="text-[11px] text-idpxyz-textMuted">{versionWord(row.version)}</div>
      </div>
    ),
  },
  {
    id: 'coverage',
    header: '覆盖',
    render: (row) =>
      row.coverage ? (
        <div className="min-w-0">
          <div className="text-idpxyz-textBright">
            <CountryLabel code={row.coverage.country} />
          </div>
          <div className="text-[11px] text-idpxyz-textMuted">
            {row.coverage.postalPrefixes && row.coverage.postalPrefixes.length > 0
              ? `邮编以 ${row.coverage.postalPrefixes.join('、')} 开头`
              : '全境'}
          </div>
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">没登覆盖</span>
      ),
  },
  { id: 'origin', header: '揽收网点', render: (row) => <span className="font-mono text-[12px]">{nodesText(row.coverage?.originNodes)}</span> },
  { id: 'destination', header: '派送网点', render: (row) => <span className="font-mono text-[12px]">{nodesText(row.coverage?.destinationNodes)}</span> },
  {
    id: 'period',
    header: '有效期',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => (row.effectiveFrom ? formatRange(row.effectiveFrom, row.effectiveTo) : '—'),
  },
  {
    id: 'validity',
    header: '状态',
    align: 'center',
    className: 'w-[96px]',
    render: (row) => {
      const code = validityOf(row.effectiveFrom, row.effectiveTo, new Date());
      return code ? <StatusPill code={code} /> : '—';
    },
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => (
      <TechnicalDetails
        rows={technicalRows([
          { label: '版本号', value: String(row.version) },
          { label: '国家代码', value: row.coverage?.country },
        ])}
        className="max-w-md open:min-w-[18rem]"
      />
    ),
  },
];

// 与网络目录页同一读口(family=service-area),同一种接法:检索词沿地址上的 ?q= 停稳后下推,翻页走游标,换检索词回第一页
// (ADR-0144,票 catalogue-read-pagination/03、05);次序是读口本族的缺省序,不在答回来的那一页上再筛。
function ServiceAreasTable() {
  const [keyword, setKeyword] = useAddressKeyword();
  const conditions = networkCatalogConditions('service-area', { q: useSettledKeyword(keyword) });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listNetworkCatalog);
  const areas = answer?.kind === 'outcome' ? answer.body.versions : [];

  return (
    <ListPageTemplate<NetworkVersionRecord>
      title={info.title}
      description="每个服务区域覆盖哪个国家、哪些邮编段，由哪些网点揽收和派送。下单时按收件地址落到哪个区域，决定这票货能不能走、走哪条路线。"
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: keywordPlaceholders['service-area'],
      }}
      filterSummary={
        // 同 NetworkCatalogPage:无业务答案不报数,免与未配置态说明打架。
        answer?.kind === 'outcome' ? `本页 ${areas.length} 条` : undefined
      }
      columns={columns}
      rows={areas}
      rowKey={(row) => `${row.code}@${row.version}`}
      emptyRowsNote="没有符合条件的服务区域；换个关键词试试"
      pagination={pagination}
      viewState={catalogueViewState(answer, areas.length, retry, {
        module: info,
        endpoint: 'GET /network-catalog?family=service-area',
        emptyTitle: '还没有登记服务区域',
        emptyDescription: '在「新增或更新」签里登记服务区域后，会在这里出现；没有服务区域之前，路线规划找不到派送范围。',
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
