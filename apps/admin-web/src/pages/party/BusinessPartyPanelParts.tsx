import { Children, type ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import { Instant } from './detail-primitives';
import type { PartyRelationshipView } from './party-overview';
import './business-party-panel.css';

// 往来单位工作台右栏各签共用的几件（票 business-party-redesign/05）。一条规则管各签的容器：按话题分组的字段用卡片加小留白
// （PanelCard、PanelNoteCard），表格与时间线通栏、分区头是一条带底色的窄带（PanelBand）。

/** 行文里的链接：字号随所在的那一行，长了照常折行。链接色取深一档的那一个，淡底与悬停底上都读得清。 */
export const panelTextLinkClass =
  'rounded text-[color:var(--ds-text-link-hover)] hover:underline focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-idpxyz-accent';

/** 卡头、段头与行尾的文字链接：比正文小一档，不折行。 */
export const panelLinkClass = `whitespace-nowrap type-body ${panelTextLinkClass}`;

/**
 * 概要与联系人签里的一张卡：卡头是图标、标题与右端的一个去处（「全部 N 段」「编辑」），卡身贴着内容的高度，不为与旁边的卡
 * 等高留白。`readout` 的卡里摆资料读出的字段格（LegalEntityProfileReadout.tsx）：卡自己是那套字段格的容器，窄了标签改到值上面。
 * `flush` 的卡身不留内边距，给一行一行通到卡边的清单。
 */
export function PanelCard({
  icon: Icon,
  title,
  extra,
  action,
  readout = false,
  flush = false,
  children,
}: {
  icon: LucideIcon;
  title: string;
  /** 标题后的小字（「2 个」）。 */
  extra?: string;
  action?: ReactNode;
  readout?: boolean;
  flush?: boolean;
  children: ReactNode;
}) {
  return (
    <section className={`${readout ? 'legal-entity-readout-card ' : ''}min-w-0 overflow-hidden rounded-lg border border-idpxyz-border bg-idpxyz-bg`}>
      <header className="flex items-center gap-2 border-b border-idpxyz-border px-3.5 py-2.5">
        <Icon className="h-4 w-4 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
        <h3 className="type-section leading-5 text-idpxyz-textBright">{title}</h3>
        {extra ? <span className="type-body leading-5 text-idpxyz-breadcrumb">{extra}</span> : null}
        {action ? <span className="ml-auto flex shrink-0 items-center">{action}</span> : null}
      </header>
      {flush ? children : <div className="px-3.5 py-3">{children}</div>}
    </section>
  );
}

/** 卡身里的一句「没有」：居中、弱色，下面可以再跟一两行说明与一个去处。 */
export function PanelCardEmpty({ title, lines = [], children }: { title: string; lines?: readonly string[]; children?: ReactNode }) {
  return (
    <div className="px-4 py-6 text-center">
      <p className="type-value leading-6 text-idpxyz-breadcrumb">{title}</p>
      {lines.map((line) => (
        <p key={line} className="type-body leading-5 text-idpxyz-breadcrumb">
          {line}
        </p>
      ))}
      {Children.toArray(children).length > 0 ? <div className="mt-3 flex flex-wrap justify-center gap-2">{children}</div> : null}
    </div>
  );
}

/**
 * 说明卡：这一块没有内容可摆时的一张卡——一句标题说怎么回事，下面几行说空在哪、下一步去哪，再下面是去处。只有一句话
 * （还在读、读不到）时不给标题。
 */
export function PanelNoteCard({ title, lines, children }: { title?: string; lines: readonly string[]; children?: ReactNode }) {
  return (
    <section className="min-w-0 rounded-lg border border-idpxyz-border bg-idpxyz-bg px-4 py-9 text-center">
      {title ? <h3 className="mb-1 type-section leading-6 text-idpxyz-text">{title}</h3> : null}
      {lines.map((line) => (
        <p key={line} className="type-value leading-6 text-idpxyz-breadcrumb">
          {line}
        </p>
      ))}
      {Children.toArray(children).length > 0 ? <div className="mt-3 flex flex-wrap justify-center gap-2">{children}</div> : null}
    </section>
  );
}

/**
 * 通栏分区的段头：一条带底色的窄带，文字的左缘与右栏头区对齐。标题后是这一段的数，右端是这一段的去处（文字链接，不用按钮——
 * 按钮会把窄带撑高）。
 */
export function PanelBand({
  icon: Icon,
  title,
  count,
  children,
}: {
  icon: LucideIcon;
  title: string;
  count?: string;
  children?: ReactNode;
}) {
  return (
    <header className="flex min-h-10 flex-wrap items-center gap-x-2 gap-y-1 border-b border-idpxyz-border bg-idpxyz-editor px-6 py-2.5">
      <Icon className="h-4 w-4 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
      <h3 className="type-section leading-5 text-idpxyz-textBright">{title}</h3>
      {count ? <span className="type-body leading-5 tabular-nums text-idpxyz-breadcrumb">{count}</span> : null}
      {Children.toArray(children).length > 0 ? <span className="ml-auto flex shrink-0 items-center gap-4">{children}</span> : null}
    </header>
  );
}

/**
 * 一段合作关系的有效期，只显到日（完整时刻在悬停里）。无终点说「长期有效」，与本页「合作关系」视图同一个词：有效期间没有声明
 * 终点即一直有效，不是「未知」。
 */
export function RelationshipPeriod({ view }: { view: Pick<PartyRelationshipView, 'effectiveStartsAt' | 'effectiveEndsAt'> }) {
  return (
    <>
      <Instant value={view.effectiveStartsAt} precision="day" /> →{' '}
      {view.effectiveEndsAt ? <Instant value={view.effectiveEndsAt} precision="day" /> : '长期有效'}
    </>
  );
}
