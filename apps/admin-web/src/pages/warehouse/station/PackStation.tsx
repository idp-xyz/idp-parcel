import { useEffect, useState } from 'react';
import { ReloginButton } from '../../../auth/ReloginButton';
import { TechnicalDetails, httpRows, type TechnicalRow } from '../../../components/TechnicalDetails';
import { statusToneBanner } from '../../../components/status-tone';
import type { WorkDeviceRecord } from '../../work-devices/api';
import { acceptingScans, mismatchPlan, packActions, packProgress, packScreen, scanPlaceholder, type PackState } from './pack-flow';
import { goodsKey, queueCount, type QueueList } from './pack-order';
import { packBalance } from './pack-parcels';
import { CountsDialog, MismatchDialog, PackagingDialog } from './PackDialogs';
import { ParcelCard } from './ParcelCard';
import { ScanBox } from './ScanBox';
import { panelClass, ScanFeedback } from './ScanFeedback';
import { StationFrame, stationActionClass, stationButtonClass, stationPrimaryClass } from './StationFrame';
import { usePackSession } from './use-pack-session';
import { useStationSound } from './use-station-sound';
import type { WorkFactVerdict } from './work-fact-submit';

// 复核打包工位（地址 #/station/pack，可带 ?request=出库请求号）。屏上的字、扫码的顺序与每一步的判定都在 pack-flow.ts，这里只摆与接线：
// 左边最大的字是下一步，下面是扫描框、反馈、（打包时）包裹的表单和按钮；右边等扫出库单号时是本仓等复核与等打包的单，
// 做一张单时是这张单该有的货。窄屏上右边叠到左边下面。

/** 队列每一段最多摆几行：再多的靠扫单号。 */
const queueRowLimit = 50;

const parcelPhases: ReadonlyArray<PackState['phase']> = ['packing', 'identifying', 'pack-submitting', 'pack-unsent', 'pack-pending', 'pack-refused'];

