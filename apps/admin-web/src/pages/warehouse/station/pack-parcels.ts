// 打包的包裹草稿（纯 TypeScript，node:test 直接测）：一张单装成几个包裹、每个包裹装什么、各自什么包材什么重量什么尺寸，
// 以及这些草稿装得出一笔什么样的打包事实。
//
// 到 Go 端核过的规则（application.WaveHandler.Pack、registrationjson.PackCommand、postgres.Waves.SavePack）：
//   - 一笔提交整张单：参数是 parcels 一个数组，一次说完，不分几次提交。
//   - 数组里一项就是一个包裹，**一项只带一种货**（商品编码、单位、数量各一格）；包裹序号在整笔里不许重复，所以一个包裹装不了两种货，
//     库里一个包裹也只存一行货。一张单有几种货，就至少要几个包裹。
//   - 所有项的数量合计必须等于这张单已拣的合计，否则答内容不合；Go 只对总数，不管各种货分别装了几件。这里更严一步：每种货
//     装进包裹的件数都要等于它已拣的件数——「全单的货都装进包裹才能提交」。
//   - 包材只当文本记下，不查册；重量与三边是文本，都不许空。后面本产品网络要提交委托时，数值要是不带符号的十进制数且大于 0
//     （parcelshipment 的 NewMeasurementValue），不合格的在打包落库之后才会在提交委托那一步失败，所以这里先按它拦住。
//   - 数值的写法再按旧作业台的表单收一道：整数最多 9 位、小数最多 6 位。单位是文本，不许空；不替租户定是 KG 还是别的。

import type { GuidedFormValues } from '../guided-forms';
import { goodsKey, type PackGoods, type PackOrder } from './pack-order';

/** 包材册里一个包材的当前修订里这个工位用得着的几格。 */
export interface PackagingMaterial {
  code: string;
  name: string;
  /** 包材自己登记的外廓与单位；打包时尺寸先取它。 */
  length: string;
  width: string;
  height: string;
  dimensionUnit: string;
  /** 包材自重的单位：只作为重量单位的一个提示，不代人填。 */
  weightUnit: string;
  deactivated: boolean;
}

export interface ParcelDraft {
  /** 本地标识：增删包裹时不随次序变，界面上拿它当 key，状态机里拿它认是哪个包裹。 */
  key: number;
  goodsCode: string;
  unit: string;
  /** 装几件，文本：人在输入框里改。 */
  quantity: string;
  /** 包材码；还没选是空串。 */
  packaging: string;
  packagingName: string;
  /** 登记的包材重量单位，选了包材才有：给人一个一点就填的提示，不自动填。 */
  packagingWeightUnit: string;
  weight: string;
  weightUnit: string;
  length: string;
  width: string;
  height: string;
  dimensionUnit: string;
  /** 尺寸是人自己改过的：之后换包材不再覆盖他改的。 */
  dimsEdited: boolean;
}

export const emptyParcel = (key: number, goods: Pick<PackGoods, 'goodsCode' | 'unit'>, quantity: number): ParcelDraft => ({
  key,
  goodsCode: goods.goodsCode,
  unit: goods.unit,
  quantity: quantity > 0 ? String(quantity) : '',
  packaging: '',
  packagingName: '',
  packagingWeightUnit: '',
  weight: '',
  weightUnit: '',
  length: '',
  width: '',
  height: '',
  dimensionUnit: '',
  dimsEdited: false,
});

// 旧作业台表单里十进制数的写法（components/guided-form 的 decimal）：整数最多 9 位、小数最多 6 位。
const decimalShape = /^\d{1,9}(\.\d{1,6})?$/;
const wholeShape = /^\d+$/;

/** 一个数值格（重量、长宽高）填得对不对：写法对且大于 0。 */
export function judgeMeasure(text: string): boolean {
  const trimmed = text.trim();
  return decimalShape.test(trimmed) && Number(trimmed) > 0;
}

/** 装几件：正整数；填得不对答 null。 */
export function parcelQuantity(parcel: Pick<ParcelDraft, 'quantity'>): number | null {
  const trimmed = parcel.quantity.trim();
  if (!wholeShape.test(trimmed)) return null;
  const value = Number.parseInt(trimmed, 10);
  return Number.isSafeInteger(value) && value >= 1 ? value : null;
}

/** 选了一个包材：记下码与名称；尺寸取包材的外廓，前提是它登记的外廓三边与单位都成立，且人没自己改过尺寸。没有就留着让人填。 */
export function withMaterial(parcel: ParcelDraft, material: PackagingMaterial): ParcelDraft {
  const next: ParcelDraft = {
    ...parcel,
    packaging: material.code,
    packagingName: material.name,
    packagingWeightUnit: material.weightUnit,
  };
  const usable =
    judgeMeasure(material.length) && judgeMeasure(material.width) && judgeMeasure(material.height) && material.dimensionUnit.trim() !== '';
  if (!usable || parcel.dimsEdited) return next;
  return {
    ...next,
    length: material.length,
    width: material.width,
    height: material.height,
    dimensionUnit: material.dimensionUnit,
  };
}

// —— 全单装得齐不齐 ——

export interface GoodsBalance {
  goodsCode: string;
  unit: string;
  /** 这种货该装的件数（已拣）。 */
  expected: number;
  /** 各包裹里写了几件。 */
  assigned: number;
}

