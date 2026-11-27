import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { CodeRef } from '../../components/CodeRef';

/**
 * 库存右栏。库存格、整箱、汇总、占用、流水都没有自己的单据地址，所以不摆「整页打开」。
 * 头区照单据栏：身份在左，能做的动作是一组靠右，关闭钉在右上角。正文是数量板、一句说明、资料格，清单用子表。
 */
export function StockDetail({
  kind,
  code,
  codeTitle,
  badge,
  subtitle,
  actions,
  onClose,
  children,
}: {
  kind: string;
  code?: string;
  codeTitle?: string;
  badge?: ReactNode;
  subtitle?: string;
  actions?: ReactNode;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="shrink-0 border-b border-idpxyz-border px-4 py-3">
        <div className="flex items-start gap-2">
          <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
            <div className="flex min-w-0 grow items-center gap-2">
              <span className="type-section font-semibold text-idpxyz-textBright">{kind}</span>
              {code ? <CodeRef value={code} title={codeTitle ?? kind} /> : null}
              {badge}
            </div>
            {actions ? <div className="flex w-max shrink-0 flex-nowrap items-center gap-2">{actions}</div> : null}
          </div>
          <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={onClose}>
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
        {subtitle ? <p className="mt-1 type-caption leading-4 text-idpxyz-textMuted">{subtitle}</p> : null}
      </div>
      <div className="min-h-0 flex-1 overflow-auto">
        <div className="flex flex-col gap-5 p-4">{children}</div>
      </div>
    </div>
  );
}

/** 这一行要先回答的几个数。字号只在这块放大，列表里的数不跟着变。 */
export function LedgerFigures({ items }: { items: ReadonlyArray<{ label: string; value: ReactNode }> }) {
  if (items.length === 1) {
    const item = items[0];
    return (
      <div className="flex items-baseline justify-between gap-3 rounded-lg border border-idpxyz-border bg-idpxyz-sidebar px-4 py-3">
        <span className="type-caption text-idpxyz-textMuted">{item.label}</span>
        <span className="type-title tabular-nums text-idpxyz-textBright">{item.value}</span>
      </div>
    );
  }
  return (
    <div className="grid grid-cols-3 gap-px overflow-hidden rounded-lg border border-idpxyz-border bg-idpxyz-border">
      {items.map((item) => (
        <div key={item.label} className="min-w-0 bg-idpxyz-sidebar px-3 py-3">
          <div className="type-caption text-idpxyz-textMuted">{item.label}</div>
          <div className="mt-1 type-title tabular-nums text-idpxyz-textBright">{item.value}</div>
        </div>
      ))}
    </div>
  );
}

/** 可发是 0 时的那一句，单独一块，不混进资料格。 */
export function StockNote({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-lg border border-[color:var(--ds-status-danger-border)] bg-[color:var(--ds-status-danger-bg-subtle)] px-3 py-2 type-body text-idpxyz-text">
      {children}
    </p>
  );
}
