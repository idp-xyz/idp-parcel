import { Fragment, type ReactNode } from 'react';
import type { RegisteredAddressRecord } from './api';
import { CountryLabel } from '../refdata-labels';
import './legal-entity-profile-readout.css';

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
