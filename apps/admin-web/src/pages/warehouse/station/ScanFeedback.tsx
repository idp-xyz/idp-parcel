import { AlertTriangle, CheckCircle2, XCircle } from 'lucide-react';
import { statusToneText } from '../../../components/status-tone';

// 工位的反馈块：等扫 / 扫对 / 扫错三种样子，外加「没送到」。六个工位共用，各自只给 kind 与话。
//
// 颜色走主题运行时按当前主题写好的语义状态色（`--ds-status-*`，见 components/status-tone.ts），不用 Tailwind 色板加 `dark:`：
// 本仓 Tailwind 没配 darkMode，`dark:` 跟的是系统偏好而不是应用主题，应用是浅色而系统是深色时红字会变成 300 色阶落在浅底上读不清。
// 按那份文件的约定，淡底之上的字用正文色，语气色只给整块的底色、描边与图标：扫错整块变红，写明的原因照样是正文色，读得清。
//
// 读屏要读得出来：扫错与没送到用 role="alert"（打断式播报），等扫与扫对用 role="status"（礼貌式）。两种角色挂在不同的节点上、
// 按 kind 换着挂，而不是在同一个节点上改 role 或 aria-live——后者在不少读屏里换了属性但不报，刚挂上的 alert 节点才稳稳地读。

export type ScanTone = 'waiting' | 'ok' | 'wrong' | 'unsent';

/** 整块的底色。 */
export const panelClass: Record<ScanTone, string> = {
  waiting: 'bg-idpxyz-editor',
  ok: 'bg-[color:var(--ds-status-success-bg-subtle)]',
  wrong: 'bg-[color:var(--ds-status-danger-bg-subtle)]',
  unsent: 'bg-[color:var(--ds-status-warning-bg-subtle)]',
};

/** 扫描框的边：等扫用强调色，扫对绿，扫错红，没送到琥珀。 */
export const edgeClass: Record<ScanTone, string> = {
  waiting: 'border-idpxyz-accent',
  ok: 'border-[color:var(--ds-status-success-fg)]',
  wrong: 'border-[color:var(--ds-status-danger-fg)]',
  unsent: 'border-[color:var(--ds-status-warning-fg)]',
};

const icons = {
  ok: <CheckCircle2 className={`mt-0.5 h-5 w-5 shrink-0 ${statusToneText.success}`} aria-hidden />,
  wrong: <XCircle className={`mt-0.5 h-5 w-5 shrink-0 ${statusToneText.danger}`} aria-hidden />,
  unsent: <AlertTriangle className={`mt-0.5 h-5 w-5 shrink-0 ${statusToneText.warning}`} aria-hidden />,
};

export function ScanFeedback({ kind, text }: { kind: ScanTone; text: string }) {
  const className = 'type-value font-medium leading-6 text-idpxyz-textBright';
  const icon = kind === 'waiting' ? null : icons[kind];
  return (
    <div className="flex items-start gap-2">
      {icon}
      {/* key 不同，React 才会卸掉旧节点、挂一个新的 alert 节点；同一个节点上换 role 不会触发播报 */}
      {kind === 'wrong' || kind === 'unsent' ? (
        <p key="alert" role="alert" className={className}>
          {text}
        </p>
      ) : (
        <p key="status" role="status" aria-live="polite" className={className}>
          {text}
        </p>
      )}
    </div>
  );
}
