// 授权交付方关系的适用范围拼法（票 ps-external-mark-relations/16）。纯函数，node:test 钉着；登记表单只调它。
//
// 服务端只认一种拼法：party-commercial domain 里 AuthorizedDeliveryScope 的 Reference 拼、ParseAuthorizedDeliveryScope
// 只收它拼出的那一种——URL 查询串，`customerAccount` 在前、可再接 `&node=`，值按 Go 的 url.QueryEscape 转义。范围串
// 进内容摘要与重放判据，换一种拼法就被拒在构造门，所以表单不让配置员手写，由这里拼。
//
// 不用 URLSearchParams：它不转 `*`、却把 `~` 转成 `%7E`，与 Go 的转义表恰好各差一边，拼出来服务端不认。
//
// 首尾空白不裁，同 party-relationship-form 各格：服务端不裁，裁了就把操作者填的值换成了另一个范围。只填空白按没填
// 算，因为服务端也这样判——空白账户、空白节点都拒。

import type { PartyRoleCode } from './presentation';

/** 适用范围要按这一拼法填写的角色。键取自 partyRoleLabels，词表改名时这里编译不过。 */
export const authorizedDeliveringPartyRole: PartyRoleCode = 'AUTHORIZED_DELIVERING_PARTY';

export interface AuthorizedDeliveryScopeInput {
  /** 客户账户标识原值，必填。 */
  customerAccount: string;
  /** 节点标识原值；空串即不收窄到节点。 */
  node: string;
}

export type AuthorizedDeliveryScopeSpelling = { ok: true; scope: string } | { ok: false; problems: string[] };

// Go 的 url.QueryEscape 只留 A–Z a–z 0–9 与 `-_.~`，空格写成 `+`，其余按 UTF-8 字节写大写 `%XX`。
// encodeURIComponent 多留了 `!'()*` 五个，补转；它写空格的 `%20` 只可能来自空格本身（`%` 自己会被写成 `%25`）。
// 孤立的代理码元编不成 UTF-8，encodeURIComponent 会抛——这里答 null，不让表单在渲染时崩掉。
function queryEscape(value: string): string | null {
  let escaped: string;
  try {
    escaped = encodeURIComponent(value);
  } catch {
    return null;
  }
  return escaped
    .replace(/[!'()*]/g, (char) => `%${char.charCodeAt(0).toString(16).toUpperCase()}`)
    .replace(/%20/g, '+');
}

/**
 * 两格原值 → 那一种拼法。客户账户空白、节点填了却只有空白、含编不成 UTF-8 的字符，都拼不出，答 problems；
 * 节点留空串即不带 `node` 键。
 */
export function spellAuthorizedDeliveryScope(input: AuthorizedDeliveryScopeInput): AuthorizedDeliveryScopeSpelling {
  const problems: string[] = [];
  let account: string | null = null;
  if (input.customerAccount.trim() === '') {
    problems.push('客户账户要填：授权交付方的适用范围至少收到一个客户账户');
  } else {
    account = queryEscape(input.customerAccount);
    if (account === null) problems.push('客户账户里有编不成 UTF-8 的字符');
  }
  let node: string | null = null;
  if (input.node !== '') {
    if (input.node.trim() === '') {
      problems.push('节点只填了空白：不收窄到节点就留空');
    } else {
      node = queryEscape(input.node);
      if (node === null) problems.push('节点里有编不成 UTF-8 的字符');
    }
  }
  if (account === null || problems.length > 0) return { ok: false, problems };
  return { ok: true, scope: node === null ? `customerAccount=${account}` : `customerAccount=${account}&node=${node}` };
}
