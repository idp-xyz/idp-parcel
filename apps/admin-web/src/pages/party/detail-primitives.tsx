import type { ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { formatInstant } from '../catalogue-view';

// 参与方模块册页共用的呈现件（票 admin-web-group-legal-entities/09 第 2、5 条）。它们先在票 01 的法人页里长出来，
// 参与方页两签要同一份形状——两页各写一份会在改时刻写法或复制反馈时分叉，所以抬到这里，两页引同一份。
// 只放呈现层的小件：不放取数、不放业务判读。

/**
 * 时刻格：显本地墙钟带显式偏移（moment.ts 按装配点配置的区算），原 ISO 串放进 dateTime 与 title——
 * 跨 CN/SG 两地对同一事件时悬停给的是线格式那一份，两边比对不必各自换算（票 01 裁决 3）。
 */
export function Instant({ value }: { value: string }) {
  return (
    <time dateTime={value} title={value}>
      {formatInstant(value)}
    </time>
  );
}

/**
 * 有效区间格：两端各是一个 Instant，悬停各给原串；无终点时说「持续有效」——那是 CONTEXT 的口径（有效期间未
 * 声明终点即持续有效），不是「未知」。与 moment.ts 的 formatRange 同一种写法，只是把两端换成可悬停的格。
 */
export function InstantRange({ from, to }: { from: string; to?: string }) {
  return (
    <span>
      <Instant value={from} /> → {to ? <Instant value={to} /> : '持续有效'}
    </span>
  );
}

// 过滤条里的下拉比表单里的格矮一号（py-1 / 12px），与同栏的搜索框齐高；表单那份 selectClass 不借来用。
export const filterSelectClass =
  'rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1 text-[12px] ' +
  'text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

/** 复制到剪贴板，成败都以 toast 反馈；不支持 clipboard 的上下文（非 https）如实说，不静默。 */
export function useCopyToClipboard() {
  const { addToast } = useToast();
  return (label: string, value: string) => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      addToast({ type: 'warning', title: '无法复制', message: '当前页面上下文不支持剪贴板（需 https 或 localhost）。' });
      return;
    }
    void navigator.clipboard.writeText(value).then(
      () => addToast({ type: 'success', title: '已复制', message: `${label}：${value}` }),
      (error: unknown) =>
        addToast({
          type: 'error',
          title: '复制失败',
          message: error instanceof Error ? error.message : String(error),
        }),
    );
  };
}

/** 抽屉里的一格：标签 + 值，可选复制按钮。用在 `<dl>` 里。 */
export function DetailRow({
  label,
  children,
  mono = false,
  onCopy,
}: {
  label: string;
  children: ReactNode;
  mono?: boolean;
  onCopy?: () => void;
}) {
  return (
    <div className="flex flex-col gap-0.5 py-2 border-b border-idpxyz-border last:border-b-0">
      <dt className="text-[11px] text-idpxyz-textMuted">{label}</dt>
      <dd className={`flex items-start justify-between gap-2 text-[13px] text-idpxyz-text ${mono ? 'font-mono' : ''}`}>
        <span className="break-all">{children}</span>
        {onCopy ? (
          <Button variant="ghost" size="sm" className="shrink-0" onClick={onCopy}>
            复制
          </Button>
        ) : null}
      </dd>
    </div>
  );
}
