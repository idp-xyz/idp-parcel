// 编辑器里用的几种控件。每个控件带 data-field（控件定位串）：问题标回控件、点问题滚到控件都靠它找；
// 控件上的问题（本地校验或预览返回的）由 EditorContext 的 errors 查出，红框加下方一行话。

import type { ReactNode } from 'react';
import { Input } from '@idpxyz/ui-primitives';
import { fieldLabel, selectClass } from '../../party/PublicationFormFields';
import { useEditor, type FieldError } from './EditorContext';

export const hintClass = 'text-[11px] leading-4 text-idpxyz-textMuted';
const invalidClass = 'border-[color:var(--ds-status-danger-fg)] focus:border-[color:var(--ds-status-danger-fg)]';

export interface Option {
  value: string;
  label: string;
}

export function useFieldErrors(id: string): FieldError[] {
  return useEditor().errors.get(id) ?? [];
}

export function ErrorLines({ errors, withWhere = false }: { errors: readonly FieldError[]; withWhere?: boolean }) {
  if (errors.length === 0) return null;
  return (
    <ul className="mt-1 list-disc pl-4 text-[12px] text-[color:var(--ds-status-danger-fg)]" role="alert">
      {errors.map((error, index) => (
        <li key={`${index}-${error.text}`}>
          {withWhere ? `${error.where}：` : ''}
          {error.text}
          {error.original ? (
            <details className="ml-1 inline-block align-top text-idpxyz-breadcrumb">
              <summary className="cursor-pointer select-none">原话</summary>
              <span className="font-mono break-all">{error.original}</span>
            </details>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

/** 一格：标签、控件、这个控件上的问题。 */
export function FieldBox({
  id,
  label,
  required,
  hint,
  children,
  className,
  labelsControl = true,
}: {
  id: string;
  label: string;
  required?: boolean;
  hint?: ReactNode;
  children: ReactNode;
  className?: string;
  /** 标签指向 id 同名的控件；一组复选框这类没有单一控件的格设为 false。 */
  labelsControl?: boolean;
}) {
  const errors = useFieldErrors(id);
  const text = (
    <>
      {label}
      {required ? <span className="text-[color:var(--ds-status-danger-fg)]"> *</span> : null}
    </>
  );
  return (
    <div className={className}>
      {labelsControl ? (
        <label htmlFor={id} className={fieldLabel}>
          {text}
        </label>
      ) : (
        <span className={fieldLabel}>{text}</span>
      )}
      {children}
      {hint ? <p className={`${hintClass} mt-1`}>{hint}</p> : null}
      <ErrorLines errors={errors} />
    </div>
  );
}

export function TextField({
  id,
  label,
  value,
  onChange,
  required,
  hint,
  placeholder,
  mono,
  inputMode,
  className,
  readOnly,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (next: string) => void;
  required?: boolean;
  hint?: ReactNode;
  placeholder?: string;
  mono?: boolean;
  inputMode?: 'decimal' | 'numeric' | 'text';
  className?: string;
  readOnly?: boolean;
}) {
  const errors = useFieldErrors(id);
  return (
    <FieldBox id={id} label={label} required={required} hint={hint} className={className}>
      <Input
        id={id}
        data-field={id}
        value={value}
        readOnly={readOnly}
        placeholder={placeholder}
        inputMode={inputMode}
        aria-invalid={errors.length > 0}
        className={`text-[13px] ${mono ? 'font-mono' : ''} ${errors.length > 0 ? invalidClass : ''}`}
        onChange={(event) => onChange(event.target.value)}
      />
    </FieldBox>
  );
}

export function SelectField({
  id,
  label,
  value,
  onChange,
  options,
  required,
  hint,
  placeholder = '请选择',
  className,
  disabled,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (next: string) => void;
  options: readonly Option[];
  required?: boolean;
  hint?: ReactNode;
  placeholder?: string;
  className?: string;
  disabled?: boolean;
}) {
  const errors = useFieldErrors(id);
  return (
    <FieldBox id={id} label={label} required={required} hint={hint} className={className}>
      <SelectControl id={id} value={value} onChange={onChange} options={options} placeholder={placeholder} disabled={disabled} invalid={errors.length > 0} />
    </FieldBox>
  );
}

/** 不带标签的下拉，给表格行和条件行里用。当前值不在选项里（旧值、规格变了）也留一格，不悄悄换掉。 */
export function SelectControl({
  id,
  value,
  onChange,
  options,
  placeholder = '请选择',
  disabled,
  invalid,
  label,
}: {
  id: string;
  value: string;
  onChange: (next: string) => void;
  options: readonly Option[];
  placeholder?: string;
  disabled?: boolean;
  invalid?: boolean;
  label?: string;
}) {
  const known = value === '' || options.some((option) => option.value === value);
  return (
    <select
      id={id}
      data-field={id}
      aria-label={label}
      aria-invalid={invalid}
      className={`${selectClass} ${invalid ? invalidClass : ''}`}
      value={value}
      disabled={disabled}
      onChange={(event) => onChange(event.target.value)}
    >
      <option value="">{placeholder}</option>
      {known ? null : <option value={value}>{value}</option>}
      {options.map((option) => (
        <option key={option.value} value={option.value}>
          {option.label}
        </option>
      ))}
    </select>
  );
}

/** 表格里的一格输入。问题红框；话由表格底部的 ErrorLines 统一列。 */
export function CellInput({
  id,
  value,
  onChange,
  label,
  placeholder,
  mono = true,
}: {
  id: string;
  value: string;
  onChange: (next: string) => void;
  label: string;
  placeholder?: string;
  mono?: boolean;
}) {
  const errors = useFieldErrors(id);
  return (
    <Input
      data-field={id}
      aria-label={label}
      aria-invalid={errors.length > 0}
      value={value}
      placeholder={placeholder}
      inputMode={mono ? 'decimal' : 'text'}
      className={`text-[12px] ${mono ? 'font-mono' : ''} ${errors.length > 0 ? invalidClass : ''}`}
      onChange={(event) => onChange(event.target.value)}
    />
  );
}

/** 一组控件的问题合并列在底部，带位置话。 */
export function GroupErrors({ ids }: { ids: readonly string[] }) {
  const { errors } = useEditor();
  const all = ids.flatMap((id) => errors.get(id) ?? []);
  const seen = new Set<string>();
  const unique = all.filter((error) => {
    const key = `${error.where}|${error.text}|${error.original ?? ''}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
  return <ErrorLines errors={unique} withWhere />;
}

export function SectionTitle({ children, hint }: { children: ReactNode; hint?: ReactNode }) {
  return (
    <div className="mb-2">
      <h3 className="text-[13px] font-semibold text-idpxyz-textBright">{children}</h3>
      {hint ? <p className={`${hintClass} mt-0.5`}>{hint}</p> : null}
    </div>
  );
}
