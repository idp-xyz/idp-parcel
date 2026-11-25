import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { enterStation } from '../../shell/enter-station';
import type { ApiResult } from '../catalogue-api';
import { formatInstant } from '../catalogue-view';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { listWaves, type WaveRecord } from './api';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { openPrint } from './print-sheet';
import { pickListSheet } from './print-forms';
import { packStationHash, pickStationHash } from './station/stations';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { useReturnToList } from './use-document';
import { WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsSubTable } from './wms-ui';

/**
 * 波次没有按号取一张的读口，只有列表：按波次号检索，再在答回来的那一页里认出号完全相同的那一个。检索是按包含匹配的，
 * 号恰好是另一批波次号的一段、而那一批多到挤出第一页时会认不出，答找不到。
 */
async function loadWave(waveId: string): Promise<ApiResult<{ wave?: WaveRecord }>> {
  const answer = await listWaves({ q: waveId });
  if (answer.kind !== 'outcome') return answer;
  return { ...answer, body: { wave: answer.body.waves.find((wave) => wave.waveId === waveId) } };
}

/** 波次页：一次放出去的那一批出库单，和它们的每一条拣货任务拣到哪了。 */
export function WaveDocument({ waveId }: { waveId: string }) {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const back = useReturnToList('warehouse-waves');
  const { answer, loading, reload } = useKeyedLoad(waveId, loadWave);
  const wave = answer?.kind === 'outcome' ? answer.body.wave : undefined;
  const failure = readFailureMessage(answer) ?? (answer?.kind === 'outcome' && !wave ? `没有 ${waveId} 这个波次。` : null);

  const tasks = wave?.tasks ?? [];
  const open = tasks.filter((task) => task.status === 'OPEN');
  const short = tasks.filter((task) => task.status === 'SHORT');
  const allPicked = tasks.length > 0 && open.length === 0;
  // 余下的格按库位排：拣货的人顺着库位走，不照任务生成的先后。
  const ordered = [...tasks].sort((a, b) => {
    if ((a.status === 'OPEN') !== (b.status === 'OPEN')) return a.status === 'OPEN' ? -1 : 1;
    return a.locationCode.localeCompare(b.locationCode);
  });

  return (
    <>
      <WmsDocumentPage
        kind="波次"
        title={waveId}
        subtitle={wave ? `仓 ${wave.warehouseCode} · ${wave.requests.length} 张出库单 · 放出于 ${formatInstant(wave.releasedAt)}` : undefined}
        badge={wave ? <WmsBadge label={allPicked ? '拣货完成' : '拣货中'} tone={allPicked ? 'success' : 'info'} /> : undefined}
        loading={loading}
        error={failure}
        onBack={back}
        onReload={reload}
        actions={
          wave ? (
            <>
              {drawer.has('confirmation-correction') ? (
                <Button type="button" variant="outline" onClick={() => drawer.open('confirmation-correction')}>
                  更正出库确认
                </Button>
              ) : null}
              <Button type="button" variant="outline" onClick={() => openPrint(pickListSheet(waveId, ordered))}>
                打印拣货单
              </Button>
              <Button
                type="button"
                className={primaryActionClass}
                onClick={() => {
                  if (open.length > 0) enterStation(pickStationHash(waveId));
                  else enterStation(packStationHash());
                }}
              >
                {open.length > 0 ? '去拣货' : '去复核打包'}
              </Button>
            </>
          ) : undefined
        }
        aside={
          wave ? (
            <>
              <WmsAsideBlock title="这个波次里的出库单">
                {wave.requests.length === 0 ? <span className="text-idpxyz-textMuted">没有出库单。</span> : null}
                {wave.requests.map((requestId) => (
                  <DocumentLink key={requestId} moduleId="warehouse-outbound" parts={[requestId]} />
                ))}
              </WmsAsideBlock>
              <WmsAsideBlock title="放波次">
                <span className="break-all font-mono type-caption text-idpxyz-textMuted">操作者 {wave.operator}</span>
                <span className="type-caption text-idpxyz-textMuted">
                  {wave.prefetch ? '放出时为各单预取了面单' : '放出时没有预取面单'}
                </span>
              </WmsAsideBlock>
            </>
          ) : undefined
        }
      >
        {wave ? (
          <>
            <WmsKpiStrip
              items={[
                { label: '拣货任务', value: tasks.length, note: `${wave.requests.length} 张出库单` },
                { label: '还没拣', value: open.length, tone: open.length > 0 ? 'warning' : 'success' },
                { label: '已拣', value: tasks.length - open.length - short.length },
                { label: '短拣', value: short.length, tone: short.length > 0 ? 'danger' : undefined },
              ]}
            />
            <WmsInfoGrid
              items={[
                { label: '仓库', value: wave.warehouseCode, mono: true },
                { label: '预取面单', value: wave.prefetch ? '是' : '否' },
                { label: '放出时刻', value: formatInstant(wave.releasedAt), mono: true },
                { label: '放波次人', value: wave.operator, mono: true },
              ]}
            />
            <WmsSubTable
              title="拣货任务"
              count={tasks.length}
              extra={<span className="type-caption text-idpxyz-textMuted">还没拣的在前，按库位排</span>}
              emptyText="这个波次没有拣货任务。"
              rows={ordered}
              rowKey={(task) => task.taskId}
              columns={[
                { id: 'loc', header: '库位', mono: true, render: (task) => task.locationCode },
                { id: 'goods', header: '商品', mono: true, render: (task) => task.goodsCode },
                { id: 'state', header: '库存状态', render: (task) => <WmsBadge code={task.stockState} /> },
                { id: 'qty', header: '已拣 / 应拣', align: 'right', mono: true, render: (task) => `${task.picked} / ${task.quantity} ${task.unit}` },
                { id: 'status', header: '状态', render: (task) => <WmsBadge code={task.status} /> },
                { id: 'stage', header: '暂存位', mono: true, render: (task) => task.stagingLocation || '—' },
                { id: 'req', header: '出库单', render: (task) => <DocumentLink moduleId="warehouse-outbound" parts={[task.requestId]} /> },
                { id: 'task', header: '任务', mono: true, render: (task) => task.taskId },
              ]}
            />
          </>
        ) : null}
      </WmsDocumentPage>
      <ActionDrawer
        moduleId="warehouse-waves"
        active={drawer.active}
        lookups={drawer.lookups}
        onClose={drawer.close}
        onSubmitted={reload}
      />
    </>
  );
}
