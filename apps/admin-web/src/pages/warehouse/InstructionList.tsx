import { useMemo, useState, type ReactNode } from 'react';
import { Plus, TabletSmartphone } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listInstructions, type InstructionRecord } from './api';
import { ClearFilters, OwnerWarehouseFilters } from './counts-list-chrome';
import { DocumentLink } from './document-page';
import { instructionNextStep, instructionStatusLabel, instructionWaitingSince } from './instruction-flow';
import { InstructionDocument } from './InstructionDocument';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { instructionKind, processingModule as moduleId } from './processing-address';
import type { QueueFilters } from './use-queue-filters';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const info = moduleInfoById[moduleId];
const receivedKey = 'receivedAt';
const pending = 'PENDING';
const allQueues = 'ALL';
const noRows: InstructionRecord[] = [];

const queueStatuses = [pending] as const;
const settledStatuses = ['DONE', 'VOID'] as const;
const knownStatuses = [allQueues, ...queueStatuses, ...settledStatuses];
const kindWords = ['PROCESS', 'DISASSEMBLE', 'RELABEL'] as const;

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/** 读口按受理时刻排。待执行先受理的在上面；看全部或已了结的，最新受理的在上面。人点过表头就照他的。 */
function receivedSort(stored: string | null, inQueue: boolean): WorkbenchSort {
  if (stored === receivedKey) return { key: receivedKey, dir: 1 };
  if (stored === `-${receivedKey}`) return { key: receivedKey, dir: -1 };
  return { key: receivedKey, dir: inQueue ? 1 : -1 };
}

/**
 * 加工、拆解与换标指令。打开落在待执行。待执行的数量在标题行上。
 * 点一行在右边开这条指令，指令号仍进整页。行上待执行的只留去作业台；作废在右栏。
 */
export function InstructionList({ filters }: { filters: QueueFilters }) {
  const drawer = useActionDrawer(internalOpsActionSpecs);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.instructions.open`, null);
  const [refreshes, setRefreshes] = useState(0);
  const status = (knownStatuses as readonly string[]).includes(filters.status) ? filters.status : pending;
  const settled = useSettledKeyword(filters.keyword);
  const baseFilters: Record<string, string[]> = {};
  if (filters.owner) baseFilters.ownerAccount = [filters.owner];
  if (filters.warehouse) baseFilters.warehouseCode = [filters.warehouse];
  if (filters.kind) baseFilters.kind = [filters.kind];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  const sort = receivedSort(filters.sort, status === pending);
  const conditions: CatalogueConditions = {
    ...base,
    sort: sort.dir === 1 ? receivedKey : `-${receivedKey}`,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listInstructions);
  const held = heldCataloguePage(answer, previous, (body) => body.instructions.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.instructions : (held?.body.instructions ?? noRows);
  const counts = useStatusCounts(listInstructions, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const ownerNames = useMemo(
    () => new Map(drawer.lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    [drawer.lookups.owners],
  );
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((warehouse) => [warehouse.warehouseCode, warehouse.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const named = (names: ReadonlyMap<string, string>, code: string) => {
    const name = nameOf(names, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };
  const now = new Date();

  const columns: WorkbenchColumn<InstructionRecord>[] = [
    {
      key: 'id',
      header: '指令号',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[instructionKind, row.instructionId]} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={instructionStatusLabel(row.status)} /> },
    { key: 'kind', header: '种类', width: fitColumnWidth, render: (row) => nowrap(labelOf(row.kind)) },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 900, render: (row) => named(ownerNames, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1080, render: (row) => named(warehouseNames, row.warehouseCode) },
    {
      key: 'goods',
      header: '成品 / 目标',
      render: (row) => <WmsTwoLine fill primary={<span className="font-mono">{row.goodsCode}</span>} title={row.goodsCode} />,
    },
    {
      key: 'progress',
      header: nowrap('完成 / 计划'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1200,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{`${row.doneQuantity} / ${row.quantity}`}</span>,
    },
    {
      key: 'waiting',
      header: nowrap('已等'),
      right: true,
      width: fitColumnWidth,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{elapsedLabel(instructionWaitingSince(row), now) || '—'}</span>,
    },
    {
      key: 'received',
      header: nowrap('受理'),
      right: true,
      width: fitColumnWidth,
      sortKey: receivedKey,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.receivedAt)}>
          {shortInstant(row.receivedAt, now) || '—'}
        </span>
      ),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <InstructionStep row={row} /> },
  ];

  const chooseStatus = (next: string) => filters.setStatus((knownStatuses as readonly string[]).includes(next) ? next : pending);
  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: instructionStatusLabel(code),
    count: counts[code] ?? null,
    active: status === code,
    attention: true,
    onClick: () => chooseStatus(status === code ? allQueues : code),
  }));
  const narrowed =
    settled.trim() !== '' ||
    filters.owner !== '' ||
    filters.warehouse !== '' ||
    filters.kind !== '' ||
    (status !== pending && status !== allQueues) ||
    pastFirstPage(pagination);
  const clearFilters = () => {
    filters.setKeyword('');
    filters.setOwner('');
    filters.setWarehouse('');
    filters.setKind('');
    filters.setStatus(pending);
    filters.setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-instructions',
        emptyTitle: status === pending ? '没有待执行的指令' : '还没有加工或换标指令',
        emptyDescription: '点右上角下发加工或换标。执行请到仓内作业台。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<InstructionRecord>
        title="加工与换标指令"
        viewStateKey={`${moduleId}.instructions`}
        subtitle="点一行在右边看这条指令。执行在作业台做"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          <>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
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
          </>
        }
        search={{ value: filters.keyword, onChange: filters.setKeyword, placeholder: '搜指令号、货主、仓、商品、新别名或原因' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => chooseStatus(next === '' ? allQueues : next)}
              options={[...queueStatuses, ...settledStatuses].map((code) => ({ value: code, label: instructionStatusLabel(code) }))}
            />
            <OwnerWarehouseFilters drawer={drawer} owner={filters.owner} warehouse={filters.warehouse} onOwner={filters.setOwner} onWarehouse={filters.setWarehouse} />
            <WmsFilterSelect label="种类" value={filters.kind} onChange={filters.setKind} options={kindWords.map((word) => ({ value: word, label: labelOf(word) }))} />
            {narrowed ? <ClearFilters onClear={clearFilters} /> : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.instructionId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => {
          const next = toggleWorkbenchSort(sort, key);
          filters.setSort(next.dir === 1 ? receivedKey : `-${receivedKey}`);
        }}
        noMatchText="当前筛选下没有加工或换标指令"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          <InstructionDocument instructionId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

function InstructionStep({ row }: { row: InstructionRecord }) {
  const next = instructionNextStep(row.status);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        window.location.hash = `#/${next.moduleId}`;
      }}
    >
      {next.label}
    </Button>
  );
}
