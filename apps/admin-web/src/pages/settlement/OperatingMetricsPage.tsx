import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listSettlementOperatingResults,
  type CostAllocationListResponseBody,
  type CostAllocationRecord,
  type OperatingResultListResponseBody,
  type OperatingResultRecord,
} from './api';
import { componentEffectLabels, labelOf, operatingBasisLabels } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['operating-metrics'];

// 本页两签，都接 GET /settlement-operating-results 按 registry 分派。成本分摊在本页而不在
// 费用页：两册同属 UC-SA-006（分摊与指标由同一个用例形成、经同一个 OperatingIntent 交下游），
// 而分摊只改变经营归因、不转移原债权债务责任——摆进费用页会让读者以为它动了应收应付。
//
// 旧骨架的「客户侧采用／供应商侧采用／其中：审核应付／其中：供应商费用贷项」四栏随对栏裁定
// 撤下，改为组成逐项呈现：组成元素上只有来源、增减向与金额三个键，**没有角色维**。要填那
// 四栏，读面得先判某个来源属于客户侧还是供应商侧、是不是审核应付、是不是贷项——而那正是
// CONTEXT 硬要求「审核应付与贷项按各自借贷方向分别计入一次、不得净含贷项」想让人看见的
// 东西，由读面代贴标签等于把它重新藏起来。
//
// 「经营损失」栏一并撤下：它与经营毛利是同一个数按正负分两栏，会让「零」落进两栏都不占的
// 缝里。毛利一栏带符号呈现，负值即经营损失。

// —— 经营结果快照签 ——

const operatingResultColumns: ListColumn<OperatingResultRecord>[] = [
  { id: 'scope', header: '分析范围', className: 'font-mono text-xs', render: (row) => row.scope },
  { id: 'period', header: '账期', className: 'font-mono text-xs', render: (row) => row.period },
  {
    id: 'basis',
    header: '口径',
    align: 'center',
    className: 'w-[72px]',
    // 封闭三格。索赔调整后口径必须声明所依附的基础口径，属编排层组合，不是第四格。
    render: (row) => labelOf(operatingBasisLabels, row.basis),
  },
  { id: 'version', header: '计算版本', className: 'font-mono text-xs', render: (row) => row.version },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[64px] font-mono', render: (row) => row.currency },
  {
    id: 'margin',
    header: '经营毛利',
    align: 'right',
    className: 'font-mono text-xs',
    // 带符号照实呈现，负值即经营损失。不重算：派生它的门在写口（重建时复验组成与毛利
    // 是否相符），读侧再算一遍就成了第二处定义，两处一旦不一致，页面上看到的会是没人验过的那个。
    render: (row) => row.margin,
  },
  {
    id: 'components',
    header: '组成（来源／增减向／金额）',
    // 逐项呈现而不按角色归栏（理由见文件头）。空数组是「这份快照没有组成项」的如实一格，
    // 不留白——留白读起来像渲染掉了东西。
    render: (row) =>
      row.components.length > 0 ? (
        <div className="min-w-56 font-mono text-xs">
          {row.components.map((component) => (
            <p key={`${component.source}:${component.effect}:${component.amount}`}>
              {component.source}／{labelOf(componentEffectLabels, component.effect)}／
              {component.amount}
            </p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">无组成项</span>
      ),
  },
  {
    id: 'as-of',
    header: '截至时点',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.asOf),
  },
  {
    id: 'corrects',
    header: '纠错回指',
    className: 'font-mono text-xs',
    // 缺席即这不是一次纠错重算，不是「未登记」——重算形成新版本，原快照保留。
    render: (row) => row.corrects ?? <span className="text-idpxyz-textMuted">非纠错</span>,
  },
  {
    id: 'recorded-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.recordedAt),
  },
];

function OperatingResultsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<OperatingResultListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementOperatingResults('operating-result').then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const results = answer?.kind === 'outcome' ? answer.body.results : [];
  const needle = search.trim().toLowerCase();
  const visibleResults = needle
    ? results.filter((row) =>
        [row.scope, row.period, row.version].some((value) => value.toLowerCase().includes(needle)),
      )
    : results;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<OperatingResultRecord>
      title="经营结果快照"
      description={`${info.owner}——毛利只能派生不能直接修改；组成按各自借贷方向逐项呈现，不净含贷项`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索分析范围 / 账期 / 计算版本',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `当前返回 ${results.length} 份快照` : undefined
      }
      columns={operatingResultColumns}
      rows={visibleResults}
      // 行键循口径三件加计算版本：同一范围同一账期下各口径各版本分别成行。
      rowKey={(row) => [row.scope, row.period, row.basis, row.version].join(':')}
      viewState={catalogueViewState(answer, results.length, retry, {
        module: info,
        endpoint: 'GET /settlement-operating-results?registry=operating-result',
        emptyTitle: '当前租户尚无已登记的经营结果快照',
        emptyDescription:
          '读取入口已配置，但经营结果登记册为空；快照由 UC-SA-006 按口径派生，其来源费用与应付的事务链在接入渠道墙后面。页面不会为好看造任何毛利或损失数字。',
      })}
    />
  );
}

