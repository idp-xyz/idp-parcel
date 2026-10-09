import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, useAddressKeyword, type ListColumn } from '../../templates';
import { RegistrationPanel } from '../../components/registration';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatRange } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
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

const info = moduleInfoById['service-areas'];

// 本轮只读 0008 的服务区域版本骨架(spec「明确不做」与 MCP-3 裁决④):地理覆盖
// (包含/排除区域)属 0007 network_definition 登记册,该册尚无写入方(PAR-NET-14),
// 覆盖列尚不存在——页面如实说明,不为它发请求、不虚构列。
const columns: ListColumn<NetworkVersionRecord>[] = [
  {
    id: 'area',
    header: '服务区域 / 版本',
    render: (row) => (
      <div className="min-w-48">
        <p className="font-mono font-medium text-idpxyz-text">{row.code ?? '—'}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">v{row.version}</p>
      </div>
    ),
  },
  {
    id: 'effective',
    header: '适用区间',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => (row.effectiveFrom ? formatRange(row.effectiveFrom, row.effectiveTo) : '—'),
  },
  {
    id: 'coverage',
    header: '地理覆盖',
    className: 'min-w-64 text-xs text-idpxyz-textMuted',
    render: () => '尚不存在——0007 登记册无写入方(PAR-NET-14),本页不虚构覆盖关系',
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
      description={`${info.owner}——版本骨架查阅;地理覆盖列尚不存在(PAR-NET-14),页面不虚构覆盖关系`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: keywordPlaceholders['service-area'],
      }}
      filterSummary={
        // 同 NetworkCatalogPage:无业务答案不报数,免与未配置态说明打架。
        answer?.kind === 'outcome' ? `本页 ${areas.length} 个版本` : undefined
      }
      columns={columns}
      rows={areas}
      rowKey={(row) => `${row.code}@${row.version}`}
      emptyRowsNote="当前检索条件下没有匹配的服务区域版本"
      pagination={pagination}
      viewState={catalogueViewState(answer, areas.length, retry, {
        module: info,
        endpoint: 'GET /network-catalog?family=service-area',
        emptyTitle: '当前租户尚无服务区域版本',
        emptyDescription: '读取入口已配置,但服务区域目录为空;页面不会虚构区域与覆盖关系。',
        narrowed: catalogueConditionsNarrow(conditions),
      })}
    />
  );
}

/**
 * 服务区域:版本骨架查阅,外加登记签(ADR-0085,票 admin-write-faces/02 切片 02a)。
 *
 * 服务区域族的登记签落在本页而不在网络目录页,判据与读面同一条(MCP-3 裁决③):该族由
 * 专页承担,写签跟着读签走,登进去的结果才看得见。
 *
 * 登的仍是 0008 的版本骨架:地理覆盖属 0007 登记册,该册尚无写入方(PAR-NET-14)——本签
 * 因此收不了覆盖关系,快照里也没有那几格,页面不为它造字段。
 */
export function ServiceAreasPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="catalog" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="catalog">服务区域</TabsTrigger>
          <TabsTrigger value="register">登记服务区域</TabsTrigger>
        </TabsList>
        <TabsContent
          value="catalog"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ServiceAreasTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
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
      </Tabs>
    </div>
  );
}
