import { useEffect, useState } from 'react';
import { ReloginButton } from '../../../auth/ReloginButton';
import { TechnicalDetails, httpRows, type TechnicalRow } from '../../../components/TechnicalDetails';
import { statusToneBanner } from '../../../components/status-tone';
import type { WorkDeviceRecord } from '../../work-devices/api';
import { formatInstant } from '../../moment';
import { listWaves } from '../api';
import { waveStatusLabel } from '../wave-flow';
import { ScanBox } from './ScanBox';
import { panelClass, ScanFeedback } from './ScanFeedback';
import {
  acceptingScans,
  orderLeft,
  orderName,
  orderOpen,
  orderSorted,
  scanPlaceholder,
  sortActions,
  sortProgress,
  sortScreen,
  sortSummary,
  type SortOrder,
  type SortState,
} from './sort-flow';
import { loadSortWaves, type SortWavesAnswer } from './sort-plan';
import { StationFrame, stationActionClass, stationButtonClass, stationPrimaryClass } from './StationFrame';
import { packStationHash, sortStationHash } from './stations';
import { useSortSession } from './use-sort-session';
import { useStationSound } from './use-station-sound';
import type { WorkFactVerdict } from './work-fact-submit';

// 分拣工位（地址 #/station/sort，可带 ?wave=）。屏上的字、扫码的顺序与每一步的判定都在 sort-flow.ts，这里只摆与接线：
// 左边最大的字是下一步，下面是扫描框、反馈和按钮；右边是这个波次里各张单的箱与进度。没带波次时先选一个合并拣货的波次。

function VerdictExtras({ verdict, state }: { verdict: WorkFactVerdict | null; state: SortState }) {
  if (verdict === null || (state.feedback.kind !== 'wrong' && state.feedback.kind !== 'unsent')) return null;
  const rows: TechnicalRow[] = [];
  if (verdict.technical?.status !== undefined) rows.push(...httpRows(verdict.technical.status, verdict.technical.code ?? ''));
  else if (verdict.technical?.code) rows.push({ label: '答复', value: verdict.technical.code });
  if (verdict.technical?.message) rows.push({ label: '网络错误', value: verdict.technical.message });
  if (rows.length === 0 && verdict.access !== 'loginRequired') return null;
  return (
    <div className="flex flex-col gap-2">
      {verdict.access === 'loginRequired' ? (
        <div>
          <ReloginButton />
        </div>
      ) : null}
      <TechnicalDetails rows={rows} className="text-idpxyz-textMuted" />
    </div>
  );
}

function orderNote(order: SortOrder): string {
  if (orderSorted(order)) return '分完了';
  const left = orderLeft(order);
  const open = orderOpen(order);
  if (left === 0) return open > 0 ? `等拣货 ${open} 件` : '还没拣';
  return open > 0 ? `还差 ${left} 件，另有 ${open} 件没拣回来` : `还差 ${left} 件`;
}

