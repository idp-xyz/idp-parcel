import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { SettingsOverviewBody, ToteListBody } from '../api';

export type ToteReader = (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<ToteListBody>>;
export type SettingsReader = () => Promise<ApiResult<SettingsOverviewBody>>;

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

/**
 * 租户给这个仓登记的「拣货必须用周转箱」此刻是不是要求。没登记是未配置，不要求；读不成也当不要求——
 * 真要求的话后端会拒不带箱的那一笔（TOTE_REQUIRED），工位从那一笔起改成必须用箱，不会把没带箱的货记进去。
 */
export async function warehouseRequiresTotes(warehouse: string, read: SettingsReader): Promise<boolean> {
  const answer = await read();
  if (answer.kind !== 'outcome') return false;
  const requirement = (answer.body.toteRequirements ?? []).find((item) => item.warehouseCode === warehouse);
  return requirement?.required === true;
}

/** 拣货工位开工后读一次：提示不提示扫箱、能不能点「不用箱」。 */
export async function warehouseToteMode(
  warehouse: string,
  totes: ToteReader,
  settings: SettingsReader,
): Promise<{ on: boolean; required: boolean }> {
  const [has, required] = await Promise.all([warehouseHasTotes(warehouse, totes), warehouseRequiresTotes(warehouse, settings)]);
  return { on: has || required, required };
}
