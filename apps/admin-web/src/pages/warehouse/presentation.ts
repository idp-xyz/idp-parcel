import type { CatalogueFamily } from './rows';

export const familyLabels: Record<CatalogueFamily, string> = {
  warehouse: '仓',
  zone: '库区',
  'location-type': '库位类型',
  location: '库位',
  packaging: '包材',
};

export const registrationTitles: Record<CatalogueFamily, string> = {
  warehouse: '登记仓',
  zone: '登记库区',
  'location-type': '登记库位类型',
  location: '登记库位',
  packaging: '登记包材',
};

const commandHint = '报文不要带租户和登记人。格式与命令行 parcel-warehouse-register 的同一册相同。修订从 1 起，改资料用下一个修订号，同一修订重投内容必须相同。';

export const registrationSnapshotHints: Record<CatalogueFamily, string> = {
  warehouse: `请粘贴一份 JSON。写明 warehouseCode、revision、name、kind（SELF_OPERATED 自营，AGENT 代理）、address.country 与 address.text。业务时区、同址网点、是否停用可以不填。${commandHint}`,
  zone: `请粘贴一份 JSON。写明 warehouseCode、zoneCode、revision、name、usage（RECEIVING_STAGING 收货暂存，STORAGE 存储，PICKING 拣货，DEFECTIVE 不良，RETURN_STAGING 退货暂存，OUTBOUND_STAGING 出库暂存）。是否停用可以不填。${commandHint}`,
  'location-type': `请粘贴一份 JSON。写明 locationTypeCode、revision、name。是否停用可以不填。本租户还没有仓时答未配置。${commandHint}`,
  location: `请粘贴一份 JSON。写明 warehouseCode、locationCode、revision、zoneCode。库位类型与优先级可以不填；优先级是整数。${commandHint}`,
  packaging: `请粘贴一份 JSON。写明 packagingMaterialCode、revision、name、dimensions（length、width、height、unit，数值用字符串）与 weight（value、unit，数值用字符串）。是否停用可以不填。本租户还没有仓时答未配置。${commandHint}`,
};

export const catalogueOutcomeLabels: Record<string, string> = {
  REGISTERED: '已登记',
  ALREADY_REGISTERED: '这一修订已经在册，内容相同，没有再写一次',
  CONTENT_CONFLICT: '这一修订已经在册，但内容不同，没有覆盖',
  NOT_ACCEPTED: '没有登记',
  NOT_CONFIGURED: '未配置：本租户还没有仓，这一册先不登记',
};

export const catalogueRefusalLabels: Record<string, string> = {
  REVISION_OUT_OF_SEQUENCE: '修订号不接在册上最新的下一号',
  IMMUTABLE_FIELD: '这一格登记后不能改',
  CONTENT_INVALID: '这一格的取值不成立',
  REFERENCE_NOT_FOUND: '指名的对象还没登记',
  REFERENCE_DEACTIVATED: '指名的对象已停用',
};

export const stockImportOutcomeLabels: Record<string, string> = {
  SUBMITTED: '已提交，等批准后才入账',
  EXISTING_RESULT: '这一份已经有结果，没有再写一次',
  SOURCE_CONFLICT: '同一标识已经提交过，内容不同，没有覆盖',
  WAREHOUSE_NOT_FOUND: '这个仓还没登记',
  WAREHOUSE_NOT_ACCEPTING: '这个仓不受理建账导入',
  CONTENT_INVALID: '内容不合，一行都没落',
  POSTED: '已批准并入账',
  NO_LONGER_VALID: '批准时内容已不再成立，没有入账',
  REJECTED: '已驳回，账没有改',
  STATUS_MISMATCH: '这一份已经不是待批，没有改',
  NOT_FOUND: '没有这一份导入',
  NOT_CONFIGURED: '未配置：批准职责规则还没登记',
  DUTY_BLOCKED: '提交人不能批准自己这一份',
};

export const stockImportSnapshotHints = {
  submit:
    '请粘贴一份 JSON。写明 importId、ownerAccount、warehouseCode、reason，以及 goodsLines 或 cartonLines 至少一组。数量要是正整数。报文不要带租户和提交人。提交不改账。',
  approve: '请粘贴一份 JSON，只写 importId。货主与内容取册上那一份，报文里不要再带。没登记批准职责时答未配置。提交人不能批自己的。',
  reject: '请粘贴一份 JSON。写明 importId 与 reason。驳回不改账，也不比批准职责。',
};

export function problemNote(code: string): string {
  switch (code) {
    case 'MALFORMED_REQUEST':
      return '报文读不成，未提交。';
    case 'NO_ANSWER_FORMED':
      return '系统暂时没能处理，结果还不确定；可稍后重试同一份。';
    default:
      return `这次没有成功（错误码 ${code}）。`;
  }
}
