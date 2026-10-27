// 候选多时选单要能搜索（票 party-commercial-uiux/20）。
//
// 引用类的选单（责任法人、客户账户、服务产品、合同版本……）列的是册上有的东西，册一大，原生下拉就成了翻页找针。候选超过
// 这个数就换成带搜索框的选单，不超过的仍用原生下拉：几项的短单不值得多一层弹出，原生控件的键盘与读屏行为也最稳。
//
// 带搜索的选单（ui-primitives 的 Combobox）按条目的 `value` 串匹配，不看显示的名字。引用的 `value` 往往只是标识，名字在
// label 里，只按 value 搜就搜不到名字。所以这里给每个条目另起一个「搜索键」：label 与 value 拼在一起，两样都搜得到，也
// 保证键唯一、能从键找回真正的值。纯函数，node:test 直接跑。

/** 超过这个数的候选才换成带搜索的选单。 */
export const searchableThreshold = 10;

export function usesSearchList(optionCount: number): boolean {
  return optionCount > searchableThreshold;
}

export interface SearchItem {
  /** 交给选单当 value 的搜索键。 */
  key: string;
  /** 真正的引用值；「未选」是空串。 */
  value: string;
  label: string;
}

export const noneLabel = '未选';

/** 当前值不在候选里时那一项的显示：有括注就带上，没有就是值本身。 */
export function unknownLabelOf(value: string, note?: string): string {
  return note ? `${value}（${note}）` : value;
}

function keyOf(value: string, label: string): string {
  return `${label}  ${value}`;
}

/**
 * 选单的条目：「未选」在最前；当前值是手填的、候选里没有时保留为一项（读面刷新不静默清掉人填好的东西）；然后是候选。
 */
export function searchItemsOf(
  options: readonly { value: string; label: string }[],
  current: string,
  unknownNote?: string,
  unknownLabel?: string,
): SearchItem[] {
  const items: SearchItem[] = [{ key: noneLabel, value: '', label: noneLabel }];
  if (current !== '' && !options.some((option) => option.value === current)) {
    // 值本身不好读时（对象与版本合成的键）由调用方给整句显示。
    const label = unknownLabel ?? unknownLabelOf(current, unknownNote);
    items.push({ key: keyOf(current, label), value: current, label });
  }
  for (const option of options) items.push({ key: keyOf(option.value, option.label), value: option.value, label: option.label });
  return items;
}

/** 当前值对应的搜索键；找不到（理论上不会，当前值总在条目里）就落到「未选」。 */
export function currentSearchKey(items: readonly SearchItem[], current: string): string {
  return (items.find((item) => item.value === current) ?? items[0]).key;
}

/** 选单交回搜索键，找回真正的值；键不认识当作未选。 */
export function valueOfSearchKey(items: readonly SearchItem[], key: string): string {
  return items.find((item) => item.key === key)?.value ?? '';
}
