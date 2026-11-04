import { exchangeMasterData, type ApiResult } from '../catalogue-api';
import type { CommercialObjectKindName } from './publication-draft-api';

/**
 * 商业版本按身份读回一条（ADR-0361；票 commercial-version-object-pages/01）：身份是（类别、对象、版本），范围不是键。
 * `version` 与该类别目录行同形——读口是目录的投影，不另立第二套转写；页面把它当作目录里的一行用。
 * 不在册与别的租户同形，答 `COMMERCIAL_VERSION_NOT_FOUND`。
 */
export type CommercialVersionReadBody<Record> =
  | { outcome: 'COMMERCIAL_VERSION_READ'; kind: CommercialObjectKindName; version: Record }
  | { outcome: 'COMMERCIAL_VERSION_NOT_FOUND' };

export const commercialVersionReadbackEndpoint = '/commercial-version-readbacks';

export function readCommercialVersion<Record>(
  kind: CommercialObjectKindName,
  objectId: string,
  version: string,
): Promise<ApiResult<CommercialVersionReadBody<Record>>> {
  return exchangeMasterData<CommercialVersionReadBody<Record>>(
    `${commercialVersionReadbackEndpoint}/${encodeURIComponent(kind)}/${encodeURIComponent(objectId)}/${encodeURIComponent(version)}`,
  );
}

/** 读回答复里的那一版；没读到（未答、未形成、不在册）是 null。 */
export function readVersionOf<Record>(answer: ApiResult<CommercialVersionReadBody<Record>> | null): Record | null {
  return answer?.kind === 'outcome' && answer.body.outcome === 'COMMERCIAL_VERSION_READ' ? answer.body.version : null;
}
