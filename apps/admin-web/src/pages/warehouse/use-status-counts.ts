import { useEffect, useState } from 'react';
import type { ApiResult } from '../catalogue-api';
import { catalogueConditionsQuery, type CatalogueConditions, type CatalogueFacets, type CataloguePage } from '../catalogue-query';

/**
 * 状态签上的数量：在别的条件（检索词、货主、仓……）不变的前提下，每个状态各有几张。
 *
 * 读口点了名的分面，一问就带着各值的行数（不含这一维自己的筛选）。没点名的册仍一个状态问一次、只取总数。
 *
 * 答不出的状态（这一问没成）是 null：签上不写数，不写成 0。分面里没有的词是 0。`base` 不含状态这一维；`load` 要是稳定引用。
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
    void (async () => {
      const once = await load(base, null);
      const facet = once.kind === 'outcome' ? (once.body as { facets?: CatalogueFacets }).facets?.[dimension] : undefined;
      if (facet) {
        const fromFacet: Record<string, number | null> = {};
        for (const value of values) {
          fromFacet[value] = facet.find((item) => item.value === value)?.count ?? 0;
        }
        if (!cancelled) setCounts(fromFacet);
        return;
      }
      const entries = await Promise.all(
        values.map(async (value): Promise<[string, number | null]> => {
          const answer = await load({ ...base, filters: { ...base.filters, [dimension]: [value] } }, null);
          return [value, answer.kind === 'outcome' ? answer.body.page.total : null];
        }),
      );
      if (!cancelled) setCounts(Object.fromEntries(entries));
    })();
    return () => {
      cancelled = true;
    };
    // base 与 values 每次渲染可以是新对象，认的是它们的签名。
  }, [load, signature, refresh]);

  return counts;
}
