import type { ReactNode } from 'react';
import { Button, Spinner, type ConfirmDialogRenderFooterContext } from '@idpxyz/ui-primitives';
import { dangerActionClass, primaryActionClass } from './primary-action';

/**
 * 确认对话框的页脚（票 group-legal-entities-uiux/12），交给上游 ConfirmDialog 的 `renderFooter`。上游默认页脚的确认键按
 * tone 取 Button 变体：danger 取 danger（淡红底配 400 红字，浅色下读不清），其余全取 default，而 default 的
 * `bg-idpxyz-accent/20` 在本仓生不出来（见 primary-action.ts），确认键就只剩强调色字、没有底。这里换成实底的
 * 主动作 / 危险动作，行为照上游默认页脚：取消在左、确认在右，提交中两键都禁用，确认键带转圈与 aria-busy。
 */
export function confirmDialogFooter(ctx: ConfirmDialogRenderFooterContext): ReactNode {
  const confirmClass = ctx.confirmVariant === 'danger' ? dangerActionClass : primaryActionClass;
  return (
    <>
      <Button variant="outline" size="sm" className="text-[12px] text-idpxyz-breadcrumb" onClick={ctx.onCancel} disabled={ctx.pending}>
        {ctx.cancelLabel}
      </Button>
      <Button
        variant="default"
        size="sm"
        className={`${confirmClass} text-[12px]`}
        onClick={ctx.onConfirm}
        disabled={ctx.pending}
        aria-busy={ctx.pending}
      >
        {ctx.pending ? (
          <span className="inline-flex items-center gap-1.5">
            <Spinner size="sm" className="!h-3 !w-3 shrink-0 text-current" />
            {ctx.confirmLabel}
          </span>
        ) : (
          ctx.confirmLabel
        )}
      </Button>
    </>
  );
}
