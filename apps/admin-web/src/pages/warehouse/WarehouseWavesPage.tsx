import { useMemo, useState, type ReactNode } from 'react';
import { Plus, Send } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { enterStation } from '../../shell/enter-station';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { operatorLabel } from '../operator-access/operator-names';
import { useOperatorNames } from '../operator-access/use-operator-names';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listWaves, type WaveRecord } from './api';
import { DocumentLink } from './document-page';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { packStationHash, pickStationHash, sortStationHash } from './station/stations';
import { ToteList } from './ToteList';
import { WaveRuleList } from './WaveRuleList';
import { WaveRuleRunDialog } from './WaveRuleRunDialog';
import { useDocumentParts } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { activeWaveStatuses, settledWaveStatuses, wavePickingLabel, waveProgress, waveStatusLabel } from './wave-flow';
import { WaveDocument } from './WaveDocument';
import { WmsBadge, WmsFilterSelect, WmsProgress, WmsQueueStats, WmsTwoLine, type WmsQueueStat } from './wms-ui';

const moduleId = 'warehouse-waves';
const info = moduleInfoById[moduleId];
const allQueues = 'ALL';
const releasedKey = 'releasedAt';
const newestFirst: WorkbenchSort = { key: releasedKey, dir: -1 };
const oldestFirst: WorkbenchSort = { key: releasedKey, dir: 1 };

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

const knownStatus = (value: string) =>
  (activeWaveStatuses as readonly string[]).includes(value) || (settledWaveStatuses as readonly string[]).includes(value) ? value : allQueues;

/** 波次：列表与每一个波次各占自己的标签。周转箱名册只有列表，与波次列表并排成两个页签。 */
export function WarehouseWavesPage() {
  const parts = useDocumentParts();
  return parts ? <WaveDocument waveId={parts[0]} /> : <WavesHome />;
}

function WavesHome() {
  const [tab, setTab] = useState('waves');
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="waves">波次</TabsTrigger>
          <TabsTrigger value="totes">周转箱</TabsTrigger>
          <TabsTrigger value="rules">成波规则</TabsTrigger>
        </TabsList>
        <TabsContent value="waves" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <WaveList />
        </TabsContent>
        <TabsContent value="totes" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ToteList />
        </TabsContent>
        <TabsContent value="rules" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <WaveRuleList />
        </TabsContent>
      </Tabs>
    </div>
  );
}

