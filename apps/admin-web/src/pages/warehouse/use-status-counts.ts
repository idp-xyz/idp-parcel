import { useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import { catalogueConditionsQuery, type CatalogueConditions, type CataloguePage } from '../catalogue-query';

/**
 * 状态签上的数量：在别的条件（检索词、货主、仓……）不变的前提下，每个状态各有几张。
 *
 * 仓配的列表读口不答分面计数，只在 `page.total` 里答这一问的总数，所以这里一个状态问一次、只取总数。状态多的页会多发几问；
 * 读口声明了分面之后改成从一问的答复里取，调用方不用动。
 *
 * 答不出的状态（读口没给总数、这一问没成）是 null：签上不写数，不写成 0。`base` 不含状态这一维；`load` 要是稳定引用。
 * `refresh` 变一次重数一遍——列表重取、做完一个动作之后数量才跟得上。
 */
export function useStatusCounts<Body extends { page: CataloguePage }>(
  load: (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<Body>>,
  base: CatalogueConditions,
  dimension: string,
  values: readonly string[],
  refresh: number,
): Readonly<Record<string, number | null>> {
  const [counts, setCounts] = useState<Record<string, number | null>>({});
  const signature = `${catalogueConditionsQuery(base)}\n${dimension}\n${values.join(',')}`;

  useEffect(() => {
    let cancelled = false;
    void Promise.all(
      values.map(async (value): Promise<[string, number | null]> => {
        const answer = await load({ ...base, filters: { ...base.filters, [dimension]: [value] } }, null);
        return [value, answer.kind === 'outcome' ? answer.body.page.total : null];
      }),
    ).then((entries) => {
      if (!cancelled) setCounts(Object.fromEntries(entries));
    });
    return () => {
      cancelled = true;
    };
    // base 与 values 每次渲染可以是新对象，认的是它们的签名。
  }, [load, signature, refresh]);

  return counts;
}
