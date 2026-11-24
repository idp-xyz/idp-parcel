import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@idpxyz/ui-primitives';
import { GuidedFormBody, type GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { problemNote } from './presentation';
import type { WarehouseLookups } from './use-catalogues';

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
  return (
    <Dialog open={active !== null} onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className="max-w-3xl max-h-[85vh] flex flex-col">
        {active ? (
          <>
            <DialogHeader>
              <DialogTitle>{active.target.title}</DialogTitle>
              <DialogDescription className="font-mono type-caption">{active.target.endpoint}</DialogDescription>
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
