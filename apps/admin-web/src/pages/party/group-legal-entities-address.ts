// 集团与法人页「地址 → 视图」的判读（票 group-legal-entities-uiux/04）。纯函数，node:test 钉着；页面只负责摆。
//
// 三种视图都由 hash 定，hash 是位置的唯一权威：
// - 对象页 `#/group-legal-entities/<法人标识>`——壳层拿前两段作标签 id，对象页因此是自己的一张标签；
// - 登记视图 `#/group-legal-entities?register=1`——还是列表那张标签，刷新与浏览器后退都按地址走；
// - 其余是列表。
// 登记视图不占路径第二段：第二段是法人标识，法人标识是自由文本，拿一个保留词占它，同名法人的对象页就打不开了。
// 也不用 `?view=`：那个键已归保存视图（address-query.ts 文件头）。

import { decodeHashSegment, hashQueryValue, hashWithQueryValue } from '../../templates/address-query';

const MODULE_ID = 'group-legal-entities';

/** 登记视图在地址查询串里的键；值为 `1` 即在登记视图。 */
export const REGISTER_VIEW_PARAM = 'register';

export type GroupLegalEntitiesView = { kind: 'list' } | { kind: 'register' } | { kind: 'object'; legalEntityId: string };

/**
 * 对象地址先认：对象地址上即便带了登记键，也是对象页。查询串不进对象标识——`?q=` 是列表检索词，粘在第二段上会认错行。
 * 畸形百分号序列退回列表，不抛（理由见 decodeHashSegment）。
 */
export function groupLegalEntitiesViewOf(hash: string): GroupLegalEntitiesView {
  const path = hash.replace(/^#\/?/, '').split('?')[0];
  const [moduleId, objectId] = path.split('/');
  if (moduleId === MODULE_ID && objectId) {
    const legalEntityId = decodeHashSegment(objectId);
    return legalEntityId === null ? { kind: 'list' } : { kind: 'object', legalEntityId };
  }
  return hashQueryValue(hash, REGISTER_VIEW_PARAM) === '1' ? { kind: 'register' } : { kind: 'list' };
}

/** 进出登记视图的地址：只改登记键，路径与检索词等别的键原样。 */
export function registerViewHash(hash: string, open: boolean): string {
  return hashWithQueryValue(hash, REGISTER_VIEW_PARAM, open ? '1' : '');
}
