// 商业版本对象页的地址（票 commercial-version-object-pages/02）：`#/<页>/<objectId>/<version>`。纯函数、只引零依赖的 address-query，
// node:test 直接跑。对象段先认：对象地址上即便带了别的键（商业规则页的 `?register=`、检索词），也是对象页。
// 工作区标签只取前两段（shell/workspace-state.ts 的 tabIdFromHash），同一对象的各版共用一张标签、地址换版。

import { decodeHashSegment } from '../../templates/address-query';

export interface VersionObjectAddress {
  objectId: string;
  version: string;
}

/** 这个页名下的 hash 指的是不是某一版：两段都在且解得开才是；畸形百分号序列退回 null，不抛。 */
export function versionObjectOf(hash: string, moduleId: string): VersionObjectAddress | null {
  const path = hash.replace(/^#\/?/, '').split('?')[0];
  const [first, second, third] = path.split('/');
  if (first !== moduleId || !second || !third) return null;
  const objectId = decodeHashSegment(second);
  const version = decodeHashSegment(third);
  return objectId === null || version === null ? null : { objectId, version };
}

/** 对象页地址；`query` 是要带上的查询串键值（商业规则页带册名），不带就没有。 */
export function versionObjectHash(moduleId: string, objectId: string, version: string, query?: Record<string, string>): string {
  const base = `#/${moduleId}/${encodeURIComponent(objectId)}/${encodeURIComponent(version)}`;
  const params = new URLSearchParams(query ?? {}).toString();
  return params ? `${base}?${params}` : base;
}
