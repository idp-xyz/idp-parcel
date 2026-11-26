import { useEffect, useState } from 'react';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@idpxyz/ui-primitives';
import { ReloginButton } from '../../../auth/ReloginButton';
import { TechnicalDetails, httpRows, type TechnicalRow } from '../../../components/TechnicalDetails';
import { statusToneBanner } from '../../../components/status-tone';
import type { WorkDeviceRecord } from '../../work-devices/api';
import { QuantityDialog, ShortPickDialog } from './PickDialogs';
import { acceptingScans, pickActions, pickProgress, pickScreen, queueNotes, scanPlaceholder, shortPickPlan, type PickState } from './pick-flow';
import { allWavesChoice, waveChoices, waveOfChoice, type PickCell, type PickQueue } from './pick-queue';
import { ScanBox } from './ScanBox';
import { panelClass, ScanFeedback } from './ScanFeedback';
import { StationFrame, stationActionClass, stationButtonClass, stationPrimaryClass } from './StationFrame';
import { packStationHash, pickStationHash, sortStationHash } from './stations';
import { usePickSession } from './use-pick-session';
import { useStationSound } from './use-station-sound';
import type { WorkFactVerdict } from './work-fact-submit';

// 拣货工位（地址 #/station/pick，可带 ?wave=）。屏上的字、扫码的顺序与每一步的判定都在 pick-flow.ts，这里只摆与接线：
// 左边最大的字是下一步，下面是扫描框、反馈和按钮；右边是余下的格，按库位排。

const queueRowLimit = 100;

