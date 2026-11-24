// 仓配六页共用的引导式表单定义与载荷装配（纯 TypeScript，不引 React，node:test 直接测）。
// 逐格输入、必填校验与嵌套键装配都在这里完成；不替租户填任何业务默认值。

export type GuidedFieldKind =
  | 'text'
  | 'integer'
  | 'signed-integer'
  | 'decimal'
  | 'datetime'
  | 'date'
  | 'select'
  | 'boolean'
  | 'string-list'
  | 'rows'
  | 'json';

export interface GuidedSelectOption {
  value: string;
  label: string;
}

export interface GuidedFieldSpec {
  /** JSON 键路径，支持用点号表达一层嵌套，如 address.country、dimensions.length、weight.value。 */
  path: string;
  label: string;
  kind: GuidedFieldKind;
  required?: boolean;
  placeholder?: string;
  hint?: string;
  options?: readonly GuidedSelectOption[];
  /** kind 为 rows 时每一行的列定义。 */
  rowFields?: readonly GuidedFieldSpec[];
  rowLabel?: string;
  /** kind 为 boolean 且为可选停用/标志位时，false 不写入报文（保持缺省即不传）。 */
  omitWhenFalse?: boolean;
  /** kind 为 integer 时是否允许 0（如商品首版 basisRevision=0、代理仓库存声明 quantity=0）。 */
  allowZero?: boolean;
  /** kind 为 rows 且允许空数组（如空库位清点 lines: []）时，无行仍输出 [] 而不是省略该键。 */
  emitEmptyArray?: boolean;
}

export type GuidedFormValues = Record<string, unknown>;

export interface GuidedFormSeed {
  targetId: string;
  values: GuidedFormValues;
  nonce: number;
}

export interface GuidedFormDefinition {
  fields: readonly GuidedFieldSpec[];
  /** 跨字段校验，返回一句中文说明或 null。 */
  validate?: (values: GuidedFormValues, payload: Record<string, unknown>) => string | null;
}

export type GuidedBuildResult =
  | { ok: true; payload: Record<string, unknown> }
  | { ok: false; errors: Record<string, string>; formError?: string };

const intPattern = /^\d+$/;
const signedIntPattern = /^-?\d+$/;
const decimalPattern = /^\d{1,9}(\.\d{1,6})?$/;
const datePattern = /^\d{4}-\d{2}-\d{2}$/;

/** 为一组字段生成初始表单值，并与列表行带出的预填值合并。不填任何租户业务默认值。 */
export function initialGuidedValues(
  fields: readonly GuidedFieldSpec[],
  prefill?: GuidedFormValues,
): GuidedFormValues {
  const values: GuidedFormValues = {};
  for (const field of fields) {
    const seeded = prefill?.[field.path];
    if (seeded !== undefined) {
      values[field.path] = seeded;
      continue;
    }
    if (field.kind === 'boolean') {
      values[field.path] = false;
    } else if (field.kind === 'rows') {
      values[field.path] = field.required ? [emptyRowFor(field.rowFields ?? [])] : [];
    } else {
      values[field.path] = '';
    }
  }
  return values;
}

export function emptyRowFor(rowFields: readonly GuidedFieldSpec[]): GuidedFormValues {
  const row: GuidedFormValues = {};
  for (const field of rowFields) {
    if (field.kind === 'boolean') row[field.path] = false;
    else if (field.kind === 'rows') row[field.path] = [];
    else row[field.path] = '';
  }
  return row;
}

function setNested(target: Record<string, unknown>, path: string, value: unknown): void {
  const dot = path.indexOf('.');
  if (dot < 0) {
    target[path] = value;
    return;
  }
  const head = path.slice(0, dot);
  const tail = path.slice(dot + 1);
  const child =
    typeof target[head] === 'object' && target[head] !== null && !Array.isArray(target[head])
      ? (target[head] as Record<string, unknown>)
      : {};
  target[head] = child;
  setNested(child, tail, value);
}

function isRowBlank(row: GuidedFormValues, rowFields: readonly GuidedFieldSpec[]): boolean {
  for (const field of rowFields) {
    const raw = row[field.path];
    if (field.kind === 'boolean') {
      if (raw === true) return false;
    } else if (field.kind === 'rows') {
      if (Array.isArray(raw) && raw.length > 0) return false;
    } else if (typeof raw === 'string' && raw.trim() !== '') {
      return false;
    }
  }
  return true;
}

