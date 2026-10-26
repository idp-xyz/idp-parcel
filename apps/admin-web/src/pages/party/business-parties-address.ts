// 业务参与方页「登记 › 身份停用」的深链（票 party-commercial-uiux/06）。法人与客户账户的停用也在这一张表单里做，
// 别的页要把人直接送到这张表单、并预先选好种类与标识，就把这两样写进地址的查询串：
// `#/business-parties?deactivate=LEGAL_ENTITY&id=SYN-LE-01`。
//
// 只带种类与标识，不带修订号、依据与时点：那几格要停用的人自己看过当前修订后填，地址里带过来的值会被当成已核对过。
// 纯函数、只引零依赖的 address-query，node:test 直接跑。

import { hashQueryValue, hashWithQueryValue } from '../../templates/address-query';
import { isIdentityKind } from './identity-deactivation-form';
import type { IdentityKind } from './presentation';

export const DEACTIVATE_KIND_PARAM = 'deactivate';
export const DEACTIVATE_ID_PARAM = 'id';

export interface DeactivationPreset {
  kind: IdentityKind;
  id: string;
}

/** 指向停用表单、带好种类与标识的地址。 */
export function deactivationLinkHash(preset: DeactivationPreset): string {
  return hashWithQueryValue(
    hashWithQueryValue('#/business-parties', DEACTIVATE_KIND_PARAM, preset.kind),
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
 * 改回去。检索词等别的键原样保留。
 */
export function withoutDeactivationPreset(hash: string): string {
  return hashWithQueryValue(hashWithQueryValue(hash, DEACTIVATE_KIND_PARAM, ''), DEACTIVATE_ID_PARAM, '');
}
