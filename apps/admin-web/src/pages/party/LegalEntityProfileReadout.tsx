import type { ReactNode } from 'react';
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
  value,
  mono = false,
  emphasized = false,
  wide = false,
}: {
  label: string;
  /** 字串或已排好版的节点（如 ProfileReadoutAddress）；空串、undefined 与旧写法的「未配置」都算没有。 */
  value?: ReactNode;
  mono?: boolean;
  emphasized?: boolean;
  wide?: boolean;
}) {
  const absent = value === undefined || value === null || value === '' || value === '未配置';
  return (
    <div className={`legal-entity-readout-field ${wide ? 'legal-entity-readout-field-wide' : ''}`}>
      <dt className="text-[12px] leading-6 text-idpxyz-breadcrumb">{label}</dt>
      <dd className={`min-w-0 text-[13px] leading-6 ${absent ? 'text-idpxyz-breadcrumb' : 'text-idpxyz-text'} ${mono ? 'font-mono' : ''} ${emphasized && !absent ? 'font-medium text-idpxyz-textBright' : ''}`}>
        {absent ? absentValueMark : value}
      </dd>
    </div>
  );
}

/**
 * 地址格：地址行在前，国家 / 地区另起一行经基础数据译名（ADR-0335 决定七）。不再把国家码当前缀——运营读地址先看街道，
 * 国家是补充信息；译成「中国（CN）」后放句首也压过了地址本身。资料读出与版本历史快照共用这一份，版式只定一处。
 */
export function ProfileReadoutAddress({ address }: { address: RegisteredAddressRecord }) {
  const lines = address.lines.filter((line) => line.trim() !== '');
  return (
    <>
      <span className="block">{lines.length > 0 ? lines.join(' · ') : absentValueMark}</span>
      {address.country ? (
        <span className="block text-[12px] text-idpxyz-breadcrumb">
          <CountryLabel code={address.country} />
        </span>
      ) : null}
    </>
  );
}
