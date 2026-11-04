// 公司主体详情栏与对象页的三种读（当前资料、资料版本链、登记记录链），经取数层读（ADR-0360，query/answers.ts）。
//
// 详情栏、对象页、签上计数与签里的区读的是同一份：同键同缓存，换行切回来、在签之间来回都不再问；列表取回后把前若干行预读，
// 选中行的上下邻行也预读——方向键与点击都落在已读好的答案上，右栏不先清空成「正在读取…」再撑开（用户 2026-11-03 指出
// 「这么点数据，左边换行右边感觉慢、闪」）。
//
// 键里带身份版本（登记记录号与状态）：身份更正或停用之后版本变了，旧答案不再命中。本页的写口答到后整域作废（party-writes.ts），
// 这一格防的是别处改了、列表重读带来的新版本。

import type { ApiResult } from '../catalogue-api';
import { prefetchAnswer, useAnswer, type AnswerKey, type AnswerOptions, type AnswerQuery } from '../../query/answers';
import {
  listLegalEntityProfileRevisions,
  listLegalEntityRevisions,
  resolveLegalEntityProfile,
  type LegalEntityProfileResolutionResponseBody,
  type LegalEntityProfileRevisionListResponseBody,
  type LegalEntityRevisionListResponseBody,
} from './api';
import { partyAnswersDomain } from './party-writes';

export interface LegalEntityReadBodies {
  profile: LegalEntityProfileResolutionResponseBody;
  profileRevisions: LegalEntityProfileRevisionListResponseBody;
  identityRevisions: LegalEntityRevisionListResponseBody;
}

export type LegalEntityReadKind = keyof LegalEntityReadBodies;

const fetchers: { [Kind in LegalEntityReadKind]: (legalEntityId: string) => Promise<ApiResult<LegalEntityReadBodies[Kind]>> } = {
  // 当前资料按「此刻」解析：缓存的那一份是上一次问的此刻，新鲜期过了再按新的此刻问。
  profile: (legalEntityId) => resolveLegalEntityProfile(legalEntityId, new Date().toISOString()),
  profileRevisions: listLegalEntityProfileRevisions,
  identityRevisions: listLegalEntityRevisions,
};

/** 身份版本：登记记录号加状态。读口的答案随它变，键带上它。 */
export function legalEntityReadVersion(row: { revision: number; status: string }): string {
  return `${row.revision}:${row.status}`;
}

/** 一种读的键。不给身份版本的（资料表单预填只认此刻的当前资料）单占一格，与按版本读的互不顶替。 */
export function legalEntityReadKey(kind: LegalEntityReadKind, legalEntityId: string, version: string | undefined): AnswerKey {
  return [partyAnswersDomain, 'legal-entity', legalEntityId, version ?? '(any)', kind];
}

/** 读这家公司的一种读；没给公司（对象页还没认出是哪一家）时不发。`fresh` 见 query/answers.ts 的同名选项。 */
export function useLegalEntityRead<Kind extends LegalEntityReadKind>(
  kind: Kind,
  legalEntityId: string | undefined,
  version?: string,
  options?: Pick<AnswerOptions, 'fresh'>,
): AnswerQuery<LegalEntityReadBodies[Kind]> {
  return useAnswer(
    legalEntityReadKey(kind, legalEntityId ?? '', version),
    () => fetchers[kind](legalEntityId ?? ''),
    { enabled: legalEntityId !== undefined, fresh: options?.fresh },
  );
}

/** 预读一家公司的三种读：新鲜期内已有的不再问。 */
export function prefetchLegalEntityReads(row: { legalEntityId: string; revision: number; status: string }): void {
  const version = legalEntityReadVersion(row);
  for (const kind of Object.keys(fetchers) as LegalEntityReadKind[]) {
    const fetch = fetchers[kind] as (legalEntityId: string) => Promise<ApiResult<unknown>>;
    prefetchAnswer(legalEntityReadKey(kind, row.legalEntityId, version), () => fetch(row.legalEntityId));
  }
}

/** 选中行与它在当前排序下的上下邻行——方向键下一步会落到的地方。 */
export function neighborsOf<Row>(rows: readonly Row[], keyOfRow: (row: Row) => string, selectedKey: string | null): Row[] {
  if (selectedKey === null) return [];
  const at = rows.findIndex((row) => keyOfRow(row) === selectedKey);
  if (at < 0) return [];
  return [rows[at - 1], rows[at + 1]].filter((row): row is Row => row !== undefined);
}
