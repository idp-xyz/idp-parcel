import { useMemo, useState, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { receivingActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer, type ActionDrawerControl } from './ActionDrawer';
import { listUnclaimedArrivals, type DisposalRuleRecord, type UnclaimedArrivalRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { claimPrefill, claimWindowElapsed, unclaimedNextStep } from './unclaimed-next-step';
import { UnclaimedDocument } from './UnclaimedDocument';
import { useDocumentParts } from './use-document';
import { useDisposalRules } from './use-disposal-rules';
import { useStatusCounts } from './use-status-counts';
import { elapsedLabel } from './waiting';
import { labelOf, WmsBadge, WmsFilterSelect, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const moduleId = 'warehouse-unclaimed';
const info = moduleInfoById[moduleId];
const receivedKey = 'receivedAt';
const allQueues = 'ALL';

// 签的先后照一件到货往下走的先后。待认领与已认领是要人动手的两队：认领被撤销后到货退回待认领，两个数都会往回走，所以都数。
// 已处置只增不减，数出来的不是待办，不数。页面打开就落在待认领上。
const queueStatuses = ['UNCLAIMED', 'CLAIMED'] as const;
const settledStatuses = ['DISPOSED'] as const;
const kindWords = ['INBOUND', 'RETURN'] as const;
const oldestFirst: WorkbenchSort = { key: receivedKey, dir: 1 };
const newestFirst: WorkbenchSort = { key: receivedKey, dir: -1 };

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

const knownStatus = (value: string) =>
  (queueStatuses as readonly string[]).includes(value) || (settledStatuses as readonly string[]).includes(value) || value === allQueues
    ? value
    : 'UNCLAIMED';

/** 待认领到货：列表与每一件到货各占自己的标签，同一个模块按地址里有没有单据键分。 */
export function WarehouseUnclaimedPage() {
  const parts = useDocumentParts();
  return parts ? <UnclaimedDocument arrivalId={parts[0]} /> : <UnclaimedList />;
}

function UnclaimedList() {
  const drawer = useActionDrawer(receivingActionSpecs);
  const [keyword, setKeyword] = useTabViewState('warehouse-unclaimed.keyword', '');
  const [storedStatus, setStatus] = useTabViewState('warehouse-unclaimed.status', 'UNCLAIMED');
  const status = knownStatus(storedStatus);
  const [warehouse, setWarehouse] = useTabViewState('warehouse-unclaimed.warehouse', '');
  const [kind, setKind] = useTabViewState('warehouse-unclaimed.kind', '');
  const [chosenSort, setChosenSort] = useTabViewState<WorkbenchSort | null>('warehouse-unclaimed.sort', null);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-unclaimed.open', null);
  const [refreshes, setRefreshes] = useState(0);
  const rules = useDisposalRules(refreshes);

  const settled = useSettledKeyword(keyword);
  const baseFilters: Record<string, string[]> = {};
  if (warehouse) baseFilters.warehouseCode = [warehouse];
  if (kind) baseFilters.kind = [kind];
  const base: CatalogueConditions = { q: settled, filters: baseFilters };
  // 选了待认领或已认领这样的队，按登记先后排，放得最久的在最上面；看全部或已处置的，最新的在上面。人点过表头就照他的。
  // 读口可排的是登记时刻（receivedAt），认领期限却是从到货发生时刻起算：线下先记、后同步上来的到货两者会差开，差得不大。
  const inQueue = (queueStatuses as readonly string[]).includes(status);
  const sort = chosenSort?.key === receivedKey ? chosenSort : inQueue ? oldestFirst : newestFirst;
  const conditions: CatalogueConditions = {
    ...base,
    sort: sort.dir === -1 ? `-${receivedKey}` : receivedKey,
    filters: status === allQueues ? baseFilters : { ...baseFilters, status: [status] },
  };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listUnclaimedArrivals);
  const held = heldCataloguePage(answer, previous, (body) => body.arrivals.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.arrivals : (held?.body.arrivals ?? []);
  const counts = useStatusCounts(listUnclaimedArrivals, base, 'status', queueStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };
  const now = new Date();
  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((item) => [item.warehouseCode, item.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const named = (code: string) => {
    const name = nameOf(warehouseNames, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };

  // 开了详情栏之后列表变窄，次要的列收起：到货号、状态、已放与下一步一直在。
  const columns: WorkbenchColumn<UnclaimedArrivalRecord>[] = [
    {
      key: 'id',
      header: '到货号',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[row.arrivalId]} />,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} /> },
    { key: 'warehouse', header: '仓库', width: fitColumnWidth, minListWidth: 980, render: (row) => named(row.warehouseCode) },
    { key: 'kind', header: '种类', width: fitColumnWidth, minListWidth: 1120, render: (row) => nowrap(labelOf(row.kind)) },
    {
      key: 'obs',
      header: '现场观察',
      minListWidth: 760,
      render: (row) =>
        row.observation ? <WmsTwoLine fill primary={row.observation} title={row.observation} /> : <span className="text-idpxyz-textMuted">—</span>,
    },
    {
      key: 'age',
      header: nowrap('已放'),
      right: true,
      width: fitColumnWidth,
      sortKey: receivedKey,
      // 已处置的不在等谁，不写放了多久；认领期限也是从到货发生时刻起算，所以这里同样从发生时刻起算。
      render: (row) =>
        row.status === 'DISPOSED' ? (
          <span className="whitespace-nowrap tabular-nums" title={`登记于 ${formatInstant(row.receivedAt)}`}>
            {shortInstant(row.receivedAt, now) || '—'}
          </span>
        ) : (
          <span className="whitespace-nowrap tabular-nums" title={`发生于 ${formatInstant(row.occurredAt)}`}>
            {elapsedLabel(row.occurredAt, now) || '—'}
          </span>
        ),
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => <NextStepCell row={row} rule={rules?.get(row.warehouseCode)} now={now} drawer={drawer} />,
    },
  ];

  const queueStats: WmsQueueStat[] = queueStatuses.map((code) => ({
    id: code,
    label: labelOf(code),
    count: counts[code] ?? null,
    active: status === code,
    onClick: () => setStatus(status === code ? allQueues : code),
  }));
  const homeQueue = status === 'UNCLAIMED' && chosenSort === null;
  const narrowed = settled.trim() !== '' || warehouse !== '' || kind !== '' || !homeQueue;
  const clearFilters = () => {
    setKeyword('');
    setWarehouse('');
    setKind('');
    setStatus('UNCLAIMED');
    setChosenSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-unclaimed-arrivals',
        emptyTitle: status === 'UNCLAIMED' ? '没有待认领到货' : '还没有待认领记录',
        emptyDescription: '在「仓内作业台」登记无主到货，或预报取消、完结后再收货时，记录会出现在这里。',
        narrowed: settled.trim() !== '' || warehouse !== '' || kind !== '' || (status !== 'UNCLAIMED' && status !== allQueues) || pastFirstPage(pagination),
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<UnclaimedArrivalRecord>
        title={info.title}
        viewStateKey={moduleId}
        subtitle="点一行在右边看这件到货、做它的下一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          <>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-settings'; }}>
              管理处置规则
            </Button>
            <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-workstation'; }}>
              去作业台登记到货
            </Button>
          </>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜到货号、仓或观察描述' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => setStatus(next === '' ? allQueues : next)}
              options={[...queueStatuses, ...settledStatuses].map((code) => ({ value: code, label: labelOf(code) }))}
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
            <WmsFilterSelect label="种类" value={kind} onChange={setKind} options={kindWords.map((word) => ({ value: word, label: labelOf(word) }))} />
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
        rowKey={(row) => row.arrivalId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAttention={(row) => {
          const rule = rules?.get(row.warehouseCode);
          return row.status === 'UNCLAIMED' && rule !== undefined && claimWindowElapsed(row.occurredAt, rule.claimWindowSeconds, now);
        }}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => setChosenSort(toggleWorkbenchSort(sort, key))}
        noMatchText="当前筛选下没有待认领到货"
        onClearFilters={clearFilters}
        renderDetail={(key) => (
          // 不按行重挂：重挂会把整栏卸掉再从加载骨架画起。换行留着上一件，新的读到再换。
          <UnclaimedDocument arrivalId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />
        )}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}

/** 每行最右的那一格：这件到货此刻该做的一步。能做的是一个钮，没有授予的只说在等什么。 */
function NextStepCell({
  row,
  rule,
  now,
  drawer,
}: {
  row: UnclaimedArrivalRecord;
  rule: DisposalRuleRecord | undefined;
  now: Date;
  drawer: ActionDrawerControl;
}) {
  const next = unclaimedNextStep(row, rule, now);
  if (next === null) return <span className="text-idpxyz-textMuted">—</span>;
  const actionId = next.kind === 'claim' ? 'unclaimed-claim' : next.kind === 'dispose' ? 'unclaimed-dispose' : '';
  if (actionId !== '' && !drawer.has(actionId)) return <span className="text-idpxyz-textMuted">{next.kind === 'claim' ? '等认领' : '等处置'}</span>;
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        // 行上的单击是开详情：这一下只做这一步，不顺带把详情栏开合一次。
        event.stopPropagation();
        if (next.kind === 'go') window.location.hash = `#/${next.moduleId}`;
        else drawer.open(actionId, next.kind === 'claim' ? claimPrefill(row) : { arrivalId: row.arrivalId });
      }}
    >
      {next.label}
    </Button>
  );
}
