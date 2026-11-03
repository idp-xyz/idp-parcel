// 公司主体详情栏与对象页的三种读（当前资料、资料版本链、登记记录链）在页面内的缓存与预读。
//
// 数据本身很快（演示租户实测每个读口 7–8ms），慢的是换行那一刻右栏先清空成「正在读取…」再撑开——用户 2026-11-03 指出
// 「这么点数据，左边换行右边感觉慢、闪」。所以：读过的公司再切回来先显缓存、后台照常再读一遍（答案到了才替换），列表取回后
// 把前若干行预读好，选中行的上下邻行也预读——方向键与点击都落在已读好的数据上。
//
// 只缓存「答到了」的答案（outcome）：拒绝、未配置、读不动不缓存，下一次照常去问，不让一次失败粘住。
// 缓存键带身份版本（登记记录号与状态）：身份更正或停用之后版本变了，旧缓存自然不再命中；资料更新落册由调用方显式作废。
// 只活在本页内存里，刷新即清空——它不是数据源，只是避免换行时的空窗。

import type { ApiResult } from '../catalogue-api';
import {
  listLegalEntityProfileRevisions,
  listLegalEntityRevisions,
  resolveLegalEntityProfile,
  type LegalEntityProfileResolutionResponseBody,
  type LegalEntityProfileRevisionListResponseBody,
  type LegalEntityRevisionListResponseBody,
} from './api';

export interface LegalEntityReadBodies {
  profile: LegalEntityProfileResolutionResponseBody;
  profileRevisions: LegalEntityProfileRevisionListResponseBody;
  identityRevisions: LegalEntityRevisionListResponseBody;
}

export type LegalEntityReadKind = keyof LegalEntityReadBodies;

const fetchers: { [Kind in LegalEntityReadKind]: (legalEntityId: string) => Promise<ApiResult<LegalEntityReadBodies[Kind]>> } = {
  // 当前资料按「此刻」解析：缓存的那一份是上一次问的此刻，换回来先显它、再按新的此刻问一遍。
  profile: (legalEntityId) => resolveLegalEntityProfile(legalEntityId, new Date().toISOString()),
  profileRevisions: listLegalEntityProfileRevisions,
  identityRevisions: listLegalEntityRevisions,
};

const answers = new Map<string, ApiResult<unknown>>();
const inflight = new Map<string, Promise<ApiResult<unknown>>>();

/** 身份版本：登记记录号加状态。读口的答案随它变，缓存键带上它。 */
export function legalEntityReadVersion(row: { revision: number; status: string }): string {
  return `${row.revision}:${row.status}`;
}

function keyOf(kind: LegalEntityReadKind, legalEntityId: string, version: string): string {
  return `${kind}\u0000${legalEntityId}\u0000${version}`;
}

/** 缓存里这一读答到过的那份；没有就是 undefined。 */
export function cachedLegalEntityRead<Kind extends LegalEntityReadKind>(
  kind: Kind,
  legalEntityId: string,
  version: string,
): ApiResult<LegalEntityReadBodies[Kind]> | undefined {
  return answers.get(keyOf(kind, legalEntityId, version)) as ApiResult<LegalEntityReadBodies[Kind]> | undefined;
}

/**
 * 去问一遍并把答到的那份记下。同一把键正在问着就跟上那一次，不另发——预读还在路上时点了那一行，等的是同一个请求。
 */
export function loadLegalEntityRead<Kind extends LegalEntityReadKind>(
  kind: Kind,
  legalEntityId: string,
  version: string,
): Promise<ApiResult<LegalEntityReadBodies[Kind]>> {
  const key = keyOf(kind, legalEntityId, version);
  const pending = inflight.get(key);
  if (pending) return pending as Promise<ApiResult<LegalEntityReadBodies[Kind]>>;
  const request = fetchers[kind](legalEntityId).then((answer) => {
    inflight.delete(key);
    if (answer.kind === 'outcome') answers.set(key, answer);
    return answer;
  });
  inflight.set(key, request as Promise<ApiResult<unknown>>);
  return request;
}

/** 资料更新落册之后作废这家公司的全部缓存：版本号没变（身份没动），旧的那份不能再先显出来。 */
export function invalidateLegalEntityReads(legalEntityId: string): void {
  const marker = `\u0000${legalEntityId}\u0000`;
  for (const key of [...answers.keys()]) if (key.includes(marker)) answers.delete(key);
}

/** 预读一家公司的三种读：已有缓存或正在问的跳过。 */
export function prefetchLegalEntityReads(row: { legalEntityId: string; revision: number; status: string }): void {
  const version = legalEntityReadVersion(row);
  for (const kind of Object.keys(fetchers) as LegalEntityReadKind[]) {
    const key = keyOf(kind, row.legalEntityId, version);
    if (answers.has(key) || inflight.has(key)) continue;
    void loadLegalEntityRead(kind, row.legalEntityId, version);
  }
}

/** 选中行与它在当前排序下的上下邻行——方向键下一步会落到的地方。 */
export function neighborsOf<Row>(rows: readonly Row[], keyOfRow: (row: Row) => string, selectedKey: string | null): Row[] {
  if (selectedKey === null) return [];
  const at = rows.findIndex((row) => keyOfRow(row) === selectedKey);
  if (at < 0) return [];
  return [rows[at - 1], rows[at + 1]].filter((row): row is Row => row !== undefined);
}
