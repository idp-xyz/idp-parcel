import type { DeclaredParcelDraft, ShipmentRequestDraft } from './api';
import { emptyGoodsLine, goodsDraftOfLines, validateGoodsLines, type GoodsLineForm } from './goods-form';
import { duplicatedAt, positiveDecimal, requestEffectiveAtOf } from './form-shapes';
import { packingDraftsOf, packingErrorsOf, type PackingMatrix } from './packing-form';

// 平台货件下单页的表单状态与组装(ADR-0374,票 declared-goods-lines/05)。平台货件(如亚马逊 FBA)就是一份普通委托——一个平台
// 货件一份委托、一箱一个包裹,同一个 /v1/shipment-requests;这一页只是按这个场景排表单:收件资料是平台仓,箱子成表,货物明细加
// 装箱矩阵。
//
// 不设 FBA 标志(决定三):平台货件号、箱标号要不要必填、装箱分配要不要报全,由服务产品的接单规则说,页面不拦。页面只拦服务端
// 必拒的形状——平台与平台货件号同在同缺、报了箱标号就要有平台货件、一票里箱标号与客户包裹号不重复、报了装箱就报全。

export interface FbaHeadForm {
  customerShipmentReference: string;
  requestedServiceProduct: string;
  requestEffectiveAt: string;
  senderRelation: string;
  senderAddress: string;
  platform: string;
  shipmentId: string;
  referenceId: string;
  /** 平台仓代码(如亚马逊的 ONT8),作委托的收件人;平台仓地址作收件地址。 */
  warehouseCode: string;
  warehouseAddress: string;
  destinationServiceScope: string;
  /** 一票货件的箱子用同一个重量单位与尺寸单位,填一次。 */
  weightUnit: string;
  dimensionsUnit: string;
}

export interface FbaBoxForm {
  customerParcelReference: string;
  platformBoxLabel: string;
  weight: string;
  length: string;
  width: string;
  height: string;
}

export interface FbaForm {
  head: FbaHeadForm;
  boxes: FbaBoxForm[];
  lines: GoodsLineForm[];
  contentCategory: string;
  contentCategoryNote: string;
  packing: PackingMatrix;
}

export const emptyFbaHead: FbaHeadForm = {
  customerShipmentReference: '',
  requestedServiceProduct: '',
  requestEffectiveAt: '',
  senderRelation: '',
  senderAddress: '',
  platform: '',
  shipmentId: '',
  referenceId: '',
  warehouseCode: '',
  warehouseAddress: '',
  destinationServiceScope: '',
  weightUnit: '',
  dimensionsUnit: '',
};

export const emptyFbaBox: FbaBoxForm = {
  customerParcelReference: '',
  platformBoxLabel: '',
  weight: '',
  length: '',
  width: '',
  height: '',
};

export const emptyFbaForm: FbaForm = {
  head: emptyFbaHead,
  boxes: [emptyFbaBox],
  lines: [emptyGoodsLine],
  contentCategory: '',
  contentCategoryNote: '',
  packing: [],
};

/**
 * 按序号填一列:第 i 箱(下标从 0)填 `前缀 + (起始 + i)`,序号补零到 width 位。只填空格——已经填了的是人照箱上抄的,不覆盖。
 * 前缀与起始号都是人给的,页面不猜平台的编号规则;填出来与手填的撞号时,提交前的重号核对会点出来。
 */
export function withSequence(
  boxes: FbaBoxForm[],
  field: 'customerParcelReference' | 'platformBoxLabel',
  prefix: string,
  start: number,
  width: number,
): FbaBoxForm[] {
  return boxes.map((box, index) =>
    box[field].trim() !== '' ? box : { ...box, [field]: `${prefix}${String(start + index).padStart(width, '0')}` },
  );
}

/** 「其余照第 1 箱」:第 1 箱的重量与尺寸填进其余箱的空格;同规格的箱子多,不用逐箱敲。已填的不动。 */
export function withMeasurementsFromFirstBox(boxes: FbaBoxForm[]): FbaBoxForm[] {
  if (boxes.length === 0) return boxes;
  const first = boxes[0];
  const fields = ['weight', 'length', 'width', 'height'] as const;
  return boxes.map((box, index) => {
    if (index === 0) return box;
    const next = { ...box };
    for (const field of fields) if (next[field].trim() === '') next[field] = first[field];
    return next;
  });
}

const required = '请填写';

/**
 * 只拦服务端必拒的形状与领域构造期就要在场的格。键:头部字段名;箱 `b{下标}.{字段}`;货物行 `g{下标}.{字段}`;装箱格
 * `k{箱}.{行}`;装箱整体 `packing`。timeZone 是读「客户要求的开始时间」用的显示时区。
 */
