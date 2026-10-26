import { useEffect, useState } from 'react';
import { TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import {
  commercialRegistrationEndpoints,
  listServiceProducts,
  productChannelOutcomeLabels,
  registerCommercial,
  type ServiceProductListResponseBody,
  type ServiceProductRecord,
} from './api';
import { nextRequest, newVersionFrom, type NewVersionRequest } from './new-version';
import { withNewVersionAction } from './NewVersionAction';
import { productKnownVersions } from './known-versions';
import { closableKeys, closureKeyOf, closureTargetOf, type ClosureTarget } from './version-closure';
import { VersionClosureDrawer } from './VersionClosureDrawer';
import { serviceProductDraftFromRecord } from './service-product-form';
import { KeepAliveTabs, KeepAliveTabsContent } from './keep-alive-tabs';
import { VersionStatusChips } from './VersionStatusChips';
import {
  groupedByObject,
  matchesVersionStatus,
  versionCountSummary,
  versionStatusCounts,
  type VersionFacts,
  type VersionStatusFilter,
} from './version-groups';
import { statusBadge } from './detail-primitives';
import {
  commercialStatusLabels,
  labelOf,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
  serviceFormLabels,
} from './presentation';
import { DeliveryConditionsCell } from './DeliveryConditionsCell';
import { ServiceProductPublicationForm } from './ServiceProductPublicationForm';

const info = moduleInfoById['service-products'];

// 只列版本壳(MCP-3 裁决⑥)外加一节可缺的产品层交付条件(票 admin-write-faces/25 裁读回折进目录行):那一节照
// ServiceProductRecord.deliveryConditions 显,键缺席即这一版没有声明、显「未声明」不显默认,页面不拿别的键去推它在不在;
// 产品—渠道映射归渠道产品目录页上列,渠道账号授权不在读面体内,不以空列伪装已实现。
const productFacts = (row: ServiceProductRecord): VersionFacts => ({
  objectId: row.objectId,
  version: row.version,
  status: row.status,
  effectiveStartsAt: row.effectiveStartsAt,
});

const columns: ListColumn<ServiceProductRecord>[] = [
  {
    id: 'product',
    header: '服务产品 / 版本',
    render: (row) => (
      <div className="min-w-48">
        <p className="font-mono font-medium text-idpxyz-text">{row.objectId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.version}</p>
      </div>
    ),
  },
  {
    id: 'form',
    header: '服务形态',
    render: (row) => (row.form ? labelOf(serviceFormLabels, row.form) : '—'),
  },
  {
    id: 'scope',
    header: '适用范围',
    className: 'font-mono text-xs',
    render: (row) => row.scope,
  },
  {
    id: 'status',
    header: '状态',
    render: (row) => statusBadge(commercialStatusLabels, row.status),
  },
  {
    id: 'delivery-conditions',
    header: '交付条件（产品层）',
    className: 'min-w-64',
    render: (row) => <DeliveryConditionsCell conditions={row.deliveryConditions} />,
  },
  {
    id: 'effective',
    header: '有效区间',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => formatRange(row.effectiveStartsAt, row.effectiveEndsAt),
  },
  {
    id: 'published-at',
    header: '发布时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.publishedAt),
  },
];

// 渠道账号授权不在本端点体内，页面不以空列伪装该类读取已经实现；产品—渠道映射自
// 票 admin-remainder-mechanism-batch/02 起有自己的页（channel-product-catalog）与
// 端点，不并进本页——那边上列登记册信封，这边上列版本壳，行形状与修订轴不同。
//
// `refreshKey` 由页面递增：「发布版本」签的载体到达发布那一步时刷本读面，让新版本在同页立刻可见。
function ServiceProductVersionsTable({
  refreshKey,
  onStartNewVersion,
}: {
  refreshKey: number;
  /** 点行上的「发新版本」：把这一版带去发布签预填（票 party-commercial-uiux/14）。 */
  onStartNewVersion: (record: ServiceProductRecord) => void;
}) {
  // 检索词在地址里（`?q=`）：别处的引用链接带着它过来，筛到那一条；刷新与换标签回来也还在。
  const [statusFilter, setStatusFilter] = useState<VersionStatusFilter>('all');
  const [search, setSearch] = useAddressKeyword();
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ServiceProductListResponseBody> | null>(null);
  // 点行上的「收尾…」：记下是哪一版，抽屉据此重读册面（票 party-commercial-uiux/16）。
  const [closing, setClosing] = useState<ClosureTarget | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listServiceProducts().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey, refreshKey]);

  const products = answer?.kind === 'outcome' ? answer.body.products : [];
  const needle = search.trim().toLowerCase();
  const searchedProducts = needle
    ? products.filter((row) =>
        [row.objectId, row.version, row.scope, row.status, row.form ?? ''].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : products;
  // 同一产品的各版排在一起、新版在上；「在用」只看服务端答的状态（票 party-commercial-uiux/08）。
  const visibleProducts = groupedByObject(
    searchedProducts.filter((row) => matchesVersionStatus(productFacts(row), statusFilter)),
    productFacts,
  );
  const retry = () => setReloadKey((value) => value + 1);
  const closable = closableKeys(answer?.kind === 'outcome' ? productKnownVersions(answer.body) : []);

  return (
    <>
      <ListPageTemplate<ServiceProductRecord>
        title={info.title}
        // 渠道账号的业务使用授权还没有读口，本页不为它留列。
        description="服务产品的各个版本与交付条件。产品对应哪些渠道，看「渠道产品目录」页。"
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索服务产品、版本或适用范围',
        }}
        filterSummary={
          // 计数只在拿到业务答案后显示,未配置态不报「0 个版本」(与 pricing 两页同一守卫)。
          answer?.kind === 'outcome' ? versionCountSummary(products.length, visibleProducts.length, '个', '版本') : undefined
        }
        filters={
          answer?.kind === 'outcome' && products.length > 0 ? (
            <VersionStatusChips
              counts={versionStatusCounts(products, productFacts)}
              value={statusFilter}
              onChange={setStatusFilter}
            />
          ) : undefined
        }
        emptyRowsNote="这组条件下没有匹配的产品版本"
        columns={withNewVersionAction(columns, {
          labelOf: (row) => `服务产品 ${row.objectId} ${row.version}`,
          onStart: onStartNewVersion,
          closure: {
            canClose: (row) => closable.has(closureKeyOf(row)),
            onClose: (row) => setClosing(closureTargetOf('SERVICE_PRODUCT', '服务产品', row)),
          },
        })}
        rows={visibleProducts}
        rowKey={(row) => `${row.objectId}@${row.version}`}
        viewState={catalogueViewState(answer, products.length, retry, {
          module: info,
          endpoint: 'GET /commercial-service-products',
          emptyTitle: '当前租户尚无服务产品版本',
          emptyDescription:
            '还没有发布任何服务产品。在「发布版本」签发布第一个版本；「登记服务形态」签只给已有版本补形态，不产生新版本。',
        })}
      />
      <VersionClosureDrawer target={closing} onClose={() => setClosing(null)} onClosed={retry} />
    </>
  );
}

