import { Dialog, DialogContent } from '@idpxyz/ui-primitives';
import { ClosureBody } from './VersionClosureDrawer';
import type { ClosureTarget } from './version-closure';

/**
 * 「收尾…」的对话框（票 commercial-registers-edit-containers/01）：从右栏详情的动作打开；收尾是一次短而不可撤回的写，照公司主体
 * 停用对话框的做法用模态，不再在右栏之上再叠一层抽屉。本体 ClosureBody 与旧抽屉同一份：打开时现读册面、认旧版与后继、确认框
 * 复述、答复原样示出（票 party-commercial-uiux/16）。内容随对话框关闭卸载，下次打开从头读。
 */
export function VersionClosureDialog({
  target,
  onClose,
  onClosed,
}: {
  target: ClosureTarget | null;
  onClose: () => void;
  /** 答复说收尾已落（或认出已落过）时调用，页面借它重取目录。 */
  onClosed: () => void;
}) {
  return (
    <Dialog open={target !== null} onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent size="xl" className="max-h-[85vh] overflow-y-auto p-0" aria-label="收尾旧版">
        {target ? (
          <ClosureBody key={`${target.kind}|${target.scope}|${target.objectId}|${target.version}`} target={target} onClose={onClose} onClosed={onClosed} />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