function OrderTable({ state }: { state: SortState }) {
  if (state.orders.length === 0) return <p className="type-body text-idpxyz-textMuted">这个波次没有要分的单。</p>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse type-body">
        <thead>
          <tr className="text-left text-idpxyz-textMuted">
            <th className="py-1.5 pr-3 font-medium">出库单</th>
            <th className="py-1.5 pr-3 font-medium">周转箱</th>
            <th className="py-1.5 pr-3 text-right font-medium">已分 / 已拣</th>
            <th className="py-1.5 font-medium">进度</th>
          </tr>
        </thead>
        <tbody>
          {state.orders.map((order) => {
            const current = state.target?.requestId === order.requestId;
            const picked = order.lines.reduce((sum, line) => sum + line.picked, 0);
            const sorted = order.lines.reduce((sum, line) => sum + line.sorted, 0);
            return (
              <tr key={order.requestId} className={current ? 'bg-idpxyz-activeItem' : undefined} aria-current={current ? 'true' : undefined}>
                <td className="break-all border-t border-idpxyz-border py-2 pr-3 font-mono">
                  {orderName(order)}
                  {current ? <span className="block font-sans type-caption text-idpxyz-accent">手里这一件归它</span> : null}
                </td>
                <td className="break-all border-t border-idpxyz-border py-2 pr-3 font-mono">{order.totes.length === 0 ? '还没有箱' : order.totes.join('、')}</td>
                <td className="whitespace-nowrap border-t border-idpxyz-border py-2 pr-3 text-right font-mono">
                  {sorted} / {picked}
                </td>
                <td className="border-t border-idpxyz-border py-2">{orderNote(order)}</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

/** 没带波次：列本仓合并拣货的波次，点一个进去；也可以扫波次号。 */
function SortWaveChooser({ device, onChangeDevice }: { device: WorkDeviceRecord; onChangeDevice?: () => void }) {
  const warehouse = device.workScopeRef;
  const [answer, setAnswer] = useState<SortWavesAnswer | null>(null);
  const [tick, setTick] = useState(0);
  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void loadSortWaves(warehouse, listWaves).then((loaded) => {
      if (!cancelled) setAnswer(loaded);
    });
    return () => {
      cancelled = true;
    };
  }, [warehouse, tick]);
  return (
    <StationFrame
      title="分拣"
      device={{ id: device.deviceId, warehouse }}
      onChangeDevice={onChangeDevice}
      context="还没选定波次"
      left={
        <div className="flex flex-col gap-4">
          <div>
            <h2 className="type-display font-semibold leading-tight text-idpxyz-textBright">选一个波次，或扫波次号</h2>
            <p className="mt-2 type-body text-idpxyz-textMuted">
              只有放出时选了合并拣货的波次要分拣：它的货是几张单合在一起拿回来的，在这里一件一件分进各张单的周转箱，分完才能复核打包。
            </p>
          </div>
          <ScanBox
            onScan={(raw) => {
              const wave = raw.trim();
              if (wave !== '') window.location.hash = sortStationHash(wave);
            }}
            placeholder="扫波次号，或敲进去按回车"
            busy={false}
            suspended={false}
            tone="waiting"
          />
          {answer === null ? (
            <p role="status" className="type-body text-idpxyz-textMuted">
              正在读本仓合并拣货的波次…
            </p>
          ) : answer.kind === 'failed' ? (
            <p role="alert" className={`rounded border px-3 py-2 type-body ${statusToneBanner.danger}`}>
              没读出波次：{answer.reason}。
            </p>
          ) : answer.waves.length === 0 ? (
            <p className="type-body text-idpxyz-textMuted">本仓现在没有合并拣货的波次。放波次时勾上「合并拣货」，它拣出来的货才到这里分。</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {answer.waves.map((wave) => (
                <li key={wave.waveId}>
                  <a className={`${stationActionClass} h-auto w-full justify-between py-2 text-left`} href={sortStationHash(wave.waveId)}>
                    <span className="break-all font-mono">{wave.waveId}</span>
                    <span className="type-caption text-idpxyz-textMuted">
                      {waveStatusLabel(wave.status)} · {wave.requests} 张单 · {formatInstant(wave.releasedAt)}
                    </span>
                  </a>
                </li>
              ))}
            </ul>
          )}
          {answer !== null && answer.kind === 'waves' && answer.more ? (
            <p className="type-caption text-idpxyz-textMuted">只列了最近放出的这些；更早的波次扫它的波次号进。</p>
          ) : null}
          <div>
            <button type="button" className={stationButtonClass} onClick={() => setTick((value) => value + 1)}>
              刷新
            </button>
          </div>
        </div>
      }
    />
  );
}

function SortWave({ device, wave, onChangeDevice }: { device: WorkDeviceRecord; wave: string; onChangeDevice?: () => void }) {
  const warehouse = device.workScopeRef;
  const sound = useStationSound();
  const session = useSortSession({ deviceId: device.deviceId, warehouse, wave, cue: sound.cue });
  const { state, load } = session;
  const dismiss = session.dismiss;

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') dismiss();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [dismiss]);

  const context = (
    <span className="flex flex-wrap items-center gap-2">
      波次 <span className="font-mono text-idpxyz-text">{wave}</span>
      <a className={stationButtonClass} href={sortStationHash()}>
        换波次
      </a>
    </span>
  );

  if (state === null) {
    return (
      <StationFrame
        title="分拣"
        device={{ id: device.deviceId, warehouse }}
        onChangeDevice={onChangeDevice}
        context={context}
        left={
          load.phase === 'loading' || load.phase === 'ready' ? (
            <p role="status" className="type-body text-idpxyz-textMuted">
              正在读这个波次的分拣情况…
            </p>
          ) : (
            <div className="flex flex-col gap-3">
              <h2 className="type-section font-semibold text-idpxyz-textBright">{load.phase === 'refused' ? '这个波次不在这里分拣' : '读不出分拣情况'}</h2>
              <p role="alert" className={`rounded border px-3 py-2 type-body ${load.phase === 'refused' ? statusToneBanner.warning : statusToneBanner.danger}`}>
                {load.message}。
              </p>
              <div className="flex flex-wrap gap-2">
                {load.phase === 'error' ? (
                  <button type="button" className={stationActionClass} onClick={session.refresh}>
                    再试一次
                  </button>
                ) : null}
                <a className={stationActionClass} href={sortStationHash()}>
                  换一个波次
                </a>
              </div>
            </div>
          )
        }
      />
    );
  }

  const screen = sortScreen(state);
  const actions = sortActions(state);
  const summary = sortSummary(state);
  const allPut = state.target !== null && state.target.quantity > 1;

  return (
    <StationFrame
      title="分拣"
      device={{ id: device.deviceId, warehouse }}
      onChangeDevice={onChangeDevice}
      context={context}
      progress={sortProgress(state)}
      sound={{ on: sound.on, onToggle: sound.toggle }}
      left={
        <div className={`flex min-h-full flex-col gap-4 p-4 md:p-6 ${panelClass[state.feedback.kind]}`}>
          <p className="type-body text-idpxyz-textMuted">{screen.label}</p>
          <h2 className="type-display break-words font-semibold leading-tight text-idpxyz-textBright">{screen.headline}</h2>
          <ScanBox
            onScan={session.scan}
            placeholder={scanPlaceholder(state)}
            busy={!acceptingScans(state)}
            suspended={false}
            tone={state.feedback.kind}
            scanned={state.feedback.scanned}
          />
          <ScanFeedback kind={state.feedback.kind} text={state.feedback.text} />
          {screen.counter === '' ? null : <p className="type-body text-idpxyz-textMuted">{screen.counter}</p>}
          <VerdictExtras verdict={session.verdict} state={state} />
          <div className="mt-auto flex flex-wrap gap-2 pt-2">
            {state.phase === 'unsent' ? (
              <button type="button" className={stationPrimaryClass} onClick={session.resend}>
                再送一次
              </button>
            ) : null}
            {state.phase === 'tote' ? (
              <>
                <button type="button" className={stationActionClass} disabled={!actions.putAll} aria-pressed={allPut} onClick={session.putAll}>
                  {allPut ? '改回一次放一件' : '剩下的都放进去'}
                </button>
                <button type="button" className={stationActionClass} disabled={!actions.addTote} onClick={session.addTote}>
                  加一个箱
                </button>
                <button type="button" className={stationActionClass} disabled={!actions.setAside} onClick={session.setAside}>
                  这一件先放一边
                </button>
              </>
            ) : null}
            {state.phase === 'unit' && summary.left === 0 ? (
              <a className={summary.done > 0 ? stationPrimaryClass : stationActionClass} href={packStationHash()}>
                去复核打包
              </a>
            ) : null}
          </div>
        </div>
      }
      right={
        <div className="flex flex-col gap-3 p-4 md:p-6">
          <div className="flex items-center justify-between gap-2">
            <h2 className="type-body font-semibold text-idpxyz-textBright">这个波次的出库单</h2>
            <button type="button" className={stationButtonClass} disabled={!actions.refresh || session.refreshing} onClick={session.refresh}>
              刷新
            </button>
          </div>
          {session.refreshError === null ? null : (
            <p role="alert" className={`rounded border px-3 py-2 type-caption ${statusToneBanner.danger}`}>
              刷新没读成：{session.refreshError}。下面是刷新之前读到的，可能已不是最新。
            </p>
          )}
          <p className="type-caption text-idpxyz-textMuted">这一趟分了 {state.put} 件。已经打包、取消或撤出的单不列。</p>
          <OrderTable state={state} />
        </div>
      }
    />
  );
}

export interface SortStationProps {
  device: WorkDeviceRecord;
  /** 分这个波次；null 是还没选。 */
  wave: string | null;
  onChangeDevice?: () => void;
}

export function SortStation({ device, wave, onChangeDevice }: SortStationProps) {
  return wave === null ? (
    <SortWaveChooser device={device} onChangeDevice={onChangeDevice} />
  ) : (
    <SortWave device={device} wave={wave} onChangeDevice={onChangeDevice} />
  );
}
