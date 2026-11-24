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

const commandHint = '租户与登记人由登录身份自动记下；修订从 1 起连续，改资料或停用用下一修订号。';

export const registrationSnapshotHints: Record<CatalogueFamily, string> = {
  warehouse: `逐格填写仓库基础档案。业务时区、同址物流节点可留空（未登记按未配置处理）。${commandHint}`,
  zone: `选择所属仓库与库区用途（收货暂存、存储、拣货、不良、退货暂存、出库暂存）。${commandHint}`,
  'location-type': `登记全租户共用的库位类型。本租户还没有仓时答未配置。${commandHint}`,
  location: `先选仓库再选所属库区（已按仓库级联过滤）；库位类型与拣选优先级可选。${commandHint}`,
  packaging: `登记全租户共用的包材尺寸与自重，供打包与预取面单引用。本租户还没有仓时答未配置。${commandHint}`,
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

// 退件到仓的结果词与盘点状态同名（COUNTED），不能走通用状态词表。
export const returnReceiptOutcomeLabels: Record<string, string> = {
  COUNTED: '已点收入暂存',
  UNCLAIMED: '认不出货主，转待认领',
};

export const stockImportSnapshotHints = {
  submit:
    '选择货主与仓库，填写原因并至少添加一组商品或整箱明细。提交本身不改库存账，批准后才真正入账。',
  approve: '确认建账导入号后批准入账；如尚未登记批准职责规则则答未配置，提交人受分离规则约束时不能自批。',
  reject: '确认建账导入号并填写驳回原因；驳回不改动库存账。',
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
