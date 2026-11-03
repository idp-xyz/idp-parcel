// 公司主体详情栏与对象页「资料版本」「登记记录」两签上的计数（票 company-subject-ux/09）：各读一次那一签要读的修订链，
// 只取条数。签里的区（LegalEntityProfileHistory、RevisionHistorySection）去到时各自再读一份——它们是导出件、各带自己的
// 重试与判读，不为省一次 GET 把答案穿过去。「关联」签的计数在 legal-entity-references.ts 的 referencedCount，与首屏
// 「被引用」同一份答案，这里不管。

import { useEffect, useMemo, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import { listLegalEntityProfileRevisions, listLegalEntityRevisions } from './api';
import { cachedLegalEntityRead, loadLegalEntityRead } from './legal-entity-read-cache';

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

interface Answered {
  request: object;
  profile?: ApiResult<RevisionChainBody>;
  identity?: ApiResult<RevisionChainBody>;
}

/**
 * 两条链各读一次。`refreshKey` 由页面给：资料更新落册、身份登了下一修订或停用之后都要重读，页面已为当前资料的读口
 * 拼了一把同样的钥匙，这里照用——一把钥匙两处转，少一处忘了拧。答案钉在发问那一刻的 request 上：换了公司或钥匙，
 * 旧答案即使后到也不算数。
 */
export function useLegalEntityTabCounts(
  legalEntityId: string | undefined,
  refreshKey: string,
  version?: string,
): LegalEntityTabCounts {
  const request = useMemo(() => ({ legalEntityId, refreshKey }), [legalEntityId, refreshKey]);
  const [answered, setAnswered] = useState<Answered | null>(null);

  useEffect(() => {
    if (!request.legalEntityId) return;
    let cancelled = false;
    // 给了身份版本就经页面内缓存问（与当前资料同一份缓存，见 legal-entity-read-cache.ts）：读过的先显、问到了再替换。
    const profileLoad =
      version === undefined
        ? listLegalEntityProfileRevisions(request.legalEntityId)
        : loadLegalEntityRead('profileRevisions', request.legalEntityId, version);
    const identityLoad =
      version === undefined
        ? listLegalEntityRevisions(request.legalEntityId)
        : loadLegalEntityRead('identityRevisions', request.legalEntityId, version);
    void profileLoad.then((profile) => {
      if (!cancelled) setAnswered((previous) => ({ ...(previous?.request === request ? previous : {}), request, profile }));
    });
    void identityLoad.then((identity) => {
      if (!cancelled) setAnswered((previous) => ({ ...(previous?.request === request ? previous : {}), request, identity }));
    });
    return () => {
      cancelled = true;
    };
  }, [request, version]);

  if (!legalEntityId) return {};
  const fresh = answered?.request === request ? answered : undefined;
  const profile = fresh?.profile ?? (version !== undefined ? cachedLegalEntityRead('profileRevisions', legalEntityId, version) : undefined);
  const identity = fresh?.identity ?? (version !== undefined ? cachedLegalEntityRead('identityRevisions', legalEntityId, version) : undefined);
  return {
    profileRevisions: revisionCountOf(profile ?? null, legalEntityId),
    identityRevisions: revisionCountOf(identity ?? null, legalEntityId),
  };
}
