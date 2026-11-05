import { Fragment, type ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import type { RegisteredAddressRecord } from './api';
import { countryLabel } from '../refdata-names';
import { useRefdataNames } from '../use-refdata-names';
import { CopyAside } from './detail-primitives';
import { addressCopyText, addressLinesOf, glueBracketGroups } from './legal-entity-profile';
import './legal-entity-profile-readout.css';

/**
 * 读出的一张卡（票 company-subject-ux/09，照 WMS「ASN 管理」详情栏的「供应商与来源 / 物流与预约」）：细边框、浅底标题条
 * （图标 + 半粗标题），卡内才是标签 / 值网格或表。此前各组只靠标题与间距分开，宽屏上几组读出摊成一面，眼睛找不到组的
 * 边界；卡把边界画出来，标题条的浅底与图标让人一眼认出这一块讲什么。图标与头区 LegalEntityHeaderParts 同一套画法
 * （lucide、3.5 单位、弱显）。标题条取工作区那层冷灰（bg-idpxyz-editor），不取 hover 色——浅色主题里 hover 色就是选中蓝，
 * 会把标题条读成被选中的一行。不用 ui-primitives 的 Card：它带悬停投影与过渡，是给可点的卡的；读出卡不可点。
 */
export function ProfileReadoutCard({
  icon: Icon,
  title,
  extra,
  actions,
  children,
}: {
  icon: LucideIcon;
  title: string;
  /** 标题后的小字（「2 个」「4 位」），弱显。 */
  extra?: string;
  /** 标题条右端的动作（「编辑」「添加…」，票 company-subject-section-edit/01）；只读的读出不给。 */
  actions?: ReactNode;
  children: ReactNode;
}) {
  // 读出的字号（票 company-subject-ux/13 第 3 条）：值 14 / 标签 13 / 等宽 13 / 卡标题 14，行高 6；联系人卡与收款账户表同一把尺。
  // 卡自己是容器（legal-entity-readout-card）：窄于 28rem 时字段的标签改到值上面，规则在 legal-entity-profile-readout.css。
  return (
    <section className="legal-entity-readout-card overflow-hidden rounded-lg border border-idpxyz-border bg-idpxyz-bg">
      <div className="flex items-center gap-2 border-b border-idpxyz-border bg-idpxyz-editor px-4 py-2">
        <Icon className="h-3.5 w-3.5 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
        <h4 className="type-section font-semibold leading-5 text-idpxyz-textBright">{title}</h4>
        {extra ? <span className="type-body leading-5 text-idpxyz-breadcrumb">{extra}</span> : null}
        {actions ? <div className="ml-auto flex shrink-0 items-center gap-1">{actions}</div> : null}
      </div>
      <div className="px-4 py-3">{children}</div>
    </section>
  );
}

export function ProfileReadoutGrid({ children }: { children: ReactNode }) {
  return <dl className="legal-entity-readout-grid">{children}</dl>;
}

/** 空格一律显「—」（spec 公司主体 UX「统一用语」）：只有结论句才用文字，「未配置」一词留给资料状态那种句子。 */
export const absentValueMark = '—';

export function ProfileReadoutField({
  label,
  title,
  value,
  mono = false,
  emphasized = false,
  wide = false,
}: {
  label: string;
  /** 标签的悬停说明：标签列固定宽、不折行，放不下的限定语（「税务 / 海关」）进这里。 */
  title?: string;
  /** 字串或已排好版的节点（如 ProfileReadoutAddress）；空串、undefined 与旧写法的「未配置」都算没有。 */
  value?: ReactNode;
  mono?: boolean;
  emphasized?: boolean;
  wide?: boolean;
}) {
  const absent = value === undefined || value === null || value === '' || value === '未配置';
  return (
    <div className={`legal-entity-readout-field ${wide ? 'legal-entity-readout-field-wide' : ''}`}>
      <dt className="type-body leading-6 text-idpxyz-breadcrumb" title={title}>
        {label}
      </dt>
      <dd className={`min-w-0 leading-6 ${mono ? 'font-mono type-body' : 'type-value'} ${absent ? 'text-idpxyz-breadcrumb' : 'text-idpxyz-text'} ${emphasized && !absent ? 'font-medium text-idpxyz-textBright' : ''}`}>
        {absent ? absentValueMark : value}
      </dd>
    </div>
  );
}

/**
 * 地址格：按录入的行逐行显（票 company-subject-ux/12 第 1 条），国家 / 地区经基础数据译名作末行、弱色（ADR-0335 决定七）。
 * 不把国家码当前缀——运营读地址先看街道，国家是补充信息，译成「中国（CN）」后放句首会压过地址本身。末行取灰而不取
 * 强调色：票 08 时担心的「另起一行像链接」出在换成链接色上，灰的末行只是退后一步。资料读出与版本历史快照共用这一份，
 * 版式只定一处。
 *
 * 地址是文本类（票 company-subject-ux/14）：整段挂在 CopyAside 里，复制出去按行换行、国家译名作末行（addressCopyText）。
 * 行与行用 <br /> 隔、不再各自成块：复制图标要挂在末行的行尾，块级的行会把它挤到下一行去。国家译名在这里按名表查而不摆
 * CountryLabel：复制出去的那一份也要同一个译名。
 */
export function ProfileReadoutAddress({ address, name = '地址' }: { address: RegisteredAddressRecord; name?: string }) {
  const lines = addressLinesOf(address);
  const country = address.country ? countryLabel(address.country, useRefdataNames()) : '';
  return (
    <CopyAside value={addressCopyText(address, country)} name={name}>
      {lines.length > 0
        ? lines.map((line, index) => (
            <Fragment key={`${index}-${line}`}>
              {index > 0 ? <br /> : null}
              <ReadoutText text={line} />
            </Fragment>
          ))
        : absentValueMark}
      {country ? (
        <>
          {lines.length > 0 ? <br /> : null}
          <span className="text-idpxyz-breadcrumb">{country}</span>
        </>
      ) : null}
    </CopyAside>
  );
}

/**
 * 三个地址并排的一行（票 company-subject-ux/12 第 1 条）：标签在上、地址在下，宽时一地址一栏、窄时纵向堆叠。地址逐行显
 * 之后一格就有三四行高，再塞进「标签在左」的两列网格，半列宽的地址行行折、左边的标签孤零零悬在第一行；标签在上让
 * 每栏自成一块，三个地址对着读也齐。没有的地址不画栏：画出来是一栏「—」，占掉另两栏的宽却没有信息。
 */
export function ProfileReadoutAddresses({
  items,
}: {
  items: readonly { key: string; label: string; address?: RegisteredAddressRecord }[];
}) {
  return (
    <dl className="legal-entity-readout-addresses">
      {items.map((item) => (
        <div key={item.key} className="min-w-0">
          <dt className="type-body leading-6 text-idpxyz-breadcrumb">{item.label}</dt>
          <dd className={`min-w-0 type-value leading-6 ${item.address ? 'text-idpxyz-text' : 'text-idpxyz-breadcrumb'}`}>
            {item.address ? <ProfileReadoutAddress address={item.address} name={item.label} /> : absentValueMark}
          </dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * 读出里会折行的一段文字（户名、抬头、地址行、备注）：全角括号组粘住前一个字不折开（glueBracketGroups 说明为什么），
 * 其余照常。粘住的一段用 nowrap 而不是插零宽连字，复制出去的仍是原文。
 */
export function ReadoutText({ text }: { text: string }) {
  return (
    <>
      {glueBracketGroups(text).map((segment, index) =>
        segment.glue ? <span key={`${index}-${segment.text}`} className="whitespace-nowrap">{segment.text}</span> : <Fragment key={`${index}-${segment.text}`}>{segment.text}</Fragment>,
      )}
    </>
  );
}

/**
 * 值格里的两列小表（票 company-subject-ux/08 第 5 条）：左列是类型名或语言（弱显），右列是号或抬头。几个号用分号串成一段
 * 扫不出来，一行一个才认得出哪个号是哪种。左列最多占一半宽，长类型名折行而不是把号挤掉；等宽字体只给号。
 * 左列按标签那一档、右列按值那一档（票 company-subject-ux/13 第 3 条把值提到 14、等宽留在 13）：两列同一行高，
 * 号比字小一号是等宽字本来就显大一圈，看着才齐。
 */
export function ProfileReadoutPairs({
  rows,
  mono = false,
  headNowrap = false,
}: {
  rows: readonly { key: string; head: ReactNode; headTitle?: string; value: ReactNode }[];
  /** 右列用等宽字体（号）；抬头是文字，不用。 */
  mono?: boolean;
  /** 左列不折行：给语言那种短而固定的头（「简体中文（zh-Hans）」在 zh- 处折开就认不出）；类型名可以很长，仍要折。 */
  headNowrap?: boolean;
}) {
  return (
    <dl className="grid grid-cols-[fit-content(50%)_minmax(0,1fr)] gap-x-3 gap-y-1">
      {rows.map((row) => (
        <Fragment key={row.key}>
          <dt className={`type-body leading-6 text-idpxyz-breadcrumb ${headNowrap ? 'whitespace-nowrap' : ''}`} title={row.headTitle}>
            {row.head}
          </dt>
          <dd className={`min-w-0 leading-6 text-idpxyz-text ${mono ? 'select-text font-mono type-body' : 'type-value'}`}>{row.value}</dd>
        </Fragment>
      ))}
    </dl>
  );
}
