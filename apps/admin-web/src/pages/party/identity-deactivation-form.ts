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
import { formatInstant, wallTimeToRfc3339 } from '../moment';
import { identityKindLabels, type IdentityKind } from './presentation';
import { revisionOf, suggestedNextRevision } from './registration-form';
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

/** 种类串是不是词表里的一格。词表是封闭集（identityKindLabels），集外的串不是「另一种」，是未选或坏值。 */
export function isIdentityKind(kind: string): kind is IdentityKind {
  return kind in identityKindLabels;
}

/**
 * 种类 → 该册一行投成「标识 + 修订」（票 13 第 2 条：三册按种类分派的事合成按种类键的表，不再各处 switch）。
 * 按 IdentityKind 键成 Record，词表多一格这里就编不过——分派表与词表一起长，漏一处在编译期就显。
 *
 * 「种类 → 三册里取哪一册」不在这里：那张表连读口、候选与读面名一起住在组件（IdentityDeactivationForm.tsx 的
 * kindRegisters），键在同一个 IdentityKind 上，取到册后逐行调这里的投影。「取哪一册」只有那一张表——同一件事两张表，
 * 改词表的人会先撞见死的那张。
 */
export const identityTargetOf = {
  BUSINESS_PARTY: (row: BusinessPartyRecord): RevisionedIdentity => ({ id: row.partyId, revision: row.revision }),
  LEGAL_ENTITY: (row: GroupLegalEntityRecord): RevisionedIdentity => ({ id: row.legalEntityId, revision: row.revision }),
  CUSTOMER_ACCOUNT: (row: CustomerAccountRecord): RevisionedIdentity => ({ id: row.accountId, revision: row.revision }),
} as const satisfies Record<IdentityKind, (row: never) => RevisionedIdentity>;

/**
 * 修订号建议值（规则在 suggestedNextRevision，这里只交投影后的取键）：建议的是停用落点的修订号，错位由服务端判；
 * 按原串比不裁空白，理由同 suggestedBusinessPartyRevision。
 */
export function suggestedDeactivationRevision(rows: readonly RevisionedIdentity[] | null, id: string): number {
  return suggestedNextRevision(rows, id, (row) => row.id);
}

/** 停用确认框里要复述的一项。 */
export interface DeactivationFact {
  label: string;
  value: string;
}

export interface DeactivationConfirmation {
  title: string;
  message: string;
  facts: DeactivationFact[];
}

/**
 * 停用确认框的文案（票 party-commercial-uiux/06）。停用在 PC CONTEXT 里是终态，按下之前把要停的是谁、从什么时候起、
 * 凭什么复述一遍：种类、名称与标识、停用时点（本地墙钟与 UTC 各一份，跨 CN/SG 两地对时不必换算）、依据、修订号。
 * 未填的格照实说未填——确认框不替表单判，送上去由服务端点名。
 *
 * `draft` 取表单真正要送的那一份（修订号未改时带着建议值）。`pickedLabel` 是下拉里选中那一项的文字，三册都写成
 * 「名称 · 标识 · 当前状态」，有它就照它复述；手填的标识没有候选文字，只报标识。
 */
export function deactivationConfirmation(
  draft: IdentityDeactivationDraft,
  timeZone: string,
  pickedLabel: string | null,
): DeactivationConfirmation {
  const kindLabel = isIdentityKind(draft.kind) ? identityKindLabels[draft.kind] : '';
  const instant = wallTimeToRfc3339(draft.at, timeZone);
  const id = draft.id.trim();
  const revision = draft.revision.trim();
  return {
    title: kindLabel ? `停用这个${kindLabel}？` : '停用这个身份？',
    message: '停用后不能撤回；要恢复，只能登记一个新的身份。从停用时点起，它不再支持新的商业决定。',
    facts: [
      { label: '身份种类', value: kindLabel || '（未选）' },
      { label: '身份', value: id === '' ? '（未填）' : (pickedLabel ?? id) },
      { label: '停用时点', value: instant ? `${formatInstant(instant, { timeZone })}（UTC ${instant}）` : '（未填或格式不对）' },
      { label: '停用依据', value: draft.basis.trim() || '（未填）' },
      { label: '修订号', value: revision === '' ? '（未填）' : `r${revision}` },
    ],
  };
}
