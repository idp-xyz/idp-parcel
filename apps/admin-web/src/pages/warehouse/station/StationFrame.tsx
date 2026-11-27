import type { ReactNode } from 'react';
import { Volume2, VolumeX } from 'lucide-react';
import { legacyWorkstationHash, stationHomeHash } from './stations';

// 作业台的全屏外框：顶条加主区。六个工位共用，各自只给工位名、这一趟的上下文、进度、左边（下一步、扫描框、反馈、按钮）
// 与右边（队列或这一单该有的东西）。没有侧栏、标签栏、检查器。
//
// 顶条：工位名、设备与仓、这一趟的上下文、进度、声音开关、换工位、退出作业台。主区宽屏左右两栏（三比二），窄屏叠成一栏：
// 手持设备 360 宽起能看能点，顶条按钮 40 高、动作按钮 44 高（触屏够大）。窄屏下顶条自己换行，不横向滚。
//
// 「退出作业台」回到旧的仓内作业台页：旧页留到最后一个工位做完才换掉，这之前它是进外壳的唯一路。

const base =
  'inline-flex items-center justify-center gap-1.5 rounded border type-body focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-idpxyz-accent disabled:cursor-not-allowed disabled:opacity-40';

/** 顶条上的按钮与链接。 */
export const stationButtonClass = `${base} h-10 border-idpxyz-border bg-idpxyz-editor px-3 text-idpxyz-text hover:bg-idpxyz-hover`;
/** 主区里的动作按钮，比顶条的高一档。 */
export const stationActionClass = `${base} h-11 border-idpxyz-border bg-idpxyz-editor px-4 text-idpxyz-text hover:bg-idpxyz-hover`;
/** 主区里的主动作：实色底白字，与 components/primary-action.ts 同一个理由不用透明度变体。 */
export const stationPrimaryClass = `${base} h-11 border-transparent bg-idpxyz-accent px-4 font-medium text-white hover:bg-[color-mix(in_srgb,var(--idpxyz-accent)_85%,black)] hover:text-white`;

export interface StationFrameProps {
  /** 工位名；选工位那一页写「作业台」。 */
  title: string;
  /** 绑着的设备与仓；还没绑好时不给。 */
  device?: { id: string; warehouse: string };
  /** 给了才摆「换设备」。 */
  onChangeDevice?: () => void;
  /** 这一趟的上下文：波次、放货位这类，摆在顶条第二行。 */
  context?: ReactNode;
  /** 进度，如「第 3 格，共 12 格」。 */
  progress?: string;
  /** 给了才摆声音开关。 */
  sound?: { on: boolean; onToggle: () => void };
  /** 有这句话时，换工位和退出要先确认。没有就不拦。 */
  confirmLeave?: string;
  /** 左边：下一步、扫描框、反馈、按钮；没有右边时占满。 */
  left: ReactNode;
  right?: ReactNode;
}

function StationLeaveLink({ href, label, confirmLeave }: { href: string; label: string; confirmLeave?: string }) {
  return (
    <a
      className={stationButtonClass}
      href={href}
      onClick={(event) => {
        if (confirmLeave !== undefined && !window.confirm(confirmLeave)) event.preventDefault();
      }}
    >
      {label}
    </a>
  );
}

export function StationFrame({ title, device, onChangeDevice, context, progress, sound, confirmLeave, left, right }: StationFrameProps) {
  return (
    <div className="flex h-screen w-screen flex-col overflow-hidden bg-idpxyz-bg text-idpxyz-text">
      <header className="shrink-0 border-b border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 md:px-5">
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
          <h1 className="type-title font-semibold text-idpxyz-textBright">{title}</h1>
          {device ? (
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1 type-body text-idpxyz-textMuted">
              <span>
                设备 <span className="font-mono text-idpxyz-text">{device.id}</span>
              </span>
              <span>
                仓 <span className="font-mono text-idpxyz-text">{device.warehouse}</span>
              </span>
              {onChangeDevice ? (
                <button type="button" className={stationButtonClass} onClick={onChangeDevice}>
                  换设备
                </button>
              ) : null}
            </div>
          ) : null}
          <span className="hidden flex-1 md:block" />
          {progress ? <p className="type-body font-medium text-idpxyz-textBright">{progress}</p> : null}
          <div className="flex flex-wrap items-center gap-2">
            {sound ? (
              <button type="button" className={stationButtonClass} aria-pressed={sound.on} onClick={sound.onToggle}>
                {sound.on ? <Volume2 className="h-4 w-4" aria-hidden /> : <VolumeX className="h-4 w-4" aria-hidden />}
                {sound.on ? '声音：开' : '声音：关'}
              </button>
            ) : null}
            <StationLeaveLink href={stationHomeHash} label="换工位" confirmLeave={confirmLeave} />
            <StationLeaveLink href={legacyWorkstationHash} label="退出作业台" confirmLeave={confirmLeave} />
          </div>
        </div>
        {context ? <div className="mt-2 flex flex-wrap items-center gap-x-4 gap-y-2 type-body text-idpxyz-textMuted">{context}</div> : null}
      </header>
      <main className="min-h-0 flex-1 overflow-y-auto">
        {right === undefined ? (
          <div className="mx-auto w-full max-w-3xl p-4 md:p-6">{left}</div>
        ) : (
          <div className="grid min-h-full grid-cols-1 md:grid-cols-5">
            <section className="md:col-span-3">{left}</section>
            <aside className="border-t border-idpxyz-border bg-idpxyz-bg md:col-span-2 md:border-l md:border-t-0">{right}</aside>
          </div>
        )}
      </main>
    </div>
  );
}
