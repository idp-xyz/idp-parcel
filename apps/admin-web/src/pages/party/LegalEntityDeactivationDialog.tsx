import { X } from 'lucide-react';
import { Button, Dialog, DialogBody, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@idpxyz/ui-primitives';
import type { GroupLegalEntityRecord } from './api';
import { IdentityDeactivationForm } from './IdentityDeactivationForm';
import { nounOf } from '../../vocabulary/objects';

/**
 * 公司主体页的停用对话框（票 company-subject-ux/03）：停用留在这家公司自己的页上，不再把人送去往来单位页的停用表单。
 * 表单还是 IdentityDeactivationForm 那一份（写口不变，仍是 `/commercial-party-identity-deactivations` 一个命令带种类），
 * 只是把种类与对象锁死——开在谁的页上就停谁，停错一个收不回。标题带公司名，让人在按下之前再认一眼。
 *
 * 内容随对话框关闭卸载（Radix 默认），下次打开草稿从头起：停用不是会反复填的表单，留着上一次填了一半的时间只会误导。
 */
export function LegalEntityDeactivationDialog({
  row,
  open,
  onOpenChange,
  onDeactivated,
}: {
  row: GroupLegalEntityRecord;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 登记册答 `DEACTIVATED` 后回调；对话框已先关掉，页面借它重取列表让「已停用」立刻可见。 */
  onDeactivated: () => void;
}) {
  const name = row.partyNameKnown && row.partyName ? row.partyName : row.legalEntityId;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" className="max-h-[85vh] overflow-y-auto">
        <DialogHeader className="items-start">
          <div>
            <DialogTitle>停用{nounOf('companyEntity')} {name}</DialogTitle>
            <DialogDescription className="mt-1">从停用时间起，新的签约、开票不再用它；已有记录保留，停用后不能撤回。</DialogDescription>
          </div>
          <Button type="button" variant="ghost" size="icon" aria-label="关闭" onClick={() => onOpenChange(false)}>
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </DialogHeader>
        <DialogBody>
          <IdentityDeactivationForm
            key={row.legalEntityId}
            // 参与方册的答案只在种类为业务参与方时用得上；这里种类锁死为法人，法人册由表单自己读一次（候选与版本号建议同一份）。
            parties={null}
            locked={{ kind: 'LEGAL_ENTITY', id: row.legalEntityId }}
            // 确认框里只说「公司主体」：词表的「责任法人」只在解释概念时出现（spec 统一用语）。
            subjectLabel={nounOf('companyEntity')}
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