export function packBalance(order: PackOrder, parcels: readonly ParcelDraft[]): GoodsBalance[] {
  const assigned = new Map<string, number>();
  for (const parcel of parcels) {
    const quantity = parcelQuantity(parcel);
    if (quantity === null) continue;
    const key = goodsKey(parcel);
    assigned.set(key, (assigned.get(key) ?? 0) + quantity);
  }
  return order.goods.map((goods) => ({ goodsCode: goods.goodsCode, unit: goods.unit, expected: goods.quantity, assigned: assigned.get(goodsKey(goods)) ?? 0 }));
}

/** 第一种还没装满的货，和还差几件；都装满了答 null。新加的包裹先装它。 */
export function firstUnpacked(order: PackOrder, parcels: readonly ParcelDraft[]): { goods: PackGoods; remaining: number } | null {
  const balance = packBalance(order, parcels);
  for (const [index, line] of balance.entries()) {
    if (line.assigned < line.expected) return { goods: order.goods[index], remaining: line.expected - line.assigned };
  }
  return null;
}

// —— 一个包裹填得对不对 ——

/** 包裹上的哪一格没填好；次序就是人填的次序：货与件数、包材、重量、尺寸。 */
export type ParcelField = 'goods' | 'quantity' | 'packaging' | 'weight' | 'weightUnit' | 'dimensions' | 'dimensionUnit';

export interface ParcelProblem {
  field: ParcelField;
  /** 整句，带句号，写明是哪个包裹。 */
  text: string;
}

/** 包裹第一处没填好的地方；填好了答 null。position 是包裹在单里的次序（从 1 数起）。 */
export function judgeParcel(order: PackOrder, parcel: ParcelDraft, position: number): ParcelProblem | null {
  const name = `包裹 ${position}`;
  if (!order.goods.some((goods) => goods.goodsCode === parcel.goodsCode && goods.unit === parcel.unit)) {
    return { field: 'goods', text: `${name}：选一种这张单里的货。` };
  }
  if (parcelQuantity(parcel) === null) return { field: 'quantity', text: `${name}：装几件要填正整数。` };
  if (parcel.packaging.trim() === '') return { field: 'packaging', text: `${name}：还没选包材，扫包材码或点选。` };
  if (parcel.weight.trim() === '') return { field: 'weight', text: `${name}：还没录重量。` };
  if (!judgeMeasure(parcel.weight)) return { field: 'weight', text: `${name}：重量要填大于 0 的数（整数最多 9 位、小数最多 6 位）。` };
  if (parcel.weightUnit.trim() === '') return { field: 'weightUnit', text: `${name}：重量单位还没填。` };
  if (parcel.length.trim() === '' || parcel.width.trim() === '' || parcel.height.trim() === '') {
    return { field: 'dimensions', text: `${name}：长、宽、高要填全。` };
  }
  if (!judgeMeasure(parcel.length) || !judgeMeasure(parcel.width) || !judgeMeasure(parcel.height)) {
    return { field: 'dimensions', text: `${name}：长、宽、高要填大于 0 的数（整数最多 9 位、小数最多 6 位）。` };
  }
  if (parcel.dimensionUnit.trim() === '') return { field: 'dimensionUnit', text: `${name}：尺寸单位还没填。` };
  return null;
}

export type PackJudgement =
  | { ok: true }
  | {
      ok: false;
      reason: string;
      /** 毛病在哪个包裹上（第几个、哪一格）；全单装不齐不属于哪一个包裹，没有。 */
      at?: { position: number; field: ParcelField };
    };

/** 能不能提交：至少一个包裹，每个都填好，每种货装进包裹的件数都等于已拣的件数。 */
export function judgePack(order: PackOrder, parcels: readonly ParcelDraft[]): PackJudgement {
  if (parcels.length === 0) return { ok: false, reason: '还没有包裹：点「再加一个包裹」。' };
  for (const [index, parcel] of parcels.entries()) {
    const problem = judgeParcel(order, parcel, index + 1);
    if (problem !== null) return { ok: false, reason: problem.text, at: { position: index + 1, field: problem.field } };
  }
  for (const line of packBalance(order, parcels)) {
    if (line.assigned < line.expected) {
      return { ok: false, reason: `${line.goodsCode} 还有 ${line.expected - line.assigned} 件没装进包裹：再加一个包裹，或把某个包裹的件数改大。` };
    }
    if (line.assigned > line.expected) {
      return { ok: false, reason: `${line.goodsCode} 装进包裹的比拣出来的多 ${line.assigned - line.expected} 件：把某个包裹的件数改小。` };
    }
  }
  return { ok: true };
}

/**
 * 打包要填的格：出库请求号与每个包裹一项（序号按次序从 1 数起）。数量按文本交，由旧作业台那条定义里的整数格去校验与转成数，
 * 事实号与仓由 work-fact-submit 按定义补，所以装出来的报文与旧作业台表单逐键相同（pack-parcels.test.ts 把两边钉在一起）。
 */
export function packFactValues(order: PackOrder, parcels: readonly ParcelDraft[]): GuidedFormValues {
  return {
    requestId: order.requestId,
    parcels: parcels.map((parcel, index) => ({
      sequence: String(index + 1),
      packagingMaterial: parcel.packaging.trim(),
      weight: parcel.weight.trim(),
      weightUnit: parcel.weightUnit.trim(),
      length: parcel.length.trim(),
      width: parcel.width.trim(),
      height: parcel.height.trim(),
      dimensionUnit: parcel.dimensionUnit.trim(),
      goodsCode: parcel.goodsCode,
      unit: parcel.unit,
      quantity: parcel.quantity.trim(),
    })),
  };
}

/** 复核要填的格：出库请求号与实点总数（按文本交，与旧作业台表单里敲进去的一样）。 */
export function checkFactValues(order: PackOrder, observed: number): GuidedFormValues {
  return { requestId: order.requestId, observed: String(observed) };
}
