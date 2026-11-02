import type { ReactNode } from 'react';
import './legal-entity-profile-readout.css';

export function ProfileReadoutGrid({ children }: { children: ReactNode }) {
  return <dl className="legal-entity-readout-grid">{children}</dl>;
}

export function ProfileReadoutField({
  label,
  value,
  mono = false,
  emphasized = false,
  wide = false,
}: {
  label: string;
  value?: string;
  mono?: boolean;
  emphasized?: boolean;
  wide?: boolean;
}) {
  const absent = !value || value === '未配置';
  return (
    <div className={`legal-entity-readout-field ${wide ? 'legal-entity-readout-field-wide' : ''}`}>
      <dt className="text-[12px] leading-6 text-idpxyz-breadcrumb">{label}</dt>
      <dd className={`min-w-0 text-[13px] leading-6 ${absent ? 'text-idpxyz-breadcrumb' : 'text-idpxyz-text'} ${mono ? 'font-mono' : ''} ${emphasized && !absent ? 'font-medium text-idpxyz-textBright' : ''}`}>
        {absent ? '未配置' : value}
      </dd>
    </div>
  );
}
