import { labelFor, toneFor, toneForLabel, type StatusTone } from '../vocabulary/status';

// 状态胶囊：语气图标 + 中文词，词与色调取用语词典（vocabulary/status.ts）。全站凡显状态或枚举码的格都用它，不再各页自己拼
// 徽章——词典改一个词，全站一起变。
//
// 色取主题运行时写进根元素的 `--ds-status-<语气>-*` 变量（理由见 components/status-tone.ts 文件头：Tailwind 色板在浅色
// 画布上对比度不够，`dark:` 又跟系统偏好不跟应用主题）。正文用正文色、语气只落在描边、淡底与图标上——语气字色叠在
// 淡底上不足 4.5:1，图标是非文字、门槛 3:1。任意值一律带 `color:` 类型提示。
//
// 图标按语气（票 business-party-ux/08 同批的票 07）：此前只有一粒圆点，色是唯一载体，色弱与深色主题下几格分不开。图标的
// 意思取词典文件头「设计原则四」给语气下的定义——待处理（warn）是等着办的，画钟；推进中（info）画 i；成立（good）画勾；
// 失败 / 阻断（bad）画叉；中性 / 已结束（muted）画横。按语气而不按码：词典几十个码，按码配图是第二张表，会与词典走散。
// 图标是内联 SVG 而不是图标库：本件只引 react 与词典，node:test 里 renderToStaticMarkup 直接钉。

const toneClass: Record<StatusTone, { pill: string; icon: string }> = {
  good: {
    pill: 'border-[color:var(--ds-status-success-border)] bg-[color:var(--ds-status-success-bg-subtle)]',
    icon: 'text-[color:var(--ds-status-success-fg)]',
  },
  info: {
    pill: 'border-[color:var(--ds-status-info-border)] bg-[color:var(--ds-status-info-bg-subtle)]',
    icon: 'text-[color:var(--ds-status-info-fg)]',
  },
  warn: {
    pill: 'border-[color:var(--ds-status-warning-border)] bg-[color:var(--ds-status-warning-bg-subtle)]',
    icon: 'text-[color:var(--ds-status-warning-fg)]',
  },
  bad: {
    pill: 'border-[color:var(--ds-status-danger-border)] bg-[color:var(--ds-status-danger-bg-subtle)]',
    icon: 'text-[color:var(--ds-status-danger-fg)]',
  },
  muted: {
    pill: 'border-idpxyz-border bg-idpxyz-hover',
    icon: 'text-idpxyz-textMuted',
  },
};

/** 语气 → 图标名与 24 格里的描边路径；名字落在 data-icon 上供测试钉。 */
const toneIcon: Record<StatusTone, { name: string; paths: string[] }> = {
  good: { name: 'check', paths: ['m5 12 4 4L19 6'] },
  warn: { name: 'clock', paths: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z', 'M12 7v5l3 2'] },
  info: { name: 'info', paths: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z', 'M12 11v6', 'M12 7h.01'] },
  bad: { name: 'x', paths: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18Z', 'm9 9 6 6', 'm15 9-6 6'] },
  muted: { name: 'minus', paths: ['M7 12h10'] },
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
  const icon = toneIcon[resolvedTone];
  return (
    <span
      data-tone={resolvedTone}
      className={`inline-flex items-center gap-1 whitespace-nowrap rounded-full border px-2 py-0.5 type-body leading-4 text-idpxyz-text ${classes.pill} ${className ?? ''}`.trim()}
    >
      <svg
        aria-hidden="true"
        data-icon={icon.name}
        viewBox="0 0 24 24"
        className={`h-3 w-3 shrink-0 ${classes.icon}`}
        fill="none"
        stroke="currentColor"
        strokeWidth="2.25"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        {icon.paths.map((d) => (
          <path key={d} d={d} />
        ))}
      </svg>
      {word}
    </span>
  );
}
