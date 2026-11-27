// 引导式表单的字段定义与载荷装配（纯 TypeScript，不引 React，node:test 直接测）。
// 仓配动作与运营决定签共用。逐格校验与嵌套键装配都在这里；不替租户填任何业务默认值。

export type GuidedFieldKind =
  | 'text'
  | 'integer'
  | 'signed-integer'
  | 'decimal'
  | 'datetime'
  | 'date'
  | 'select'
  /** 封闭词里多选，报文里是词的数组，顺序随 options。 */
  | 'choices'
  | 'boolean'
  | 'string-list'
  | 'rows'
  | 'json';

export type WarehouseLookupKind =
  | 'warehouse'
  | 'self-warehouse'
  | 'agent-warehouse'
  | 'zone'
  | 'location-type'
  | 'location'
  | 'outbound-staging-location'
  | 'receiving-location'
  | 'return-staging-location'
  | 'packaging'
  | 'owner'
  | 'service-product';

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
  /** 从实时名册加载候选下拉列表（仍允许手输），库区与库位按同表单已选仓库联动过滤。 */
  lookup?: WarehouseLookupKind;
  /** 新开表单时自动生成可读单据/决定标识的前缀（保留可手动改写）。 */
  autoIdPrefix?: string;
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
const rfc3339Pattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/;

/** 生成易读且不撞号的决定/单据标识（如 DEC-20261124-120835-42）。不替代租户业务取值。 */
export function generateReadableId(prefix: string, now: Date = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  const stamp =
    `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-` +
    `${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}`;
  const tail = String(Math.floor(Math.random() * 90) + 10);
  return `${prefix}-${stamp}-${tail}`;
}

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
    } else if (field.kind === 'choices') {
      values[field.path] = [];
    } else if (field.autoIdPrefix) {
      values[field.path] = generateReadableId(field.autoIdPrefix);
    } else {
      values[field.path] = '';
    }
  }
  return values;
}

/**
 * 列表行带出的标识里，这张表单认得的那几格。带出的键是按决定口分好的，这里再按格名筛一遍：
 * 表单里没有的格不写进去，免得跟着表单值一起留在状态里。
 */
export function carriedGuidedValues(fields: readonly GuidedFieldSpec[], carried: GuidedFormValues | undefined): GuidedFormValues {
  const known: GuidedFormValues = {};
  if (!carried) return known;
  for (const field of fields) {
    const value = carried[field.path];
    if (value !== undefined) known[field.path] = value;
  }
  return known;
}

export function emptyRowFor(rowFields: readonly GuidedFieldSpec[]): GuidedFormValues {
  const row: GuidedFormValues = {};
  for (const field of rowFields) {
    if (field.kind === 'boolean') row[field.path] = false;
    else if (field.kind === 'rows' || field.kind === 'choices') row[field.path] = [];
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
    } else if (field.kind === 'rows' || field.kind === 'choices') {
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

  if (field.kind === 'choices') {
    const picked = Array.isArray(raw) ? raw : [];
    const ordered = (field.options ?? []).map((option) => option.value).filter((value) => picked.includes(value));
    if (ordered.length === 0) {
      if (field.required) errors[errorKey] = `请至少选一项${field.label}`;
      return { present: false, value: undefined };
    }
    return { present: true, value: ordered };
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
      // 服务端按 RFC 3339 解：秒与时区都要有。只查「能解析」会放过 2026-11-24T08:00 这种，到服务端才被整份拒掉。
      if (!rfc3339Pattern.test(trimmed) || Number.isNaN(Date.parse(trimmed))) {
        errors[errorKey] = `${field.label}请写全秒与时区（如 2026-11-24T08:00:00Z 或 2026-11-24T16:00:00+08:00）`;
        return { present: false, value: undefined };
      }
      return { present: true, value: trimmed };
    case 'string-list': {
      const items = (Array.isArray(raw) ? raw.map((item) => String(item)) : trimmed.split(/[\n,，]+/))
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
