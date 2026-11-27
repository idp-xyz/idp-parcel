import { useMemo, useState, type ReactNode } from 'react';
import { Plus, X } from 'lucide-react';
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
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listTotes, type ToteRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf } from './list-cells';
import { canRetireTote, liveToteStates, toteStateBadge, toteStateLabel } from './tote';
import { useStatusCounts } from './use-status-counts';
import { queueCount } from './outbound-queues';
import { WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const moduleId = 'warehouse-waves';
const info = moduleInfoById[moduleId];
const codeKey = 'code';
const codeOrder: WorkbenchSort = { key: codeKey, dir: 1 };
const noRows: ToteRecord[] = [];
const toteStates = [...liveToteStates, 'RETIRED'] as const;

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

const knownState = (value: string) => (value === 'ALL' || (toteStates as readonly string[]).includes(value) ? value : 'ALL');

/**
 * 周转箱名册。打开看全部。在用与空着的数量在标题行上，点了就只看那一队。
 * 箱没有自己的单据页：点一行在右边看这只箱，行上只留此刻该做的那一步。
 * 读口只按箱号排。
 */
export function ToteList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [keyword, setKeyword] = useTabViewState('warehouse-waves.totes.keyword', '');
  const [warehouse, setWarehouse] = useTabViewState('warehouse-waves.totes.warehouse', '');
  const [storedState, setState] = useTabViewState('warehouse-waves.totes.state', 'ALL');
  const state = knownState(storedState);
  const [chosenSort, setChosenSort] = useTabViewState<WorkbenchSort | null>('warehouse-waves.totes.sort', null);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-waves.totes.open', null);
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(keyword);
  const sort = chosenSort ?? codeOrder;
  const base: CatalogueConditions = {
    q: settled,
    filters: warehouse ? { warehouseCode: [warehouse] } : {},
    ...(sort.dir === -1 ? { sort: '-toteCode' } : {}),
  };
  const conditions: CatalogueConditions = state === 'ALL' ? base : { ...base, filters: { ...base.filters, state: [state] } };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listTotes);
  const held = heldCataloguePage(answer, previous, (body) => body.totes.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.totes : (held?.body.totes ?? noRows);
  const counts = useStatusCounts(listTotes, { q: settled, filters: warehouse ? { warehouseCode: [warehouse] } : {} }, 'state', liveToteStates, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((item) => [item.warehouseCode, item.name ?? ''])),
    [drawer.lookups.warehouses],
  );

  const columns: WorkbenchColumn<ToteRecord>[] = [
    {
      key: 'code',
      header: '箱号',
      width: fitColumnWidth,
      sortKey: codeKey,
      render: (row) => <span className="whitespace-nowrap font-mono">{row.toteCode}</span>,
    },
    {
      key: 'state',
      header: '状态',
      width: fitColumnWidth,
      render: (row) => <WmsBadge code={toteStateBadge(row.state)} label={toteStateLabel(row.state)} />,
    },
    {
      key: 'warehouse',
      header: '仓',
      width: fitColumnWidth,
      minListWidth: 720,
      render: (row) => {
        const name = nameOf(warehouseNames, row.warehouseCode);
        return name ? <WmsTwoLine primary={name} secondary={row.warehouseCode} code title={`${name} · ${row.warehouseCode}`} /> : <span className="whitespace-nowrap font-mono">{row.warehouseCode}</span>;
      },
    },
    {
      key: 'request',
      header: '绑着的出库单',
      render: (row) =>
        row.requestId ? (
          <DocumentLink moduleId="warehouse-outbound" parts={[row.requestId]} />
        ) : (
          <span className="text-idpxyz-textMuted">没有</span>
        ),
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => <ToteStep row={row} drawer={drawer} />,
    },
  ];

  const chooseState = (next: string) => setState(knownState(next));
  const queueStats: WmsQueueStat[] = liveToteStates.map((id) => ({
    id,
    label: toteStateLabel(id),
    count: queueCount(counts, id),
    active: state === id,
    attention: id === 'IN_USE',
    onClick: () => chooseState(state === id ? 'ALL' : id),
  }));
  const narrowed = settled.trim() !== '' || warehouse !== '' || state !== 'ALL';
  const clearFilters = () => {
    setKeyword('');
    setWarehouse('');
    setState('ALL');
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-totes',
        emptyTitle: '还没有登记周转箱',
        emptyDescription: '点右上角「登记周转箱」，按仓一次登一批箱号。拣货车上的格口也按箱登记。',
        narrowed: narrowed || pastFirstPage(pagination),
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<ToteRecord>
        title="周转箱"
        viewStateKey={`${moduleId}.totes`}
        subtitle="点一行在右边看这只箱。拣货只认登记过的箱号。"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          drawer.has('tote-register') ? (
            <Button type="button" className={primaryActionClass} onClick={() => drawer.open('tote-register', warehouse ? { warehouse } : undefined)}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              登记周转箱
            </Button>
          ) : undefined
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜箱号或绑着的出库请求号' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={state === 'ALL' ? '' : state}
              onChange={(next) => chooseState(next === '' ? 'ALL' : next)}
              options={toteStates.map((id) => ({ value: id, label: toteStateLabel(id) }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={setWarehouse}
              options={drawer.lookups.warehouses.map((item) => ({
                value: item.warehouseCode,
                label: item.name ? `${item.name} · ${item.warehouseCode}` : item.warehouseCode,
              }))}
            />
            {narrowed ? (
              <button
                type="button"
                className="whitespace-nowrap type-body text-idpxyz-textMuted hover:text-idpxyz-textBright focus-visible:underline focus-visible:outline-none"
                onClick={clearFilters}
              >
                清除
              </button>
            ) : null}
          </>
        }
        columns={columns}
        rows={rows}
        rowKey={toteKey}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => setChosenSort(toggleWorkbenchSort(sort, key))}
        noMatchText="当前筛选下没有周转箱"
        onClearFilters={clearFilters}
        renderDetail={(key) => {
          const row = rows.find((item) => toteKey(item) === key);
          return row ? <ToteDetail row={row} drawer={drawer} onClose={() => setOpenKey(null)} /> : <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这只箱。</p>;
        }}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

function toteKey(row: ToteRecord): string {
  return `${row.warehouseCode}/${row.toteCode}`;
}

function ToteStep({ row, drawer }: { row: ToteRecord; drawer: ReturnType<typeof useActionDrawer> }) {
  const step = toteStep(row, drawer.has('tote-release'), drawer.has('tote-retire'), drawer.has('tote-register'));
  if (step === null) return <span className="text-idpxyz-textMuted">—</span>;
  if (!step.enabled) return <span className="whitespace-nowrap text-idpxyz-textMuted">{step.label}</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        drawer.open(
          step.action,
          step.action === 'tote-register'
            ? { warehouse: row.warehouseCode, toteCodes: row.toteCode }
            : { warehouse: row.warehouseCode, toteCode: row.toteCode },
        );
      }}
    >
      {step.label}
    </Button>
  );
}

