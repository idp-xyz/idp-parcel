import { X } from 'lucide-react';
import { Button, Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyListResponseBody, BusinessPartyRecord } from './api';
import { deactivationLiveRelationshipsNote } from './business-party-panel-first-screen';
import { IdentityDeactivationForm } from './IdentityDeactivationForm';

/**
 * 往来单位页的停用对话框（票 business-party-edit-mode/01，照 LegalEntityDeactivationDialog）：停用留在这家单位自己的页上，
 * 不再把人送去列表页登记视图的停用表单。表单还是 IdentityDeactivationForm 那一份（写口不变，仍是一个命令带种类），只是把种类
 * 与对象锁死——开在谁的页上就停谁，停错一个收不回。标题带单位名，让人在按下之前再认一眼。参与方册的答案由页面传进来：
 * 这一册的候选与版本号建议都从它取，表单不另读。
 *
 * 内容随对话框关闭卸载（Radix 默认），下次打开草稿从头起：停用不是会反复填的表单，留着上一次填了一半的时间只会误导。
 */
export function BusinessPartyDeactivationDialog({
  row,
  parties,
  liveRelationships = null,
  open,
  onOpenChange,
  onDeactivated,
}: {
  row: BusinessPartyRecord;
  /** 页面持有的参与方列表答案；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 它名下在用的合作关系有几段（business-parties-workbench.ts 的 partyLiveRelationshipsOf）；不知道就是 null，说明里不提。 */
  liveRelationships?: number | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 登记册答 `DEACTIVATED` 后回调；对话框已先关掉，页面借它重取列表让「已停用」立刻可见。 */
  onDeactivated: () => void;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" className="max-h-[85vh] overflow-y-auto">
        <DialogHeader className="items-start">
          <div>
            <DialogTitle>停用往来单位 {row.partyName}</DialogTitle>
            <DialogDescription className="mt-1">
              从停用时间起，不再与它建立新的合作关系、合同与账户；已有记录保留，停用后不能撤回。
              {deactivationLiveRelationshipsNote(liveRelationships)}
            </DialogDescription>
          </div>
          <Button type="button" variant="ghost" size="icon" aria-label="关闭" onClick={() => onOpenChange(false)}>
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </DialogHeader>
        <DialogBody>
          <IdentityDeactivationForm
            key={row.partyId}
            parties={parties}
            locked={{ kind: 'BUSINESS_PARTY', id: row.partyId }}
            subjectLabel="往来单位"
            onDeactivated={() => {
              onOpenChange(false);
              onDeactivated();
            }}
          />
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