/** 没送到与没成时，把后端答复的技术详情（状态码、问题码、网络错误原文）收在折叠里；需要登录的给重新登录。 */
function VerdictExtras({ verdict, state }: { verdict: WorkFactVerdict | null; state: PackState }) {
  const shows = state.phase === 'check-unsent' || state.phase === 'check-refused' || state.phase === 'pack-unsent' || state.phase === 'pack-refused';
  if (verdict === null || !shows) return null;
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

function QueueSection({ title, list, canOpen, onOpen }: { title: string; list: QueueList; canOpen: boolean; onOpen: (requestId: string) => void }) {
  const shown = list.rows.slice(0, queueRowLimit);
  const total = queueCount(list);
  return (
    <section className="flex flex-col gap-2">
      <h3 className="type-body font-semibold text-idpxyz-textBright">
        {title}（{total} 张）
      </h3>
      {shown.length === 0 ? (
        <p className="type-body text-idpxyz-textMuted">现在没有。</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {shown.map((row) => (
            <li key={row.requestId}>
              <button
                type="button"
                disabled={!canOpen}
                onClick={() => onOpen(row.requestId)}
                className="flex w-full flex-col items-start gap-0.5 rounded border border-idpxyz-border bg-idpxyz-editor px-3 py-2 text-left type-body hover:bg-idpxyz-hover focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-idpxyz-accent disabled:cursor-not-allowed disabled:opacity-40"
              >
                <span className="break-all font-mono text-idpxyz-textBright">{row.documentNumber ? row.documentNumber : row.requestId}</span>
                <span className="break-all type-caption text-idpxyz-textMuted">货主 {row.ownerAccount}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
      {total > shown.length ? <p className="type-caption text-idpxyz-textMuted">另有 {total - shown.length} 张没列出，扫它的单号也能做。</p> : null}
    </section>
  );
}

function GoodsPanel({ state }: { state: PackState }) {
  const order = state.order;
  if (order === null) return null;
  const packSide = parcelPhases.includes(state.phase);
  const balance = packBalance(order, state.parcels);
  return (
    <div className="flex flex-col gap-3 p-4 md:p-6">
      <h2 className="type-body font-semibold text-idpxyz-textBright">这张单该有的货</h2>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse type-body">
          <thead>
            <tr className="text-left text-idpxyz-textMuted">
              <th className="py-1.5 pr-3 font-medium">商品</th>
              <th className="py-1.5 pr-3 text-right font-medium">该有</th>
              <th className="py-1.5 text-right font-medium">{packSide ? '已装进包裹' : '已扫'}</th>
            </tr>
          </thead>
          <tbody>
            {order.goods.map((goods, index) => {
              const have = packSide ? balance[index].assigned : (state.counted[goodsKey(goods)] ?? 0);
              const complete = have === goods.quantity;
              return (
                <tr key={goodsKey(goods)} className={complete ? 'bg-[color:var(--ds-status-success-bg-subtle)]' : undefined}>
                  <td className="break-all border-t border-idpxyz-border py-2 pr-3 font-mono">{goods.goodsCode}</td>
                  <td className="whitespace-nowrap border-t border-idpxyz-border py-2 pr-3 text-right font-mono">
                    {goods.quantity} {goods.unit}
                  </td>
                  <td className="whitespace-nowrap border-t border-idpxyz-border py-2 text-right font-mono">{have}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      <p className="type-caption text-idpxyz-textMuted">「该有」是这张单已拣出来的件数，不是单上要的件数。</p>
    </div>
  );
}

export interface PackStationProps {
  device: WorkDeviceRecord;
  /** 地址里带的出库请求号；没带是 null。 */
  request: string | null;
  onChangeDevice?: () => void;
}

export function PackStation({ device, request, onChangeDevice }: PackStationProps) {
  const warehouse = device.workScopeRef;
  const sound = useStationSound();
  const session = usePackSession({ deviceId: device.deviceId, warehouse, request, cue: sound.cue });
  const { state, queue } = session;
  const [countsOpen, setCountsOpen] = useState(false);
  const [mismatchOpen, setMismatchOpen] = useState(false);
  // 给哪个包裹选包材（包裹的本地标识）；没在选是 null
  const [pickerFor, setPickerFor] = useState<number | null>(null);
  const suspended = countsOpen || mismatchOpen || pickerFor !== null;
  const dismiss = session.dismiss;

  // Esc 清掉扫错与没核成。几个框开着时 Esc 归它们（收起），这里不插手。
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !suspended) dismiss();
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [suspended, dismiss]);

  const actions = packActions(state);
  const screen = packScreen(state, queue);
  const progress = packProgress(state);
  const order = state.order;
  const editable = state.phase === 'packing';
  const showParcels = order !== null && parcelPhases.includes(state.phase);
  const waitingSide = order === null;
  const pickerPosition = pickerFor === null ? null : state.parcels.findIndex((parcel) => parcel.key === pickerFor) + 1;
  const goto = state.feedback.goto;

  const context =
    order === null ? undefined : (
      <>
        <span>
          出库单 <span className="font-mono text-idpxyz-text">{order.number}</span>
        </span>
        <span>
          货主 <span className="font-mono text-idpxyz-text">{order.ownerAccount}</span>
        </span>
      </>
    );

  const left = (
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
      {goto === undefined ? null : (
        <div>
          <a className={stationActionClass} href={goto.hash}>
            {goto.label}
          </a>
        </div>
      )}
      <p className="type-body text-idpxyz-textMuted">{screen.counter}</p>
      {state.phase === 'checking' || state.phase === 'resolving' ? (
        <p className="type-caption text-idpxyz-textMuted">已扫的数还没有记下，扫齐自动提交复核（或点「数量不符，照实提交」）才算数；换一张单会丢掉它。</p>
      ) : null}
      {state.phase === 'packing' ? (
        <p className="type-caption text-idpxyz-textMuted">包裹里填的内容提交打包才算数；换一张单会丢掉它。填完一格按回车，焦点回到扫描框，接着扫包材码。</p>
      ) : null}
      <VerdictExtras verdict={session.verdict} state={state} />

      {showParcels && order !== null ? (
        <div className="flex flex-col gap-3">
          {state.parcels.map((parcel, index) => (
            <ParcelCard
              key={parcel.key}
              parcel={parcel}
              position={index + 1}
              order={order}
              editable={editable}
              onEdit={(patch) => session.editParcel(parcel.key, patch)}
              onRemove={() => session.removeParcel(parcel.key)}
              onPickMaterial={() => setPickerFor(parcel.key)}
            />
          ))}
        </div>
      ) : null}

      <div className="mt-auto flex flex-wrap gap-2 pt-2">
        {state.phase === 'waiting' ? (
          <button type="button" className={stationActionClass} disabled={!actions.refresh || session.queueLoading} onClick={session.refreshQueue}>
            刷新队列
          </button>
        ) : null}
        {state.phase === 'checking' || state.phase === 'resolving' ? (
          <>
            <button type="button" className={stationActionClass} disabled={!actions.changeCount} onClick={() => setCountsOpen(true)}>
              改数量
            </button>
            <button type="button" className={stationActionClass} disabled={!actions.mismatch} onClick={() => setMismatchOpen(true)}>
              数量不符，照实提交
            </button>
          </>
        ) : null}
        {parcelPhases.includes(state.phase) ? (
          <>
            <button type="button" className={stationActionClass} disabled={!actions.addParcel} onClick={session.addParcel}>
              再加一个包裹
            </button>
            <button type="button" className={stationPrimaryClass} disabled={!actions.submitPack} onClick={session.submitPack}>
              提交打包
            </button>
          </>
        ) : null}
        {actions.resend ? (
          <button type="button" className={stationPrimaryClass} onClick={session.resend}>
            再送一次
          </button>
        ) : null}
        {actions.backToForm ? (
          <button type="button" className={stationPrimaryClass} onClick={session.backToForm}>
            回去改
          </button>
        ) : null}
        {order !== null ? (
          <button type="button" className={state.phase === 'mismatch' || state.phase === 'check-refused' ? stationPrimaryClass : stationActionClass} disabled={!actions.leave} onClick={session.leave}>
            换一张单
          </button>
        ) : null}
      </div>
    </div>
  );

  const right = waitingSide ? (
    <div className="flex flex-col gap-4 p-4 md:p-6">
      <div className="flex items-center justify-between gap-2">
        <h2 className="type-body font-semibold text-idpxyz-textBright">本仓等着做的出库单</h2>
        <button type="button" className={stationButtonClass} disabled={!actions.refresh || session.queueLoading} onClick={session.refreshQueue}>
          刷新
        </button>
      </div>
      {session.queueError === null ? null : (
        <p role="alert" className={`rounded border px-3 py-2 type-caption ${statusToneBanner.danger}`}>
          没核成：{session.queueError}。
          {queue === null ? '队列没读出来，不能说现在没有单；可以直接扫出库单号。' : '下面的单是读成过的那一份，可能已不是最新。'}
        </p>
      )}
      {queue === null ? (
        session.queueError === null ? (
          <p role="status" className="type-body text-idpxyz-textMuted">
            正在读本仓等复核与等打包的出库单…
          </p>
        ) : null
      ) : (
        <>
          <QueueSection title="已拣完、等复核" list={queue.checkable} canOpen={state.phase === 'waiting'} onOpen={session.open} />
          <QueueSection title="已复核、等打包" list={queue.packable} canOpen={state.phase === 'waiting'} onOpen={session.open} />
        </>
      )}
    </div>
  ) : (
    <GoodsPanel state={state} />
  );

  return (
    <>
      <StationFrame
        title="复核打包"
        device={{ id: device.deviceId, warehouse }}
        onChangeDevice={onChangeDevice}
        context={context}
        progress={progress === '' ? undefined : progress}
        sound={{ on: sound.on, onToggle: sound.toggle }}
        left={left}
        right={right}
      />
      <CountsDialog
        open={countsOpen}
        order={order}
        counted={state.counted}
        onClose={() => setCountsOpen(false)}
        onSubmit={session.changeCounts}
      />
      <MismatchDialog open={mismatchOpen} plan={mismatchPlan(state)} onClose={() => setMismatchOpen(false)} onConfirm={session.confirmMismatch} />
      <PackagingDialog
        open={pickerFor !== null}
        position={pickerPosition === null || pickerPosition < 1 ? null : pickerPosition}
        choices={session.packagingChoices}
        onClose={() => setPickerFor(null)}
        onLoad={session.loadPackagingChoices}
        onPick={(material) => {
          if (pickerFor !== null) session.chooseMaterial(pickerFor, material);
        }}
      />
    </>
  );
}
