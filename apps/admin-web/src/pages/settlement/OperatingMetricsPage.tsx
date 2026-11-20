import { useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import { catalogueConditionsNarrow } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import {
  listSettlementOperatingResults,
  registryConditions,
  type CostAllocationRecord,
  type RegistryConditions,
} from './api';
import { classifyOperatingMetrics, operatingMetricsRowKey, type OperatingMetricsRow } from './operating-metrics';
import { formatMoney, technicalRows } from './ops-ui';

// 经营核算页（主责上下文 settlement-accounting，UC-SA-006 按口径、版本、币种与截至时点派生经营毛利）。两签都接
// GET /settlement-operating-results 按 registry 分派。成本分摊在本页而不在费用页：分摊只改变经营归因、不转移原债权债务责任。
//
// 两本册已迁到 ADR-0144，各按游标翻页（模板的翻页条）。检索框下推成 `q`，两签检索的列读口都覆盖，页内不再筛；读口另有的
// 可排维（落库时刻）与经营结果的 basis 一维，页面今天没有对应的表头排序与筛选条，不为此新加控件。条数只数这一页，字面写「本页」。
const info = moduleInfoById['operating-metrics'];

// 组成项逐项呈现而不按角色合计成四栏：求和这一层放在页面就成了第二处定义，而「审核应付与贷项按各自借贷方向分别计入
// 一次、不得净含贷项」要人看见的恰恰是逐项。毛利一栏带符号呈现，负值即经营损失，不另设「损失」栏（「零」会落进两栏都
// 不占的缝里）。

// —— 经营结果签 ——

function resultTechnicalRows(row: OperatingMetricsRow) {
  if (row.kind === 'disposition') {
    return technicalRows([
      { label: '分析范围', value: row.record.scope },
      { label: '答案', value: labelFor(row.record.disposition) },
      { label: '登记时间', value: formatInstant(row.record.recordedAt) },
    ]);
  }
  return technicalRows([
    { label: '分析范围', value: row.record.scope },
    { label: '账期', value: row.record.period },
    { label: '计算版本', value: row.record.version },
    { label: '纠错自', value: row.record.corrects },
    { label: '登记时间', value: formatInstant(row.record.recordedAt) },
  ]);
}

const operatingResultColumns: ListColumn<OperatingMetricsRow>[] = [
  {
    id: 'scope',
    header: '分析范围',
    className: 'font-mono type-body leading-4 text-idpxyz-textBright',
    render: (row) => row.record.scope,
  },
  {
    id: 'period',
    header: '账期',
    className: 'font-mono type-body leading-4',
    render: (row) => (row.kind === 'result' ? row.record.period : '—'),
  },
  {
    id: 'basis',
    header: '口径',
    align: 'center',
    // 封闭三格。索赔调整后口径必须声明所依附的基础口径，属编排层组合，不是第四格。
    render: (row) => <StatusPill code={row.record.basis} />,
  },
  {
    id: 'margin',
    header: '毛利',
    align: 'right',
    className: 'font-mono type-body leading-4 tabular-nums',
    // 快照带符号照实呈现，负值即经营损失。答不出毛利的那一行不写数字：判不出、成本不全、待判断都不是把缺的一侧当零。
    render: (row) =>
      row.kind === 'result' ? (
        <span className={row.record.margin.startsWith('-') ? 'text-[color:var(--ds-status-danger-fg)]' : 'text-idpxyz-textBright'}>
          {formatMoney(row.record.margin, row.record.currency)}
        </span>
      ) : (
        <StatusPill code={row.record.disposition} />
      ),
  },
  {
    id: 'components',
    header: '组成',
    // 角色照册转写，不是页面判出来的。答不出毛利的行没有组成项，也不留成空白。
    render: (row) =>
      row.kind === 'result' && row.record.components.length > 0 ? (
        <div className="min-w-64 type-body leading-4">
          {row.record.components.map((component) => (
            <p key={`${component.role}:${component.source}:${component.effect}:${component.amount}`} className="whitespace-nowrap">
              {labelFor(component.role)}
              <span className="text-idpxyz-textMuted">（{labelFor(component.effect)}）</span> {formatMoney(component.amount, '')}
              <span className="ml-1 font-mono type-caption text-idpxyz-textMuted">{component.source}</span>
            </p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">{row.kind === 'result' ? '无组成项' : '不出毛利'}</span>
      ),
  },
  {
    id: 'as-of',
    header: '截至',
    className: 'whitespace-nowrap font-mono type-body leading-4',
    render: (row) => (row.kind === 'result' ? formatInstant(row.record.asOf) : formatInstant(row.record.recordedAt)),
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={resultTechnicalRows(row)} className="max-w-md open:min-w-[18rem]" />,
  },
];

const loadOperatingResults = (conditions: RegistryConditions<'operating-result'>, after: string | null) =>
  listSettlementOperatingResults(conditions, after);

function OperatingResultsTable() {
  const [search, setSearch] = useState('');
  const conditions = registryConditions('operating-result', { q: useSettledKeyword(search) });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, loadOperatingResults);
  const classified = answer?.kind === 'outcome' ? classifyOperatingMetrics(answer.body) : null;
  const rows = classified?.kind === 'listed' ? classified.rows : [];
  const unconfigured = classified?.kind === 'unconfigured';

  return (
    <ListPageTemplate<OperatingMetricsRow>
      title="经营结果"
      description="按包裹和账期派生的直接成本毛利；没采用、判不出、成本不全各自成格，不写成一条毛利"
      search={{ value: search, onChange: setSearch, placeholder: '搜分析范围、账期、计算版本' }}
      filterSummary={classified?.kind === 'listed' ? `本页 ${rows.length} 行` : undefined}
      columns={operatingResultColumns}
      rows={rows}
      rowKey={operatingMetricsRowKey}
      emptyRowsNote="没有匹配的快照"
      pagination={pagination}
      viewState={
        unconfigured
          ? {
              kind: 'unconfigured',
              title: '经营口径还没有采用',
              description:
                '这一户还没有登记采用预估或已确认。这不是已经采用、只是还没有结果：没有这一行，系统不派生毛利。',
            }
          : catalogueViewState(answer, rows.length, retry, {
              module: info,
              endpoint: 'GET /settlement-operating-results?registry=operating-result',
              emptyTitle: '还没有经营结果',
              emptyDescription:
                '已经采用的口径下，费用与成本成对之后这里会出现组成项。只有一侧是判不出，网络服务或共享成本是成本不全，都不会写成一条毛利。',
              narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
            })
      }
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
  { id: 'source', header: '来源成本', className: 'font-mono type-body leading-4 text-idpxyz-textBright', render: (row) => row.source },
  {
    id: 'source-amount',
    header: '金额',
    align: 'right',
    className: 'font-mono type-body leading-4 tabular-nums',
    render: (row) => formatMoney(row.sourceAmount, row.currency),
  },
  {
    id: 'portions',
    header: '分到哪里',
    // 空数组即「全额未分摊」——与未分摊余额栏互相印证，不折进份额里凑平。
    render: (row) =>
      row.portions.length > 0 ? (
        <div className="min-w-48 type-body leading-4">
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
    className: 'font-mono type-body leading-4 tabular-nums',
    // 单独成栏：未分摊余额是第一类结果不是尾差——没有合格对象或分母为零时来源金额整笔留在这里等新依据。
    render: (row) => formatMoney(row.unallocatedAmount, ''),
  },
  {
    id: 'allocated-at',
    header: '分摊时间',
    className: 'whitespace-nowrap font-mono type-body leading-4',
    render: (row) => formatInstant(row.allocatedAt),
  },
  {
    id: 'technical',
    header: '',
    className: 'align-top',
    render: (row) => <TechnicalDetails rows={allocationTechnicalRows(row)} className="max-w-md open:min-w-[18rem]" />,
  },
];

const loadCostAllocations = (conditions: RegistryConditions<'cost-allocation'>, after: string | null) =>
  listSettlementOperatingResults(conditions, after);

function CostAllocationsTable() {
  const [search, setSearch] = useState('');
  const conditions = registryConditions('cost-allocation', { q: useSettledKeyword(search) });
  const { answer, pagination, retry } = useCatalogueCursor(conditions, loadCostAllocations);
  const allocations = answer?.kind === 'outcome' ? answer.body.allocations : [];

  return (
    <ListPageTemplate<CostAllocationRecord>
      title="成本分摊"
      description="把一笔成本按规则分到各个分析对象上；只改经营归因，不改谁欠谁的钱"
      search={{ value: search, onChange: setSearch, placeholder: '搜分摊、来源成本、规则' }}
      filterSummary={answer?.kind === 'outcome' ? `本页 ${allocations.length} 次分摊` : undefined}
      columns={costAllocationColumns}
      rows={allocations}
      // 行键循分摊标识加版本：纠错重算换版本，原分摊保留。
      rowKey={(row) => `${row.allocation}:${row.version}`}
      emptyRowsNote="没有匹配的分摊"
      pagination={pagination}
      viewState={catalogueViewState(answer, allocations.length, retry, {
        module: info,
        endpoint: 'GET /settlement-operating-results?registry=cost-allocation',
        emptyTitle: '还没有成本分摊',
        emptyDescription: '分摊规则登记后系统按规则分摊成本，这里会随之出现；页面不会预置分摊。',
        narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
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
