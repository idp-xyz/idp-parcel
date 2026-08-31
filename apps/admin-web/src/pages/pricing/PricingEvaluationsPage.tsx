import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listPricingEvaluations,
  type PricingEvaluationListResponseBody,
  type PricingEvaluationRecord,
} from './api';
import { evaluationStatusLabels, labelOf } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['pricing-evaluation'];

// 本页接 GET /pricing-evaluations（票 admin-skeleton-closure-batch/03 阶段二）。
// 上列的是评价登记册的检索列面——评价是业务事实不是主数据，册上只有引用、
// 状态与比对列；语义细节住在评价快照（jsonb）内，属详情读法，端点不透出。
//
// 旧骨架各栏随对栏裁定处置（判据同结算申请页：目录事实只有本册登了什么）：
//   评价对象、价格方向、计算目的、计价基准时点、版本清单——**全撤**。五者都住在
//     快照内，检索列面上册级缺席；留栏就是请页面去拆 jsonb 自造第二个权威读法。
//   结算币种金额——**全撤且不换栏**。CONTEXT 硬句「不得以金额为零的已完成评价表达
//     不可计价」；列面不透出金额，连用 0 顶替的机会也不给。金额归详情读法。
//   加两栏：语义摘要与计划内容摘要（ADR-0014 的双摘要比对列，争议复核按此对账）、
//     规范化版本。摘要是 64 位十六进制，照登全文不截断——比对列截半就不能比对。
//   分页——**撤**。端点无分页参数，一次交回注入上限内的行（与代收分户账页同款）。
const columns: ListColumn<PricingEvaluationRecord>[] = [
  {
    id: 'evaluation-id',
    header: '评价引用',
    className: 'font-mono text-xs',
    render: (row) => <span className="text-idpxyz-accent">{row.evaluationId}</span>,
  },
  {
    id: 'status',
    header: '结果',
    align: 'center',
    className: 'w-[88px]',
    // 封闭五格词表转写；集外取值原样显示，不代折成已知格。
    render: (row) => labelOf(evaluationStatusLabels, row.status),
  },
  {
    id: 'semantic-digest',
    header: '语义摘要',
    className: 'font-mono text-xs min-w-64',
    render: (row) => row.semanticDigest,
  },
  {
    id: 'plan-digest',
    header: '计划内容摘要',
    className: 'font-mono text-xs min-w-64',
    render: (row) => row.planContentDigest,
  },
  {
    id: 'canonicalization',
    header: '规范化版本',
    align: 'center',
    className: 'w-[104px] font-mono text-xs',
    render: (row) => row.canonicalization,
  },
  {
    id: 'recorded-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.recordedAt),
  },
];

/**
 * 价格评价：已保存评价的登记册查阅面，服务争议复核与回放场景的检索一步。
 * 页面没有「重算 / 修改」动作——已完成评价不可变；回放属「发起一次新评价」，
 * 入口归渠道墙后的评价编排，不在查阅面上。
 */
export function PricingEvaluationsPage() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<PricingEvaluationListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listPricingEvaluations().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const evaluations = answer?.kind === 'outcome' ? answer.body.evaluations : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的行上做，不下推成查询参数——那要改端点契约。可检索字段即列面
  // 的引用与两条摘要（比对场景手里拿的就是这三种串）。
  const visibleEvaluations = needle
    ? evaluations.filter((row) =>
        [row.evaluationId, row.semanticDigest, row.planContentDigest].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : evaluations;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<PricingEvaluationRecord>
      title={info.title}
      description={`${info.owner}——登记册检索列面；语义细节与金额住在评价快照内，属详情读法`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索评价引用 / 语义摘要 / 计划内容摘要',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 条」会与状态区
        // 「这不是登记册为空」直接矛盾（README 列表页上列通则第六条）。
        answer?.kind === 'outcome' ? `当前返回 ${evaluations.length} 条评价` : undefined
      }
      columns={columns}
      rows={visibleEvaluations}
      rowKey={(row) => row.evaluationId}
      viewState={catalogueViewState(answer, evaluations.length, retry, {
        module: info,
        endpoint: 'GET /pricing-evaluations',
        emptyTitle: '当前租户尚无已登记的价格评价',
        emptyDescription:
          '读取入口已配置，但评价登记册为空；评价的写入方是渠道墙后的评价编排用例，尚未接入任何进程——册空是墙拦不是缺陷，本页不预置数据。',
      })}
    />
  );
}
