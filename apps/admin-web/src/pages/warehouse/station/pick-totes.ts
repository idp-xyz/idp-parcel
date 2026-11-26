import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { ToteListBody } from '../api';

export type ToteReader = (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<ToteListBody>>;

/**
 * 这个仓有没有能用的周转箱（空着或在用的；已停用的不算）。有，拣货工位才提示扫箱。
 * 两种状态各问一次、只看有没有：读口的状态筛选一次只收一个词。任何一次没读成都当没有——
 * 提示扫箱是帮人把货分开，不是拣货成立的条件，读不到名册不该拦着拣。
 */
export async function warehouseHasTotes(warehouse: string, read: ToteReader): Promise<boolean> {
  for (const state of ['FREE', 'IN_USE']) {
    const answer = await read({ filters: { warehouseCode: [warehouse], state: [state] } }, null);
    if (answer.kind !== 'outcome') return false;
    const total = answer.body.page?.total;
    if (answer.body.totes.length > 0 || (typeof total === 'number' && total > 0)) return true;
  }
  return false;
}
