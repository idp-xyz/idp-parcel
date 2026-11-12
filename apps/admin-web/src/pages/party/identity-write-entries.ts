// 登记册上一个身份（往来单位、公司主体、客户账户）的几个写入口摆不摆。全部是纯函数，node:test 钉着。
//
// 这里只管呈现：入口不摆，不等于写口不判。各张表单照旧不裁任何门（identity-deactivation-form.ts 的头注）——从陈旧的页面交
// 上去的，由服务端答。收在这里的只有一种：登记册**必定**拒绝的那几笔，入口摆着只会让人填完一张必被拒的表。要看时点才知道
// 拒不拒的（给登过停用的单位登一段起始更早的合作关系、新增一个生效更早的客户账户）不在此列，入口照摆。
//
// 判据是「登过停用没有」，不是状态：行上带着停用时点即登过，不论那个时点到没到。到没到只决定状态是不是「已停用」；登记册
// 看的是最新一笔修订上有没有停用（后端 application 包的 deactivationReplay、checkRevisionSlot，以及往来单位资料登记那一道门
// RegisterBusinessPartyProfileHandler 的 checkSuccessor）。

/** 三本身份册的行都带这一格：登过停用才有。 */
type DeactivationCarrier = { deactivatedAt?: string };

function deactivationRegistered(row: DeactivationCarrier): boolean {
  return row.deactivatedAt !== undefined;
}

/**
 * 这个身份还登不登得了停用。登记册对同一个身份只落一笔停用：再交一笔，除了与册上那一笔一字不差的重放，都答「停用不重复落笔」；
 * 停用对话框预填的是下一个修订号，人从界面上交的正是被拒的那一种。还没生效的登记可以停用（那是撤下一笔还没生效的登记），
 * 所以不看状态，只看登过没有。往来单位、公司主体的「更多」与客户账户的「停用…」都照它。
 */
export function identityMayBeDeactivated(row: DeactivationCarrier): boolean {
  return !deactivationRegistered(row);
}

/**
 * 这个身份还收不收新的一笔修订——更正登记信息，以及往来单位新的一版资料（「更新资料」）。登过停用的都不再收——身份是
 * 稳定的，停用不是中场休息；已有的各笔修订、各版资料照旧看得到。与 identityMayBeDeactivated 此刻是同一个判据，分开写是因为
 * 它们照的是登记册的不同规则，哪一条改了只动自己这一个。
 */
export function identityTakesRevisions(row: DeactivationCarrier): boolean {
  return !deactivationRegistered(row);
}
