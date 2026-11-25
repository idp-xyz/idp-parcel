import { TabletSmartphone } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { listInternalJobs, type InternalJobRecord } from './api';
import { DocumentLink } from './document-page';
import { jobSignal } from './internal-job-flow';
import { internalJobKind, processingModule as moduleId } from './processing-address';
import { useWarehouseLookups } from './use-catalogues';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import { labelOf, WmsBadge, WmsFilterSelect } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 库内作业是已经发生的事实，没有状态，读口也不声明状态这一维，所以这一页没有状态签：能筛的只有读口声明的作业种类与仓。
// 种类的词表就是读口声明的这四个。
const kindWords = ['INTERNAL_MOVE', 'FORM_CONVERSION', 'CONDITION_CONVERSION', 'SHORTAGE'] as const;

const newestFirst = '-receivedAt';
const oldestFirst = 'receivedAt';

/** 库内作业台账：作业台提交的上架、移库、形态与状态转换和短少上报。点事实号打开库内作业。 */
export function InternalJobList({ filters }: { filters: QueueFilters }) {
  const lookups = useWarehouseLookups();
  const settled = useSettledKeyword(filters.keyword);
  const effectiveSort = filters.sort ?? newestFirst;
  const baseFilters: Record<string, string[]> = {};
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  if (filters.kind) baseFilters.kind = [filters.kind];
  const conditions: CatalogueConditions = { q: settled, sort: effectiveSort, filters: baseFilters };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listInternalJobs);
  const rows = answer?.kind === 'outcome' ? answer.body.jobs : [];

  const columns: ListColumn<InternalJobRecord>[] = [
    {
      id: 'fact',
      header: '事实号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[internalJobKind, row.factId]} />,
    },
    { id: 'kind', header: '作业种类', render: (row) => <WmsBadge code={row.kind} label={labelOf(row.kind)} tone="info" /> },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouse },
    { id: 'lines', header: '明细行', align: 'right', className: mono, render: (row) => String(row.lines.length) },
    {
      id: 'signal',
      header: '入账情况',
      className: 'whitespace-nowrap',
      render: (row) => {
        const signal = jobSignal(row.kind, row.lines);
        return signal.warn ? <span className="text-amber-600 dark:text-amber-400">{signal.text}</span> : signal.text;
      },
    },
    { id: 'occurred', header: '发生时刻', className: mono, render: (row) => formatInstant(row.occurredAt) },
  ];

  const inspectorOf = (row: InternalJobRecord): InspectorContent => ({
    title: row.factId,
    subtitle: labelOf(row.kind),
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '仓库', value: row.warehouse, mono: true },
          { label: '行数', value: String(row.lines.length), mono: true },
          { label: '入账情况', value: jobSignal(row.kind, row.lines).text },
          { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
          { label: '设备序号', value: `#${row.deviceSequence}`, mono: true },
        ],
      },
      { kind: 'actions', actions: [{ label: '打开库内作业', onRun: () => openDocument(moduleId, [internalJobKind, row.factId]) }] },
    ],
  });

  return (
    <ListPageTemplate<InternalJobRecord>
      moduleId={moduleId}
      title="库内作业台账"
      description="作业台提交的上架、移库、形态与状态转换和短少上报。按作业种类与仓筛；点事实号打开库内作业，逐行看账上入了多少、没入的是为什么。"
      headerActions={
        <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
          <TabletSmartphone className="mr-1 h-3.5 w-3.5" />
          打开作业台
        </Button>
      }
      search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜事实号或仓库' }}
      sort={{
        value: effectiveSort,
        onChange: filters.setSort,
        options: [
          { value: newestFirst, label: '最新收到的在前' },
          { value: oldestFirst, label: '最早收到的在前' },
        ],
      }}
      filters={
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <WmsFilterSelect label="作业种类" value={filters.kind} onChange={filters.setKind} options={kindWords.map((word) => ({ value: word, label: labelOf(word) }))} />
          <WmsFilterSelect
            label="仓库"
            value={filters.warehouse}
            onChange={filters.setWarehouse}
            options={lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
          />
        </div>
      }
      columns={columns}
      rows={rows}
      rowKey={(row) => row.factId}
      inspector={inspectorOf}
      onRowOpen={(row) => openDocument(moduleId, [internalJobKind, row.factId])}
      pagination={pagination}
      emptyRowsNote="当前条件下没有库内作业"
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-internal-jobs',
        emptyTitle: '还没有库内作业记录',
        emptyDescription: '在仓内作业台提交移库或报短少后，记录会出现在这里。',
        narrowed: settled.trim() !== '' || filters.warehouse !== '' || filters.kind !== '',
      })}
    />
  );
}
