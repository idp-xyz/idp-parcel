import { Fragment, type ReactNode } from 'react';
import type { LucideIcon } from 'lucide-react';
import type { RegisteredAddressRecord } from './api';
import { CountryLabel } from '../refdata-labels';
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
 * 地址格：地址行在前，国家 / 地区经基础数据译名并进行末（ADR-0335 决定七）。不把国家码当前缀——运营读地址先看街道，
 * 国家是补充信息，译成「中国（CN）」后放句首会压过地址本身；也不另起一行（票 company-subject-ux/08 第 5 条）——另起一行
 * 又换一种颜色，读起来像一条链接。弱显只降不透明度、不换色。资料读出与版本历史快照共用这一份，版式只定一处。
 */
export function ProfileReadoutAddress({ address }: { address: RegisteredAddressRecord }) {
  const lines = address.lines.filter((line) => line.trim() !== '');
  return (
    <>
      {lines.length > 0 ? lines.join(' · ') : absentValueMark}
      {address.country ? (
        <span className="opacity-70">
          {' · '}
          <CountryLabel code={address.country} />
        </span>
      ) : null}
    </>
  );
}

/**
 * 值格里的两列小表（票 company-subject-ux/08 第 5 条）：左列是类型名或语言（弱显），右列是号或抬头。几个号用分号串成一段
 * 扫不出来，一行一个才认得出哪个号是哪种。左列最多占一半宽，长类型名折行而不是把号挤掉；等宽字体只给号。
 */
export function ProfileReadoutPairs({
  rows,
  mono = false,
}: {
  rows: readonly { key: string; head: ReactNode; headTitle?: string; value: ReactNode }[];
  /** 右列用等宽字体（号）；抬头是文字，不用。 */
  mono?: boolean;
}) {
  return (
    <dl className="grid grid-cols-[fit-content(50%)_minmax(0,1fr)] gap-x-3 gap-y-1">
      {rows.map((row) => (
        <Fragment key={row.key}>
          <dt className="text-[12px] leading-5 text-idpxyz-breadcrumb" title={row.headTitle}>
            {row.head}
          </dt>
          <dd className={`min-w-0 leading-5 text-idpxyz-text ${mono ? 'select-text font-mono text-[12px]' : ''}`}>{row.value}</dd>
        </Fragment>
      ))}
    </dl>
  );
}
