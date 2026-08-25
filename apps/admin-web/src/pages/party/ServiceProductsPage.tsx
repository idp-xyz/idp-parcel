import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listServiceProducts,
  type ServiceProductListResponseBody,
  type ServiceProductRecord,
} from './api';
import { commercialStatusLabels, labelOf, serviceFormLabels } from './presentation';

const info = moduleInfoById['service-products'];

// 只列版本壳(MCP-3 裁决⑥):读面交回的就是 service_product_form 的版本行;
// 产品—渠道映射与渠道账号授权不在读面体内,不上列、由页面说明,不以空列伪装已实现。
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

// 产品—渠道映射和渠道账号授权不在本端点体内;页面不以空列伪装这两类读取已经实现。
export function ServiceProductsPage() {
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
      description={`${info.owner}——当前读面只展示服务产品版本壳,渠道映射与账号授权读取尚未建立,不上列`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索服务产品、版本或适用范围',
      }}
      filterSummary={`当前返回 ${products.length} 个版本`}
      columns={columns}
      rows={visibleProducts}
      rowKey={(row) => `${row.objectId}@${row.version}`}
      viewState={catalogueViewState(answer, products.length, retry, {
        module: info,
        endpoint: 'GET /commercial-service-products',
        emptyTitle: '当前租户尚无服务产品版本',
        emptyDescription: '读取入口已配置,但目录为空;页面不会预置服务产品或渠道映射。',
      })}
    />
  );
}
