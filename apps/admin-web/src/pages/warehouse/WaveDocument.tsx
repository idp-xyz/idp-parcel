import { Button } from '@idpxyz/ui-primitives';
import { useEffect, useRef, useState } from 'react';
import { primaryActionClass } from '../../components/primary-action';
import { enterStation } from '../../shell/enter-station';
import { formatInstant } from '../catalogue-view';
import { outboundWaveActionSpecs } from './actions';
import { ActionDrawer, useActionDrawer } from './ActionDrawer';
import { getWave } from './api';
import { DocumentLink, WmsAsideBlock, WmsDocumentPage } from './document-page';
import { openPrint } from './print-sheet';
import { pickListSheet } from './print-forms';
import { packStationHash, pickStationHash, sortStationHash } from './station/stations';
import { readFailureMessage, useKeyedLoad } from './use-detail';
import { openDocument, useReturnToList } from './use-document';
import { waveZones } from './wave-assign';
import { WaveAssignDialog } from './WaveAssignDialog';
import { operatorLabel } from '../operator-access/operator-names';
import { useOperatorNames } from '../operator-access/use-operator-names';
import { wavePickingNote, waveProgress, waveStatusLabel, waveTrack, withdrawableRequests } from './wave-flow';
import { WmsBadge, WmsInfoGrid, WmsKpiStrip, WmsStepper, WmsSubTable } from './wms-ui';

/**
 * 波次页：一次放出去的那一批出库单，和它们的每一条拣货任务拣到哪了。
 *
 * 同一张页也摆在波次列表的详情栏里（给了 `panel`）：点一行就在栏里看这个波次。栏里做完一个动作告诉列表重取，
 * 列表重取一次 `revision` 加一，栏见它变了就重读——不然行上已经拣完了，栏里还摆着「去拣货」。
 */
