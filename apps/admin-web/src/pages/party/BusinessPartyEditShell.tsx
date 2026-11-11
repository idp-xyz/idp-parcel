import type { ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { PencilLine, type LucideIcon } from 'lucide-react';
import { StatusPill } from '../../components/StatusPill';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import type { BusinessPartyRecord } from './api';
import type { PartyTileKind } from './business-parties-workbench';
import { editSectionDomId } from './business-party-edit-origin';
import { PartyTile } from './party-role-visuals';
import './business-party-edit-shell.css';

// 往来单位编辑态的外壳与它的几件小零件（票 business-party-redesign/11）。「更新资料」与「更正登记信息」共用：这家单位的页进入
// 编辑态后只剩三样——页头（认得出是哪一家、正在做什么）、正文（左边一列分段的锚点，右边一张张带标题的卡）、底部一条固定的
// 动作条。只读时挂在下面的资料版本、登记记录、关联都不摆：一页一个写口在开，别的内容留着只会让人以为还能点。
//
// 不用共用的对象页模板（DetailPageTemplate）：模板在页头下面带一条分区签，编辑态只有一个分区，那条签只剩一枚；本 spec 也定了
// 本页先行、不动共用模板。只读的对象页照旧用模板。
//
// 页头右端的标记、锚点列与动作条的内容都由表单自己经槽位送进来（createPortal）：在哪一版的基础上改、表单分几段、此刻是填写
// 还是核对，只有表单知道。

/** 外壳留给表单的三个槽位；还没挂上时是 null，表单把那一块暂时摆在自己的正文里。 */
export interface EditSlots {
  tag: HTMLElement | null;
  nav: HTMLElement | null;
  footer: HTMLElement | null;
}

/** 把一块内容送进槽位；槽位还没挂上就原地摆。 */
export function intoSlot(slot: HTMLElement | null, node: ReactNode): ReactNode {
  return slot ? createPortal(node, slot) : node;
}

export function BusinessPartyEditShell({
  row,
  tileKind,
  label,
  onTagSlot,
  onNavSlot,
  onFooterSlot,
  children,
}: {
  row: BusinessPartyRecord;
  tileKind: PartyTileKind;
  /** 这一整块的读屏名：「更新资料」或「更正登记信息」。 */
  label: string;
  onTagSlot: (element: HTMLElement | null) => void;
  onNavSlot: (element: HTMLElement | null) => void;
  onFooterSlot: (element: HTMLElement | null) => void;
  children: ReactNode;
}) {
  return (
    <section className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor" aria-label={label}>
      {/* 页头与名录右栏的头区同一种摆法（图标块、名称、状态），名称下面是编码；动作一枚都不摆，右端只有表单送来的那枚标记。 */}
      <header className="flex flex-wrap items-center gap-x-3.5 gap-y-2 border-b border-idpxyz-border bg-idpxyz-bg px-6 py-3.5">
        <PartyTile large kind={tileKind} />
        <div className="min-w-0 flex-[1_1_280px]">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h1 className="line-clamp-2 min-w-0 break-words type-display font-semibold leading-7 text-idpxyz-textBright" title={row.partyName}>
              {row.partyName}
            </h1>
            <StatusPill code={row.status} />
          </div>
          <p className="mt-0.5 font-mono type-body text-idpxyz-breadcrumb">{row.partyId}</p>
        </div>
        <div ref={onTagSlot} className="ml-auto shrink-0" />
      </header>
      <div className="business-party-edit-body min-h-0 flex-1 overflow-y-auto">
        <div className="business-party-edit-wrap">
          <div ref={onNavSlot} className="business-party-edit-nav" />
          <div className="min-w-0">{children}</div>
        </div>
      </div>
      <footer className="business-party-edit-footer border-t border-idpxyz-border bg-idpxyz-bg px-6 py-3">
        <div ref={onFooterSlot} className="business-party-edit-footer-inner" />
      </footer>
    </section>
  );
}

/** 页头右端的标记：正在做什么、在哪一版的基础上改。等着的事的语气（与「待生效」的提示条同一种），不是报错。 */
export function EditTag({ children }: { children: ReactNode }) {
  return (
    <span className={`inline-flex min-h-7 items-center gap-1.5 rounded-md border px-2.5 py-1 type-body leading-4 ${statusToneBanner.warning}`}>
      <PencilLine className={`h-3.5 w-3.5 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
      {children}
    </span>
  );
}

/** 把表单的某一段滚到正文区顶上。 */
export function scrollToEditSection(section: string): void {
  document.getElementById(editSectionDomId(section))?.scrollIntoView({ block: 'start' });
}

/** 锚点列：表单分几段就几枚，点了跳到那一段。段的次序与名字由表单交进来。 */
export function EditSectionNav({ sections }: { sections: readonly { id: string; label: string }[] }) {
  return (
    <nav aria-label="表单分段" className="flex flex-col gap-0.5">
      {sections.map((section) => (
        <button
          key={section.id}
          type="button"
          className="h-[30px] rounded-md px-2.5 text-left type-value text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent"
          onClick={() => scrollToEditSection(section.id)}
        >
          {section.label}
        </button>
      ))}
    </nav>
  );
}

/**
 * 表单的一段：一张带标题的卡，与只读时右栏概要的卡同一种长相（图标、标题），卡身里是这一段的格。`note` 是标题后的一句小字。
 * 滚到这一段时上面留出正文区的内边距那么多，卡的上缘不贴着正文区的边。
 */
export function EditSectionCard({
  id,
  icon: Icon,
  title,
  note,
  children,
}: {
  id: string;
  icon: LucideIcon;
  title: string;
  note?: string;
  children: ReactNode;
}) {
  const headingId = `${editSectionDomId(id)}-title`;
  return (
    <section id={editSectionDomId(id)} aria-labelledby={headingId} className="scroll-mt-4 rounded-lg border border-idpxyz-border bg-idpxyz-bg">
      <header className="flex flex-wrap items-center gap-x-2 gap-y-0.5 border-b border-idpxyz-border px-4 py-2.5">
        <Icon className="h-4 w-4 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
        <h2 id={headingId} className="type-section leading-5 text-idpxyz-textBright">
          {title}
        </h2>
        {note ? <span className="type-body leading-5 text-idpxyz-breadcrumb">{note}</span> : null}
      </header>
      <div className="flex flex-col gap-5 px-4 py-4">{children}</div>
    </section>
  );
}
