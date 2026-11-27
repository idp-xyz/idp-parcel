// 服务形态登记的逐字段载荷（在线口一次一项，与 registrationjson.ServiceProductFormDocument 同形）。
// 租户不进载荷：在线口见 tenantId 键在场即拒，租户取登录身份。形态不预选——两个词都是租户这次要说的取值，空着不是默认成网络服务产品。

export const serviceProductFormCodes = ['NETWORK_SERVICE', 'LABEL_CHANNEL_SERVICE'] as const;

export type ServiceProductFormCode = (typeof serviceProductFormCodes)[number];

export interface ServiceProductFormDraft {
  productId: string;
  version: string;
  scope: string;
  form: '' | ServiceProductFormCode;
}

export interface ServiceProductFormSnapshot {
  scope: string;
  forms: [{ productId: string; version: string; form: ServiceProductFormCode }];
}

export interface ServiceProductFormGap {
  path: 'productId' | 'version' | 'scope' | 'form';
  label: string;
}

/** 技术详情里那一句。在线口不收 tenantId，也不收渠道映射那一节。 */
export const serviceProductFormSnapshotNote =
  '粘贴一份对象：scope，以及 forms 里的一项（productId、version、form）。不要带 tenantId，租户由登录身份决定。';

export function emptyServiceProductFormDraft(): ServiceProductFormDraft {
  return { productId: '', version: '', scope: '', form: '' };
}

/** 点保存才报的空格，按页面从上到下：产品、版本、适用范围、形态。 */
export function serviceProductFormGaps(draft: ServiceProductFormDraft): ServiceProductFormGap[] {
  const gaps: ServiceProductFormGap[] = [];
  if (draft.productId.trim() === '') gaps.push({ path: 'productId', label: '产品' });
  if (draft.version.trim() === '') gaps.push({ path: 'version', label: '版本' });
  if (draft.scope.trim() === '') gaps.push({ path: 'scope', label: '适用范围' });
  if (draft.form === '') gaps.push({ path: 'form', label: '形态' });
  return gaps;
}

export function serviceProductFormGapSummary(gaps: readonly ServiceProductFormGap[]): string {
  return `还有 ${gaps.length} 项要填：${gaps.map((gap) => gap.label).join('、')}`;
}

export function serviceProductFormPayload(draft: ServiceProductFormDraft): ServiceProductFormSnapshot | null {
  if (serviceProductFormGaps(draft).length > 0 || draft.form === '') return null;
  return {
    scope: draft.scope.trim(),
    forms: [{ productId: draft.productId.trim(), version: draft.version.trim(), form: draft.form }],
  };
}

export type ServiceProductFormSnapshotRead =
  | { ok: true; snapshot: ServiceProductFormSnapshot }
  | { ok: false; problem: string };

const formItemKeys = new Set(['productId', 'version', 'form']);

function isFormCode(value: string): value is ServiceProductFormCode {
  return (serviceProductFormCodes as readonly string[]).includes(value);
}

/** 技术详情里粘贴的那一份。形状不对就不送：送上去也是同一句「格式不对」。 */
export function readServiceProductFormSnapshot(text: string): ServiceProductFormSnapshotRead {
  let value: unknown;
  try {
    value = JSON.parse(text);
  } catch {
    return { ok: false, problem: '不是合法 JSON，未提交。' };
  }
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    return { ok: false, problem: '要一份对象。' };
  }
  const record = value as Record<string, unknown>;
  if ('tenantId' in record) return { ok: false, problem: '不要带 tenantId：租户由登录身份决定。' };
  if ('mappings' in record) return { ok: false, problem: '这一页只登记服务形态。' };
  for (const key of Object.keys(record)) {
    if (key !== 'scope' && key !== 'forms') return { ok: false, problem: `认不出的键 ${key}。` };
  }
  if (typeof record.scope !== 'string' || record.scope.trim() === '') {
    return { ok: false, problem: '适用范围不能留空。' };
  }
  if (!Array.isArray(record.forms) || record.forms.length !== 1) {
    return { ok: false, problem: 'forms 里要有且只有一项。' };
  }
  const item = record.forms[0];
  if (item === null || typeof item !== 'object' || Array.isArray(item)) {
    return { ok: false, problem: 'forms 里的那一项要是对象。' };
  }
  const formItem = item as Record<string, unknown>;
  for (const key of Object.keys(formItem)) {
    if (!formItemKeys.has(key)) return { ok: false, problem: `形态那一项认不出的键 ${key}。` };
  }
  if (typeof formItem.productId !== 'string' || formItem.productId.trim() === '') {
    return { ok: false, problem: '产品不能留空。' };
  }
  if (typeof formItem.version !== 'string' || formItem.version.trim() === '') {
    return { ok: false, problem: '版本不能留空。' };
  }
  if (typeof formItem.form !== 'string' || !isFormCode(formItem.form)) {
    return { ok: false, problem: '形态只能是网络服务产品或面单渠道服务。' };
  }
  return {
    ok: true,
    snapshot: {
      scope: record.scope.trim(),
      forms: [{ productId: formItem.productId.trim(), version: formItem.version.trim(), form: formItem.form }],
    },
  };
}