export function validateFba(form: FbaForm, timeZone?: string): Record<string, string> {
  const { head, boxes } = form;
  const errors: Record<string, string> = {};
  const requireText = (key: string, value: string) => {
    if (!value.trim()) errors[key] = required;
  };
  requireText('customerShipmentReference', head.customerShipmentReference);
  requireText('requestedServiceProduct', head.requestedServiceProduct);
  requireText('senderRelation', head.senderRelation);
  requireText('senderAddress', head.senderAddress);
  requireText('warehouseCode', head.warehouseCode);
  requireText('warehouseAddress', head.warehouseAddress);
  requireText('destinationServiceScope', head.destinationServiceScope);
  requireText('weightUnit', head.weightUnit);
  if (requestEffectiveAtOf(head.requestEffectiveAt, timeZone) === null) errors.requestEffectiveAt = '日期和时刻要填全';

  // 平台货件:平台与平台货件号同在同缺(服务端按这一对认货件),参考号只能随它们来。
  const platformPart = [head.platform, head.shipmentId, head.referenceId].some((value) => value.trim() !== '');
  const anyBoxLabel = boxes.some((box) => box.platformBoxLabel.trim() !== '');
  if (platformPart || anyBoxLabel) {
    const why = anyBoxLabel && !platformPart ? '报了箱标号就要填平台货件' : '平台与平台货件号要一起填';
    if (!head.platform.trim()) errors.platform = why;
    if (!head.shipmentId.trim()) errors.shipmentId = why;
  }

  if (boxes.length === 0) errors.boxes = '至少要有一箱';
  let anyDimensions = false;
  boxes.forEach((box, index) => {
    const key = (field: string) => `b${index}.${field}`;
    requireText(key('customerParcelReference'), box.customerParcelReference);
    if (!box.weight.trim()) errors[key('weight')] = required;
    else if (!positiveDecimal(box.weight.trim())) errors[key('weight')] = '大于 0 的数';
    const dims = [
      ['length', box.length],
      ['width', box.width],
      ['height', box.height],
    ] as const;
    const filled = dims.filter(([, value]) => value.trim() !== '');
    if (filled.length > 0) anyDimensions = true;
    if (filled.length > 0 && filled.length < dims.length) {
      for (const [field, value] of dims) if (!value.trim()) errors[key(field)] = '长宽高一起填';
    }
    for (const [field, value] of dims) {
      if (value.trim() && !positiveDecimal(value.trim())) errors[key(field)] = '大于 0 的数';
    }
  });
  if (anyDimensions) requireText('dimensionsUnit', head.dimensionsUnit);
  for (const index of duplicatedAt(boxes.map((box) => box.customerParcelReference))) errors[`b${index}.customerParcelReference`] = '重号';
  for (const index of duplicatedAt(boxes.map((box) => box.platformBoxLabel))) errors[`b${index}.platformBoxLabel`] = '重号';

  Object.assign(errors, validateGoodsLines(form.lines, form.contentCategory), packingErrorsOf(form.lines, form.packing, boxes.length));
  return errors;
}

/** 组装提交草案。各格两端空白去掉,别的原样;没填的可选格整键缺席,不送空串。先过 validateFba。 */
export function fbaDraftOf(form: FbaForm, timeZone?: string): ShipmentRequestDraft {
  const { head, boxes } = form;
  const optional = (value: string) => {
    const text = value.trim();
    return text === '' ? undefined : text;
  };
  const packing = packingDraftsOf(form.lines, form.packing, boxes.length);
  const parcels = boxes.map<DeclaredParcelDraft>((box, index) => {
    const parcel: DeclaredParcelDraft = {
      customerParcelReference: box.customerParcelReference.trim(),
      declaredWeightValue: box.weight.trim(),
      declaredWeightUnit: head.weightUnit.trim(),
    };
    if (box.length.trim() && box.width.trim() && box.height.trim()) {
      parcel.declaredLength = box.length.trim();
      parcel.declaredWidth = box.width.trim();
      parcel.declaredHeight = box.height.trim();
      parcel.declaredDimensionsUnit = head.dimensionsUnit.trim();
    }
    const label = optional(box.platformBoxLabel);
    if (label !== undefined) parcel.platformBoxLabel = label;
    if (packing) parcel.packing = packing[index];
    return parcel;
  });
  const draft: ShipmentRequestDraft = {
    customerShipmentReference: head.customerShipmentReference.trim(),
    requestedServiceProduct: head.requestedServiceProduct.trim(),
    requestEffectiveAt: requestEffectiveAtOf(head.requestEffectiveAt, timeZone) ?? undefined,
    senderRelation: head.senderRelation.trim(),
    senderAddress: head.senderAddress.trim(),
    recipientRelation: head.warehouseCode.trim(),
    recipientAddress: head.warehouseAddress.trim(),
    destinationServiceScope: head.destinationServiceScope.trim(),
    parcels,
    goods: goodsDraftOfLines(form.lines, form.contentCategory, form.contentCategoryNote),
  };
  const platform = optional(head.platform);
  const shipmentId = optional(head.shipmentId);
  if (platform !== undefined && shipmentId !== undefined) {
    draft.platformShipment = { platform, shipmentId };
    const referenceId = optional(head.referenceId);
    if (referenceId !== undefined) draft.platformShipment.referenceId = referenceId;
  }
  return draft;
}
