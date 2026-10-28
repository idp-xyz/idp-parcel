import { labelFor, toneFor, toneForLabel, type StatusTone } from '../vocabulary/status';

// 状态胶囊：圆点 + 中文词，词与色调取用语词典（vocabulary/status.ts）。全站凡显状态或枚举码的格都用它，不再各页自己拼
// 徽章——词典改一个词，全站一起变。
//
// 色取主题运行时写进根元素的 `--ds-status-<语气>-*` 变量（理由见 components/status-tone.ts 文件头：Tailwind 色板在浅色
// 画布上对比度不够，`dark:` 又跟系统偏好不跟应用主题）。正文用正文色、语气只落在描边、淡底与圆点上——语气字色叠在
// 淡底上不足 4.5:1，圆点是非文字、门槛 3:1。任意值一律带 `color:` 类型提示。
//
// 只引 react 与词典，不引组件库：node:test 里要用 renderToStaticMarkup 钉它。

const toneClass: Record<StatusTone, { pill: string; dot: string }> = {
  good: {
    pill: 'border-[color:var(--ds-status-success-border)] bg-[color:var(--ds-status-success-bg-subtle)]',
    dot: 'bg-[color:var(--ds-status-success-fg)]',
  },
  info: {
    pill: 'border-[color:var(--ds-status-info-border)] bg-[color:var(--ds-status-info-bg-subtle)]',
    dot: 'bg-[color:var(--ds-status-info-fg)]',
  },
  warn: {
    pill: 'border-[color:var(--ds-status-warning-border)] bg-[color:var(--ds-status-warning-bg-subtle)]',
    dot: 'bg-[color:var(--ds-status-warning-fg)]',
  },
  bad: {
    pill: 'border-[color:var(--ds-status-danger-border)] bg-[color:var(--ds-status-danger-bg-subtle)]',
    dot: 'bg-[color:var(--ds-status-danger-fg)]',
  },
  muted: {
    pill: 'border-idpxyz-border bg-idpxyz-hover',
    dot: 'bg-idpxyz-textMuted',
  },
};

export interface StatusPillProps {
  /** 状态码，经词典译成词与色调。 */
  code?: string;
  /** 词典分册（vocabulary/status.ts 的 scopedStatusVocabulary 键）；同一个码在这本册里另有意思时传。 */
  scope?: string;
  /** 已经译好的词：行模型只交字符串时用；给了词不给码，色调按词反查。 */
  label?: string;
  /** 直接指定色调，覆盖词典的判断。 */
  tone?: StatusTone;
  className?: string;
}

/** 状态胶囊。code 与 label 至少给一个；两个都给时词取 label、色调取 code。 */
export function StatusPill({ code, scope, label, tone, className }: StatusPillProps) {
  const word = label ?? (code !== undefined ? labelFor(code, scope) : '');
  const resolvedTone = tone ?? (code !== undefined ? toneFor(code, scope) : toneForLabel(word));
  const classes = toneClass[resolvedTone];
  return (
    <span
      data-tone={resolvedTone}
      className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2 py-0.5 text-[12px] leading-4 text-idpxyz-text ${classes.pill} ${className ?? ''}`.trim()}
    >
      <span aria-hidden="true" className={`h-1.5 w-1.5 shrink-0 rounded-full ${classes.dot}`} />
      {word}
    </span>
  );
}