function parseField(
  field: GuidedFieldSpec,
  raw: unknown,
  errorKey: string,
  errors: Record<string, string>,
): { present: boolean; value: unknown } {
  if (field.kind === 'boolean') {
    const checked = raw === true;
    if (!checked && field.omitWhenFalse) return { present: false, value: undefined };
    return { present: true, value: checked };
  }

  if (field.kind === 'rows') {
    const list = Array.isArray(raw) ? (raw as GuidedFormValues[]) : [];
    const rowFields = field.rowFields ?? [];
    const builtRows: Record<string, unknown>[] = [];
    list.forEach((row, index) => {
      if (isRowBlank(row, rowFields)) return;
      const rowPayload: Record<string, unknown> = {};
      for (const sub of rowFields) {
        const parsed = parseField(sub, row[sub.path], `${errorKey}[${index}].${sub.path}`, errors);
        if (parsed.present) setNested(rowPayload, sub.path, parsed.value);
      }
      builtRows.push(rowPayload);
    });
    if (builtRows.length === 0) {
      if (field.required) errors[errorKey] = `请至少填写一行${field.rowLabel ?? field.label}`;
      if (field.emitEmptyArray) return { present: true, value: [] };
      return { present: false, value: undefined };
    }
    return { present: true, value: builtRows };
  }

  const text = typeof raw === 'string' ? raw : raw === undefined || raw === null ? '' : String(raw);
  const trimmed = text.trim();

  if (trimmed === '') {
    if (field.required) errors[errorKey] = `请填写${field.label}`;
    return { present: false, value: undefined };
  }

  switch (field.kind) {
    case 'text':
    case 'select':
      return { present: true, value: trimmed };
    case 'decimal':
      if (!decimalPattern.test(trimmed)) {
        errors[errorKey] = `${field.label}请填十进制数（如 12 或 12.50，整数最多 9 位、小数最多 6 位）`;
        return { present: false, value: undefined };
      }
      return { present: true, value: trimmed };
    case 'integer': {
      if (!intPattern.test(trimmed)) {
        errors[errorKey] = `${field.label}请填非负整数`;
        return { present: false, value: undefined };
      }
      const parsed = Number.parseInt(trimmed, 10);
      if (!Number.isSafeInteger(parsed) || (!field.allowZero && parsed < 1)) {
        errors[errorKey] = field.allowZero ? `${field.label}请填非负整数` : `${field.label}请填正整数`;
        return { present: false, value: undefined };
      }
      return { present: true, value: parsed };
    }
    case 'signed-integer': {
      if (!signedIntPattern.test(trimmed)) {
        errors[errorKey] = `${field.label}请填整数`;
        return { present: false, value: undefined };
      }
      const parsed = Number.parseInt(trimmed, 10);
      if (!Number.isSafeInteger(parsed) || parsed < -2147483648 || parsed > 2147483647) {
        errors[errorKey] = `${field.label}超出 32 位整数范围`;
        return { present: false, value: undefined };
      }
      return { present: true, value: parsed };
    }
    case 'date':
      if (!datePattern.test(trimmed)) {
        errors[errorKey] = `${field.label}请用 YYYY-MM-DD 格式`;
        return { present: false, value: undefined };
      }
      return { present: true, value: trimmed };
    case 'datetime':
      if (Number.isNaN(Date.parse(trimmed)) || !trimmed.includes('T')) {
        errors[errorKey] = `${field.label}请用 RFC 3339 时刻（如 2026-11-24T08:00:00Z）`;
        return { present: false, value: undefined };
      }
      return { present: true, value: trimmed };
    case 'string-list': {
      const items = trimmed
        .split(/[\n,，]+/)
        .map((item) => item.trim())
        .filter((item) => item !== '');
      if (items.length === 0) {
        if (field.required) errors[errorKey] = `请至少填写一项${field.label}`;
        return { present: false, value: undefined };
      }
      return { present: true, value: items };
    }
    case 'json':
      try {
        return { present: true, value: JSON.parse(trimmed) };
      } catch {
        errors[errorKey] = `${field.label}不是合法 JSON`;
        return { present: false, value: undefined };
      }
  }
}

/** 按表单定义逐格校验并组装出后端接口要吃的 JSON 对象。 */
export function buildGuidedPayload(
  definition: GuidedFormDefinition,
  values: GuidedFormValues,
): GuidedBuildResult {
  const errors: Record<string, string> = {};
  const payload: Record<string, unknown> = {};

  for (const field of definition.fields) {
    const parsed = parseField(field, values[field.path], field.path, errors);
    if (parsed.present) setNested(payload, field.path, parsed.value);
  }

  const formError = Object.keys(errors).length === 0 && definition.validate ? definition.validate(values, payload) : null;
  if (Object.keys(errors).length > 0 || formError) {
    return { ok: false, errors, ...(formError ? { formError } : {}) };
  }
  return { ok: true, payload };
}
