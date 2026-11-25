import {
  Button,
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@idpxyz/ui-primitives';
import { useState } from 'react';
import type { WarehouseActionSpec } from './actions';
import { useWarehouseTargetMap } from './ActionPanel';
import { GuidedFormBody, type GuidedActionTarget } from './GuidedActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { problemNote } from './presentation';
import { useWarehouseLookups, type WarehouseLookups } from './use-catalogues';
import { useCloseOnHashChange } from './use-close-on-hash';

export interface ActiveDrawerAction {
  target: GuidedActionTarget;
  prefill?: GuidedFormValues;
  nonce: number;
}

export interface ActionDrawerControl {
  lookups: WarehouseLookups;
  /** 这个动作此刻摆不摆：操作者明确没有那一种授予的不摆。 */
  has: (id: string) => boolean;
  /** 打开一个动作的表单，可带上从行或单据带出的标识。没有授予的动作打不开。 */
  open: (id: string, prefill?: GuidedFormValues) => void;
  active: ActiveDrawerAction | null;
  close: () => void;
}

/** 一页或一张单据上的动作抽屉：按授予筛过的动作、表单要用的名册、此刻开着的那一个。 */
export function useActionDrawer(specs: readonly WarehouseActionSpec[]): ActionDrawerControl {
  const lookups = useWarehouseLookups();
  const targets = useWarehouseTargetMap(specs);
  const [active, setActive] = useState<ActiveDrawerAction | null>(null);
  return {
    lookups,
    has: (id) => targets.has(id),
    open: (id, prefill) => {
      const target = targets.get(id);
      if (!target) return;
      setActive((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
    },
    active,
    close: () => setActive(null),
  };
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
