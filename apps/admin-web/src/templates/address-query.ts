// hash 地址的读写小件（票 admin-web-workspace-form/06）：检索词这类只活在地址里的页内状态放进 hash 的查询串
// （`#/<模块>[/<对象>]?q=…`，与保存视图的 `?view=` 同层），页面重挂（离开满新鲜期回来、刷新、收藏打开）时从地址读回；另有路径段的安全解码。
// 零依赖、不引 React：run-tests 的 CommonJS 发射能直接加载，shell/ 与 pages/ 下的纯逻辑也直接引本文件而不经 templates/index
// （那里带着 ESM-only 的 @idpxyz 原语）。读写地址的钩子在 use-address-keyword.ts。

/**
 * 解 hash 路径里的一段；畸形百分号序列（`%`、`%E0` 之类）答 null 而不是抛 URIError。地址来自地址栏，一条被截断的链接不该
 * 让读它的地方抛出去——读地址的地方有的在首帧，抛出去整台起不来，而坏地址还留在地址栏里，刷新也救不回来。
 */
export function decodeHashSegment(segment: string): string | null {
  try {
    return decodeURIComponent(segment);
  } catch {
    return null;
  }
}

/** 检索词在地址里的键名；与目录读口契约的关键词同名，读口支持之后原样下推。 */
export const ADDRESS_KEYWORD_PARAM = 'q';

/** 保存视图在地址里的键。筛选条件是其余的查询串，这一键只指名用的是哪一份。 */
export const SAVED_VIEW_PARAM = 'view';

/** 地址上除保存视图 id 以外的查询串，键按字典序。没有则空串。这一份就是列表页要存的筛选态。 */
export function addressQueryWithoutView(hash: string): string {
  const params = new URLSearchParams(splitHash(hash).query);
  params.delete(SAVED_VIEW_PARAM);
  params.sort();
  return params.toString();
}

/**
 * 打开一份保存视图时的地址：路径是模块页，查询串是存下的那一份，并带上视图 id。
 * 不沿用当前路径——人可能正站在「保存的视图」页上点这一行。
 */
export function hashForSavedView(moduleId: string, query: string, viewId: string): string {
  const params = new URLSearchParams(query);
  params.delete(SAVED_VIEW_PARAM);
  if (viewId !== '') params.set(SAVED_VIEW_PARAM, viewId);
  params.sort();
  const next = params.toString();
  const path = `#/${moduleId}`;
  return next === '' ? path : `${path}?${next}`;
}

function splitHash(hash: string): { path: string; query: string } {
  const at = hash.indexOf('?');
  return at < 0 ? { path: hash, query: '' } : { path: hash.slice(0, at), query: hash.slice(at + 1) };
}

/** 读 hash 查询串里 key 的值；没有这个键即空串。 */
export function hashQueryValue(hash: string, key: string): string {
  return new URLSearchParams(splitHash(hash).query).get(key) ?? '';
}

/** 把 hash 查询串里 key 设成 value（空串即删掉这个键），路径与别的键原样；返回新的 hash。 */
export function hashWithQueryValue(hash: string, key: string, value: string): string {
  const { path, query } = splitHash(hash);
  const params = new URLSearchParams(query);
  if (value === '') params.delete(key);
  else params.set(key, value);
  const next = params.toString();
  return next === '' ? path : `${path}?${next}`;
}
