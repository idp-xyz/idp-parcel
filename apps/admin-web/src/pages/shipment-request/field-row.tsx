import type { ReactNode } from 'react';

/** 运单详情区块内的键值行，排版对齐 DetailPageTemplate 基本信息区。详情页与「受理前财务控制」一块共用。 */
export function FieldRow({ label, value, mono }: { label: string; value: ReactNode; mono?: boolean }) {
  return (
    <div className="flex gap-3 type-body leading-5">
      <dt className="w-[128px] shrink-0 text-idpxyz-textMuted">{label}</dt>
      <dd className={`min-w-0 flex-1 break-words text-idpxyz-text ${mono ? 'font-mono' : ''}`}>{value}</dd>
    </div>
  );
}
