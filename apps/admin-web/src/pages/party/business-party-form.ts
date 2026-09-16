// 业务参与方身份登记表单的纯逻辑（票 admin-web-group-legal-entities/10；ADR-0101 决定八自裁：格少、低频、
// 无矩阵——直接逐字段表单，形状照票 02 的 legal-entity-form.ts）。全部是纯函数，node:test 钉着；组件
// BusinessPartyRegistrationForm.tsx 只负责摆。
//
// **本文件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）。参与方标识与名称是否为空、
// 修订号连不连续，一律原样送上去让服务端逐格答；这里只做编码层的事：修订号编成 JSON 整数、墙钟时刻换成
// RFC 3339、可缺的键缺席而不是零值。
//
// **各格不裁首尾空白。** 这一点与票 02 的法人表单相反、与服务端一致：isolated_write_intake.go 的
// businessPartyDocument 注释写明字段不做裁切、与受控 CLI 同判，表单那层裁了会让 " X" 与 "X" 在这一口成为
// 同一身份、在 CLI 却是两个。修订号那一格例外——它编成的是数，不是身份串，周围空白不构成第二个值。
//
// **载荷不带租户格。** 镜像受控 CLI `parcel-commercial register-parties` 输入里 `businessParties` 数组的一项，
// 去掉整批的 `tenantId`：隔离 Intake 对键在场即拒、含 null（refuseSelfReportedTenant）。

import type { BusinessPartyRecord } from './api';
import { wallTimeToRfc3339 } from '../moment';

export interface BusinessPartyDraft {
  partyId: string;
  name: string;
  /** 文本框原值；编成整数在组载荷时做。 */
  revision: string;
  basis: string;
  /** `datetime-local` 原值（操作者本地墙钟），留空即缺席。 */
  effectiveFrom: string;
}

/** 键名逐字取 businessPartyDocument 的 json 标签。 */
export interface BusinessPartyRegistrationItem {
  partyId: string;
  name: string;
  revision?: number;
  basis: string;
  effectiveFrom?: string;
}

export interface BusinessPartyRegistrationPayload {
  businessParties: BusinessPartyRegistrationItem[];
}

export function emptyBusinessPartyDraft(): BusinessPartyDraft {
  return { partyId: '', name: '', revision: '', basis: '', effectiveFrom: '' };
}

/** 表单认领的 JSON 路径；一项一批，下标固定 0。 */
export const businessPartyFieldPaths = [
  'businessParties[0].partyId',
  'businessParties[0].name',
  'businessParties[0].revision',
  'businessParties[0].basis',
  'businessParties[0].effectiveFrom',
] as const;

export type BusinessPartyFieldPath = (typeof businessPartyFieldPaths)[number];

const positiveInteger = /^[1-9]\d*$/;

/** 修订号编成正整数；编不出交回 undefined。各表单的这一格同判，抬成一处。 */
export function revisionOf(raw: string): number | undefined {
  const trimmed = raw.trim();
  return positiveInteger.test(trimmed) ? Number(trimmed) : undefined;
}

/**
 * 草稿 → 载荷。身份串原样带（含首尾空白与空串——空串是服务端要点名的格）；修订号编不进正整数与生效时刻
 * 换不出来时**该键缺席**——不造一个假数顶上，由 businessPartyLocalProblems 在送之前拦。
 */
export function businessPartyPayloadOf(draft: BusinessPartyDraft, timeZone: string): BusinessPartyRegistrationPayload {
  const item: BusinessPartyRegistrationItem = {
    partyId: draft.partyId,
    name: draft.name,
    basis: draft.basis,
  };
  const revision = revisionOf(draft.revision);
  if (revision !== undefined) item.revision = revision;
  if (draft.effectiveFrom.trim() !== '') {
    const moment = wallTimeToRfc3339(draft.effectiveFrom, timeZone);
    if (moment !== null) item.effectiveFrom = moment;
  }
  return { businessParties: [item] };
}

/**
 * 本地能判的**编码层**问题，按 JSON 路径归组；空对象即可送。只有编码层的：修订号编不进正整数、生效时刻换不成
 * RFC 3339。空格、标识是否已在册、修订是否连续都不在这里判，那是服务端的话。
 */
export function businessPartyLocalProblems(draft: BusinessPartyDraft, timeZone: string): Record<string, string[]> {
  const problems: Record<string, string[]> = {};
  if (revisionOf(draft.revision) === undefined) {
    problems['businessParties[0].revision'] = ['修订号要填正整数'];
  }
  if (draft.effectiveFrom.trim() !== '' && wallTimeToRfc3339(draft.effectiveFrom, timeZone) === null) {
    problems['businessParties[0].effectiveFrom'] = ['生效时刻要是可解析的日期时间'];
  }
  return problems;
}

/**
 * 修订号建议值：标识在已取回列表里 → 最新修订 + 1；不在（或列表没取到）→ 1。**只是建议**：列表答的是最新修订、
 * 且取回那一刻起就可能过期，连续性仍由服务端按册面判。查找按原串比而不裁空白，与载荷同判——载荷里 " X" 就是
 * 另一个身份，建议也得按它是新标识算，否则建议与送上去的东西说的不是同一个对象。
 */
export function suggestedBusinessPartyRevision(rows: readonly BusinessPartyRecord[] | null, partyId: string): number {
  if (partyId === '' || rows === null) return 1;
  const latest = rows.find((row) => row.partyId === partyId);
  return latest ? latest.revision + 1 : 1;
}
