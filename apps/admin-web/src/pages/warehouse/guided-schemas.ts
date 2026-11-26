// 仓配六页全部 57 个写动作的逐字段引导式表单定义。
// 键名与 internal/warehousefulfillment/adapters/registrationjson、charge_mouth.go、warehouse_plan_mouth.go 一一对齐。

import type { GuidedFieldSpec, GuidedFormDefinition, GuidedSelectOption, WarehouseLookupKind } from './guided-forms';
import { partialShipmentOptions } from './partial-shipment';

const stockStateOptions: readonly GuidedSelectOption[] = [
  { value: 'GOOD', label: '良品（GOOD）' },
  { value: 'QUARANTINE', label: '待检（QUARANTINE）' },
  { value: 'DEFECTIVE', label: '次品（DEFECTIVE）' },
];

const zoneUsageOptions: readonly GuidedSelectOption[] = [
  { value: 'RECEIVING_STAGING', label: '收货暂存（RECEIVING_STAGING）' },
  { value: 'STORAGE', label: '存储（STORAGE）' },
  { value: 'PICKING', label: '拣货（PICKING）' },
  { value: 'DEFECTIVE', label: '不良（DEFECTIVE）' },
  { value: 'RETURN_STAGING', label: '退货暂存（RETURN_STAGING）' },
  { value: 'OUTBOUND_STAGING', label: '出库暂存（OUTBOUND_STAGING）' },
];

const warehouseKindOptions: readonly GuidedSelectOption[] = [
  { value: 'SELF_OPERATED', label: '自营仓（SELF_OPERATED）' },
  { value: 'AGENT', label: '代理仓（AGENT）' },
];

const arrivalModeOptions: readonly GuidedSelectOption[] = [
  { value: 'PARCEL', label: '包裹（PARCEL）' },
  { value: 'CARTON', label: '整箱（CARTON）' },
  { value: 'PALLET', label: '托盘（PALLET）' },
  { value: 'LOOSE', label: '散货（LOOSE）' },
];

const inboundModeOptions: readonly GuidedSelectOption[] = [
  { value: 'BY_GOODS', label: '按商品入库（BY_GOODS）' },
  { value: 'BY_CARTON', label: '按整箱入库（BY_CARTON）' },
];

const dispatchOptions: readonly GuidedSelectOption[] = [
  { value: 'PRODUCT_NETWORK', label: '本产品网络（PRODUCT_NETWORK）' },
  { value: 'OWNER_LABEL', label: '自备面单（OWNER_LABEL）' },
  { value: 'PICKUP', label: '自提（PICKUP）' },
];

