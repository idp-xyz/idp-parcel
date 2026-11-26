import { useState } from 'react';
import { Plus } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { enterStation } from '../../shell/enter-station';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listWaves, type WaveRecord } from './api';
import { DocumentLink } from './document-page';
import { pickStationHash } from './station/stations';
import { openDocument, useDocumentParts } from './use-document';
import { useStatusCounts } from './use-status-counts';
import { activeWaveStatuses, settledWaveStatuses, waveProgress, waveStatusLabel } from './wave-flow';
import { WaveDocument } from './WaveDocument';
import { WmsBadge, WmsFilterSelect, WmsProgress, WmsStatusTabs } from './wms-ui';

const moduleId = 'warehouse-waves';
const info = moduleInfoById[moduleId];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<WaveRecord>[] = [
  { id: 'wave', header: '波次号', className: 'whitespace-nowrap', render: (row) => <DocumentLink moduleId={moduleId} parts={[row.waveId]} /> },
  { id: 'state', header: '状态', render: (row) => <WmsBadge code={row.status} label={waveStatusLabel(row.status)} /> },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'requests', header: '出库单', align: 'right', className: mono, render: (row) => `${row.requests.length} 张` },
  {
    id: 'progress',
    header: '已拣 / 任务',
    render: (row) => {
      const progress = waveProgress(row.tasks);
      return <WmsProgress done={progress.done + progress.short} total={progress.tasks} flagged={progress.short} flaggedLabel="少拣 " />;
    },
  },
  { id: 'prefetch', header: '预取面单', render: (row) => (row.prefetch ? '是' : '否') },
  { id: 'operator', header: '放波次人', className: mono, render: (row) => row.operator },
  { id: 'at', header: '放出时刻', className: mono, render: (row) => formatInstant(row.releasedAt) },
];

/** 波次：列表与每一个波次各占自己的标签。 */
export function WarehouseWavesPage() {
  const parts = useDocumentParts();
  return parts ? <WaveDocument waveId={parts[0]} /> : <WaveList />;
}

function WaveList() {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const [keyword, setKeyword] = useState('');
  const [warehouse, setWarehouse] = useState('');
  const [status, setStatus] = useState('ALL');
  const [refreshes, setRefreshes] = useState(0);
  const settled = useSettledKeyword(keyword);
  const base: CatalogueConditions = { q: settled, filters: warehouse ? { warehouseCode: [warehouse] } : {} };
  const conditions: CatalogueConditions = status === 'ALL' ? base : { ...base, filters: { ...base.filters, status: [status] } };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWaves);
  const rows = answer?.kind === 'outcome' ? answer.body.waves : [];
  // 只数还有任务等着拣的两种：已完成与已取消只增不减，数出来的不是待办。
  const counts = useStatusCounts(listWaves, base, 'status', activeWaveStatuses, refreshes);
  const refresh = () => {
    retry();
    setRefreshes((count) => count + 1);
  };

  const inspectorOf = (row: WaveRecord): InspectorContent => {
    const open = row.tasks.filter((task) => task.status === 'OPEN');
    const progress = waveProgress(row.tasks);
    const actions: InspectorAction[] = [
      { label: '打开波次', onRun: () => openDocument(moduleId, [row.waveId]) },
    ];
    // 已取消的波次没有下一步：任务都作废了。
    if (row.status !== 'CANCELLED') {
      actions.push({
        label: open.length > 0 ? `去拣货（还有 ${open.length} 格）` : '去复核打包',
        onRun: () => {
          // 还有没拣的格去全屏的拣货工位，只做这个波次；拣完了去复核打包，那几步还在旧的作业台页里
          if (open.length > 0) enterStation(pickStationHash(row.waveId));
          else window.location.hash = '#/warehouse-workstation';
        },
      });
    }
    return {
      title: row.waveId,
      subtitle: `${row.warehouseCode} · ${row.requests.length} 张出库单`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '出库单', value: row.requests.join('、') || '没有出库单', mono: true },
            { label: '状态', value: waveStatusLabel(row.status) },
            {
              label: '已拣 / 任务',
              value: `${progress.done + progress.short} / ${progress.tasks}${progress.short > 0 ? `（少拣 ${progress.short}）` : ''}${progress.cancelled > 0 ? `，另有 ${progress.cancelled} 条已作废` : ''}`,
              mono: true,
            },
            {
              label: '还没拣的格',
              value:
                open
                  .slice(0, 6)
                  .map((task) => `${task.locationCode} ${task.goodsCode}×${task.quantity}`)
                  .join('；') || '都拣完了',
              mono: true,
            },
            { label: '预取面单', value: row.prefetch ? '是' : '否' },
          ],
        },
        { kind: 'actions', actions },
        { kind: 'audit', fields: [{ label: '放波次人 / 时刻', value: `${row.operator} · ${formatInstant(row.releasedAt)}`, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<WaveRecord>
        moduleId={moduleId}
        title={info.title}
        description="放出去的每一个波次，和它拣到哪了。点波次号看每一条拣货任务；拣货本身在作业台做。"
        headerActions={
          <div className="flex flex-wrap gap-2">
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
              <Button type="button" className={primaryActionClass} onClick={() => drawer.open('wave-release')}>
                <Plus className="mr-1 h-3.5 w-3.5" />
                放波次
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜波次号、仓、放波次人' }}
        filters={
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            <WmsStatusTabs
              value={status}
              onChange={setStatus}
              tabs={[
                ...activeWaveStatuses.map((code) => ({ value: code, label: waveStatusLabel(code), count: counts[code] })),
                ...settledWaveStatuses.map((code) => ({ value: code, label: waveStatusLabel(code) })),
                { value: 'ALL', label: '全部' },
              ]}
            />
            <WmsFilterSelect
              label="仓库"
              value={warehouse}
              onChange={setWarehouse}
              options={drawer.lookups.warehouses.map((w) => ({ value: w.warehouseCode, label: `${w.warehouseCode} · ${w.name}` }))}
            />
          </div>
        }
        columns={columns}
        rows={rows}
        rowKey={(row) => row.waveId}
        inspector={inspectorOf}
        onRowOpen={(row) => openDocument(moduleId, [row.waveId])}
        pagination={pagination}
        emptyRowsNote="当前筛选下没有波次"
        viewState={catalogueViewState(answer, rows.length, refresh, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-wave-list',
          emptyTitle: '还没有放过波次',
          emptyDescription: '在「出库单」里勾选已预占的出库单合并放波次，或点右上角「放波次」。',
          narrowed: settled.trim() !== '' || warehouse !== '' || status !== 'ALL',
        })}
      />
      <ActionDrawer moduleId={moduleId} active={drawer.active} lookups={drawer.lookups} onClose={drawer.close} onSubmitted={refresh} />
    </>
  );
}
