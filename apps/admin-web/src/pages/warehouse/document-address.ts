// 仓配单据的地址（纯 TypeScript，node:test 直接测）。
//
// 每张单据是 `#/<模块>/<单据键>`：壳层拿地址的前两段作标签，所以一张单据就是一张标签，能收藏、能并排开、能从别的单据链过来。
// 标签上、状态栏与「最近打开」里显示的字就是解码后的单据键，所以键要让人读得懂。
//
// 有的单据要几段才认得出：入库预报的读口要货主加预报标识；同一个模块下有几种单据时（盘点与调整）要先说是哪一种。
// 几段用「/」连成一个键，段里自带的「/」与「%」换成 %2F 与 %25。不用整段 encodeURIComponent：那样中文的种类词在标签上
// 会显示成一串百分号。键作为地址的一段写出去时再整体编码一次，由 documentHash 做。

function escapePart(part: string): string {
  return part.replace(/%/g, '%25').replace(/\//g, '%2F');
}

// 还原的先后与转义相反：先认 %2F 再认 %25，否则原文里的「%2F」（转义成 %252F）会被错认成斜杠。
function unescapePart(part: string): string {
  return part.replace(/%2F/g, '/').replace(/%25/g, '%');
}

/** 几段标识连成单据键。 */
export function documentKey(parts: readonly string[]): string {
  return parts.map(escapePart).join('/');
}

/** 单据键拆回各段。 */
export function documentKeyParts(key: string): string[] {
  return key.split('/').map(unescapePart);
}

/** 一张单据的地址。 */
export function documentHash(moduleId: string, parts: readonly string[]): string {
  return `#/${moduleId}/${encodeURIComponent(documentKey(parts))}`;
}
