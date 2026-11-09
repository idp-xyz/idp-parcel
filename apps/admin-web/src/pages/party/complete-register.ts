import type { CataloguePage } from '../catalogue-query';

// 列表页的计数与派生维筛选什么时候作数（票 business-party-ux/05 的类型指标格与状态胶囊计数；票 catalogue-read-pagination-rollout/14）。
//
// 读口按 ADR-0144 一页一页答，一页里数出来的不是册上的数，所以分页之后计数一般不摆。但租户的册常常一页就装得下：不带条件的
// 那一问已答完整册（没有下一页、到手的行数正等于总数）时，手上那份就是整册，按它数的就是真数——这时照接分页之前那套已被接受
// 的设计摆：指标格与胶囊带计数，类型、待查这些读口筛不了的派生维可点即筛。而且带条件的那一问是整册的子集、同样落在一页之内，
// 在它答回的那一页上做派生维筛选也就是精确的，不必标「只筛本页」。答不完整（多于一页，或读口不给总数）交 null：页面不显计数，
// 派生维只筛本页并写明——那时数出来、筛出来的都只是这一页。

/** 不带条件那一问的答复若已是整册，交出它的行；不是（或还没答出）交 null。 */
export function completeRegisterRows<Body extends { page: CataloguePage }, Row>(
  body: Body | null,
  rowsOf: (body: Body) => readonly Row[],
): readonly Row[] | null {
  if (body === null) return null;
  const rows = rowsOf(body);
  const { next, total } = body.page;
  return next === null && total !== null && rows.length === total ? rows : null;
}
