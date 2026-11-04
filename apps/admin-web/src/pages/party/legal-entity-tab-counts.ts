// 公司主体详情栏与对象页「资料版本」「登记记录」两签上的计数（票 company-subject-ux/09）：各读一次那一签要读的修订链，
// 只取条数。签里的区（LegalEntityProfileHistory、RevisionHistorySection）是导出件、各带自己的重试与判读，各自经取数层读——
// 与这里同键，缓存里是同一份，不必把答案穿过去。「关联」签的计数在 legal-entity-references.ts 的 referencedCount，与首屏
// 「被引用」同一份答案，这里不管。

import type { ApiResult } from '../catalogue-api';
import { useLegalEntityRead } from './legal-entity-reads';

/** 回显标识 + 修订链：资料版本与登记记录两种答复的公共形状，计数只看这两格。 */
export interface RevisionChainBody {
  legalEntityId: string;
  revisions: readonly unknown[];
}

/**
 * 签上的数：答到了、且答的是这家公司，才是修订链的条数；还没答、读不到、或答的是上一家（方向键换行时慢答案后到）
 * 都没有数——签上不显，不拿 0 冒充「没有」。0 照显：链上一条都没有是「没登记过」，是事实不是不知道（口径同 referenceLineText）。
 */
export function revisionCountOf<Body extends RevisionChainBody>(
  answer: ApiResult<Body> | null,
  legalEntityId: string,
): number | undefined {
  if (answer === null || answer.kind !== 'outcome' || answer.body.legalEntityId !== legalEntityId) return undefined;
  return answer.body.revisions.length;
}

export interface LegalEntityTabCounts {
  /** 「资料版本」签的数；没数不显。 */
  profileRevisions?: number;
  /** 「登记记录」签的数；没数不显。 */
  identityRevisions?: number;
}

/**
 * 两条链各读一次，与签里的区（LegalEntityProfileHistory、RevisionHistorySection）同键同缓存——签上先有数、点进签里不再问。
 * 换了公司或身份版本就是另一把键；资料更新与身份登记落册后整域作废（party-writes.ts），这里不另拧钥匙。
 */
export function useLegalEntityTabCounts(legalEntityId: string | undefined, version?: string): LegalEntityTabCounts {
  const profile = useLegalEntityRead('profileRevisions', legalEntityId, version).answer;
  const identity = useLegalEntityRead('identityRevisions', legalEntityId, version).answer;
  if (!legalEntityId) return {};
  return {
    profileRevisions: revisionCountOf(profile, legalEntityId),
    identityRevisions: revisionCountOf(identity, legalEntityId),
  };
}
