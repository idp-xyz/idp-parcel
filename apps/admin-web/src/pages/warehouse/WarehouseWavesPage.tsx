import { useState } from 'react';
import { Plus, TabletSmartphone } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { listWaves, type WaveRecord } from './api';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';
import { WaveDetailModal } from './document-modals';

const info = moduleInfoById['warehouse-waves'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const columns: ListColumn<WaveRecord>[] = [
  { id: 'wave', header: '波次号', className: mono, render: (row) => row.waveId },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouseCode },
  { id: 'requests', header: '含出库单', className: mono, render: (row) => `${row.requests.length} 单` },
  {
    id: 'progress',
    header: '拣货进度',
    className: mono,
    render: (row) => {
      const done = row.tasks.filter((t) => t.status !== 'OPEN').length;
      return `${done} / ${row.tasks.length}`;
    },
  },
  { id: 'prefetch', header: '预取面单', render: (row) => (row.prefetch ? '是' : '否') },
  { id: 'operator', header: '放波次人', className: mono, render: (row) => row.operator },
  { id: 'at', header: '释放时刻', className: mono, render: (row) => formatInstant(row.releasedAt) },
];

/** 波次与拣货台账页：追踪每个波次的拣货进度，支持直接释放新波次、跳转作业台拣货车，以及更正出库确认与批准确认回补。 */
export function WarehouseWavesPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(outboundWaveActionSpecs);

  const [keyword, setKeyword] = useState('');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [opened, setOpened] = useState<WaveRecord | null>(null);
  const settled = useSettledKeyword(keyword);
  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listWaves);
  const rows = answer?.kind === 'outcome' ? answer.body.waves : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const inspectorOf = (row: WaveRecord): InspectorContent => {
    const openTasks = row.tasks.filter((t) => t.status === 'OPEN');
    const previewTasks = row.tasks
      .slice(0, 4)
      .map((t) => `${t.taskId}(${t.locationCode} ${t.goodsCode}×${t.quantity} ${t.status === 'OPEN' ? '待拣' : t.status})`)
      .join('；');

    const actions: InspectorAction[] = [
      { label: '查看拣货明细', onRun: () => setOpened(row) },
      {
        label: openTasks.length > 0 ? `去作业台拣货车（余 ${openTasks.length} 项）` : '去作业台复核打包',
        onRun: () => {
          window.location.hash = '#/warehouse-workstation';
        },
      },
    ];
    if (targetMap.has('confirmation-correction')) {
      actions.push({
        label: '更正出库确认',
        onRun: () => openAction('confirmation-correction'),
      });
    }

    return {
      title: row.waveId,
      subtitle: `${row.warehouseCode} · ${row.requests.length} 单`,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '出库单列表', value: row.requests.join(', '), mono: true },
            { label: '待拣 / 总任务', value: `${openTasks.length} / ${row.tasks.length}`, mono: true },
            { label: '预取面单', value: row.prefetch ? '已开启' : '未开启' },
            { label: '拣货任务预览', value: previewTasks || '无拣货任务' },
          ],
        },
        { kind: 'actions', actions },
        { kind: 'audit', fields: [{ label: '释放人 / 时刻', value: `${row.operator} · ${formatInstant(row.releasedAt)}`, mono: true }] },
      ],
    };
  };

  return (
    <>
      <ListPageTemplate<WaveRecord>
        moduleId="warehouse-waves"
        title={info.title}
        description="已释放波次与拣货任务。双击看每一条拣货任务的库位、应拣和已拣。"
        headerActions={
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                window.location.hash = '#/warehouse-workstation';
              }}
            >
              <TabletSmartphone className="h-3.5 w-3.5 mr-1" />
              打开拣货车
            </Button>
            {targetMap.has('confirmation-correction') ? (
              <Button type="button" variant="outline" onClick={() => openAction('confirmation-correction')}>
                更正出库确认
              </Button>
            ) : null}
            {targetMap.has('confirmation-replenishment') ? (
              <Button type="button" variant="outline" onClick={() => openAction('confirmation-replenishment')}>
                批准确认回补
              </Button>
            ) : null}
            {targetMap.has('wave-release') ? (
              <Button type="button" className={primaryActionClass} onClick={() => openAction('wave-release')}>
                <Plus className="h-3.5 w-3.5 mr-1" />
                释放新波次
              </Button>
            ) : null}
          </div>
        }
        search={{ value: keyword, onChange: setKeyword, placeholder: '搜波次号或仓库' }}
        columns={columns}
        rows={rows}
        rowKey={(row) => row.waveId}
        inspector={inspectorOf}
        onRowOpen={setOpened}
        pagination={pagination}
        emptyRowsNote="当前条件下没有匹配的波次"
        viewState={catalogueViewState(answer, rows.length, retry, {
          module: info,
          endpoint: 'GET /warehouse-fulfillment-wave-list',
          emptyTitle: '还没有释放波次',
          emptyDescription: '可在「出库单」勾选「已预占」的出库单合并放波次，或点右上角「释放新波次」。',
          narrowed: settled.trim() !== '',
        })}
      />
      <WaveDetailModal row={opened} onClose={() => setOpened(null)} />
      <ActionDrawer
        moduleId="warehouse-waves"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => retry()}
      />
    </>
  );
}