export function WaveDocument({
  waveId,
  panel,
}: {
  waveId: string;
  panel?: { onClose: () => void; onChanged: () => void; revision: number };
}) {
  const drawer = useActionDrawer(outboundWaveActionSpecs);
  const back = useReturnToList('warehouse-waves');
  const [assignOpen, setAssignOpen] = useState(false);
  const names = useOperatorNames();
  const { answer, loading, reload, held, reloading } = useKeyedLoad(waveId, getWave, { holdPrevious: panel !== undefined });
  const shownId = useRef(waveId);
  useEffect(() => {
    if (shownId.current === waveId) return;
    shownId.current = waveId;
    setAssignOpen(false);
    drawer.close();
  }, [waveId, drawer]);
  const revision = panel?.revision;
  const readAt = useRef(revision);
  useEffect(() => {
    if (readAt.current === revision) return;
    readAt.current = revision;
    reload();
  }, [revision, reload]);
  const afterWrite = panel ? panel.onChanged : reload;
  const wave = answer?.kind === 'outcome' ? answer.body.wave : undefined;
  const failure = readFailureMessage(answer) ?? (answer?.kind === 'outcome' && !wave ? `没有 ${waveId} 这个波次。` : null);

  const tasks = wave?.tasks ?? [];
  const open = tasks.filter((task) => task.status === 'OPEN');
  const progress = waveProgress(tasks);
  const track = wave ? waveTrack(wave.status) : undefined;
  const withdrawn = new Set(wave?.withdrawnRequests ?? []);
  const withdrawable = wave ? withdrawableRequests(wave) : [];
  const canWithdraw = drawer.has('wave-withdraw');
  const withdraw = (requests: readonly string[]) => drawer.open('wave-withdraw', { waveId, requests: requests.join(', ') });
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
        badge={wave ? <WmsBadge code={wave.status} label={waveStatusLabel(wave.status)} /> : undefined}
        stepper={
          track ? (
            <WmsStepper
              steps={track.steps}
              currentId={track.currentId}
              currentLabel={track.currentLabel}
              failedLabel={track.halt?.label}
              failedTone={track.halt?.tone}
            />
          ) : undefined
        }
        loading={held ? false : loading}
        reloading={reloading}
        busy={held}
        error={failure}
        onBack={back}
        onReload={reload}
        panel={panel ? { onClose: panel.onClose, onOpenPage: () => openDocument('warehouse-waves', [waveId]) } : undefined}
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
              {/* 已取消的波次没有下一步：任务都作废了，没有可拣的，也没有拣出来等复核的。 */}
              {/* 合并拣货的波次：拣货的人一边拿回来，分拣的人就可以一边分，所以还没拣完时也摆「去分拣」。 */}
              {wave.status !== 'CANCELLED' && wave.batchPick && open.length > 0 ? (
                <Button type="button" variant="outline" onClick={() => enterStation(sortStationHash(waveId))}>
                  去分拣
                </Button>
              ) : null}
              {wave.status === 'CANCELLED' ? null : (
                <Button
                  type="button"
                  className={primaryActionClass}
                  onClick={() => {
                    if (open.length > 0) enterStation(pickStationHash(waveId));
                    else if (wave.batchPick) enterStation(sortStationHash(waveId));
                    else enterStation(packStationHash());
                  }}
                >
                  {open.length > 0 ? '去拣货' : wave.batchPick ? '去分拣' : '去复核打包'}
                </Button>
              )}
            </>
          ) : undefined
        }
        aside={
          wave ? (
            <>
              <WmsAsideBlock title="这个波次里的出库单">
                {wave.requests.length === 0 ? <span className="text-idpxyz-textMuted">没有出库单。</span> : null}
                {wave.requests.map((requestId) => (
                  <div key={requestId} className="flex min-w-0 items-center justify-between gap-2">
                    <DocumentLink moduleId="warehouse-outbound" parts={[requestId]} />
                    {withdrawn.has(requestId) ? (
                      <span className="shrink-0 type-caption text-idpxyz-textMuted">已撤出</span>
                    ) : canWithdraw && withdrawable.includes(requestId) ? (
                      <Button type="button" variant="ghost" size="sm" className="shrink-0" onClick={() => withdraw([requestId])}>
                        撤出
                      </Button>
                    ) : null}
                  </div>
                ))}
                {canWithdraw && withdrawable.length > 1 ? (
                  <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => withdraw(withdrawable)}>
                    把还没拣的 {withdrawable.length} 张都撤出
                  </Button>
                ) : null}
                {canWithdraw && withdrawable.length > 0 ? (
                  <span className="type-caption text-idpxyz-textMuted">撤出后出库单回到已预占，可以再放进别的波次；拣过的单撤不了，要走截单。</span>
                ) : null}
              </WmsAsideBlock>
              <WmsAsideBlock title="拣货人">
                {/* 示姓名（没登姓名的示 sub）；完整的操作者标识在悬停里。 */}
                <span className="break-all font-mono type-caption text-idpxyz-text" title={wave.assignee}>
                  {operatorLabel(wave.assignee, names) ?? '还没派，谁都可以拣'}
                </span>
                {/* 单独派了人的库区逐个列出；没列出的库区照上面整个波次的指派走。 */}
                {(wave.zoneAssignees ?? []).map((entry) => (
                  <span key={entry.zoneCode} className="break-all font-mono type-caption text-idpxyz-text" title={entry.assignee}>
                    库区 {entry.zoneCode}：{operatorLabel(entry.assignee, names)}
                  </span>
                ))}
                {/* 拣完或取消的波次没有可派的任务。 */}
                {drawer.has('wave-assign') && (wave.status === 'RELEASED' || wave.status === 'PICKING') ? (
                  <Button type="button" size="sm" variant="outline" onClick={() => setAssignOpen(true)}>
                    {wave.assignee ? '改派或取消指派' : '指派拣货人'}
                  </Button>
                ) : null}
                <span className="type-caption text-idpxyz-textMuted">
                  派了之后别人拣这个波次的任务后端不收；拣货的人也可以在拣货工位上自己领。任务分在几个库区时，可以按库区各派一个人。
                </span>
              </WmsAsideBlock>
              <WmsAsideBlock title="放波次">
                <span className="break-all font-mono type-caption text-idpxyz-textMuted" title={wave.operator}>
                  操作者 {names.get(wave.operator) ?? wave.operator}
                </span>
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
                {
                  label: '拣货任务',
                  value: progress.tasks,
                  note: progress.cancelled > 0 ? `${wave.requests.length} 张出库单，另有 ${progress.cancelled} 条已作废` : `${wave.requests.length} 张出库单`,
                },
                {
                  label: '还没拣',
                  value: progress.open,
                  tone: progress.open > 0 ? 'warning' : 'success',
                  // 百分比把少拣也算有了结果，所以摆在「还没拣」这一格下面，不摆在只数拣满的「已拣」下面。
                  note: progress.tasks > 0 ? `有结果的占 ${progress.percent}%` : undefined,
                },
                { label: '已拣', value: progress.done },
                { label: '短拣', value: progress.short, tone: progress.short > 0 ? 'danger' : undefined },
              ]}
            />
            <WmsInfoGrid
              items={[
                { label: '仓库', value: wave.warehouseCode, mono: true },
                { label: '预取面单', value: wave.prefetch ? '是' : '否' },
                { label: '件型', value: wave.singleUnit ? '单品单件：每张单只拣一件，同库位同商品合成一格拿' : '不是单品单件' },
                { label: '拣货方式', value: wavePickingNote(wave) },
                { label: '怎么成的波', value: wave.ruleCode ? `按成波规则 ${wave.ruleCode} 放出` : '由人点单放出' },
                { label: '放出时刻', value: formatInstant(wave.releasedAt), mono: true },
                { label: '放波次人', value: names.get(wave.operator) ?? wave.operator, mono: true },
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
        onSubmitted={afterWrite}
      />
      {wave ? (
        <WaveAssignDialog
          open={assignOpen}
          onClose={() => setAssignOpen(false)}
          onAssigned={afterWrite}
          waveId={waveId}
          warehouse={wave.warehouseCode}
          assignee={wave.assignee}
          zones={waveZones(wave.tasks)}
          zoneAssignees={wave.zoneAssignees}
          names={names}
        />
      ) : null}
    </>
  );
}
