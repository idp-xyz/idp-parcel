// 责任法人身份登记表单的纯逻辑（票 admin-web-group-legal-entities/02；ADR-0101 决定八自裁：五格、低频、
// 无矩阵——直接逐字段表单）。全部是纯函数，node:test 钉着；组件 LegalEntityRegistrationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）。参与方在不在册、届时
// 是否已生效、修订号连不连续、依据是否为空，一律原样送上去让服务端逐格答；这里只做编码层的事：
// 修订号编成 JSON 整数、墙钟时刻换成 RFC 3339、可缺的键缺席而不是空串。
//
// **载荷不带租户格。** 镜像受控 CLI `parcel-commercial register-parties` 输入里 `legalEntities` 数组的一项，
// 但去掉整批的 `tenantId`：在线 Intake 要把认证结果填进租户格、只从载荷取行内容（register_party_identity.go
// 包注释）；载荷里带上租户就是请求方自报租户。今天端点挂 UnconfiguredIntake{} 必答 403，这份形状没有服务端
// 消费者，真 Intake 落地时以它为准重谈，不把这里当已发布 Schema（api.ts 那段注释的原话）。

import type { GroupLegalEntityRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';
import { revisionOf } from './registration-form';

export interface LegalEntityDraft {
  legalEntityId: string;
  partyId: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟），留空即缺席。 */
  effectiveFrom: string;
}

export interface LegalEntityRegistrationItem {
  legalEntityId: string;
  partyId: string;
  revision?: number;
  basis: string;
  effectiveFrom?: string;
}

export interface LegalEntityRegistrationPayload {
  legalEntities: LegalEntityRegistrationItem[];
}

export function emptyLegalEntityDraft(): LegalEntityDraft {
  return { legalEntityId: '', partyId: '', revision: '', basis: '', effectiveFrom: '' };
}

/** 表单认领的 JSON 路径，服务端逐格问题按它点名；一项一批，下标固定 0。 */
export const legalEntityFieldPaths = [
  'legalEntities[0].legalEntityId',
  'legalEntities[0].partyId',
  'legalEntities[0].revision',
  'legalEntities[0].basis',
  'legalEntities[0].effectiveFrom',
] as const;

export type LegalEntityFieldPath = (typeof legalEntityFieldPaths)[number];

/**
 * 草稿 → 载荷。各格去首尾空白后原样带（空串照送，那是服务端要点名的格）；修订号编不进正整数
 * 与生效时刻换不出来时**该键缺席**——不造一个假数顶上，由 legalEntityLocalProblems 在送之前拦。
 */
export function legalEntityPayloadOf(draft: LegalEntityDraft, timeZone: string): LegalEntityRegistrationPayload {
  const item: LegalEntityRegistrationItem = {
    legalEntityId: draft.legalEntityId.trim(),
    partyId: draft.partyId.trim(),
    basis: draft.basis.trim(),
  };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.effectiveFrom.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.effectiveFrom, timeZone);
    if (moment !== null) item.effectiveFrom = moment;
  }
  return { legalEntities: [item] };
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。只有两种：修订号编不进正整数、
 * 生效时刻换不成 RFC 3339。空格、引用是否在册、修订是否连续都不在这里判，那是服务端的话。
 */
export function legalEntityLocalProblems(draft: LegalEntityDraft, timeZone: string): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['legalEntities[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.effectiveFrom.trim() !== '' && wallTimeToRfc3339(draft.effectiveFrom, timeZone) === null) {
    problems['legalEntities[0].effectiveFrom'] = ['生效时刻要是可解析的日期时间'];
  }
  return problems;
}

/**
 * 修订号建议值：标识在已取回列表里 → 最新修订 + 1；不在（或列表没取到）→ 1。**只是建议**：
 * 列表答的是最新修订、且取回那一刻起就可能过期，连续性仍由服务端按册面判；这一格省的是
 * 操作者翻一次册，不替服务端作数。
 */
export function suggestedRevision(rows: readonly GroupLegalEntityRecord[] | null, legalEntityId: string): number {
  const wanted = legalEntityId.trim();
  if (wanted === '' || rows === null) return 1;
  const latest = rows.find((row) => row.legalEntityId === wanted);
  return latest ? latest.revision + 1 : 1;
}
