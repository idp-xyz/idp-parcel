// 操作者 / 客户端主体引用的显示短名。
//
// 线上引用形如「发行方#主体」，发行方是整段 URL、主体常是不透明长串；原样进表格会把列撑宽。
// 这里只管显：存储与线格式不动，全文由调用处放进 `title`。无 `#` 的引用（如渠道代号）原样显示，
// 不替它猜结构。

/** 主体部分超过此长度即截短。 */
export const PRINCIPAL_SHORT_MAX = 16;

export function shortPrincipalRef(ref: string): string {
  const at = ref.indexOf('#');
  if (at < 0) return ref;
  const subject = ref.slice(at + 1);
  // `#` 后为空时退回原串：显示空串比显示长串更糟。
  if (subject === '') return ref;
  const chars = Array.from(subject);
  return chars.length > PRINCIPAL_SHORT_MAX ? `${chars.slice(0, PRINCIPAL_SHORT_MAX).join('')}…` : subject;
}

// 嵌在文本里的「发行方#主体」片段：以 http(s):// 起头的发行方，到 `#`，再到空白或常见分隔符为止。
// 「授权类型@发行方#主体」这类复合串里，`标签@` 等其余文字原样保留。
const embeddedRefPattern = /https?:\/\/[^\s#]+#[^\s,;，；、()（）|｜]+/g;

/** 把文本里每个「发行方#主体」片段换成短名，其余文字不动；无片段原样。 */
export function shortenEmbeddedPrincipalRefs(text: string): string {
  return text.replace(embeddedRefPattern, (ref) => shortPrincipalRef(ref));
}
