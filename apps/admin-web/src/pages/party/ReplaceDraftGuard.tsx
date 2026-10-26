import { useState, type ReactNode } from 'react';
import { ConfirmDialog } from '@idpxyz/ui-primitives';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { hasUnsavedDraft } from './unsaved-guard';

/**
 * 目录行上「发新版本」要换掉发布表单里一份已填未发布的草稿时，先问一句（票 party-commercial-uiux/17）。草稿跨页签保留
 * （票 09）之后，它可能躺在没打开的页签里，点目录行上的按钮悄悄换掉它，人看不见丢了什么。
 *
 * `groupOf` 交出这一行要进的发布表单所属的组（unsaved-guard 的 publicationGroup）；那一组没有没提交的输入就直接放行。
 * 交回的 `start` 给目录行当回调，`dialog` 摆在页面树里。
 */
export function useReplaceDraftGuard<Source>(
  groupOf: (source: Source) => string,
  run: (source: Source) => void,
  formName: string,
): { start: (source: Source) => void; dialog: ReactNode } {
  const [pending, setPending] = useState<Source | null>(null);
  const start = (source: Source) => (hasUnsavedDraft(groupOf(source)) ? setPending(source) : run(source));
  const dialog = (
    <ConfirmDialog
      renderFooter={confirmDialogFooter}
      open={pending !== null}
      tone="warning"
      title="换掉已填的草稿？"
      message={`「${formName}」里已经填了内容、还没有发布。继续会把它换成这一版的预填内容，原来填的不会保留。`}
      confirmLabel="换掉草稿"
      cancelLabel="保留原草稿"
      onConfirm={() => {
        const source = pending;
        setPending(null);
        if (source !== null) run(source);
      }}
      onCancel={() => setPending(null)}
    />
  );
  return { start, dialog };
}