/**
 * 服务产品：查阅版本壳，外加发布签与登记签（ADR-0085，票 admin-write-faces/02 切片 02c、09）。
 *
 * **「发布版本」签是本册的运营主路径**（票 09，ADR-0101 决定八逐字段表单）：版本壳四格 + 有效起止 +
 * 引用表，五步「表单 → 预览摘要 → 存为待批准 → 批准 → 发布」由公共半边的流程组件走；发布落定后本页
 * 目录读面刷新，新版本立刻可见。JSON 镜像签留在商业规则与策略页作高级口，不因主路径落地而删。
 *
 * 登记签只装服务形态一册。**服务产品版本本身不在登记签**：版本由发布口产生（发布快照的对象
 * 类别取 SERVICE_PRODUCT），那是另一个用例、另一套答案代数（已发布已生效 / 已计划生效 /
 * 发布未决），而形态是挂在一个已在册且已生效的版本上的内容。签名写死「登记服务形态」而不是
 * 「登记服务产品」，为的是让两条路的分工在签上看得见——版本从「发布版本」签来，形态从这里补。
 *
 * 形态钉在产品版本上：同一版本换个形态撞的是内容冲突，改形态要发新版本。所以本签只有登记
 * 一个动作，没有行级编辑或删除面。墙降之前它必然答 403「接入渠道未配置」，那是诚实答案；
 * 墙降当天在装配点换真 Intake 即点亮，本页一行不用改。
 */
export function ServiceProductsPage() {
  const [refreshKey, setRefreshKey] = useState(0);
  // 点产品行上的「发新版本」：记下是哪一版，切到发布签；发布表单据此预填并重建（票 party-commercial-uiux/14）。
  const [tab, setTab] = useState('versions');
  const [request, setRequest] = useState<NewVersionRequest<ServiceProductRecord> | null>(null);
  const startNewVersion = (record: ServiceProductRecord) => {
    setRequest((previous) => nextRequest(previous, record));
    setTab('publish');
  };
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <KeepAliveTabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="versions">服务产品</TabsTrigger>
          <TabsTrigger value="publish">发布版本</TabsTrigger>
          <TabsTrigger value="register">登记服务形态</TabsTrigger>
        </TabsList>
        <KeepAliveTabsContent
          value="versions"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ServiceProductVersionsTable refreshKey={refreshKey} onStartNewVersion={startNewVersion} />
        </KeepAliveTabsContent>
        <KeepAliveTabsContent
          value="publish"
          className="flex-1 flex flex-col overflow-auto data-[state=inactive]:hidden"
        >
          <ServiceProductPublicationForm
            // 同一版再发起一次、或换一版，表单整个重建，不留上一次的预览与草稿。
            key={request?.token ?? 0}
            onPublished={() => setRefreshKey((value) => value + 1)}
            start={
              request
                ? {
                    draft: serviceProductDraftFromRecord(request.source),
                    from: newVersionFrom('服务产品', request.source.objectId, request.source.version),
                  }
                : undefined
            }
          />
        </KeepAliveTabsContent>
        <KeepAliveTabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegistrationPanel
            moduleId="service-products"
            title={registrationTitles['service-product-form']}
            endpoint={`POST ${commercialRegistrationEndpoints['service-product-form']}`}
            snapshotHint={registrationSnapshotHints['service-product-form']}
            submit={(snapshot) => registerCommercial('service-product-form', snapshot)}
            // 形态与产品—渠道映射共用一份答案代数（服务端交回同一个 ProductChannelOutcome）。
            outcomeLabels={productChannelOutcomeLabels}
            problemNote={problemNote}
          />
        </KeepAliveTabsContent>
      </KeepAliveTabs>
    </div>
  );
}