function WaveList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [keyword, setKeyword] = useTabViewState('warehouse-waves.keyword', '');
  const [warehouse, setWarehouse] = useTabViewState('warehouse-waves.warehouse', '');
  const [storedStatus, setStatus] = useTabViewState('warehouse-waves.status', allQueues);
  const status = knownStatus(storedStatus);
  const [chosenSort, setChosenSort] = useTabViewState<WorkbenchSort | null>('warehouse-waves.sort', null);
  const [openKey, setOpenKey] = useTabViewState<string | null>('warehouse-waves.open', null);
  const [ruleRunOpen, setRuleRunOpen] = useState(false);
  const [refreshes, setRefreshes] = useState(0);
  const names = useOperatorNames();
  const settled = useSettledKeyword(keyword);
  // 看还有任务等着拣的那两队时，先放出的在上面：放得越早的越该先拣。看全部或已了结的，最新放出的在上面。人点过表头就照他的。
  const working = (activeWaveStatuses as readonly string[]).includes(status);
  const sort = chosenSort ?? (working ? oldestFirst : newestFirst);
  const filters: Record<string, string[]> = {};
  if (warehouse) filters.warehouseCode = [warehouse];
  if (status !== allQueues) filters.status = [status];
  const conditions: CatalogueConditions = { q: settled, filters, sort: sort.dir === -1 ? '-releasedAt' : 'releasedAt' };
  const base: CatalogueConditions = { q: settled, filters: warehouse ? { warehouseCode: [warehouse] } : {} };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listWaves);
  const held = heldCataloguePage(answer, previous, (body) => body.waves.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.waves : (held?.body.waves ?? []);
  // 只数还有任务等着拣的两种：已完成与已取消只增不减，数出来的不是待办。
  const counts = useStatusCounts(listWaves, base, 'status', activeWaveStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

  const warehouseNames = useMemo(
    () => new Map(drawer.lookups.warehouses.map((w) => [w.warehouseCode, w.name ?? ''])),
    [drawer.lookups.warehouses],
  );
  const now = new Date();
  const named = (code: string) => {
    const name = nameOf(warehouseNames, code);
    return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
  };
  // 开了详情栏之后列表变窄，次要的列收起：波次号、状态、进度与下一步一直在。
  const columns: WorkbenchColumn<WaveRecord>[] = [
    {
      key: 'wave',
      header: '波次',
      width: fitColumnWidth,
      render: (row) => <DocumentLink moduleId={moduleId} parts={[row.waveId]}>{row.waveId}</DocumentLink>,
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={waveStatusLabel(row.status)} /> },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 980, render: (row) => named(row.warehouseCode) },
    {
      key: 'profile',
      header: '件型',
      width: fitColumnWidth,
      minListWidth: 1120,
      hideWhenDetailOpen: true,
      render: (row) => nowrap(row.singleUnit ? '单品单件' : '—'),
    },
    { key: 'picking', header: '拣货方式', width: fitColumnWidth, minListWidth: 820, render: (row) => nowrap(wavePickingLabel(row)) },
    {
      key: 'rule',
      header: '成波规则',
      width: fitColumnWidth,
      minListWidth: 1240,
      hideWhenDetailOpen: true,
      render: (row) => nowrap(row.ruleCode ?? '—'),
    },
    {
      key: 'assignee',
      header: '拣货人',
      width: fitColumnWidth,
      minListWidth: 900,
      render: (row) => nowrap(operatorLabel(row.assignee, names) ?? '—'),
    },
    {
      key: 'requests',
      header: nowrap('出库单'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1060,
      render: (row) =>
        nowrap(row.withdrawnRequests.length > 0 ? `${row.requests.length} 张（撤出 ${row.withdrawnRequests.length}）` : `${row.requests.length} 张`),
    },
    {
      key: 'progress',
      header: '已拣 / 任务',
      render: (row) => {
        const progress = waveProgress(row.tasks);
        return <WmsProgress done={progress.done + progress.short} total={progress.tasks} flagged={progress.short} flaggedLabel="少拣 " />;
      },
    },
    {
      key: 'prefetch',
      header: '预取面单',
      width: fitColumnWidth,
      minListWidth: 1360,
      hideWhenDetailOpen: true,
      render: (row) => (row.prefetch ? '是' : '否'),
    },
    {
      key: 'operator',
      header: '放波次人',
      width: fitColumnWidth,
      minListWidth: 1480,
      hideWhenDetailOpen: true,
      render: (row) => nowrap(names.get(row.operator) ?? row.operator),
    },
    {
      key: 'at',
      header: nowrap('放出时刻'),
      width: fitColumnWidth,
      sortKey: releasedKey,
      minListWidth: 760,
      render: (row) => nowrap(shortInstant(row.releasedAt, now) || '—'),
    },
    { key: 'next', header: nowrap('下一步'), width: fitColumnWidth, render: (row) => <WaveNextStep row={row} /> },
  ];

  const queueStats: WmsQueueStat[] = activeWaveStatuses.map((code) => ({
    id: code,
    label: waveStatusLabel(code),
    count: counts[code] ?? null,
    active: status === code,
    onClick: () => setStatus(status === code ? allQueues : code),
  }));
  const narrowed = settled.trim() !== '' || warehouse !== '' || status !== allQueues;
  const clearFilters = () => {
    setKeyword('');
    setWarehouse('');
    setStatus(allQueues);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, refresh, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-wave-list',
        emptyTitle: '还没有放过波次',
        emptyDescription: '在「出库单」里勾选已预占的出库单合并放波次，或点右上角「放波次」。',
        narrowed: narrowed || pastFirstPage(pagination),
      });
  const shownPagination = pagination ?? held?.pagination;

  return (
    <>
      <WorkbenchPageTemplate<WaveRecord>
        title={info.title}
        viewStateKey={moduleId}
        subtitle="点一行在右边看这个波次、做它的下一步"
        titleAside={<WmsQueueStats items={queueStats} />}
        onRefresh={refresh}
        refreshing={answer === null}
        primaryAction={
          <>
            {drawer.has('confirmation-correction') ? (
              <Button type="button" variant="outline" onClick={() => drawer.open('confirmation-correction')}>
                更正出库确认
              </Button>
            ) : null}
            {drawer.has('confirmation-replenishment') ? (
              <Button type="button" variant="outline" onClick={() => drawer.open('confirmation-replenishment')}>
                批准确认回补
              </Button>
            ) : null}
            {drawer.has('wave-release') ? (
              <Button type="button" variant="outline" onClick={() => setRuleRunOpen(true)}>
                <Send className="mr-1 h-3.5 w-3.5" />
                按规则成波
              </Button>
            ) : null}
            {drawer.has('wave-release') ? (
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('wave-release')}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                放波次
              </Button>
            ) : null}
          </>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜波次号、仓、放波次人' }}
        extraFilters={
          <>
            <WmsFilterSelect
              label="状态"
              value={status === allQueues ? '' : status}
              onChange={(next) => setStatus(next === '' ? allQueues : next)}
              options={[...activeWaveStatuses, ...settledWaveStatuses].map((code) => ({ value: code, label: waveStatusLabel(code) }))}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: w.name ? `${w.name} · ${w.warehouseCode}` : w.warehouseCode }))}
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
        rowKey={(row) => row.waveId}
        selectedKey={openKey}
        onSelect={setOpenKey}
        rowAlign="baseline"
        sort={sort}
        onSort={(key) => setChosenSort(toggleWorkbenchSort(sort, key))}
        noMatchText="当前筛选下没有波次"
        onClearFilters={clearFilters}
        renderDetail={(key) => <WaveDocument key={key} waveId={key} panel={{ onClose: () => setOpenKey(null), onChanged: refresh, revision: refreshes }} />}
        viewState={viewState}
        listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
      <WaveRuleRunDialog
        open={ruleRunOpen}
        onClose={() => setRuleRunOpen(false)}
        onReleased={refresh}
        warehouseNames={warehouseNames}
        defaultWarehouse={warehouse || undefined}
      />
    </>
  );
}

/** 这一行此刻该做的一步。已取消没有下一步。点这一下只去工位，不把详情栏开合一次。 */
function WaveNextStep({ row }: { row: WaveRecord }) {
  const open = row.tasks.some((task) => task.status === 'OPEN');
  if (row.status === 'CANCELLED') return <span className="text-idpxyz-textMuted">—</span>;
  const label = open ? '去拣货' : row.batchPick ? '去分拣' : '去复核打包';
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        if (open) enterStation(pickStationHash(row.waveId));
        else if (row.batchPick) enterStation(sortStationHash(row.waveId));
        else enterStation(packStationHash());
      }}
    >
      {label}
    </Button>
  );
}
