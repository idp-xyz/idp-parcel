import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input, Textarea } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import {
  RegistrationAnswerNote,
  chipClass,
  type RegistrationPanelState,
  type RegistrationTarget,
} from '../../components/registration';
import { moduleInfoById } from '../../navigation';
import {
  buildGuidedPayload,
  emptyRowFor,
  initialGuidedValues,
  type GuidedFieldSpec,
  type GuidedFormDefinition,
  type GuidedFormSeed,
  type GuidedFormValues,
} from './guided-forms';

export interface GuidedActionTarget extends RegistrationTarget {
  form: GuidedFormDefinition;
}

export interface GuidedActionPanelProps {
  moduleId: string;
  targets: readonly GuidedActionTarget[];
  problemNote: (code: string) => string;
  /** 从列表行快速动作带入的预填种子；每次 nonce 变化时切到对应动作并重填。 */
  seed?: GuidedFormSeed | null;
  /** 切换动作或重置时按动作生成初始值（如模拟扫码端自动填设备绑定仓与端侧事实号）。 */
  defaultValuesFor?: (targetId: string) => GuidedFormValues;
}

const selectClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 type-value text-idpxyz-text focus:outline-none focus:border-idpxyz-accent';

function FieldControl({
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

  const text = typeof value === 'string' ? value : value === undefined || value === null ? '' : String(value);

  return (
    <div className="flex flex-col gap-1">
      <label className="type-body text-idpxyz-textMuted">
        <span>{field.label}</span>
        {field.required ? <span className="ml-0.5 text-idpxyz-danger">*</span> : null}
      </label>
      {field.kind === 'select' ? (
        <select
          className={selectClass}
          value={text}
          onChange={(event) => onChange(event.target.value)}
        >
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
          <Button
            type="button"
            variant="outline"
            onClick={() => onChange(new Date().toISOString().replace(/\.\d{3}Z$/, 'Z'))}
          >
            填此刻
          </Button>
        </div>
      ) : field.kind === 'date' ? (
        <Input
          type="date"
          className="font-mono"
          value={text}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : field.kind === 'string-list' || field.kind === 'json' ? (
        <Textarea
          rows={field.kind === 'json' ? 3 : 2}
          className="font-mono type-body"
          value={text}
          placeholder={field.placeholder}
          onChange={(event) => onChange(event.target.value)}
        />
      ) : (
        <Input
          className={field.kind === 'integer' || field.kind === 'signed-integer' || field.kind === 'decimal' ? 'font-mono' : undefined}
          inputMode={field.kind === 'integer' || field.kind === 'signed-integer' ? 'numeric' : field.kind === 'decimal' ? 'decimal' : undefined}
          value={text}
          placeholder={field.placeholder}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
      {error ? (
        <span className="type-caption text-idpxyz-danger">{error}</span>
      ) : field.hint ? (
        <span className="type-caption text-idpxyz-textMuted">{field.hint}</span>
      ) : null}
    </div>
  );
}

function RowsFieldControl({
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

  const updateRow = (index: number, subPath: string, nextValue: unknown) => {
    const copy = rows.map((row, i) => (i === index ? { ...row, [subPath]: nextValue } : row));
    onChange(copy);
  };

  const addRow = () => onChange([...rows, emptyRowFor(rowFields)]);
  const removeRow = (index: number) => onChange(rows.filter((_row, i) => i !== index));

  return (
    <div className="md:col-span-2 flex flex-col gap-2 rounded border border-idpxyz-border p-3 bg-idpxyz-editor">
      <div className="flex items-center justify-between gap-2">
        <div className="type-body font-medium text-idpxyz-text">
          {field.label}
          {field.required ? <span className="ml-0.5 text-idpxyz-danger">*</span> : null}
        </div>
        <Button type="button" variant="outline" onClick={addRow}>
          + 添加{rowLabel}
        </Button>
      </div>
      {field.hint ? <p className="type-caption text-idpxyz-textMuted">{field.hint}</p> : null}
      {errors[field.path] ? <p className="type-caption text-idpxyz-danger">{errors[field.path]}</p> : null}
      {rows.length === 0 ? (
        <p className="type-caption text-idpxyz-textMuted">当前没有{rowLabel}；点右上角按钮添加。</p>
      ) : (
        <div className="space-y-3">
          {rows.map((row, index) => (
            <div key={index} className="rounded border border-idpxyz-border bg-idpxyz-panel p-3 space-y-2">
              <div className="flex items-center justify-between">
                <span className="type-caption font-medium text-idpxyz-textMuted">
                  {rowLabel} #{index + 1}
                </span>
                <Button type="button" variant="outline" onClick={() => removeRow(index)}>
                  删除
                </Button>
              </div>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {rowFields.map((sub) => (
                  <FieldControl
                    key={sub.path}
                    field={sub}
                    value={row[sub.path]}
                    error={errors[`${field.path}[${index}].${sub.path}`]}
                    onChange={(next) => updateRow(index, sub.path, next)}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function SingleGuidedForm({
  moduleId,
  target,
  problemNote,
  prefill,
}: {
  moduleId: string;
  target: GuidedActionTarget;
  problemNote: (code: string) => string;
  prefill?: GuidedFormValues;
}) {
  const [values, setValues] = useState<GuidedFormValues>(() => initialGuidedValues(target.form.fields, prefill));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [state, setState] = useState<RegistrationPanelState>({ kind: 'idle' });
  const [rawMode, setRawMode] = useState(false);
  const [rawDraft, setRawDraft] = useState('');
  const owner = moduleInfoById[moduleId]?.owner ?? moduleId;

  const setField = (path: string, next: unknown) => {
    setValues((current) => ({ ...current, [path]: next }));
    setErrors((current) => {
      if (!(path in current)) return current;
      const copy = { ...current };
      delete copy[path];
      return copy;
    });
    setFormError(null);
    if (state.kind !== 'idle') setState({ kind: 'idle' });
  };

  const toggleRawMode = () => {
    if (!rawMode) {
      const built = buildGuidedPayload(target.form, values);
      setRawDraft(JSON.stringify(built.ok ? built.payload : {}, null, 2));
      setRawMode(true);
    } else {
      setRawMode(false);
    }
    setFormError(null);
    setState({ kind: 'idle' });
  };

  const reset = () => {
    setValues(initialGuidedValues(target.form.fields, prefill));
    setErrors({});
    setFormError(null);
    setRawDraft('');
    setState({ kind: 'idle' });
  };

  const handleSubmit = async () => {
    let payload: unknown;
    if (rawMode) {
      try {
        payload = JSON.parse(rawDraft);
      } catch (error) {
        setState({ kind: 'malformed', message: error instanceof Error ? error.message : String(error) });
        return;
      }
    } else {
      const built = buildGuidedPayload(target.form, values);
      if (!built.ok) {
        setErrors(built.errors);
        setFormError(built.formError ?? '请先补全标红的必填或格式有误的字段。');
        return;
      }
      setErrors({});
      setFormError(null);
      payload = built.payload;
    }
    setState({ kind: 'submitting' });
    const answer = await target.submit(payload);
    setState({ kind: 'answered', answer });
  };

  return (
    <div className="flex-1 overflow-auto p-4">
      <Card>
        <CardHeader className="pb-2">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div>
              <CardTitle className="type-section">{target.title}</CardTitle>
              <p className="mt-0.5 font-mono type-caption text-idpxyz-textMuted">{target.endpoint}</p>
            </div>
            <Button type="button" variant="outline" onClick={toggleRawMode}>
              {rawMode ? '切回引导表单' : '查看 / 改 JSON 原文'}
            </Button>
          </div>
          <p className="mt-1 type-body text-idpxyz-textMuted">{target.snapshotHint}</p>
        </CardHeader>
        <CardContent className="space-y-4">
          {rawMode ? (
            <div className="space-y-2">
              <p className="type-caption text-idpxyz-textMuted">
                高级模式：可直接核对或粘贴 JSON 原文。切回引导表单会回到逐字段输入。
              </p>
              <Textarea
                rows={12}
                className="font-mono type-body"
                value={rawDraft}
                onChange={(event) => setRawDraft(event.target.value)}
                placeholder="{}"
              />
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              {target.form.fields.map((field) =>
                field.kind === 'rows' ? (
                  <RowsFieldControl
                    key={field.path}
                    field={field}
                    rows={Array.isArray(values[field.path]) ? (values[field.path] as GuidedFormValues[]) : []}
                    errors={errors}
                    onChange={(next) => setField(field.path, next)}
                  />
                ) : (
                  <div key={field.path} className={field.kind === 'string-list' || field.kind === 'json' ? 'md:col-span-2' : undefined}>
                    <FieldControl
                      field={field}
                      value={values[field.path]}
                      error={errors[field.path]}
                      onChange={(next) => setField(field.path, next)}
                    />
                  </div>
                ),
              )}
            </div>
          )}

          {formError ? <p className="type-body text-idpxyz-danger">{formError}</p> : null}

          <div className="flex flex-wrap items-center gap-2">
            <Button
              type="button"
              className={primaryActionClass}
              disabled={state.kind === 'submitting'}
              onClick={() => void handleSubmit()}
            >
              {state.kind === 'submitting' ? '提交中…' : '提交'}
            </Button>
            <Button type="button" variant="outline" disabled={state.kind === 'submitting'} onClick={reset}>
              清空重填
            </Button>
          </div>

          <RegistrationAnswerNote
            state={state}
            owner={owner}
            outcomeLabels={target.outcomeLabels}
            refusalReasonLabels={target.refusalReasonLabels}
            undecidedReasonLabels={target.undecidedReasonLabels}
            declarationLandingLabels={target.declarationLandingLabels}
            problemNote={problemNote}
          />
        </CardContent>
      </Card>
    </div>
  );
}

/**
 * 仓配六页共用的引导式表单面板：按动作切换表单，换动作清草稿，列表行快速动作可带出预填值。
 */
export function GuidedActionPanel({ moduleId, targets, problemNote, seed, defaultValuesFor }: GuidedActionPanelProps) {
  const [selectedId, setSelectedId] = useState(seed?.targetId ?? targets[0]?.id ?? '');
  const [activeSeed, setActiveSeed] = useState<GuidedFormSeed | null>(seed ?? null);

  useEffect(() => {
    if (!seed) return;
    if (targets.some((target) => target.id === seed.targetId)) {
      setSelectedId(seed.targetId);
      setActiveSeed(seed);
    }
  }, [seed, targets]);

  if (targets.length === 0) {
    return (
      <div className="p-6 type-body text-idpxyz-textMuted">
        当前操作者没有这一页所需的能力面或运营决定授予，没有可执行的写动作。
      </div>
    );
  }

  const selected = targets.find((target) => target.id === selectedId) ?? targets[0];
  const seedMatches = activeSeed !== null && activeSeed.targetId === selected.id;
  const computedPrefill = seedMatches ? activeSeed.values : defaultValuesFor?.(selected.id);
  const formKey = seedMatches
    ? `${selected.id}:${activeSeed.nonce}`
    : `${selected.id}:${seed?.nonce ?? 0}`;

  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex flex-wrap items-center gap-2 px-4 pt-4 shrink-0">
        {targets.map((target) => (
          <button
            key={target.id}
            type="button"
            className={chipClass(target.id === selected.id)}
            aria-pressed={target.id === selected.id}
            onClick={() => {
              setSelectedId(target.id);
              setActiveSeed(null);
            }}
          >
            {target.label}
          </button>
        ))}
      </div>
      <SingleGuidedForm
        key={formKey}
        moduleId={moduleId}
        target={selected}
        problemNote={problemNote}
        prefill={computedPrefill}
      />
    </div>
  );
}
