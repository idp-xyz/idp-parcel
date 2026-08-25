import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatRange } from '../catalogue-view';
import {
  listServiceAreas,
  type ServiceAreaVersionRecord,
  type ServiceAreaVersionsResponseBody,
} from './api';

const info = moduleInfoById['service-areas'];

const columns: ListColumn<ServiceAreaVersionRecord>[] = [
  {
    id: 'area',
    header: '服务区域 / 版本',
    render: (row) => (
      <div className="min-w-48">
        <p className="font-mono font-medium text-idpxyz-text">{row.code}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.version}</p>
      </div>
    ),
  },
  {
    id: 'included-regions',
    header: '包含区域',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => row.includedRegions.join('、'),
  },
  {
    id: 'excluded-regions',
    header: '排除区域',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => row.excludedRegions.join('、') || '—',
  },
  {
    id: 'effective',
    header: '适用区间',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => formatRange(row.effectiveFrom, row.effectiveTo),
  },
];

export function ServiceAreasPage() {
  const [keyword, setKeyword] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<ServiceAreaVersionsResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listServiceAreas().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const areas = answer?.kind === 'outcome' ? answer.body.versions : [];
  const needle = keyword.trim().toLowerCase();
  const visibleAreas = needle
    ? areas.filter((row) =>
        [row.code, ...row.includedRegions, ...row.excludedRegions].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : areas;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<ServiceAreaVersionRecord>
      title={info.title}
      description={`${info.owner}——服务区域不取得客户地址所有权，也不直接证明逻辑可达。`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按区域代码或地理范围检索',
      }}
      filterSummary={`当前返回 ${areas.length} 个版本`}
      columns={columns}
      rows={visibleAreas}
      rowKey={(row) => `${row.code}@${row.version}`}
      viewState={catalogueViewState(answer, areas.length, retry, {
        module: info,
        endpoint: 'GET /network-catalog?family=service-area',
        emptyTitle: '当前租户尚无服务区域版本',
        emptyDescription: '读取入口已配置，但服务区域目录为空；页面不会虚构区域与节点关系。',
      })}
    />
  );
}
