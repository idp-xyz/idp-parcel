import { Fragment, type ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import type { RegisteredAddressRecord } from './api';
import { CountryLabel } from '../refdata-labels';
import { addressLinesOf } from './legal-entity-profile';
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
  children,
}: {
  icon: LucideIcon;
  title: string;
  /** 标题后的小字（「2 个」「4 位」），弱显。 */
  extra?: string;
  children: ReactNode;
}) {
  return (
    <section className="overflow-hidden rounded-lg border border-idpxyz-border bg-idpxyz-bg">
      <div className="flex items-center gap-2 border-b border-idpxyz-border bg-idpxyz-editor px-4 py-2">
        <Icon className="h-3.5 w-3.5 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
        <h4 className="text-[13px] font-semibold leading-5 text-idpxyz-textBright">{title}</h4>
        {extra ? <span className="text-[12px] leading-5 text-idpxyz-breadcrumb">{extra}</span> : null}
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
      <dt className="text-[12px] leading-5 text-idpxyz-breadcrumb" title={title}>
        {label}
      </dt>
      <dd className={`min-w-0 text-[13px] leading-5 ${absent ? 'text-idpxyz-breadcrumb' : 'text-idpxyz-text'} ${mono ? 'font-mono' : ''} ${emphasized && !absent ? 'font-medium text-idpxyz-textBright' : ''}`}>
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
 */
export function ProfileReadoutAddress({ address }: { address: RegisteredAddressRecord }) {
  const lines = addressLinesOf(address);
  return (
    <>
      {lines.length > 0 ? lines.map((line, index) => <span key={`${index}-${line}`} className="block">{line}</span>) : <span className="block">{absentValueMark}</span>}
      {address.country ? (
        <span className="block text-idpxyz-breadcrumb">
          <CountryLabel code={address.country} />
        </span>
      ) : null}
    </>
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
          <dt className="text-[12px] leading-5 text-idpxyz-breadcrumb">{item.label}</dt>
          <dd className={`min-w-0 text-[13px] leading-5 [overflow-wrap:anywhere] ${item.address ? 'text-idpxyz-text' : 'text-idpxyz-breadcrumb'}`}>
            {item.address ? <ProfileReadoutAddress address={item.address} /> : absentValueMark}
          </dd>
        </div>
      ))}
    </dl>
  );
}

/**
 * 值格里的两列小表（票 company-subject-ux/08 第 5 条）：左列是类型名或语言（弱显），右列是号或抬头。几个号用分号串成一段
 * 扫不出来，一行一个才认得出哪个号是哪种。左列最多占一半宽，长类型名折行而不是把号挤掉；等宽字体只给号。
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
          <dt className={`text-[12px] leading-5 text-idpxyz-breadcrumb ${headNowrap ? 'whitespace-nowrap' : ''}`} title={row.headTitle}>
            {row.head}
          </dt>
          <dd className={`min-w-0 leading-5 text-idpxyz-text ${mono ? 'select-text font-mono text-[12px]' : ''}`}>{row.value}</dd>
        </Fragment>
      ))}
    </dl>
  );
}
