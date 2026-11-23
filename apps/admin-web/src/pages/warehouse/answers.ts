import type { RegistrationResponseBody } from '../../components/registration';

/** 五册登记口的答复头。拒绝理由在 `refusal`，与登记面板认的 `refusalReason` 不是同一个键。 */
export interface CatalogueRegistrationBody {
  outcome: string;
  refusal?: string;
  field?: string;
  latestRevision?: number;
}

/** 建账导入写口的答复。不合之处与不受理理由不在登记面板的代数里，折进原因散文。 */
export interface StockImportWriteBody {
  outcome: string;
  reason?: string;
  issues?: ReadonlyArray<{ side: string; line: number; kind: string }>;
}

const issueKindLabels: Record<string, string> = {
  QUANTITY_NOT_POSITIVE: '数量不是正数',
  LOCATION_NOT_REGISTERED: '库位还没登记',
  LOCATION_DEACTIVATED: '库位已停用',
  ZONE_INCOMPATIBLE: '库区用途与库存状态不相容',
  GOODS_NOT_REGISTERED: '商品还没登记',
  UNIT_NOT_DECLARED: '这个单位不是该商品申报过的',
  BATCH_NUMBER_REQUIRED: '这个商品要批次号',
  BATCH_NUMBER_NOT_TRACKED: '这个商品不记批次号',
  EXPIRY_REQUIRED: '这个商品要失效日',
  EXPIRY_NOT_TRACKED: '这个商品不记失效日',
  LABEL_ALIAS_NOT_VALID: '标签别名不成立',
  DECLARED_INBOUND_AFTER_SUBMISSION: '申报入库时刻晚于提交',
  CARTON_CONTENTS_EMPTY: '箱里没有货',
  CARTON_NUMBER_REPEATED: '同一份里箱号重复',
  CARTON_NUMBER_ON_HAND: '这个箱号已经在账上',
  CARTON_GOODS_NOT_REGISTERED: '箱内商品还没登记',
  CARTON_QUANTITY_NOT_POSITIVE: '箱内数量不是正数',
};

const sideLabels: Record<string, string> = {
  GOODS: '商品行',
  CARTON: '整箱行',
};

const notAcceptingLabels: Record<string, string> = {
  DEACTIVATED: '这个仓已停用',
  AGENT: '代理仓不进自营库存账',
};

/**
 * 登记面板只认 `refusalReason` 与 `cause`。五册答复的拒绝理由与指出的格留在原键上，这里只为呈现搬一次，不改请求。
 */
export function asCatalogueRegistration(body: CatalogueRegistrationBody): RegistrationResponseBody {
  const parts: string[] = [];
  if (body.field) parts.push(`指出的格：${body.field}`);
  if (body.latestRevision !== undefined) parts.push(`册上最新修订：${body.latestRevision}`);
  return {
    outcome: body.outcome,
    ...(body.refusal ? { refusalReason: body.refusal } : {}),
    ...(parts.length > 0 ? { cause: parts.join('；') } : {}),
  };
}

/** 建账导入的不合之处与不受理理由写成登记面板能示出的原因。没收录的种类原样留下。 */
export function asStockImportWrite(body: StockImportWriteBody): RegistrationResponseBody {
  const parts: string[] = [];
  if (body.reason) parts.push(notAcceptingLabels[body.reason] ?? body.reason);
  for (const issue of body.issues ?? []) {
    const side = sideLabels[issue.side] ?? issue.side;
    const kind = issueKindLabels[issue.kind] ?? issue.kind;
    parts.push(`${side}第 ${issue.line} 行：${kind}`);
  }
  return {
    outcome: body.outcome,
    ...(parts.length > 0 ? { cause: parts.join('；') } : {}),
  };
}
