import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listSupplierAgreements,
  type SupplierAgreementListResponseBody,
  type SupplierAgreementRecord,
} from './api';
import { commercialStatusLabels, labelOf } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['supplier-agreements'];

// 只列版本壳。骨架期这里还列过供应商、采购服务范围、采购价格条件与结算条件——领域的
// SupplierAgreement 确实携这些，但今天没有对应的正文表可读（后端
// ports.SupplierAgreementCatalogueRow 记着这条），因此四列都不上：缺的是登记面，
// 不是转写。正文表落库后在读面上扩字段，那时才谈得上列它们。
const columns: ListColumn<SupplierAgreementRecord>[] = [
  {
    id: 'agreement',
    header: '协议 / 版本',
    render: (row) => (
      <div className="min-w-48">
        <p className="font-mono font-medium text-idpxyz-text">{row.objectId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.version}</p>
      </div>
    ),
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

/**
 * 供应商协议（party-commercial）。行对象是供应商商业协议版本：它定义可复用的
 * 采购条件、价格和结算责任，不等于一次实际运输委托、订舱、履约事实或供应商
 * 账单——实际委托快照归 transport-fulfillment，成本与应付归 settlement-accounting。
 * 查阅面，不设登记与发布动作。
 */
export function SupplierAgreementsPage() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<SupplierAgreementListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSupplierAgreements().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const agreements = answer?.kind === 'outcome' ? answer.body.agreements : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleAgreements = needle
    ? agreements.filter((row) =>
        [row.objectId, row.version, row.scope, row.status].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : agreements;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<SupplierAgreementRecord>
      title={info.title}
      description={`${info.owner}——当前读面只展示协议版本壳，供应商、采购价格条件与结算条件尚无正文册可读，不上列`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索协议、版本或适用范围',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区
        // 「这不是目录为空」直接矛盾（README 列表页上列通则第六条）。
        answer?.kind === 'outcome' ? `当前返回 ${agreements.length} 份协议版本` : undefined
      }
      columns={columns}
      rows={visibleAgreements}
      rowKey={(row) => `${row.objectId}@${row.version}`}
      viewState={catalogueViewState(answer, agreements.length, retry, {
        module: info,
        endpoint: 'GET /commercial-supplier-agreements',
        emptyTitle: '当前租户尚无供应商协议版本',
        emptyDescription: '读取入口已配置，但目录为空；页面不会预置供应商或采购条件。',
      })}
    />
  );
}
