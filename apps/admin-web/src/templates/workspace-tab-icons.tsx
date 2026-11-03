import type { ReactNode } from 'react';
import {
  AlertTriangle,
  ClipboardList,
  Contact,
  FileText,
  History,
  Layers,
  Link2,
  Paperclip,
  ScrollText,
  type LucideIcon,
} from 'lucide-react';
import { workspaceTabLabels, type WorkspaceTabId } from './workspace-tabs';

// 对象工作区签的小图标（票 company-subject-ux/10）。另起一份 .tsx 而不并进 workspace-tabs.ts：那份要保持不引 React，
// 让 run-tests 的 CommonJS 发射能加载它；图标是渲染层的事，归这里。
// 用 Record 而不是 Partial：固定表每添一签，这里不补图标 tsc 就报，不会出一个光秃秃的签。

/** id → 图标。一签一图、互不重复：图标是给人扫一眼认签用的，两签同图比没图更误导。 */
export const workspaceTabIcons: Record<WorkspaceTabId, LucideIcon> = {
  summary: FileText,
  contacts: Contact,
  'profile-versions': Layers,
  'identity-revisions': ClipboardList,
  timeline: History,
  related: Link2,
  exceptions: AlertTriangle,
  documents: Paperclip,
  audit: ScrollText,
};

/**
 * 签上的「小图标 + 词」。词默认取 workspaceTabLabels；页内详情栏的签另有口径（如往来单位栏的「全景」）时经 children 给词，
 * 图标仍按 id 从同一张表取——全站同一种内容同一个图标，词可以随栏不同。计数徽章由调用方接在本件之后，不进来。
 * 图标非选中态弱色，选中态跟随词的颜色而不另定一种强调色：各栏给签词的选中色不一样（模板是 textBright、公司主体栏是
 * accent），图标只管「与词同色」。`data-state` 挂在 TabsTrigger 身上，原语给它带了 `group`，这里借 group 变体读。
 */
export function WorkspaceTabLabel({ id, children }: { id: WorkspaceTabId; children?: ReactNode }) {
  const Icon = workspaceTabIcons[id];
  return (
    <>
      <Icon
        size={14}
        aria-hidden="true"
        className="shrink-0 text-idpxyz-breadcrumb transition-colors group-data-[state=active]:text-current"
      />
      {children ?? workspaceTabLabels[id]}
    </>
  );
}