// 换波次不用原生下拉：原生下拉的选项面板由浏览器自己管，收起（Esc、选了同一项）不一定通知页面，焦点就悬在下拉上，
// 枪扫出来的字会落进去触发换波次。这里的下拉收起（Esc、选完、点别处）页面都收得到 onOpenChange：开着时扫描框让位
// （suspended），收起后由扫描框自己把焦点放回去，所以不让下拉组件把焦点还给触发它的那个钮（onCloseAutoFocus）。
function WaveChooser({
  wave,
  queue,
  disabled,
  onOpenChange,
}: {
  wave: string | null;
  queue: PickQueue | null;
  disabled: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Select
      value={wave ?? allWavesChoice}
      disabled={disabled}
      onOpenChange={onOpenChange}
      onValueChange={(value) => {
        window.location.hash = pickStationHash(waveOfChoice(value) ?? undefined);
      }}
    >
      <SelectTrigger aria-label="换波次" className="h-10 w-auto min-w-[8rem] max-w-[16rem] px-2 type-body disabled:opacity-40">
        <SelectValue />
      </SelectTrigger>
      <SelectContent onCloseAutoFocus={(event) => event.preventDefault()}>
        {waveChoices(wave, queue?.waves ?? []).map((choice) => (
          <SelectItem key={choice.value} value={choice.value} className="py-3 type-body">
            {choice.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

/** 没送到与没成时，把后端答复的技术详情（状态码、问题码、网络错误原文）收在折叠里；需要登录的给重新登录。 */
function VerdictExtras({ verdict, state }: { verdict: WorkFactVerdict | null; state: PickState }) {
  if (verdict === null || (state.phase !== 'unsent' && state.phase !== 'refused')) return null;
  const rows: TechnicalRow[] = [];
  if (verdict.technical?.status !== undefined) rows.push(...httpRows(verdict.technical.status, verdict.technical.code ?? ''));
  else if (verdict.technical?.code) rows.push({ label: '答复', value: verdict.technical.code });
  if (verdict.technical?.message) rows.push({ label: '网络错误', value: verdict.technical.message });
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

function QueueTable({ cells }: { cells: readonly PickCell[] }) {
  if (cells.length === 0) return <p className="type-body text-idpxyz-textMuted">没有余下的格。</p>;
  const shown = cells.slice(0, queueRowLimit);
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse type-body">
        <thead>
          <tr className="text-left text-idpxyz-textMuted">
            <th className="py-1.5 pr-3 font-medium">库位</th>
            <th className="py-1.5 pr-3 font-medium">商品</th>
            <th className="py-1.5 text-right font-medium">要拣</th>
          </tr>
        </thead>
        <tbody>
          {shown.map((cell, index) => (
            <tr key={cell.taskId} className={index === 0 ? 'bg-idpxyz-activeItem' : undefined} aria-current={index === 0 ? 'true' : undefined}>
              <td className="whitespace-nowrap border-t border-idpxyz-border py-2 pr-3 font-mono">
                {cell.locationCode}
                {index === 0 ? <span className="block font-sans type-caption text-idpxyz-accent">这一格</span> : null}
              </td>
              <td className="break-all border-t border-idpxyz-border py-2 pr-3 font-mono">{cell.goodsCode}</td>
              <td className="whitespace-nowrap border-t border-idpxyz-border py-2 text-right font-mono">
                {cell.quantity} {cell.unit}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {cells.length > shown.length ? <p className="mt-2 type-caption text-idpxyz-textMuted">另有 {cells.length - shown.length} 格没列出。</p> : null}
    </div>
  );
}

export interface PickStationProps {
  device: WorkDeviceRecord;
  /** 只做这个波次；null 是本仓全部波次。 */
  wave: string | null;
  onChangeDevice?: () => void;
}

export function PickStation({ device, wave, onChangeDevice }: PickStationProps) {
  const warehouse = device.workScopeRef;
  const sound = useStationSound();
  const session = usePickSession({ deviceId: device.deviceId, warehouse, wave, cue: sound.cue });
  const { state, load } = session;
  const [quantityOpen, setQuantityOpen] = useState(false);
  const [shortOpen, setShortOpen] = useState(false);
  const [waveOpen, setWaveOpen] = useState(false);
  const suspended = quantityOpen || shortOpen || waveOpen;
  const dismiss = session.dismiss;

  // Esc 清掉扫错与没核成。数量框、确认框、波次下拉开着时 Esc 归它们（收起），这里不插手。
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !suspended) dismiss();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [suspended, dismiss]);

  const queue = load.phase === 'ready' ? load.queue : null;
  const actions = state === null ? null : pickActions(state);
  const screen = state === null ? null : pickScreen(state);

  const context =
    state === null ? undefined : (
      <>
        <span className="flex flex-wrap items-center gap-2">
          {wave === null ? '全部波次' : '只做这个波次'}
          <WaveChooser wave={wave} queue={queue} disabled={!actions?.refresh} onOpenChange={setWaveOpen} />
        </span>
        <span className="flex flex-wrap items-center gap-2">
          放货位 <span className="font-mono text-idpxyz-text">{state.staging ?? '还没扫'}</span>
          <button type="button" className={stationButtonClass} disabled={!actions?.changeStaging} onClick={session.changeStagingLocation}>
            换放货位
          </button>
        </span>
      </>
    );

  const left =
    load.phase === 'error' ? (
      <div className="flex flex-col gap-3 p-4 md:p-6">
        <h2 className="type-section font-semibold text-idpxyz-textBright">读不出待拣的格</h2>
        <p role="alert" className={`rounded border px-3 py-2 type-body ${statusToneBanner.danger}`}>
          {load.message}。
        </p>
        <div>
          <button type="button" className={stationActionClass} onClick={session.refresh}>
            再试一次
          </button>
        </div>
      </div>
    ) : state === null || screen === null || actions === null ? (
      <p role="status" className="p-4 type-body text-idpxyz-textMuted md:p-6">
        正在读待拣的格…
      </p>
    ) : (
      <div className={`flex min-h-full flex-col gap-4 p-4 md:p-6 ${panelClass[state.feedback.kind]}`}>
        <p className="type-body text-idpxyz-textMuted">{screen.label}</p>
        <h2 className="type-display break-words font-semibold leading-tight text-idpxyz-textBright">{screen.headline}</h2>
        <ScanBox
          onScan={session.scan}
          placeholder={scanPlaceholder(state)}
          busy={!acceptingScans(state)}
          suspended={suspended}
          tone={state.feedback.kind}
          scanned={state.feedback.scanned}
        />
        <ScanFeedback kind={state.feedback.kind} text={state.feedback.text} />
        <p className="type-body text-idpxyz-textMuted">{screen.counter}</p>
        <VerdictExtras verdict={session.verdict} state={state} />
        {state.phase === 'finished' && queue !== null
          ? queueNotes(queue, wave).map((note) => (
              <p key={note} className={`rounded border px-3 py-2 type-body ${statusToneBanner.warning}`}>
                {note}
              </p>
            ))
          : null}
        <div className="mt-auto flex flex-wrap gap-2 pt-2">
          {state.phase === 'unsent' ? (
            <button type="button" className={stationPrimaryClass} onClick={session.resend}>
              再送一次
            </button>
          ) : null}
          {state.phase === 'refused' ? (
            <button type="button" className={stationPrimaryClass} onClick={session.next}>
              下一格
            </button>
          ) : null}
          {state.phase === 'finished' ? (
            <>
              <button type="button" className={stationPrimaryClass} disabled={!actions.refresh || session.refreshing} onClick={session.refresh}>
                刷新
              </button>
              {session.sorting ? (
                <a className={stationActionClass} href={sortStationHash(wave ?? undefined)}>
                  去分拣
                </a>
              ) : null}
              <a className={stationActionClass} href={packStationHash()}>
                去复核打包
              </a>
              {wave !== null ? (
                <a className={stationActionClass} href={pickStationHash()}>
                  改成全部波次
                </a>
              ) : null}
            </>
          ) : (
            <>
              <button type="button" className={stationActionClass} disabled={!actions.changeCount} onClick={() => setQuantityOpen(true)}>
                改数量
              </button>
              <button type="button" className={stationActionClass} disabled={!actions.shortPick} onClick={() => setShortOpen(true)}>
                少拣
              </button>
              <button type="button" className={stationActionClass} disabled={!actions.skip} onClick={session.skip}>
                跳过这一格
              </button>
              {state.toteOn ? (
                <button type="button" className={stationActionClass} disabled={!actions.skipTote} onClick={session.skipTote}>
                  不用箱
                </button>
              ) : null}
            </>
          )}
        </div>
      </div>
    );

  const right =
    state === null ? (
      <div className="p-4 md:p-6" />
    ) : (
      <div className="flex flex-col gap-3 p-4 md:p-6">
        <div className="flex items-center justify-between gap-2">
          <h2 className="type-body font-semibold text-idpxyz-textBright">余下的格，按库位排</h2>
          <button
            type="button"
            className={stationButtonClass}
            disabled={!actions?.refresh || session.refreshing}
            onClick={session.refresh}
          >
            刷新
          </button>
        </div>
        {session.refreshError === null ? null : (
          <p role="alert" className={`rounded border px-3 py-2 type-caption ${statusToneBanner.danger}`}>
            刷新没读成：{session.refreshError}。下面的格是刷新之前读到的，可能已不是最新。
          </p>
        )}
        {queue === null || state.phase === 'finished'
          ? null
          : queueNotes(queue, wave).map((note) => (
              <p key={note} className={`rounded border px-3 py-2 type-caption ${statusToneBanner.warning}`}>
                {note}
              </p>
            ))}
        <QueueTable cells={state.queue} />
      </div>
    );

  return (
    <>
      <StationFrame
        title="拣货"
        device={{ id: device.deviceId, warehouse }}
        onChangeDevice={onChangeDevice}
        context={context}
        progress={state === null ? undefined : pickProgress(state)}
        sound={{ on: sound.on, onToggle: sound.toggle }}
        left={left}
        right={right}
      />
      {state === null ? null : (
        <>
          <QuantityDialog
            open={quantityOpen}
            cell={state.queue[0] ?? null}
            counted={state.counted}
            onClose={() => setQuantityOpen(false)}
            onSubmit={session.changeQuantity}
          />
          <ShortPickDialog
            open={shortOpen}
            plan={shortPickPlan(state)}
            onClose={() => setShortOpen(false)}
            onConfirm={session.confirmShortPick}
          />
        </>
      )}
    </>
  );
}
