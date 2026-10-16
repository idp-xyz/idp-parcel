// 状态语气（成功 / 提示 / 警示 / 危险）的类名（票 group-legal-entities-uiux/08）。只是几串 class，用法同 chip.ts 与
// primary-action.ts。
//
// 为什么不用 Tailwind 色板（`text-amber-400`、`bg-red-500/10` 之类）：那一套是照深色参照页挑的 400 色阶，落在浅色画布上
// 只有 1.6–2.5:1。本仓 Tailwind 没配 darkMode，`dark:` 跟的是系统偏好而不是应用主题，按主题换色这条路也走不通。主题运行时
// 已把语义状态色按当前主题写进根元素的 `--ds-status-<语气>-{fg,bg-subtle,border}`（applyThemeTokens），两套各有一份配过
// 对比度的值（浅色 fg 为 700 色阶、深色为 400 色阶，对画布与卡片都过 4.5:1），这里直接取它们。
//
// 任意值一律带 `color:` 类型提示：`text-[var(--x)]` 不带提示时 Tailwind 读成字号。

export type StatusTone = 'success' | 'info' | 'warning' | 'danger';

/** 状态字色：KPI 数字、临近时刻、警示图标。 */
export const statusToneText: Record<StatusTone, string> = {
  success: 'text-[color:var(--ds-status-success-fg)]',
  info: 'text-[color:var(--ds-status-info-fg)]',
  warning: 'text-[color:var(--ds-status-warning-fg)]',
  danger: 'text-[color:var(--ds-status-danger-fg)]',
};

/** 一行横幅：淡底、描边、状态字色。 */
export const statusToneBanner: Record<StatusTone, string> = {
  success:
    'border-[color:var(--ds-status-success-border)] bg-[color:var(--ds-status-success-bg-subtle)] text-[color:var(--ds-status-success-fg)]',
  info: 'border-[color:var(--ds-status-info-border)] bg-[color:var(--ds-status-info-bg-subtle)] text-[color:var(--ds-status-info-fg)]',
  warning:
    'border-[color:var(--ds-status-warning-border)] bg-[color:var(--ds-status-warning-bg-subtle)] text-[color:var(--ds-status-warning-fg)]',
  danger:
    'border-[color:var(--ds-status-danger-border)] bg-[color:var(--ds-status-danger-bg-subtle)] text-[color:var(--ds-status-danger-fg)]',
};

/** 圆点、进度条这类小面积实色：用前景色作底，比淡底更醒目。 */
export const statusToneSurface: Record<StatusTone, string> = {
  success: 'bg-[color:var(--ds-status-success-fg)]',
  info: 'bg-[color:var(--ds-status-info-fg)]',
  warning: 'bg-[color:var(--ds-status-warning-fg)]',
  danger: 'bg-[color:var(--ds-status-danger-fg)]',
};
