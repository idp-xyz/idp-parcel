// 伴随读走到末页（票 catalogue-read-pagination-rollout/14）。一签的行要挂上另一本册的事实时（清关单的最新进展挂申报记录与海关结果、
// 申报记录挂海关结果、申报准备把就绪与授权两册按批次并排），被挂的那本册若没有能按本签这一页收窄的筛选维，只读它的第一页就会把
// 挂不上的那几行误报成「待申报」「尚无结果」「未登记」——这时一页一页读到末页再挂。零依赖、不引 React：run-tests 直接跑。

import type { ApiResult } from '../catalogue-api';
import type { CataloguePage } from '../catalogue-query';

/** 从第一页起带着上一页的游标往下读，各页的行接成一份；任一页没答出就交回那一页的答复，不拿半份数据冒充全册。 */
export async function everyPage<Body extends { page: CataloguePage }, Row>(
  load: (after: string | null) => Promise<ApiResult<Body>>,
  rowsOf: (body: Body) => readonly Row[],
): Promise<ApiResult<Row[]>> {
  const rows: Row[] = [];
  let after: string | null = null;
  let status = 200;
  do {
    const answer: ApiResult<Body> = await load(after);
    if (answer.kind !== 'outcome') return answer;
    rows.push(...rowsOf(answer.body));
    status = answer.status;
    after = answer.body.page.next;
  } while (after !== null);
  return { kind: 'outcome', status, body: rows };
}