function toteStep(
  row: ToteRecord,
  canRelease: boolean,
  canRetire: boolean,
  canRegister: boolean,
): { label: string; action: string; enabled: boolean } | null {
  if (row.state === 'IN_USE') return { label: '放开', action: 'tote-release', enabled: canRelease };
  if (canRetireTote(row.state)) return { label: '停用', action: 'tote-retire', enabled: canRetire };
  if (row.state === 'RETIRED') return { label: '恢复在用', action: 'tote-register', enabled: canRegister };
  return null;
}

function ToteDetail({ row, drawer, onClose }: { row: ToteRecord; drawer: ReturnType<typeof useActionDrawer>; onClose: () => void }) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 items-start justify-between gap-2 border-b border-idpxyz-border px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="type-section font-semibold text-idpxyz-textBright">周转箱</span>
            <span className="font-mono">{row.toteCode}</span>
            <WmsBadge code={toteStateBadge(row.state)} label={toteStateLabel(row.state)} />
          </div>
          <p className="mt-1 type-caption text-idpxyz-textMuted">{row.warehouseCode}</p>
        </div>
        <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4 type-body">
        <p>{row.requestId ? <>绑着 <DocumentLink moduleId="warehouse-outbound" parts={[row.requestId]} /></> : '没有绑出库单'}</p>
        <p className="font-mono type-caption text-idpxyz-textMuted">绑上 {row.boundAt ? formatInstant(row.boundAt) : '—'}</p>
        <p className="font-mono type-caption text-idpxyz-textMuted">登记 {row.registeredBy} · {formatInstant(row.registeredAt)}</p>
        {row.retiredAt ? <p className="font-mono type-caption text-idpxyz-textMuted">停用 {row.retiredBy ?? ''} · {formatInstant(row.retiredAt)}</p> : null}
        <ToteStep row={row} drawer={drawer} />
      </div>
    </div>
  );
}