// —— 成本分摊签 ——

const costAllocationColumns: ListColumn<CostAllocationRecord>[] = [
  { id: 'allocation', header: '分摊标识', className: 'font-mono text-xs', render: (row) => row.allocation },
  { id: 'source', header: '来源金额身份', className: 'font-mono text-xs', render: (row) => row.source },
  {
    id: 'source-amount',
    header: '来源金额',
    align: 'right',
    className: 'font-mono text-xs',
    render: (row) => `${row.sourceAmount} ${row.currency}`,
  },
  { id: 'rule', header: '规则版本', className: 'font-mono text-xs', render: (row) => row.rule },
  {
    id: 'portions',
    header: '份额（分析对象／金额）',
    // 空数组即「全额未分摊」——与未分摊余额栏互相印证，不折进份额里凑平。
    render: (row) =>
      row.portions.length > 0 ? (
        <div className="min-w-48 font-mono text-xs">
          {row.portions.map((portion) => (
            <p key={`${portion.target}:${portion.amount}`}>
              {portion.target}／{portion.amount}
            </p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">全额未分摊</span>
      ),
  },
  {
    id: 'unallocated',
    header: '未分摊余额',
    align: 'right',
    className: 'font-mono text-xs',
    // 单独成栏：已分摊与未分摊之和严格等于来源金额是写口的不变量，未分摊余额是第一类结果
    // 不是尾差——没有合格对象或分母为零时来源金额整笔留在这里等新依据。
    render: (row) => row.unallocatedAmount,
  },
  { id: 'version', header: '版本', className: 'font-mono text-xs', render: (row) => row.version },
  {
    id: 'corrects',
    header: '纠错回指',
    className: 'font-mono text-xs',
    render: (row) => row.corrects ?? <span className="text-idpxyz-textMuted">非纠错</span>,
  },
  {
    id: 'allocated-at',
    header: '分摊时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.allocatedAt),
  },
  {
    id: 'recorded-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.recordedAt),
  },
];

function CostAllocationsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CostAllocationListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listSettlementOperatingResults('cost-allocation').then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const allocations = answer?.kind === 'outcome' ? answer.body.allocations : [];
  const needle = search.trim().toLowerCase();
  const visibleAllocations = needle
    ? allocations.filter((row) =>
        [row.allocation, row.source, row.rule].some((value) => value.toLowerCase().includes(needle)),
      )
    : allocations;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CostAllocationRecord>
      title="成本分摊"
      description={`${info.owner}——分摊只改变经营归因，不转移原债权债务责任；未分摊余额是第一类结果不是尾差`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索分摊标识 / 来源金额身份 / 规则版本',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `当前返回 ${allocations.length} 次分摊` : undefined
      }
      columns={costAllocationColumns}
      rows={visibleAllocations}
      // 行键循分摊标识加版本：纠错重算换版本，原分摊保留。
      rowKey={(row) => `${row.allocation}:${row.version}`}
      viewState={catalogueViewState(answer, allocations.length, retry, {
        module: info,
        endpoint: 'GET /settlement-operating-results?registry=cost-allocation',
        emptyTitle: '当前租户尚无已登记的成本分摊',
        emptyDescription:
          '读取入口已配置，但成本分摊登记册为空；分摊由 UC-SA-006 依规则版本形成，其来源成本的事务链在接入渠道墙后面。',
      })}
    />
  );
}

/**
 * 经营核算（settlement-accounting）。两册各自成签：经营结果快照（按口径、计算版本、币种与
 * 截至时点派生的指标）与成本分摊（经营归因）。
 *
 * 指标只能派生不能直接修改，本页是查阅面，不设任何指标编辑动作。
 */
export function OperatingMetricsPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="results" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="results">经营结果快照</TabsTrigger>
          <TabsTrigger value="allocations">成本分摊</TabsTrigger>
        </TabsList>
        <TabsContent value="results" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <OperatingResultsTable />
        </TabsContent>
        <TabsContent value="allocations" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CostAllocationsTable />
        </TabsContent>
      </Tabs>
    </div>
  );
}
