// 目录读口的请求半边（票 catalogue-read-pagination/05）：已迁到 ADR-0144 的读口收册子选择器、游标、排序、筛选维与 `q`，答复多一格
// `page`。参数名、答复形状与校验规则只在 ADR-0144 一处定义，这里只引决定号。零依赖、不引 React：run-tests 的 CommonJS 发射能直接加载。

/** 答复在各册既有集合键之外多出的一格（决定五）。 */
export interface CataloguePage {
  /** 本次使用的页大小，不是本页实际行数。 */
  size: number;
  /** 下一页的游标；null 即已到末页。 */
  next: string | null;
  /** 与本页同一组条件下的总数；null 即该读口不给，不显示、不去猜。 */
  total: number | null;
}

/**
 * 一问的条件（决定三、四）。游标不在其中：它只在造它的那组条件下成立，条件一变就作废（决定一）。键名照各册在读口里点名的维写——
 * 读口不认的键整个请求答 400（决定四），页面不发没点名的维。
 */
export interface CatalogueConditions {
  /** 册子选择器（如网络目录的 family）：保持原义、不兼作筛选维，但同样进游标摘要，换了它旧游标一样作废。 */
  readonly selectors?: Readonly<Record<string, string>>;
  /** 一次只按一维排，前缀 `-` 为倒序；缺席即该册的缺省序。 */
  readonly sort?: string;
  /** 维内为或、维间为与；空数组即这一维缺席。 */
  readonly filters?: Readonly<Record<string, readonly string[]>>;
  /** 检索词；去首尾空白，空即缺席。 */
  readonly q?: string;
}

const SORT_PARAM = 'sort';
const KEYWORD_PARAM = 'q';
const CURSOR_PARAM = 'after';

function conditionParams(conditions: CatalogueConditions): URLSearchParams {
  const params = new URLSearchParams();
  const selectors = conditions.selectors ?? {};
  for (const key of Object.keys(selectors).sort()) params.append(key, selectors[key]);
  if (conditions.sort !== undefined && conditions.sort !== '') params.append(SORT_PARAM, conditions.sort);
  const filters = conditions.filters ?? {};
  for (const key of Object.keys(filters).sort()) {
    for (const value of [...new Set(filters[key])].sort()) params.append(key, value);
  }
  const keyword = conditions.q?.trim() ?? '';
  if (keyword !== '') params.append(KEYWORD_PARAM, keyword);
  return params;
}

/**
 * 条件编成查询串（不带 `?`、不带游标）。同一组条件只有一种写法——键按名排、维内的值去重升序、检索词去首尾空白——所以它也是翻页轨迹的
 * 条件签名（cursor-pagination 的 cursorTrailFor）：请求与签名出自这一个函数，请求里带了哪一维，签名里就一定有哪一维，「换了条件还拿
 * 旧游标」的坏请求无从发出。
 */
export function catalogueConditionsQuery(conditions: CatalogueConditions): string {
  return conditionParams(conditions).toString();
}

/** 读口路径带上条件与游标；after 为 null 即第一页。 */
export function catalogueRequestPath(path: string, conditions: CatalogueConditions, after: string | null): string {
  const params = conditionParams(conditions);
  if (after !== null) params.append(CURSOR_PARAM, after);
  const query = params.toString();
  return query === '' ? path : `${path}?${query}`;
}

/**
 * 这组条件收不收窄。带检索词或筛选维的一问答出零行，说的是「这组条件下没有」，不是册为空；选择器只是选哪一份册、排序只改次序，都不收窄。
 */
export function catalogueConditionsNarrow(conditions: CatalogueConditions): boolean {
  if ((conditions.q?.trim() ?? '') !== '') return true;
  return Object.values(conditions.filters ?? {}).some((values) => values.length > 0);
}
