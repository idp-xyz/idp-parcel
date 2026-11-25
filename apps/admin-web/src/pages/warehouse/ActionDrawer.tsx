import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@idpxyz/ui-primitives';
import { GuidedFormBody, type GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { problemNote } from './presentation';
import type { WarehouseLookups } from './use-catalogues';
import { useCloseOnHashChange } from './use-close-on-hash';

export interface ActiveDrawerAction {
  target: GuidedActionTarget;
  prefill?: GuidedFormValues;
  nonce: number;
}

/**
 * 仓配上下文操作弹窗：点击页头「+ 新建」或行内快捷按钮原地打开聚焦表单，
 * 提交完成后即时回调刷新列表，不再跳进十几项按钮混排的「处理」签。
 */
export function ActionDrawer({
  moduleId,
  active,
  lookups,
  onClose,
  onSubmitted,
}: {
  moduleId: string;
  active: ActiveDrawerAction | null;
  lookups?: WarehouseLookups;
  onClose: () => void;
  onSubmitted?: (outcome: string) => void;
}) {
  // 与 WmsDetailModal 同因：标签保活下换页不卸载，开着的抽屉会盖到下一页。
  useCloseOnHashChange(active !== null, onClose);
  return (
    <Dialog open={active !== null} onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-w-3xl max-h-[85vh] flex flex-col">
        {active ? (
          <>
            {/* 标题下不摆接口路径：读这张表单的是仓里的人，路径收在表单底部的技术详情里，排障时再展开。 */}
            <DialogHeader>
              <DialogTitle>{active.target.title}</DialogTitle>
            </DialogHeader>
            <DialogBody className="overflow-y-auto">
              <GuidedFormBody
                key={`${active.target.id}:${active.nonce}`}
                moduleId={moduleId}
                target={active.target}
                problemNote={problemNote}
                prefill={active.prefill}
                lookups={lookups}
                onSubmitted={onSubmitted}
                bare
              />
            </DialogBody>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                关闭
              </Button>
            </DialogFooter>
          </>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
