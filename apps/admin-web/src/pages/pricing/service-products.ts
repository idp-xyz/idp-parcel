// 售价卡的适用服务产品（ADR-0382 决定一、二）：价卡录入、列表、详情、导入预览与试算页共用的几句话与判法。
// 纯函数，node:test 直接钉；页面只摆。
//
// 产品写参与方与商业的服务产品对象标识，不带版本：产品换版不改它卖什么价。只有售价卡能声明；不声明的售价卡接其余产品，
// 即同一计价范围里没有别的在用卡指名的那些产品——这是声明之前每张卡的原义，所以「不选」是一个正当的答案，不是漏填。

import type { PickerOption } from '../party/PublicationFormFields';

/** 只有这个方向的价卡能声明适用服务产品；采购与内部价卡按别的维度选卡（ADR-0382 决定一）。 */
export const serviceProductDirection = 'SELL';

export const servedProductsLabel = '适用产品';

/** 售价卡不声明产品时那一格的话：不是「没有产品」，是接其余产品。 */
export const restProductsText = '其余产品（不指名）';

/** 编辑器里「适用服务产品」一格的说明。 */
export const restProductsHint = '不选即接其余产品：同一计价范围里没有别的卡指名的那些产品';

/** 规格口的模板版本还不收适用服务产品时，编辑器在这一格的位置只说这一句。 */
export const serviceProductsNotTakenNote = '当前模板版本不收适用服务产品；这张售价卡不指名产品，接其余产品。';

/** 试算不选产品时的说明：选卡只看不声明产品的那一层（ADR-0382 决定二）。 */
export const estimateWithoutProductNote = '只按不指名产品的卡试算';

/** 试算选了产品时的说明：先找指名它的卡，一张都没有才看不声明的卡。 */
export const estimateWithProductNote = '先用指名这个产品的卡，没有再用不指名的卡';

export function declaresServiceProducts(direction: string): boolean {
  return direction.trim() === serviceProductDirection;
}

/**
 * 列表、详情与导入预览上「适用产品」一格怎么念。不是售价卡答 null，页面不出这一格；售价卡没声明（键缺席或空数组）念成
 * 接其余产品，声明了就列出指名的产品。
 */
export function servedProductsText(direction: string, serviceProducts: readonly string[] | undefined): string | null {
  if (!declaresServiceProducts(direction)) return null;
  const named = (serviceProducts ?? []).filter((product) => product.trim() !== '');
  return named.length === 0 ? restProductsText : named.join('、');
}

/** 勾选或取消一个产品。同一个产品写两遍按一个算（ADR-0382 决定一），勾第二遍不重复记。 */
export function toggleServiceProduct(chosen: readonly string[], objectId: string, on: boolean): string[] {
  const id = objectId.trim();
  if (id === '') return [...chosen];
  if (!on) return chosen.filter((product) => product !== id);
  return chosen.includes(id) ? [...chosen] : [...chosen, id];
}

/**
 * 册读不到时按标识添加。输入框里的分号拆成几个标识各自添加：工作簿那一格本就以分号分隔，整串当一个标识收下，写出去就
 * 成了几个，与运营看到的那一项对不上。
 */
export function addTypedServiceProducts(chosen: readonly string[], typed: string): string[] {
  return typed.split(';').reduce<string[]>((current, part) => toggleServiceProduct(current, part, true), [...chosen]);
}

/** 写进工作簿与文档的那一份：去重、排序，同一组选择永远写成同一串，与服务端规范化时的去重排序同向。 */
export function canonicalServiceProducts(chosen: readonly string[]): string[] {
  return [...new Set(chosen.map((product) => product.trim()).filter((product) => product !== ''))].sort();
}

export interface ServiceProductChoice {
  value: string;
  label: string;
  checked: boolean;
  /** 在服务产品册上；不在的是之前选过或手填的，册上已经找不到。 */
  listed: boolean;
}

/**
 * 多选框的各项：册上的产品按册的先后，后面跟着已选却不在册上的那几个。后者照样列出、照样勾着，免得读面刷新把人选好的
 * 东西静默清掉——它在不在册由服务端判。
 */
export function serviceProductChoices(options: readonly PickerOption[], chosen: readonly string[]): ServiceProductChoice[] {
  const listed = options.map((option) => ({ value: option.value, label: option.label, checked: chosen.includes(option.value), listed: true }));
  const known = new Set(options.map((option) => option.value));
  const unlisted = chosen
    .filter((product) => !known.has(product))
    .map((product) => ({ value: product, label: `${product}（不在服务产品册上）`, checked: true, listed: false }));
  return [...listed, ...unlisted];
}
