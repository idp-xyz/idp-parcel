import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listSettlementOperatingResults,
  type CostAllocationListResponseBody,
  type CostAllocationRecord,
  type OperatingResultListResponseBody,
  type OperatingResultRecord,
} from './api';
import { formatMoney, technicalRows } from './ops-ui';

// 经营核算页（主责上下文 settlement-accounting，UC-SA-006 按口径、版本、币种与截至时点派生经营毛利）。两签都接
// GET /settlement-operating-results 按 registry 分派。成本分摊在本页而不在费用页：分摊只改变经营归因、不转移原债权债务责任。
const info = moduleInfoById['operating-metrics'];

// 组成项逐项呈现而不按角色合计成四栏：求和这一层放在页面就成了第二处定义，而「审核应付与贷项按各自借贷方向分别计入
// 一次、不得净含贷项」要人看见的恰恰是逐项。毛利一栏带符号呈现，负值即经营损失，不另设「损失」栏（「零」会落进两栏都
// 不占的缝里）。

// —— 经营结果签 ——

function resultTechnicalRows(row: OperatingResultRecord) {
  return technicalRows([
    { label: '分析范围', value: row.scope },
    { label: '账期', value: row.period },
    { label: '计算版本', value: row.version },
    { label: '纠错自', value: row.corrects },
    { label: '登记时间', value: formatInstant(row.recordedAt) },
  ]);
}

