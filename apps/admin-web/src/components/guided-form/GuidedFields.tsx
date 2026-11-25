import { Button, Input, Textarea } from '@idpxyz/ui-primitives';
import { emptyRowFor, type GuidedFieldSpec, type GuidedFormValues } from './guided-forms';

const selectClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 type-value text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

/**
 * 运营决定签的逐格输入。不带名册下拉：这些口点名的是已有单据标识，由列表行带出或手填。
 * 仓配动作的表单另有一份带名册下拉的输入（pages/warehouse 的 GuidedActionPanel），两份读同一套字段定义。
 */
export function GuidedFields({
  fields,
  values,
  errors,
  onChange,
}: {
  fields: readonly GuidedFieldSpec[];
  values: GuidedFormValues;
  errors: Record<string, string>;
  onChange: (path: string, next: unknown) => void;
}) {
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
      {fields.map((field) =>
        field.kind === 'rows' ? (
          <RowsField
            key={field.path}
            field={field}
            rows={Array.isArray(values[field.path]) ? (values[field.path] as GuidedFormValues[]) : []}
            errors={errors}
            onChange={(next) => onChange(field.path, next)}
          />
        ) : (
          <div key={field.path} className={field.kind === 'string-list' || field.kind === 'json' ? 'md:col-span-2' : undefined}>
            <ScalarField field={field} value={values[field.path]} error={errors[field.path]} onChange={(next) => onChange(field.path, next)} />
          </div>
        ),
      )}
    </div>
  );
}

function ScalarField({
  field,
  value,
  error,
  onChange,
}: {
  field: GuidedFieldSpec;
  value: unknown;
  error?: string;
  onChange: (next: unknown) => void;
}) {
  if (field.kind === 'boolean') {
    return (
      <div className="flex flex-col gap-1">
        <label className="inline-flex items-center gap-2 type-body text-idpxyz-text cursor-pointer">
          <input
            type="checkbox"
            checked={value === true}
            onChange={(event) => onChange(event.target.checked)}
            className="h-4 w-4 rounded border-idpxyz-border"
          />
          <span>{field.label}</span>
        </label>
        {field.hint ? <span className="type-caption text-idpxyz-textMuted">{field.hint}</span> : null}
        {error ? <span className="type-caption text-idpxyz-danger">{error}</span> : null}
      </div>
    );
  }

  if (field.kind === 'choices') {
    const picked = Array.isArray(value) ? (value as string[]) : [];
    return (
      <div className="flex flex-col gap-1">
        <span className="type-body text-idpxyz-textMuted">
          {field.label}
          {field.required ? <span className="ml-0.5 text-idpxyz-danger">*</span> : null}
        </span>
        <div className="flex flex-wrap gap-x-4 gap-y-1">
          {(field.options ?? []).map((option) => (
            <label key={option.value} className="inline-flex items-center gap-2 type-body text-idpxyz-text cursor-pointer">
              <input
                type="checkbox"
                checked={picked.includes(option.value)}
                onChange={(event) =>
                  onChange(event.target.checked ? [...picked, option.value] : picked.filter((item) => item !== option.value))
                }
                className="h-4 w-4 rounded border-idpxyz-border"
              />
              <span>{option.label}</span>
            </label>
          ))}
        </div>
        {error ? <span className="type-caption text-idpxyz-danger">{error}</span> : field.hint ? <span className="type-caption text-idpxyz-textMuted">{field.hint}</span> : null}
      </div>
    );
  }

  const text = typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value);
  return (
    <div className="flex flex-col gap-1">
      <label className="type-body text-idpxyz-textMuted">
        <span>{field.label}</span>
        {field.required ? <span className="ml-0.5 text-idpxyz-danger">*</span> : null}
      </label>
      {field.kind === 'select' ? (
        <select className={selectClass} value={text} onChange={(event) => onChange(event.target.value)}>
          <option value="">请选择…</option>
          {(field.options ?? []).map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      ) : field.kind === 'datetime' ? (
        <div className="flex gap-1.5">
          <Input
            className="flex-1 font-mono"
            value={text}
            placeholder={field.placeholder ?? '2026-11-24T08:00:00Z'}
            onChange={(event) => onChange(event.target.value)}
          />
          <Button type="button" variant="outline" onClick={() => onChange(new Date().toISOString().replace(/\.\d{3}Z$/, 'Z'))}>
            填此刻
          </Button>
        </div>
      ) : field.kind === 'date' ? (
        <Input type="date" className="font-mono" value={text} onChange={(event) => onChange(event.target.value)} />
      ) : field.kind === 'string-list' || field.kind === 'json' ? (
        <Textarea
          rows={field.kind === 'json' ? 3 : 2}
          className="font-mono type-body"
          value={text}
          placeholder={field.placeholder ?? '一行一项，或用逗号分隔'}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : (
        <Input
          className={field.kind === 'integer' || field.kind === 'decimal' ? 'font-mono' : undefined}
          value={text}
          placeholder={field.placeholder}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
      {error ? <span className="type-caption text-idpxyz-danger">{error}</span> : field.hint ? <span className="type-caption text-idpxyz-textMuted">{field.hint}</span> : null}
    </div>
  );
}

function RowsField({
  field,
  rows,
  errors,
  onChange,
}: {
  field: GuidedFieldSpec;
  rows: GuidedFormValues[];
  errors: Record<string, string>;
  onChange: (next: GuidedFormValues[]) => void;
}) {
  const rowFields = field.rowFields ?? [];
  const rowLabel = field.rowLabel ?? field.label;
  return (
    <div className="md:col-span-2 flex flex-col gap-2 rounded border border-idpxyz-border p-3">
      <div className="flex items-center justify-between gap-2">
        <div className="type-body font-medium text-idpxyz-text">
          {field.label}
          {field.required ? <span className="ml-0.5 text-idpxyz-danger">*</span> : null}
        </div>
        <Button type="button" variant="outline" onClick={() => onChange([...rows, emptyRowFor(rowFields)])}>
          + 添加{rowLabel}
        </Button>
      </div>
      {field.hint ? <p className="type-caption text-idpxyz-textMuted">{field.hint}</p> : null}
      {errors[field.path] ? <p className="type-caption text-idpxyz-danger">{errors[field.path]}</p> : null}
      {rows.map((row, index) => (
        <div key={index} className="rounded border border-idpxyz-border p-3 space-y-2">
          <div className="flex items-center justify-between">
            <span className="type-caption text-idpxyz-textMuted">
              {rowLabel} #{index + 1}
            </span>
            <Button type="button" variant="outline" onClick={() => onChange(rows.filter((_row, i) => i !== index))}>
              删除
            </Button>
          </div>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {rowFields.map((sub) => (
              <ScalarField
                key={sub.path}
                field={sub}
                value={row[sub.path]}
                error={errors[`${field.path}[${index}].${sub.path}`]}
                onChange={(next) => onChange(rows.map((item, i) => (i === index ? { ...item, [sub.path]: next } : item)))}
              />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
