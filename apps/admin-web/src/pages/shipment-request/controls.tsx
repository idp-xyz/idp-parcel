import type { ReactNode } from 'react';

// 两个页面共用的表单外观。只用 idpxyz 色 token,不引入额外色板——反馈层次全部
// 靠 border / text / accent 表达,这同时替 ADR-0022 守住「业务答案不画成故障红」。

export const inputClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 ' +
  'text-[13px] text-idpxyz-text placeholder:text-idpxyz-textMuted ' +
  'focus:outline-none focus:border-idpxyz-accent';

export function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded border border-idpxyz-border p-4">
      <h2 className="text-[14px] font-bold text-idpxyz-textBright mb-3">{title}</h2>
      {children}
    </section>
  );
}

export function Field({
  label,
  required,
  error,
  hint,
  children,
}: {
  label: string;
  required?: boolean;
  error?: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="block">
      <span className="block text-[12px] text-idpxyz-textMuted mb-1">
        {label}
        {required ? <span className="text-idpxyz-accent ml-0.5">*</span> : null}
      </span>
      {children}
      {hint ? (
        <span className="block text-[11px] leading-4 text-idpxyz-textMuted mt-1">{hint}</span>
      ) : null}
      {error ? (
        // 校验失败用亮色文字陈述,不用红:形状级问题是「话没说全」,不是故障。
        <span className="block text-[11px] leading-4 text-idpxyz-textBright mt-1">
          {error}
        </span>
      ) : null}
    </label>
  );
}

/** 结果与错误通用的卡片壳,层次差异只在标题用色由调用方给。 */
export function NoticeCard({
  title,
  titleClass = 'text-idpxyz-textBright',
  meta,
  children,
}: {
  title: string;
  titleClass?: string;
  meta?: string;
  children?: ReactNode;
}) {
  return (
    <div className="rounded border border-idpxyz-border p-4">
      <div className="flex items-baseline gap-2 flex-wrap">
        <span className={`text-[15px] font-bold ${titleClass}`}>{title}</span>
        {meta ? (
          <span className="text-[11px] font-mono text-idpxyz-textMuted">{meta}</span>
        ) : null}
      </div>
      {children}
    </div>
  );
}

/** 键值明细行:标签沉底色,值亮色;引用类的值用等宽以便逐字核对。 */
export function DetailRow({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="flex gap-2 text-[12px] leading-5">
      <dt className="text-idpxyz-textMuted shrink-0 w-[104px]">{label}</dt>
      <dd className={`text-idpxyz-text break-all ${mono ? 'font-mono' : ''}`}>{value}</dd>
    </div>
  );
}