const outboundLineFields: readonly GuidedFieldSpec[] = [
  { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
  { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
  { path: 'stockState', label: '库存状态', kind: 'select', required: true, options: stockStateOptions },
  { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '1' },
];

// 后端认的回库去向就这四个词（domain.NewReturnDestination）；写成别的词整份报文被当成坏载荷退回。
const returnDestinationOptions: readonly GuidedSelectOption[] = [
  { value: 'GOOD_PUTAWAY', label: '良品上架（GOOD_PUTAWAY）' },
  { value: 'DEFECTIVE', label: '转次品（DEFECTIVE）' },
  { value: 'DESTROY', label: '销毁（DESTROY）' },
  { value: 'RETURN_TO_OWNER', label: '退还货主（RETURN_TO_OWNER）' },
];

const returnLineFields: readonly GuidedFieldSpec[] = [
  { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
  { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
  { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '1' },
];

/** 1. 仓库与库位（五册登记） */
export const catalogueGuidedForms: Record<string, GuidedFormDefinition> = {
  warehouse: {
    fields: [
      { path: 'warehouseCode', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1', hint: '从 1 起连续，改资料或停用填下一号' },
      { path: 'name', label: '名称', kind: 'text', required: true, placeholder: '深圳一号仓' },
      { path: 'kind', label: '仓种类', kind: 'select', required: true, options: warehouseKindOptions, hint: '登记后不可改' },
      { path: 'address.country', label: '国家码', kind: 'text', required: true, placeholder: 'CN' },
      { path: 'address.text', label: '地址正文', kind: 'text', required: true, placeholder: '深圳市宝安区……' },
      { path: 'businessTimeZone', label: '业务时区', kind: 'text', placeholder: 'Asia/Shanghai', hint: '可选；不填即未登记' },
      { path: 'colocatedNode', label: '同址物流节点', kind: 'text', placeholder: 'SYN-NODE-01', hint: '可选' },
      { path: 'deactivated', label: '停用这个仓', kind: 'boolean', omitWhenFalse: true },
    ],
  },
  zone: {
    fields: [
      { path: 'warehouseCode', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'zoneCode', label: '库区编码', kind: 'text', required: true, placeholder: 'SYN-ZONE-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'name', label: '名称', kind: 'text', required: true, placeholder: '存储一区' },
      { path: 'usage', label: '用途', kind: 'select', required: true, options: zoneUsageOptions, hint: '登记后不可改' },
      { path: 'deactivated', label: '停用这个库区', kind: 'boolean', omitWhenFalse: true },
    ],
  },
  'location-type': {
    fields: [
      { path: 'locationTypeCode', label: '库位类型编码', kind: 'text', required: true, placeholder: 'SYN-LT-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'name', label: '名称', kind: 'text', required: true, placeholder: '标准货架位' },
      { path: 'deactivated', label: '停用这个类型', kind: 'boolean', omitWhenFalse: true },
    ],
  },
  location: {
    fields: [
      { path: 'warehouseCode', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'locationCode', label: '库位编码', kind: 'text', required: true, placeholder: 'SYN-LOC-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'zoneCode', label: '所属库区编码', kind: 'text', required: true, placeholder: 'SYN-ZONE-01', hint: '登记后不可改' },
      { path: 'locationTypeCode', label: '库位类型编码', kind: 'text', placeholder: 'SYN-LT-01', hint: '可选' },
      { path: 'priority', label: '优先级', kind: 'signed-integer', placeholder: '10', hint: '可选 32 位整数，越大越优先' },
      { path: 'deactivated', label: '停用这个库位', kind: 'boolean', omitWhenFalse: true },
    ],
  },
  packaging: {
    fields: [
      { path: 'packagingMaterialCode', label: '包材编码', kind: 'text', required: true, placeholder: 'SYN-BOX-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'name', label: '名称', kind: 'text', required: true, placeholder: '一号纸箱' },
      { path: 'dimensions.length', label: '长', kind: 'decimal', required: true, placeholder: '30' },
      { path: 'dimensions.width', label: '宽', kind: 'decimal', required: true, placeholder: '20' },
      { path: 'dimensions.height', label: '高', kind: 'decimal', required: true, placeholder: '15.5' },
      { path: 'dimensions.unit', label: '长度单位', kind: 'text', required: true, placeholder: 'CM' },
      { path: 'weight.value', label: '重量', kind: 'decimal', required: true, placeholder: '0.25' },
      { path: 'weight.unit', label: '重量单位', kind: 'text', required: true, placeholder: 'KG' },
      { path: 'deactivated', label: '停用这个包材', kind: 'boolean', omitWhenFalse: true },
    ],
  },
};

/** 2. 建账导入 */
export const stockImportGuidedForms: Record<string, GuidedFormDefinition> = {
  submit: {
    fields: [
      { path: 'importId', label: '导入号', kind: 'text', required: true, placeholder: 'SYN-IMP-01' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'warehouseCode', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'reason', label: '建账原因', kind: 'text', required: true, placeholder: '开仓期初建账' },
      {
        path: 'goodsLines',
        label: '商品行',
        rowLabel: '商品行',
        kind: 'rows',
        hint: '商品行与整箱行至少填一组；提交不改账，批准后才入账',
        rowFields: [
          { path: 'locationCode', label: '库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
          { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
          { path: 'unit', label: '单位', kind: 'text', required: true, placeholder: '件' },
          { path: 'stockState', label: '状态', kind: 'select', required: true, options: stockStateOptions },
          { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '10' },
          { path: 'batchNumber', label: '批号', kind: 'text', placeholder: '可选' },
          { path: 'expiryDate', label: '失效日', kind: 'date', placeholder: 'YYYY-MM-DD' },
          { path: 'labelAlias', label: '贴标别名', kind: 'text', placeholder: '可选' },
          { path: 'declaredInboundAt', label: '申报入库时刻', kind: 'datetime', placeholder: '可选 RFC 3339' },
        ],
      },
      {
        path: 'cartonLines',
        label: '整箱行',
        rowLabel: '整箱行',
        kind: 'rows',
        rowFields: [
          { path: 'locationCode', label: '库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
          { path: 'cartonNumber', label: '箱号', kind: 'text', required: true, placeholder: 'SYN-CARTON-01' },
          {
            path: 'contents',
            label: '箱内商品 JSON',
            kind: 'json',
            required: true,
            placeholder: '[{"goodsCode":"SYN-GOODS-01","quantity":5}]',
          },
          { path: 'declaredInboundAt', label: '申报入库时刻', kind: 'datetime', placeholder: '可选 RFC 3339' },
        ],
      },
    ],
    validate: (_values, payload) => {
      const hasGoods = Array.isArray(payload.goodsLines) && payload.goodsLines.length > 0;
      const hasCartons = Array.isArray(payload.cartonLines) && payload.cartonLines.length > 0;
      return hasGoods || hasCartons ? null : '商品行与整箱行请至少填写一组';
    },
  },
  approve: {
    fields: [
      { path: 'importId', label: '导入号', kind: 'text', required: true, placeholder: 'SYN-IMP-01', hint: '可从建账导入列表行直接带出' },
    ],
  },
  reject: {
    fields: [
      { path: 'importId', label: '导入号', kind: 'text', required: true, placeholder: 'SYN-IMP-01' },
      { path: 'reason', label: '驳回原因', kind: 'text', required: true, placeholder: '核对盘点数不一致' },
    ],
  },
};

/** 3. 收货与待认领 */
export const receivingGuidedForms: Record<string, GuidedFormDefinition> = {
  'pre-advice-on-behalf': {
    fields: [
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-IPA-01' },
      { path: 'evidenceReference', label: '依据引用', kind: 'text', required: true, placeholder: 'EVID-01' },
      { path: 'warehouseCode', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'arrivalMode', label: '到仓方式', kind: 'select', required: true, options: arrivalModeOptions },
      { path: 'inboundMode', label: '入库方式', kind: 'select', required: true, options: inboundModeOptions },
      { path: 'expectedAt', label: '预计到达时刻', kind: 'datetime', required: true },
      { path: 'arrivalClue', label: '到货线索', kind: 'text', placeholder: '可选运单或车牌' },
      { path: 'ownerReference', label: '货主参考号', kind: 'text', placeholder: '可选' },
      {
        path: 'goodsLines',
        label: '商品明细（按商品入库填）',
        rowLabel: '商品行',
        kind: 'rows',
        rowFields: [
          { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
          { path: 'unit', label: '单位', kind: 'text', required: true, placeholder: '件' },
          { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '10' },
          { path: 'batchNumber', label: '批号', kind: 'text', placeholder: '可选' },
          { path: 'expiryDate', label: '失效日', kind: 'date' },
          { path: 'cartonCount', label: '箱数', kind: 'integer', placeholder: '可选' },
        ],
      },
      {
        path: 'cartons',
        label: '整箱明细（按整箱入库填）',
        rowLabel: '整箱',
        kind: 'rows',
        rowFields: [
          { path: 'cartonNumber', label: '箱号', kind: 'text', required: true, placeholder: 'SYN-CARTON-01' },
          { path: 'lines', label: '箱内商品 JSON', kind: 'json', required: true, placeholder: '[{"goodsCode":"SYN-GOODS-01","quantity":5}]' },
        ],
      },
    ],
    validate: (_values, payload) => {
      if (payload.inboundMode === 'BY_GOODS' && (!Array.isArray(payload.goodsLines) || payload.goodsLines.length === 0)) {
        return '按商品入库时请至少填写一行商品明细';
      }
      if (payload.inboundMode === 'BY_CARTON' && (!Array.isArray(payload.cartons) || payload.cartons.length === 0)) {
        return '按整箱入库时请至少填写一箱明细';
      }
      return null;
    },
  },
  'pre-advice-notice': {
    fields: [
      { path: 'preAdviceId', label: '入库预报号', kind: 'text', required: true, placeholder: 'SYN-IPA-01' },
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-NOTICE-01' },
      { path: 'evidenceReference', label: '依据引用', kind: 'text', required: true, placeholder: 'EVID-01' },
      { path: 'expectedAt', label: '新预计到达时刻', kind: 'datetime', required: true },
      { path: 'arrivalClue', label: '到货线索', kind: 'text', placeholder: '留空即清空' },
      { path: 'ownerReference', label: '货主参考号', kind: 'text', placeholder: '留空即清空' },
    ],
  },
  'pre-advice-withdrawal': {
    fields: [
      { path: 'preAdviceId', label: '入库预报号', kind: 'text', required: true, placeholder: 'SYN-IPA-01' },
      { path: 'evidenceReference', label: '依据引用', kind: 'text', required: true, placeholder: 'EVID-01' },
    ],
  },
  'pre-advice-close': {
    fields: [
      { path: 'preAdviceId', label: '入库预报号', kind: 'text', required: true, placeholder: 'SYN-IPA-01' },
      { path: 'note', label: '完结说明', kind: 'text', placeholder: '可选' },
    ],
  },
  'discrepancy-conclusion': {
    fields: [
      { path: 'discrepancyId', label: '收货差异号', kind: 'text', required: true, placeholder: 'SYN-RD-01' },
      { path: 'conclusion', label: '差异结论', kind: 'text', required: true, placeholder: '货主确认按实收入账' },
      { path: 'evidenceReference', label: '依据引用', kind: 'text', required: true, placeholder: 'EVID-01' },
      { path: 'note', label: '备注', kind: 'text', placeholder: '可选' },
    ],
  },
  'unclaimed-claim': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-CLAIM-01' },
      { path: 'arrivalId', label: '待认领到货号', kind: 'text', required: true, placeholder: 'SYN-UCA-01' },
      { path: 'ownerAccount', label: '认领货主', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'preAdviceId', label: '对应入库预报号', kind: 'text', placeholder: '入库到货必填，退件留空' },
    ],
  },
  'unclaimed-revoke': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-REVOKE-01' },
      { path: 'arrivalId', label: '待认领到货号', kind: 'text', required: true, placeholder: 'SYN-UCA-01' },
      { path: 'reason', label: '撤销原因', kind: 'text', placeholder: '可选' },
    ],
  },
  'unclaimed-dispose': {
    fields: [
      { path: 'arrivalId', label: '待认领到货号', kind: 'text', required: true, placeholder: 'SYN-UCA-01' },
      { path: 'method', label: '处置方式', kind: 'text', required: true, placeholder: '按该仓处置规则中的方式词填写' },
      { path: 'note', label: '处置说明', kind: 'text', placeholder: '可选' },
    ],
  },
  'disposal-rule': {
    fields: [
      { path: 'warehouseCode', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'claimWindowSeconds', label: '认领期限（秒）', kind: 'integer', required: true, placeholder: '2592000', hint: '30 天 = 2592000 秒' },
      { path: 'disposalMethods', label: '可用处置方式', kind: 'string-list', required: true, placeholder: 'DESTROY, AUCTION', hint: '逗号或换行分隔' },
    ],
  },
  'goods-on-behalf': {
    fields: [
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-GOODS-01' },
      { path: 'evidenceReference', label: '依据引用', kind: 'text', required: true, placeholder: 'EVID-01' },
      { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
      { path: 'basisRevision', label: '基于的修订号', kind: 'integer', required: true, allowZero: true, placeholder: '0', hint: '首版填 0，改资料填当前修订号' },
      { path: 'name', label: '商品名称', kind: 'text', required: true, placeholder: '示例商品' },
      { path: 'baseUnit', label: '基本单位', kind: 'text', required: true, placeholder: '件' },
      { path: 'trackBatch', label: '跟踪批号', kind: 'boolean' },
      { path: 'trackExpiry', label: '跟踪效期', kind: 'boolean' },
      {
        path: 'packagingLevels',
        label: '包装层级（可选）',
        rowLabel: '层级',
        kind: 'rows',
        rowFields: [
          { path: 'name', label: '层级单位', kind: 'text', required: true, placeholder: '箱' },
          { path: 'factor', label: '折合基本单位数', kind: 'integer', required: true, placeholder: '12' },
        ],
      },
      { path: 'declaration.name', label: '申报品名（可选）', kind: 'text' },
      { path: 'declaration.hsCode', label: '税则号（可选）', kind: 'text' },
      { path: 'declaration.originCountry', label: '原产国（可选）', kind: 'text', placeholder: 'CN' },
      { path: 'declaration.unitPrice', label: '申报单价（可选）', kind: 'decimal', placeholder: '12.50' },
      { path: 'declaration.currency', label: '申报币种（可选）', kind: 'text', placeholder: 'USD' },
    ],
  },
  'return-pre-advice': {
    fields: [
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'basis', label: '依据引用', kind: 'text', required: true, placeholder: 'RET-BASIS-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'trackingNumber', label: '退件单号', kind: 'text', required: true, placeholder: 'TRK-RET-01' },
      { path: 'lines', label: '预计退回商品', rowLabel: '商品行', kind: 'rows', required: true, rowFields: returnLineFields },
    ],
  },
  'return-disposition': {
    fields: [
      { path: 'basis', label: '依据引用', kind: 'text', required: true, placeholder: 'RET-DISP-01' },
      { path: 'factId', label: '退货到仓事实号', kind: 'text', required: true, placeholder: 'SYN-FACT-RET-01' },
      {
        path: 'destination',
        label: '处置去向',
        kind: 'select',
        required: true,
        options: returnDestinationOptions,
      },
    ],
  },
};

/** 4. 库内作业与盘点 */
export const internalOpsGuidedForms: Record<string, GuidedFormDefinition> = {
  'condition-restore': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-RESTORE-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'locationCode', label: '所在库位', kind: 'text', required: true, placeholder: 'SYN-LOC-RCV', hint: '须在收货暂存或退货暂存' },
      { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
      { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
      { path: 'stockState', label: '当前库存状态', kind: 'select', required: true, options: stockStateOptions },
      { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '1' },
      { path: 'reason', label: '转良品原因', kind: 'text', required: true, placeholder: '复检合格' },
      { path: 'inboundBatchId', label: '指定入库批次', kind: 'text', placeholder: '可选' },
      { path: 'batchNumber', label: '批号', kind: 'text', placeholder: '可选' },
      { path: 'expiryDate', label: '失效日', kind: 'date' },
      { path: 'labelAlias', label: '贴标别名', kind: 'text', placeholder: '可选' },
    ],
  },
  'processing-instruction': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-PROC-01' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'goodsCode', label: '组合商品编码', kind: 'text', required: true, placeholder: 'SYN-KIT-01' },
      { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '1' },
      { path: 'reason', label: '发起原因', kind: 'text', required: true, placeholder: '按货主需求组装' },
      { path: 'disassemble', label: '这是拆分指令（不是组装）', kind: 'boolean', omitWhenFalse: true },
      { path: 'goodsRevision', label: '指定组成修订号', kind: 'integer', placeholder: '可选' },
      { path: 'basis', label: '依据引用', kind: 'text', placeholder: '可选' },
    ],
  },
  'processing-void': {
    fields: [
      { path: 'instructionId', label: '加工指令号', kind: 'text', required: true, placeholder: 'INS-01' },
      { path: 'reason', label: '作废原因', kind: 'text', required: true, placeholder: '货主取消' },
    ],
  },
  'relabel-instruction': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-REL-01' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
      { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
      { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '10' },
      { path: 'toAlias', label: '目标平台贴标', kind: 'text', required: true, placeholder: 'SYN-PLAT-01' },
      { path: 'fromAlias', label: '原贴标（可选）', kind: 'text', placeholder: '无贴标换标可留空' },
      { path: 'reason', label: '发起原因', kind: 'text', required: true, placeholder: '贴平台标' },
      { path: 'batchNumber', label: '限定批号', kind: 'text', placeholder: '可选' },
      { path: 'expiryDate', label: '限定失效日', kind: 'date' },
      { path: 'basis', label: '依据引用', kind: 'text', placeholder: '可选' },
    ],
  },
  'relabel-void': {
    fields: [
      { path: 'instructionId', label: '换标指令号', kind: 'text', required: true, placeholder: 'INS-01' },
      { path: 'reason', label: '作废原因', kind: 'text', required: true, placeholder: '不再需要换标' },
    ],
  },
  'stock-freeze': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-FRZ-01' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'reason', label: '冻结原因', kind: 'text', required: true, placeholder: '质量抽检冻结' },
      { path: 'cartonNumber', label: '整箱箱号（冻结整箱时只填此格）', kind: 'text', placeholder: '整箱冻结时填写，商品冻结留空' },
      { path: 'locationCode', label: '库位（商品冻结必填）', kind: 'text', placeholder: 'SYN-LOC-STORE' },
      { path: 'goodsCode', label: '商品编码（商品冻结必填）', kind: 'text', placeholder: 'SYN-GOODS-01' },
      { path: 'unit', label: '计量单位（商品冻结必填）', kind: 'text', placeholder: '件' },
      { path: 'stockState', label: '库存状态（商品冻结必填）', kind: 'select', options: stockStateOptions },
      { path: 'inboundBatchId', label: '入库批次（商品冻结必填）', kind: 'text', placeholder: 'IB-01' },
      { path: 'quantity', label: '冻结数量（商品冻结必填）', kind: 'integer', placeholder: '5' },
      { path: 'labelAlias', label: '贴标别名（可选）', kind: 'text' },
      { path: 'batchNumber', label: '批号（可选）', kind: 'text' },
      { path: 'expiryDate', label: '失效日（可选）', kind: 'date' },
    ],
    validate: (_values, payload) => {
      const hasCarton = typeof payload.cartonNumber === 'string' && payload.cartonNumber !== '';
      const hasCell =
        payload.locationCode !== undefined ||
        payload.goodsCode !== undefined ||
        payload.inboundBatchId !== undefined ||
        payload.quantity !== undefined;
      if (hasCarton && hasCell) return '整箱冻结不能同时填商品库位、入库批次或数量';
      if (!hasCarton && (!payload.locationCode || !payload.goodsCode || !payload.unit || !payload.stockState || !payload.inboundBatchId || !payload.quantity)) {
        return '商品冻结请填齐库位、商品编码、单位、库存状态、入库批次和数量（整箱冻结只填箱号）';
      }
      return null;
    },
  },
  'stock-unfreeze': {
    fields: [
      { path: 'freezeId', label: '冻结单号', kind: 'text', required: true, placeholder: 'FRZ-01' },
      { path: 'note', label: '解冻说明', kind: 'text', placeholder: '可选' },
    ],
  },
  'count-create': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-CNT-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'locations', label: '盘点库位列表', kind: 'string-list', required: true, placeholder: 'SYN-LOC-STORE, SYN-LOC-PICK', hint: '逗号或换行分隔' },
      { path: 'ownerAccount', label: '限定货主（可选）', kind: 'text' },
      { path: 'goods', label: '限定商品（可选）', kind: 'string-list', placeholder: '逗号或换行分隔' },
      { path: 'note', label: '备注', kind: 'text' },
    ],
  },
  'count-start': {
    fields: [{ path: 'countId', label: '盘点单号', kind: 'text', required: true, placeholder: 'CNT-01' }],
  },
  'count-complete': {
    fields: [{ path: 'countId', label: '盘点单号', kind: 'text', required: true, placeholder: 'CNT-01' }],
  },
  'count-void': {
    fields: [
      { path: 'countId', label: '盘点单号', kind: 'text', required: true, placeholder: 'CNT-01' },
      { path: 'reason', label: '作废原因', kind: 'text', placeholder: '可选' },
    ],
  },
  'count-approve': {
    fields: [
      { path: 'differenceId', label: '盘点差额号', kind: 'text', required: true, placeholder: 'DIS-01' },
      { path: 'note', label: '批准说明', kind: 'text', placeholder: '可选' },
    ],
  },
  'count-reject': {
    fields: [
      { path: 'differenceId', label: '盘点差额号', kind: 'text', required: true, placeholder: 'DIS-01' },
      { path: 'reason', label: '驳回原因', kind: 'text', required: true, placeholder: '需复盘核实' },
    ],
  },
  'job-discrepancy-approve': {
    fields: [
      { path: 'discrepancyId', label: '作业差异号', kind: 'text', required: true, placeholder: 'DIS-01' },
      { path: 'note', label: '批准说明', kind: 'text', placeholder: '可选' },
    ],
  },
  'job-discrepancy-reject': {
    fields: [
      { path: 'discrepancyId', label: '作业差异号', kind: 'text', required: true, placeholder: 'DIS-01' },
      { path: 'reason', label: '驳回原因', kind: 'text', required: true, placeholder: '差异不成立' },
    ],
  },
  'adjustment-submit': {
    fields: [
      { path: 'adjustmentId', label: '调整单号', kind: 'text', required: true, placeholder: 'ADJ-01' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'reason', label: '调整原因', kind: 'text', required: true, placeholder: '账实核对调整' },
      {
        path: 'lines',
        label: '调整明细行',
        rowLabel: '调整行',
        kind: 'rows',
        required: true,
        rowFields: [
          {
            path: 'kind',
            label: '调整种类',
            kind: 'select',
            required: true,
            options: [
              { value: 'INCREASE', label: '调增（INCREASE）' },
              { value: 'DECREASE', label: '调减（DECREASE）' },
              { value: 'DESTRUCTION', label: '销毁（DESTRUCTION）' },
            ],
          },
          { path: 'locationCode', label: '库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
          { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
          { path: 'unit', label: '单位', kind: 'text', required: true, placeholder: '件' },
          { path: 'stockState', label: '库存状态', kind: 'select', required: true, options: stockStateOptions },
          { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '1' },
          { path: 'inboundBatchId', label: '入库批次（可选）', kind: 'text' },
          { path: 'basis', label: '销毁依据（销毁必填）', kind: 'text' },
        ],
      },
    ],
  },
  'adjustment-approve': {
    fields: [
      { path: 'adjustmentId', label: '调整单号', kind: 'text', required: true, placeholder: 'ADJ-01' },
      { path: 'note', label: '批准说明', kind: 'text', placeholder: '可选' },
    ],
  },
  'adjustment-reject': {
    fields: [
      { path: 'adjustmentId', label: '调整单号', kind: 'text', required: true, placeholder: 'ADJ-01' },
      { path: 'reason', label: '驳回原因', kind: 'text', required: true, placeholder: '依据不足' },
    ],
  },
  'approval-duty': {
    fields: [
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'distinctSubjects', label: '批准人必须与提交人为不同主体', kind: 'boolean' },
    ],
  },
};

/** 5. 出库、波次与代理仓 */
export const outboundWaveGuidedForms: Record<string, GuidedFormDefinition> = {
  'outbound-request': {
    fields: [
      { path: 'decisionId', label: '决定标识', kind: 'text', required: true, placeholder: 'DEC-OB-01' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'basis', label: '代录依据', kind: 'text', required: true, placeholder: 'BASIS-OB-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'dispatch', label: '发运方式', kind: 'select', required: true, options: dispatchOptions },
      { path: 'service', label: '服务产品（本产品网络必填）', kind: 'text', placeholder: 'SYN-SERVICE' },
      { path: 'recipient', label: '收件人', kind: 'text', required: true, placeholder: '收件人姓名' },
      { path: 'address', label: '收件地址', kind: 'text', required: true, placeholder: '收件地址正文' },
      { path: 'platformStocking', label: '平台备货（打包齐全后按整票交一份委托）', kind: 'boolean', omitWhenFalse: true },
      {
        path: 'partialShipment',
        label: '拣不齐时怎么发',
        kind: 'select',
        options: [...partialShipmentOptions],
        hint: '货主对这一张单另有交代时才选；不选就按货主登记的默认，货主也没登记就是未配置',
      },
      { path: 'lines', label: '出库商品明细', rowLabel: '商品行', kind: 'rows', required: true, rowFields: outboundLineFields },
    ],
    validate: (_values, payload) => {
      if (payload.dispatch === 'PRODUCT_NETWORK' && !payload.service) {
        return '发运方式为本产品网络时请填写服务产品';
      }
      return null;
    },
  },
  'outbound-split': {
    fields: [
      { path: 'requestId', label: '原出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
      {
        path: 'parts',
        label: '拆分后的新请求',
        rowLabel: '拆分份',
        kind: 'rows',
        required: true,
        rowFields: [
          { path: 'requestKey', label: '新请求键', kind: 'text', required: true, placeholder: 'SPLIT-A' },
          {
            path: 'lines',
            label: '商品明细 JSON',
            kind: 'json',
            required: true,
            placeholder: '[{"goodsCode":"SYN-GOODS-01","unit":"件","stockState":"GOOD","quantity":2}]',
          },
        ],
      },
    ],
  },
  'outbound-cutoff': {
    fields: [
      { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
      { path: 'basis', label: '截单依据', kind: 'text', required: true, placeholder: 'CUTOFF-01' },
      { path: 'parcelSequence', label: '包裹序号（整单截单留空）', kind: 'integer', placeholder: '1' },
    ],
  },
  'owner-label': {
    fields: [
      { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
      { path: 'parcelSequence', label: '包裹序号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'externalNumber', label: '货主面单号', kind: 'text', required: true, placeholder: 'EXT-LBL-01' },
    ],
  },
  'wave-release': {
    fields: [
      { path: 'waveId', label: '波次号', kind: 'text', required: true, placeholder: 'WAVE-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'requests', label: '包含的出库请求号', kind: 'string-list', required: true, placeholder: 'OBR-01, OBR-02', hint: '逗号或换行分隔，可从列表行带出' },
      { path: 'prefetch', label: '预取估算委托（按包裹序号 1）', kind: 'boolean', omitWhenFalse: true },
      { path: 'packagingMaterial', label: '预取选用包材编码（可选）', kind: 'text', placeholder: 'SYN-BOX-01' },
    ],
  },
  'short-shipment': {
    fields: [
      {
        path: 'requestId',
        label: '出库请求号',
        kind: 'text',
        required: true,
        placeholder: 'OBR-01',
        hint: '拣到的发走，没拣到的不再发；提交之后撤不回',
      },
    ],
  },
  'shortage-split': {
    fields: [
      {
        path: 'requestId',
        label: '出库请求号',
        kind: 'text',
        required: true,
        placeholder: 'OBR-01',
        hint: '拣到的先发，没拣到的另成一张出库单继续等货；提交之后撤不回',
      },
    ],
  },
  'tote-register': {
    fields: [
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      {
        path: 'toteCodes',
        label: '箱号',
        kind: 'string-list',
        required: true,
        placeholder: 'TOTE-001, TOTE-002',
        hint: '逗号或换行分隔，一次最多 500 个。箱上贴的码是什么就填什么；拣货车上的格口也按箱登记',
      },
    ],
  },
  'tote-requirement': {
    fields: [
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1', hint: '从 1 起连续；要改就登下一号' },
      {
        path: 'required',
        label: '这个仓拣货必须用周转箱',
        kind: 'boolean',
        hint: '勾上是必须：不带箱号的拣货不成立。不勾是明说不要求。这个仓要先登记好周转箱',
      },
    ],
  },
  'tote-release': {
    fields: [
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      {
        path: 'toteCode',
        label: '箱号',
        kind: 'text',
        required: true,
        placeholder: 'TOTE-001',
        hint: '先确认箱里确实没有这张单的货了；放开之后别的出库单可以用这个箱',
      },
    ],
  },
  'tote-retire': {
    fields: [
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'toteCode', label: '箱号', kind: 'text', required: true, placeholder: 'TOTE-001' },
    ],
  },
  'wave-withdraw': {
    fields: [
      { path: 'waveId', label: '波次号', kind: 'text', required: true, placeholder: 'WAVE-01' },
      {
        path: 'requests',
        label: '要撤出的出库请求号',
        kind: 'string-list',
        required: true,
        placeholder: 'OBR-01, OBR-02',
        hint: '逗号或换行分隔。只撤还没拣过的单；有一张撤不了就一张都不撤',
      },
    ],
  },
  'allocation-strategy': {
    fields: [
      { path: 'revision', label: '采用修订号', kind: 'integer', required: true, placeholder: '1', hint: '采用内置参考分配策略，修订从 1 起连续' },
    ],
  },
  'shortage-policy': {
    fields: [
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      {
        path: 'kind',
        label: '受理时库存不够怎么办',
        kind: 'select',
        options: [
          { value: 'PARTIAL', label: '部分预占：有多少先占多少' },
          { value: 'REJECT', label: '整单拒单：不够就不受理' },
        ],
      },
      {
        path: 'partialShipment',
        label: '拣不齐时怎么发（这位货主的默认）',
        kind: 'select',
        options: [...partialShipmentOptions],
        hint: '单张出库单另有交代时以那一张为准',
      },
    ],
    // 每一版是完整的一份：这一版没选的那格，就是这位货主那一件没登记，不沿用上一版。
    validate: (_values, payload) => {
      if (!payload.kind && !payload.partialShipment) return '两件至少选一件；这一版没选的那件，就是这位货主那一件没登记';
      return null;
    },
  },
  'partial-shipment-consent': {
    fields: [
      { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
      { path: 'partialShipment', label: '货主同意这一张拣不齐时怎么发', kind: 'select', required: true, options: [...partialShipmentOptions] },
      {
        path: 'basis',
        label: '依据',
        kind: 'text',
        required: true,
        placeholder: '货主书面确认的编号或存档引用',
        hint: '同一张单同一个依据只记一次；货主改了主意，用新的依据再录一次，以最后录的为准',
      },
    ],
  },
  'confirmation-correction': {
    fields: [
      { path: 'factId', label: '原出库确认事实号', kind: 'text', required: true, placeholder: 'SYN-FACT-OUT-01' },
      { path: 'occurredAt', label: '更正后的发生时刻（改时点时填）', kind: 'datetime' },
      { path: 'invalidate', label: '判这一次出库确认不成立', kind: 'boolean', omitWhenFalse: true },
    ],
    validate: (_values, payload) => {
      const hasTime = typeof payload.occurredAt === 'string' && payload.occurredAt !== '';
      const inv = payload.invalidate === true;
      if (hasTime === inv) return '改时点与判不成立请二选一';
      return null;
    },
  },
  'confirmation-replenishment': {
    fields: [
      { path: 'factId', label: '已判不成立的确认事实号', kind: 'text', required: true, placeholder: 'SYN-FACT-OUT-01' },
    ],
  },
  'agent-connector': {
    fields: [
      { path: 'warehouse', label: '代理仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-AGENT' },
    ],
  },
  'agent-statement': {
    fields: [
      { path: 'source', label: '声明来源标识', kind: 'text', required: true, placeholder: 'AGENT-SRC-01' },
      { path: 'statedAt', label: '声明时刻', kind: 'datetime', required: true },
      { path: 'validUntil', label: '有效期至', kind: 'datetime', required: true },
      { path: 'warehouse', label: '代理仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-AGENT' },
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
      { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
      { path: 'quantity', label: '声明数量', kind: 'integer', required: true, allowZero: true, placeholder: '0' },
    ],
  },
  'agent-result': {
    fields: [
      { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
      {
        path: 'kind',
        label: '结果种类',
        kind: 'select',
        required: true,
        options: [
          { value: 'FORWARDED', label: '转出给代理仓（FORWARDED）' },
          { value: 'PACKED', label: '代理仓已打包（PACKED）' },
          { value: 'HANDED_OVER', label: '代理仓已交接（HANDED_OVER）' },
          { value: 'REJECTED', label: '代理仓拒单（REJECTED）' },
          { value: 'SHORT', label: '代理仓缺货（SHORT）' },
        ],
      },
      { path: 'resultId', label: '结果标识（非转出时必填）', kind: 'text', placeholder: 'AG-RES-01' },
      { path: 'weight', label: '重量（打包时填）', kind: 'decimal', placeholder: '1.5' },
      { path: 'weightUnit', label: '重量单位（打包时填）', kind: 'text', placeholder: 'KG' },
      { path: 'length', label: '长（打包时填）', kind: 'decimal', placeholder: '30' },
      { path: 'width', label: '宽（打包时填）', kind: 'decimal', placeholder: '20' },
      { path: 'height', label: '高（打包时填）', kind: 'decimal', placeholder: '10' },
      { path: 'dimension', label: '尺寸单位（打包时填）', kind: 'text', placeholder: 'CM' },
    ],
    validate: (_values, payload) => {
      if (payload.kind !== 'FORWARDED' && !payload.resultId) return '非转出结果请填写结果标识 resultId';
      return null;
    },
  },
};

/** 6. 仓储计价与收费 */
export const pricingGuidedForms: Record<string, GuidedFormDefinition> = {
  'warehouse-plan': {
    fields: [
      { path: 'id', label: '方案标识', kind: 'text', required: true, placeholder: 'SYN-PLAN-WH-01' },
      { path: 'version', label: '版本', kind: 'text', required: true, placeholder: '1' },
      { path: 'scope', label: '计价范围', kind: 'text', required: true, placeholder: 'SYN-SCOPE-WH' },
      { path: 'currency', label: '币种', kind: 'text', required: true, placeholder: 'CNY' },
      {
        path: 'declaredValue',
        label: '未测改用声明值口径',
        kind: 'select',
        required: true,
        options: [
          { value: 'REFUSE', label: '拒绝改用（REFUSE）' },
          { value: 'ALLOW', label: '允许改用（ALLOW）' },
        ],
      },
      { path: 'effectiveFrom', label: '生效时刻', kind: 'datetime', required: true },
      {
        path: 'groups',
        label: '收费组',
        rowLabel: '收费组',
        kind: 'rows',
        required: true,
        rowFields: [
          { path: 'name', label: '组名', kind: 'text', required: true, placeholder: 'INBOUND' },
          { path: 'kind', label: '发生项种类', kind: 'text', required: true, placeholder: 'INBOUND_HANDLING' },
          {
            path: 'level',
            label: '层级',
            kind: 'select',
            required: true,
            options: [
              { value: 'OCCURRENCE', label: '发生项级（OCCURRENCE）' },
              { value: 'LINE', label: '明细行级（LINE）' },
            ],
          },
          { path: 'line', label: '行种类（行级填）', kind: 'text', placeholder: 'INBOUND_BATCH' },
          {
            path: 'onMiss',
            label: '未命中处置',
            kind: 'select',
            required: true,
            options: [
              { value: 'PENDING', label: '待判断（PENDING）' },
              { value: 'ZERO', label: '计零（ZERO）' },
            ],
          },
        ],
      },
      {
        path: 'rules',
        label: '计费规则',
        rowLabel: '规则',
        kind: 'rows',
        required: true,
        rowFields: [
          { path: 'code', label: '费用代码', kind: 'text', required: true, placeholder: 'INBOUND_PIECE' },
          { path: 'kind', label: '发生项种类', kind: 'text', required: true, placeholder: 'INBOUND_HANDLING' },
          {
            path: 'level',
            label: '层级',
            kind: 'select',
            required: true,
            options: [
              { value: 'OCCURRENCE', label: '发生项级（OCCURRENCE）' },
              { value: 'LINE', label: '明细行级（LINE）' },
            ],
          },
          { path: 'line', label: '行种类（行级填）', kind: 'text', placeholder: 'INBOUND_BATCH' },
          { path: 'group', label: '所属收费组', kind: 'text', required: true, placeholder: 'INBOUND' },
          { path: 'priority', label: '优先级', kind: 'integer', required: true, placeholder: '1' },
          {
            path: 'method',
            label: '计收方法',
            kind: 'select',
            required: true,
            options: [
              { value: 'CHARGE', label: '计收（CHARGE）' },
              { value: 'WAIVE', label: '免收（WAIVE）' },
            ],
          },
          { path: 'unit', label: '计量单位', kind: 'text', placeholder: 'PIECE' },
          { path: 'unitQuantity', label: '每计价步长', kind: 'decimal', placeholder: '1' },
          { path: 'unitPriceMinor', label: '单价（最小货币单位整数）', kind: 'integer', allowZero: true, placeholder: '100' },
          { path: 'baseFeeMinor', label: '基费（可选整数）', kind: 'integer', allowZero: true, placeholder: '0' },
          { path: 'rounding', label: '取整口径', kind: 'text', placeholder: 'EXACT' },
          { path: 'measureClass', label: '计量类别（可选）', kind: 'text', placeholder: 'VOLUME' },
          { path: 'when', label: '适用条件 JSON（可选）', kind: 'json', placeholder: '[{"measureUnit":"DAY","operator":"GE","threshold":"30"}]' },
        ],
      },
    ],
  },
  'charge-adoption': {
    fields: [
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'revision', label: '修订号', kind: 'integer', required: true, placeholder: '1' },
      { path: 'effectiveAt', label: '生效时刻', kind: 'datetime', required: true },
      { path: 'account', label: '结算账户标识', kind: 'text', required: true, placeholder: 'SYN-ACCT-WH' },
      { path: 'pricingScope', label: '计价范围', kind: 'text', required: true, placeholder: 'SYN-SCOPE-WH' },
      { path: 'basis', label: '采用依据', kind: 'text', required: true, placeholder: 'SYN-BASIS' },
    ],
  },
  'charge-units': {
    fields: [
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'weightUnit', label: '重量单位', kind: 'text', required: true, placeholder: 'KG' },
      { path: 'volumeUnit', label: '体积单位', kind: 'text', required: true, placeholder: 'M3' },
    ],
  },
  'charge-aging': {
    fields: [
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      {
        path: 'basis',
        label: '库龄口径',
        kind: 'select',
        required: true,
        options: [
          { value: 'ACTUAL_BATCH', label: '按实批（ACTUAL_BATCH）' },
          { value: 'THEORETICAL_FIFO', label: '按理论先进先出（THEORETICAL_FIFO）' },
        ],
      },
    ],
  },
  'charge-project': {
    fields: [
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'name', label: '增值服务项目名', kind: 'text', required: true, placeholder: '礼盒包装' },
    ],
  },
  'storage-day-close': {
    fields: [
      { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
      { path: 'warehouse', label: '仓编码', kind: 'text', required: true, placeholder: 'SYN-WH-01' },
      { path: 'day', label: '计费自然日', kind: 'date', required: true },
    ],
  },
};

/** 为仓配表单字段自动补齐实时名册下拉（lookup）与可读单据号生成前缀（autoIdPrefix），新建主数据自身编码不强制下拉。 */
function inferLookup(formId: string, path: string): WarehouseLookupKind | undefined {
  if (path === 'ownerAccount') return 'owner';
  if (path === 'service') return 'service-product';
  if (path === 'packagingMaterial') return 'packaging';
  if (path === 'locationTypeCode' && formId !== 'location-type') return 'location-type';
  if (path === 'zoneCode' && formId !== 'zone') return 'zone';
  if (path === 'warehouseCode' || path === 'warehouse') {
    if (formId === 'warehouse') return undefined;
    if (formId === 'agent-connector' || formId === 'agent-statement') return 'agent-warehouse';
    return 'warehouse';
  }
  if (path === 'locationCode' || path === 'sourceLocation' || path === 'targetLocation' || path === 'location' || path === 'target') {
    if (formId === 'location') return undefined;
    return 'location';
  }
  return undefined;
}

function inferAutoIdPrefix(formId: string, path: string): string | undefined {
  if (path === 'decisionId') return 'DEC';
  if (path === 'importId' && formId === 'submit') return 'IMP';
  if (path === 'waveId' && formId === 'wave-release') return 'WAVE';
  if (path === 'adjustmentId' && formId === 'adjustment-submit') return 'ADJ';
  return undefined;
}

function enrichField(formId: string, field: GuidedFieldSpec): GuidedFieldSpec {
  const next: GuidedFieldSpec = {
    ...field,
    ...(field.lookup ? {} : inferLookup(formId, field.path) ? { lookup: inferLookup(formId, field.path) } : {}),
    ...(field.autoIdPrefix ? {} : inferAutoIdPrefix(formId, field.path) ? { autoIdPrefix: inferAutoIdPrefix(formId, field.path) } : {}),
  };
  if (next.rowFields) {
    next.rowFields = next.rowFields.map((sub) => enrichField(formId, sub));
  }
  return next;
}

function enrichFormMap(map: Record<string, GuidedFormDefinition>): Record<string, GuidedFormDefinition> {
  const out: Record<string, GuidedFormDefinition> = {};
  for (const [id, def] of Object.entries(map)) {
    out[id] = {
      ...def,
      fields: def.fields.map((field) => enrichField(id, field)),
    };
  }
  return out;
}

export const enrichedCatalogueForms = enrichFormMap(catalogueGuidedForms);
export const enrichedStockImportForms = enrichFormMap(stockImportGuidedForms);
export const enrichedReceivingForms = enrichFormMap(receivingGuidedForms);
export const enrichedInternalOpsForms = enrichFormMap(internalOpsGuidedForms);
export const enrichedOutboundWaveForms = enrichFormMap(outboundWaveGuidedForms);
export const enrichedPricingForms = enrichFormMap(pricingGuidedForms);

export const allWarehouseGuidedForms: Record<string, GuidedFormDefinition> = {
  ...enrichedCatalogueForms,
  ...enrichedStockImportForms,
  ...enrichedReceivingForms,
  ...enrichedInternalOpsForms,
  ...enrichedOutboundWaveForms,
  ...enrichedPricingForms,
};
