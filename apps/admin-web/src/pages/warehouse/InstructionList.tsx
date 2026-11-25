import { useState } from 'react';
import { Plus, TabletSmartphone } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent, InspectorField } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listInstructions, type InstructionRecord } from './api';
import { DocumentLink } from './document-page';
import {
  instructionNextStep,
  instructionStatusLabel,
  instructionTitle,
  instructionVoidActionId,
  instructionWaitingSince,
} from './instruction-flow';
import { instructionKind, processingModule as moduleId } from './processing-address';
import { openDocument } from './use-document';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsStatusTabs } from './wms-ui';

const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

// 签的先后照一份指令往下走的先后：待执行是要人动手的一队，数。已执行与已作废只增不减，数出来的不是待办，不数；
// 「全部」放最后，页面打开就落在待执行上。
const queueStatuses = ['PENDING'] as const;
const settledStatuses = ['DONE', 'VOID'] as const;
// 指令种类是读口声明的 kind 筛选维，词表就这三个。
const kindWords = ['PROCESS', 'DISASSEMBLE', 'RELABEL'] as const;

const newestFirst = '-receivedAt';
const oldestFirst = 'receivedAt';

const goModule = (target: string) => {
  window.location.hash = `#/${target}`;
};

/** 加工、拆解与换标指令：下发在这里，现场执行在作业台。点指令号打开指令。 */
export function InstructionList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  if (filters.kind) baseFilters.kind = [filters.kind];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了待执行这一队，按受理先后排，等得最久的在最上面；看全部或已了结的，最新的在上面。人改过排序就照他的。
  const inQueue = (queueStatuses as readonly string[]).includes(filters.status);
  const effectiveSort = filters.sort ?? (inQueue ? oldestFirst : newestFirst);
  const conditions: CatalogueConditions = {
    ...base,
    sort: effectiveSort,
    filters: filters.status === 'ALL' ? baseFilters : { ...baseFilters, status: [filters.status] },
  };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listInstructions);
  const rows = answer?.kind === 'outcome' ? answer.body.instructions : [];
  const counts = useStatusCounts(listInstructions, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const now = new Date();

  const columns: ListColumn<InstructionRecord>[] = [
    {
      id: 'id',
      header: '指令号',
      className: 'whitespace-nowrap',
      render: (row) => <DocumentLink moduleId={moduleId} parts={[instructionKind, row.instructionId]} />,
    },
    { id: 'status', header: '状态', render: (row) => <WmsBadge code={row.status} label={instructionStatusLabel(row.status)} /> },
    { id: 'kind', header: '种类', className: 'whitespace-nowrap', render: (row) => labelOf(row.kind) },
    { id: 'owner', header: '货主', className: mono, render: (row) => row.ownerAccount },
    { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
    { id: 'goods', header: '成品 / 目标', className: mono, render: (row) => row.goodsCode },
    { id: 'progress', header: '完成 / 计划', align: 'right', className: mono, render: (row) => `${row.doneQuantity} / ${row.quantity}` },
    {
      id: 'waiting',
      header: '已等',
      align: 'right',
      className: 'whitespace-nowrap',
      // 只有待执行的在等现场；已了结的指令不写等了多久。
      render: (row) => elapsedLabel(instructionWaitingSince(row), now) || '—',
    },
    { id: 'next', header: '下一步', className: 'whitespace-nowrap', render: (row) => <NextStepCell row={row} /> },
  ];

  const inspectorOf = (row: InstructionRecord): InspectorContent => {
    const next = instructionNextStep(row.status);
    const voidId = instructionVoidActionId(row.kind);
    const actions: InspectorAction[] = [
      { label: `打开${instructionTitle(row.kind)}`, onRun: () => openDocument(moduleId, [instructionKind, row.instructionId]) },
    ];
    if (next !== null) actions.push({ label: next.label, onRun: () => goModule(next.moduleId) });
    if (row.status === 'PENDING' && drawer.has(voidId)) {
      actions.push({ label: '作废指令', onRun: () => drawer.open(voidId, { instructionId: row.instructionId }) });
    }
    const audit: InspectorField[] = [
      { label: '下发', value: `${row.operator || '—'} · ${formatInstant(row.receivedAt)}`, mono: true },
    ];
    if (row.status === 'VOID' && row.voidedAt) {
      audit.push({ label: '作废', value: `${formatInstant(row.voidedAt)}${row.voidReason ? ` · ${row.voidReason}` : ''}`, mono: true });
    }
    return {
      title: row.instructionId,
      subtitle: labelOf(row.kind),
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '货主 / 仓', value: `${row.ownerAccount} · ${row.warehouseCode}`, mono: true },
            { label: '商品', value: row.goodsCode, mono: true },
            { label: '完成 / 计划', value: `${row.doneQuantity} / ${row.quantity}`, mono: true },
          ],
        },
        { kind: 'status', items: [{ label: '状态', word: instructionStatusLabel(row.status) }] },
        { kind: 'actions', actions },
        { kind: 'audit', fields: audit },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<InstructionRecord>
        moduleId={moduleId}
        title="加工与换标指令"
        description="下发加工、拆解或换标，现场执行在作业台完成。点状态看那一队，数量是当前筛选下还有几条，待执行的等得最久的排在前面。点指令号打开指令：看要做什么、用料与产出、执行记录。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" onClick={() => goModule('warehouse-workstation')}>
              <TabletSmartphone className="mr-1 h-3.5 w-3.5" />
              去作业台执行
            </Button>
            {drawer.has('processing-instruction') ? (
              <Button type="button" variant="outline" onClick={() => drawer.open('processing-instruction')}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                下发加工
              </Button>
            ) : null}
            {drawer.has('relabel-instruction') ? (
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('relabel-instruction')}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                下发换标
              </Button>
            ) : null}
          </div>
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜指令号、货主、仓、商品、新别名或原因' }}
        sort={{
          value: effectiveSort,
          onChange: filters.setSort,
          options: [
            { value: newestFirst, label: '最新受理的在前' },
            { value: oldestFirst, label: '等得最久的在前' },
          ],
        }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={filters.status}
              onChange={filters.setStatus}
              tabs={[
                ...queueStatuses.map((code) => ({ value: code, label: instructionStatusLabel(code), count: counts[code] })),
                ...settledStatuses.map((code) => ({ value: code, label: instructionStatusLabel(code) })),
                { value: 'ALL', label: '全部' },
              ]}
            />
            <WmsFilterSelect
              label="货主"
              value={filters.owner}
              onChange={filters.setOwner}
              options={drawer.lookups.owners.map((account) => ({
                value: account.accountId,
                label: account.customerPartyName ? `${account.accountId} · ${account.customerPartyName}` : account.accountId,
              }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={filters.warehouse}
              onChange={filters.setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
            />
            <WmsFilterSelect label="种类" value={filters.kind} onChange={filters.setKind} options={kindWords.map((word) => ({ value: word, label: labelOf(word) }))} />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.instructionId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [instructionKind, row.instructionId])}
        pagination={pagination}
        emptyRowsNote="当前条件下没有加工或换标指令"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-instructions',
          emptyTitle: filters.status === 'PENDING' ? '没有待执行的指令' : '还没有加工或换标指令',
          emptyDescription: '点右上角下发加工或换标。执行请到仓内作业台。',
          narrowed:
            settled.trim() !== '' ||
            filters.owner !== '' ||
            filters.warehouse !== '' ||
            filters.kind !== '' ||
            (filters.status !== 'PENDING' && filters.status !== 'ALL'),
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 每行最右的那一格：这份指令此刻该做的一步。执行在作业台做，这里只指路；已了结的没有下一步。 */
function NextStepCell({ row }: { row: InstructionRecord }) {
  const next = instructionNextStep(row.status);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  return (
    <button
      type="button"
      className="text-[color:var(--ds-text-link-hover)] hover:underline"
      onClick={(event) => {
        // 行上的单击是预览：这一下只去作业台，不顺带把行选进右栏。
        event.stopPropagation();
        goModule(next.moduleId);
      }}
    >
      {next.label}
    </button>
  );
}