const operatingResultColumns: ListColumn<OperatingResultRecord>[] = [
  { id: 'scope', header: '分析范围', className: 'font-mono text-xs text-idpxyz-textBright', render: (row) => row.scope },
  { id: 'period', header: '账期', className: 'font-mono text-xs', render: (row) => row.period },
  {
    id: 'basis',
    header: '口径',
    align: 'center',
    // 封闭三格。索赔调整后口径必须声明所依附的基础口径，属编排层组合，不是第四格。
    render: (row) => <StatusPill code={row.basis} />,
  },
  {
    id: 'margin',
    header: '毛利',
    align: 'right',
    className: 'font-mono text-xs tabular-nums',
    // 带符号照实呈现，负值即经营损失。不重算：派生它的门在写口，读侧再算一遍就成了第二处定义。
    render: (row) => (
      <span className={row.margin.startsWith('-') ? 'text-[color:var(--ds-status-danger-fg)]' : 'text-idpxyz-textBright'}>
        {formatMoney(row.margin, row.currency)}
      </span>
    ),
  },
  {
    id: 'components',
    header: '组成',
    // 角色照册转写，不是页面判出来的。空数组是「这份快照没有组成项」的如实一格，不留白。
    render: (row) =>
      row.components.length > 0 ? (
        <div className="min-w-64 text-xs">
          {row.components.map((component) => (
            <p key={`${component.role}:${component.source}:${component.effect}:${component.amount}`} className="whitespace-nowrap">
              {labelFor(component.role)}
              <span className="text-idpxyz-textMuted">（{labelFor(component.effect)}）</span> {formatMoney(component.amount, '')}
              <span className="ml-1 font-mono text-[11px] text-idpxyz-textMuted">{component.source}</span>
            </p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">无组成项</span>
      ),
  },
  {
    id: 'as-of',
    header: '截至',
    className: 'whitespace-nowrap font-mono text-xs',
    render: (row) => formatInstant(row.asOf),
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={resultTechnicalRows(row)} className="max-w-md open:min-w-[18rem]" />,
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
    ? results.filter((row) => [row.scope, row.period, row.version].some((value) => value.toLowerCase().includes(needle)))
    : results;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<OperatingResultRecord>
      title="经营结果"
      description="按范围和账期算出的毛利，分预估、已确认、已结算三种口径"
      search={{ value: search, onChange: setSearch, placeholder: '搜分析范围、账期' }}
      filterSummary={answer?.kind === 'outcome' ? `${results.length} 份快照` : undefined}
      columns={operatingResultColumns}
      rows={visibleResults}
      // 行键循口径三件加计算版本：同一范围同一账期下各口径各版本分别成行。
      rowKey={(row) => [row.scope, row.period, row.basis, row.version].join(':')}
      emptyRowsNote="没有匹配的快照"
      viewState={catalogueViewState(answer, results.length, retry, {
        module: info,
        endpoint: 'GET /settlement-operating-results?registry=operating-result',
        emptyTitle: '还没有经营结果',
        emptyDescription: '费用确认、账单审核之后系统按口径算出毛利，这里会随之出现；页面不会为好看造任何毛利或损失数字。',
      })}
    />
  );
}

// —— 成本分摊签 ——

function allocationTechnicalRows(row: CostAllocationRecord) {
  return technicalRows([
    { label: '分摊', value: row.allocation },
    { label: '来源金额', value: row.source },
    { label: '规则版本', value: row.rule },
    { label: '版本', value: row.version },
    { label: '纠错自', value: row.corrects },
    { label: '登记时间', value: formatInstant(row.recordedAt) },
  ]);
}

const costAllocationColumns: ListColumn<CostAllocationRecord>[] = [
  { id: 'source', header: '来源成本', className: 'font-mono text-xs text-idpxyz-textBright', render: (row) => row.source },
  {
    id: 'source-amount',
    header: '金额',
    align: 'right',
    className: 'font-mono text-xs tabular-nums',
    render: (row) => formatMoney(row.sourceAmount, row.currency),
  },
  {
    id: 'portions',
    header: '分到哪里',
    // 空数组即「全额未分摊」——与未分摊余额栏互相印证，不折进份额里凑平。
    render: (row) =>
      row.portions.length > 0 ? (
        <div className="min-w-48 text-xs">
          {row.portions.map((portion) => (
            <p key={`${portion.target}:${portion.amount}`} className="whitespace-nowrap">
              <span className="font-mono">{portion.target}</span> {formatMoney(portion.amount, '')}
            </p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">全额未分摊</span>
      ),
  },
  {
    id: 'unallocated',
    header: '未分摊',
    align: 'right',
    className: 'font-mono text-xs tabular-nums',
    // 单独成栏：未分摊余额是第一类结果不是尾差——没有合格对象或分母为零时来源金额整笔留在这里等新依据。
    render: (row) => formatMoney(row.unallocatedAmount, ''),
  },
  {
    id: 'allocated-at',
    header: '分摊时间',
    className: 'whitespace-nowrap font-mono text-xs',
    render: (row) => formatInstant(row.allocatedAt),
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={allocationTechnicalRows(row)} className="max-w-md open:min-w-[18rem]" />,
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
    ? allocations.filter((row) => [row.allocation, row.source, row.rule].some((value) => value.toLowerCase().includes(needle)))
    : allocations;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CostAllocationRecord>
      title="成本分摊"
      description="把一笔成本按规则分到各个分析对象上；只改经营归因，不改谁欠谁的钱"
      search={{ value: search, onChange: setSearch, placeholder: '搜来源成本、规则' }}
      filterSummary={answer?.kind === 'outcome' ? `${allocations.length} 次分摊` : undefined}
      columns={costAllocationColumns}
      rows={visibleAllocations}
      // 行键循分摊标识加版本：纠错重算换版本，原分摊保留。
      rowKey={(row) => `${row.allocation}:${row.version}`}
      emptyRowsNote="没有匹配的分摊"
      viewState={catalogueViewState(answer, allocations.length, retry, {
        module: info,
        endpoint: 'GET /settlement-operating-results?registry=cost-allocation',
        emptyTitle: '还没有成本分摊',
        emptyDescription: '分摊规则登记后系统按规则分摊成本，这里会随之出现；页面不会预置分摊。',
      })}
    />
  );
}

/**
 * 经营核算。两册各自成签：经营结果快照（按口径、计算版本、币种与截至时点派生的指标）与成本分摊（经营归因）。
 * 指标只能派生不能直接修改，本页是查阅面，不设任何指标编辑动作。
 */
export function OperatingMetricsPage() {
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="results" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="results">经营结果</TabsTrigger>
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
