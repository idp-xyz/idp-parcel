import type { ReactNode } from 'react';
import {
  AlertTriangle,
  AlignLeft,
  ClipboardList,
  Contact,
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

/**
 * id → 图标。一签一图、互不重复：图标是给人扫一眼认签用的，两签同图比没图更误导。概要不用 FileText：它在同一张页上
 * 已是「更新公司资料」主动作与导航「供应商协议」的图，签上再用就撞义（评审钉 `faafb578d`）；AlignLeft 读作「几行摘要」。
 */
export const workspaceTabIcons: Record<WorkspaceTabId, LucideIcon> = {
  summary: AlignLeft,
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
 * 图标始终跟随词的颜色（`text-current`），不另定一种色：各栏给签词的非选中色与选中色都不一样（模板是 textMuted →
 * textBright、公司主体栏是 breadcrumb → accent），图标只管「与词同色」。浅色主题下 breadcrumb 与 textMuted 同值，暗色下
 * 差一档——那是色板的漂移，不该在图标上再漂一次（评审钉 `faafb578d`）。
 */
export function WorkspaceTabLabel({ id, children }: { id: WorkspaceTabId; children?: ReactNode }) {
  const Icon = workspaceTabIcons[id];
  return (
    <>
      <Icon size={14} aria-hidden="true" className="shrink-0 text-current transition-colors" />
      {children ?? workspaceTabLabels[id]}
    </>
  );
}
