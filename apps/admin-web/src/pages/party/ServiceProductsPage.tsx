import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { RegistrationPanel } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  commercialRegistrationEndpoints,
  listServiceProducts,
  productChannelOutcomeLabels,
  registerCommercial,
  type ServiceProductListResponseBody,
  type ServiceProductRecord,
} from './api';
import {
  commercialStatusLabels,
  labelOf,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
  serviceFormLabels,
} from './presentation';

const info = moduleInfoById['service-products'];

// 只列版本壳(MCP-3 裁决⑥):读面交回的就是 service_product_form 的版本行;
// 产品—渠道映射归渠道产品目录页上列,渠道账号授权不在读面体内,不以空列伪装已实现。
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
    header: '生命周期状态',
    render: (row) => labelOf(commercialStatusLabels, row.status),
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
function ServiceProductVersionsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ServiceProductListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listServiceProducts().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const products = answer?.kind === 'outcome' ? answer.body.products : [];
  const needle = search.trim().toLowerCase();
  const visibleProducts = needle
    ? products.filter((row) =>
        [row.objectId, row.version, row.scope, row.status, row.form ?? ''].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : products;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<ServiceProductRecord>
      title={info.title}
      description={`${info.owner}——本页上列服务产品版本壳,产品—渠道映射在渠道产品目录页,账号授权读取尚未建立`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索服务产品、版本或适用范围',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示,未配置态不报「0 个版本」(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome' ? `当前返回 ${products.length} 个版本` : undefined
      }
      columns={columns}
      rows={visibleProducts}
      rowKey={(row) => `${row.objectId}@${row.version}`}
      viewState={catalogueViewState(answer, products.length, retry, {
        module: info,
        endpoint: 'GET /commercial-service-products',
        emptyTitle: '当前租户尚无服务产品版本',
        emptyDescription:
          '读取入口已配置,但目录为空;页面不会预置服务产品或渠道映射。本页「登记服务形态」签补的是既有版本的形态,不产生版本本身——版本来自发布口。',
      })}
    />
  );
}

/**
 * 服务产品：查阅版本壳，外加登记签（ADR-0085，票 admin-write-faces/02 切片 02c）。
 *
 * 登记签只装服务形态一册。**服务产品版本本身不在本签**：版本由发布口产生（发布快照的对象
 * 类别取 SERVICE_PRODUCT），那是另一个用例、另一套答案代数（已发布已生效 / 已计划生效 /
 * 发布未决），而形态是挂在一个已在册且已生效的版本上的内容。签名写死「登记服务形态」而不是
 * 「登记服务产品」，为的是让这一半的缺席在签上看得见——否则下一个人会以为产品版本也能从
 * 这里发。
 *
 * 形态钉在产品版本上：同一版本换个形态撞的是内容冲突，改形态要发新版本。所以本签只有登记
 * 一个动作，没有行级编辑或删除面。墙降之前它必然答 403「接入渠道未配置」，那是诚实答案；
 * 墙降当天在装配点换真 Intake 即点亮，本页一行不用改。
 */
export function ServiceProductsPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="versions" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="versions">服务产品</TabsTrigger>
          <TabsTrigger value="register">登记服务形态</TabsTrigger>
        </TabsList>
        <TabsContent
          value="versions"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <ServiceProductVersionsTable />
        </TabsContent>
        <TabsContent
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
        </TabsContent>
      </Tabs>
    </div>
  );
}
