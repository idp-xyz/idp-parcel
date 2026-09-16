// 身份停用表单的纯逻辑（票 admin-web-group-legal-entities/10 第 3 条）。全部是纯函数，node:test 钉着；组件
// IdentityDeactivationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：身份在不在册、已否停用、修订
// 是否错位，一律送上去让服务端答（停用口的 revision 是操作者声明自己看到的册面，错位由用例拒——
// isolated_write_intake.go 停用口注释）。这里只做编码层：修订号编成整数、停用时刻换成 RFC 3339、留空缺席。
//
// 停用口一个命令带种类（kind），法人与客户账户的停用也走它。种类词只从 identityKindLabels 派生；修订建议按
// 种类去对应的册上数，各册的行类型各不相同，先投成「标识 + 修订」再数——不把参与方册的修订拿去建议法人的。
//
// 各格不裁首尾空白、载荷不带租户格，理由同 business-party-form.ts。

import type { BusinessPartyRecord, CustomerAccountRecord, GroupLegalEntityRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';
import { identityKindLabels } from './presentation';
import { revisionOf } from './registration-form';
import type { PickerOption } from './PublicationFormFields';

export interface IdentityDeactivationDraft {
  /** identityKindLabels 的键之一；空串是「未选」，照送让服务端点名，表单不预选。 */
  kind: string;
  id: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟），留空即缺席。 */
  at: string;
}

/** 键名逐字取 deactivationDocument 的 json 标签。 */
export interface IdentityDeactivationItem {
  kind: string;
  id: string;
  revision?: number;
  basis: string;
  at?: string;
}

export interface IdentityDeactivationPayload {
  deactivations: IdentityDeactivationItem[];
}

export function emptyIdentityDeactivationDraft(): IdentityDeactivationDraft {
  return { kind: '', id: '', revision: '', basis: '', at: '' };
}

/** 表单认领的 JSON 路径；一项一批，下标固定 0。 */
export const identityDeactivationFieldPaths = [
  'deactivations[0].kind',
  'deactivations[0].id',
  'deactivations[0].revision',
  'deactivations[0].basis',
  'deactivations[0].at',
] as const;

export type IdentityDeactivationFieldPath = (typeof identityDeactivationFieldPaths)[number];

/** 种类下拉的选项：只从 identityKindLabels 派生，码是它的键、顺序同它。 */
export function identityKindOptions(): PickerOption[] {
  return Object.entries(identityKindLabels).map(([code, label]) => ({ value: code, label: `${label} · ${code}` }));
}

/**
 * 草稿 → 载荷。种类、标识与依据原样带；修订号编不进正整数与停用时刻换不出来时**该键缺席**，由
 * identityDeactivationLocalProblems 在送之前拦。
 */
export function identityDeactivationPayloadOf(
  draft: IdentityDeactivationDraft,
  timeZone: string,
): IdentityDeactivationPayload {
  const item: IdentityDeactivationItem = { kind: draft.kind, id: draft.id, basis: draft.basis };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.at.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.at, timeZone);
    if (moment !== null) item.at = moment;
  }
  return { deactivations: [item] };
}

/** 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。种类未选、标识与依据为空是服务端的话。 */
export function identityDeactivationLocalProblems(
  draft: IdentityDeactivationDraft,
  timeZone: string,
): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['deactivations[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.at.trim() !== '' && wallTimeToRfc3339(draft.at, timeZone) === null) {
    problems['deactivations[0].at'] = ['停用时刻要是可解析的日期时间'];
  }
  return problems;
}

/** 各册的行投成的同一形状：停用建议只需要这两格。 */
export interface RevisionedIdentity {
  id: string;
  revision: number;
}

/** 各册各自「已取回的列表」；没取到（未配置 / 出错 / 加载中）为 null，与空数组分开——空数组是册上确实没有。 */
export interface IdentityRegisters {
  parties: readonly BusinessPartyRecord[] | null;
  legalEntities: readonly GroupLegalEntityRecord[] | null;
  accounts: readonly CustomerAccountRecord[] | null;
}

/**
 * 按种类把对应册投成「标识 + 修订」。种类未选或不在词表内 → null；对应册没取到 → null——两种情形都让建议退回 1，
 * 不拿别的册冒充。
 */
export function deactivationTargetsOf(kind: string, registers: IdentityRegisters): readonly RevisionedIdentity[] | null {
  switch (kind) {
    case 'BUSINESS_PARTY':
      return registers.parties?.map((row) => ({ id: row.partyId, revision: row.revision })) ?? null;
    case 'LEGAL_ENTITY':
      return registers.legalEntities?.map((row) => ({ id: row.legalEntityId, revision: row.revision })) ?? null;
    case 'CUSTOMER_ACCOUNT':
      return registers.accounts?.map((row) => ({ id: row.accountId, revision: row.revision })) ?? null;
    default:
      return null;
  }
}

/**
 * 修订号建议值：该身份在已取回的对应册里 → 最新修订 + 1（停用落点的修订号）；不在（或册没取到）→ 1。只是建议，
 * 错位由服务端判；按原串比不裁空白，理由同 suggestedBusinessPartyRevision。
 */
export function suggestedDeactivationRevision(rows: readonly RevisionedIdentity[] | null, id: string): number {
  if (id === '' || rows === null) return 1;
  const latest = rows.find((row) => row.id === id);
  return latest ? latest.revision + 1 : 1;
}
