// 业务参与方页「地址 → 视图」的判读与几条深链。纯函数、只引零依赖的 address-query，node:test 直接跑；页面只负责摆。
//
// 三种视图都由 hash 定，hash 是位置的唯一权威（做法同 group-legal-entities-address.ts）：
// - 对象页 `#/business-parties/<参与方标识>`——壳层拿前两段作标签 id，对象页因此是自己的一张标签；
// - 登记视图 `#/business-parties?register=<册>`——还是列表那张标签，刷新与浏览器后退都按地址走。本页登记有三本册
//   （参与方身份、参与方关系、身份停用），键值点名是哪一本，三张表单在视图里切换只换这个值；
// - 其余是列表。
// 登记视图不占路径第二段：第二段是参与方标识，标识是自由文本，拿一个保留词占它，同名参与方的对象页就打不开了。
// 也不用 `?view=`：那个键已归保存视图（address-query.ts 文件头）。
//
// 停用深链（票 party-commercial-uiux/06）：法人与客户账户的停用也在本页的停用表单里做，别的页要把人直接送到这张表单、
// 并预先选好种类与标识，就把这两样写进查询串：`#/business-parties?register=identity-deactivation&deactivate=LEGAL_ENTITY&id=SYN-LE-01`。
// 只带种类与标识，不带修订号、依据与时点：那几格要停用的人自己看过当前修订后填，地址里带过来的值会被当成已核对过。
// 只带 `deactivate` 与 `id`、不带 `register` 的旧链接同样落到停用表单——那两个键在场本身就说明了去处。

import { decodeHashSegment, hashQueryValue, hashWithQueryValue } from '../../templates/address-query';
import { isIdentityKind } from './identity-deactivation-form';
import type { IdentityKind } from './presentation';

const MODULE_ID = 'business-parties';

/** 登记视图在地址查询串里的键；值是三本册之一。 */
export const REGISTER_VIEW_PARAM = 'register';
export const DEACTIVATE_KIND_PARAM = 'deactivate';
export const DEACTIVATE_ID_PARAM = 'id';

/** 本页三本登记册，顺序即登记视图里切换条的顺序。 */
export const businessPartyRegisters = ['business-party', 'party-relationship', 'identity-deactivation'] as const;

export type BusinessPartyRegister = (typeof businessPartyRegisters)[number];

export function isBusinessPartyRegister(value: string): value is BusinessPartyRegister {
  return (businessPartyRegisters as readonly string[]).includes(value);
}

export type BusinessPartiesView =
  | { kind: 'list' }
  | { kind: 'register'; register: BusinessPartyRegister }
  | { kind: 'object'; partyId: string };

export interface DeactivationPreset {
  kind: IdentityKind;
  id: string;
}

/**
 * 对象地址先认：对象地址上即便带了登记键，也是对象页。查询串不进对象标识——`?q=` 是列表检索词，粘在第二段上会认错行。
 * 畸形百分号序列退回列表，不抛（理由见 decodeHashSegment）。登记键认不得的值当没带：别人发来的链接拼错了册名，落回列表
 * 比落进某一本猜出来的册稳。
 */
export function businessPartiesViewOf(hash: string): BusinessPartiesView {
  const path = hash.replace(/^#\/?/, '').split('?')[0];
  const [moduleId, objectId] = path.split('/');
  if (moduleId === MODULE_ID && objectId) {
    const partyId = decodeHashSegment(objectId);
    return partyId === null ? { kind: 'list' } : { kind: 'object', partyId };
  }
  const register = hashQueryValue(hash, REGISTER_VIEW_PARAM);
  if (isBusinessPartyRegister(register)) return { kind: 'register', register };
  if (register === '' && deactivationPresetOf(hash) !== null) return { kind: 'register', register: 'identity-deactivation' };
  return { kind: 'list' };
}

/** 进出登记视图、或在视图里换册的地址：只改登记键，路径与检索词等别的键原样；传 null 即回列表。 */
export function registerViewHash(hash: string, register: BusinessPartyRegister | null): string {
  return hashWithQueryValue(hash, REGISTER_VIEW_PARAM, register ?? '');
}

/** 参与方对象页的地址（hash 二段）。壳层拿前两段作标签 id，对象页因此另开一张标签，列表留在原处。 */
export function businessPartyObjectHash(partyId: string): string {
  return `#/${MODULE_ID}/${encodeURIComponent(partyId)}`;
}

/** 指向停用表单、带好种类与标识的地址。 */
export function deactivationLinkHash(preset: DeactivationPreset): string {
  return hashWithQueryValue(
    hashWithQueryValue(registerViewHash(`#/${MODULE_ID}`, 'identity-deactivation'), DEACTIVATE_KIND_PARAM, preset.kind),
    DEACTIVATE_ID_PARAM,
    preset.id,
  );
}

/** 地址里带着的预填；种类不在封闭集或标识为空都当没带，不猜。 */
export function deactivationPresetOf(hash: string): DeactivationPreset | null {
  const kind = hashQueryValue(hash, DEACTIVATE_KIND_PARAM);
  const id = hashQueryValue(hash, DEACTIVATE_ID_PARAM);
  if (!isIdentityKind(kind) || id.trim() === '') return null;
  return { kind, id };
}

/**
 * 预填用过之后从地址里去掉：留着的话，换到别的标签再回来、页面重新挂载时会再预填一次，把操作者已改过的种类与标识
 * 改回去。去掉预填的同时把登记键写实为停用册——旧链接只带预填两键时视图靠它们认册，去掉之后刷新得仍停在停用表单。
 * 检索词等别的键原样保留。
 */
export function withoutDeactivationPreset(hash: string): string {
  const stripped = hashWithQueryValue(hashWithQueryValue(hash, DEACTIVATE_KIND_PARAM, ''), DEACTIVATE_ID_PARAM, '');
  return deactivationPresetOf(hash) === null ? stripped : registerViewHash(stripped, 'identity-deactivation');
}
