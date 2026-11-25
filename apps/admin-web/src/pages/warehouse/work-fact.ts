// 仓内作业台（模拟 PDA / 扫码客户端）纯逻辑与全部 18 个作业事实口的引导表单定义。
// 遵循 ADR-0149（人 + 已登记在用设备 + 该仓 WORK_FACT_REGISTRATION 授予）与 ADR-0023（端侧签发事实标识与单调序号）。
// 租户、操作者与设备绝不写进请求体，全部经 Bearer 令牌与 Parcel-Device-Id / Parcel-Work-Scope-* 请求头传递。

import type { GuidedFieldSpec, GuidedFormDefinition, GuidedFormValues, GuidedSelectOption } from './guided-forms';

export type WorkFactCategory = 'inbound' | 'internal' | 'count' | 'outbound';

export interface WorkFactSpec {
  id: string;
  category: WorkFactCategory;
  label: string;
  title: string;
  endpoint: string;
  /** 载荷里固定附带的字段（如收货确认的商品/整箱 shape），不在表单里让操作员错选。 */
  fixedPayload?: Record<string, unknown>;
  usesDeviceSequence: boolean;
  usesOccurredAt: boolean;
  snapshotHint: string;
  form: GuidedFormDefinition;
}

export const workFactCategories: ReadonlyArray<{ id: WorkFactCategory; label: string }> = [
  { id: 'inbound', label: '入库与退货收货' },
  { id: 'internal', label: '库内上架与加工' },
  { id: 'count', label: '盘点清点' },
  { id: 'outbound', label: '出库拣货打包确认' },
];

const stockStateOptions: readonly GuidedSelectOption[] = [
  { value: 'GOOD', label: '良品（GOOD）' },
  { value: 'QUARANTINE', label: '待检（QUARANTINE）' },
  { value: 'DEFECTIVE', label: '次品（DEFECTIVE）' },
];

const directionOptions: readonly GuidedSelectOption[] = [
  { value: 'OUT', label: '出库消耗（OUT）' },
  { value: 'IN', label: '产出入库（IN）' },
];

/**
 * 端侧按设备号生成单调递增序号与唯一事实标识（ADR-0023）。
 * 同一笔需要幂等重投时，操作员保留原 factId 与内容再点提交即可。
 */
export function mintWorkFactIdentity(
  deviceId: string,
  sequence: number,
  now: Date = new Date(),
): { factId: string; occurredAt: string; deviceSequence: string } {
  const pad = (n: number) => String(n).padStart(2, '0');
  const stamp =
    `${now.getUTCFullYear()}${pad(now.getUTCMonth() + 1)}${pad(now.getUTCDate())}` +
    `${pad(now.getUTCHours())}${pad(now.getUTCMinutes())}${pad(now.getUTCSeconds())}`;
  const cleanDevice = deviceId.replace(/[^A-Za-z0-9_-]/g, '') || 'PDA';
  return {
    factId: `PDA-${cleanDevice}-${stamp}-${sequence}`,
    occurredAt: now.toISOString().replace(/\.\d{3}Z$/, 'Z'),
    deviceSequence: String(sequence),
  };
}

/** 按选中的已登记仓库设备与当前端侧序号生成作业事实表单的预填值。 */
export function workFactPrefillFor(
  spec: WorkFactSpec,
  deviceId: string,
  warehouseCode: string,
  sequence: number,
  now: Date = new Date(),
): GuidedFormValues {
  const minted = mintWorkFactIdentity(deviceId, sequence, now);
  const values: GuidedFormValues = {
    factId: minted.factId,
    warehouse: warehouseCode,
  };
  if (spec.usesOccurredAt) values.occurredAt = minted.occurredAt;
  if (spec.usesDeviceSequence) values.deviceSequence = minted.deviceSequence;
  return values;
}

const commonHeaderFields = (usesOccurredAt: boolean, usesDeviceSequence: boolean): GuidedFieldSpec[] => [
  { path: 'factId', label: '作业事实标识（端侧自动签发，重放留原值）', kind: 'text', required: true },
  { path: 'warehouse', label: '作业仓库（须等于所选设备绑定仓）', kind: 'text', required: true },
  ...(usesOccurredAt ? [{ path: 'occurredAt', label: '发生时刻', kind: 'datetime' as const, required: true }] : []),
  ...(usesDeviceSequence
    ? [{ path: 'deviceSequence', label: '设备序号（本机单调递增）', kind: 'integer' as const, required: true }]
    : []),
];

const baseInternalLineFields: readonly GuidedFieldSpec[] = [
  { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, lookup: 'owner', placeholder: 'SYN-OWNER-01' },
  { path: 'goodsCode', label: '商品编码（整箱留空）', kind: 'text', placeholder: 'SYN-GOODS-01' },
  { path: 'unit', label: '计量单位（整箱留空）', kind: 'text', placeholder: '件' },
  { path: 'stockState', label: '库存状态（整箱留空）', kind: 'select', options: stockStateOptions },
  { path: 'quantity', label: '数量（整箱留空）', kind: 'integer', placeholder: '1' },
  { path: 'sourceLocation', label: '源库位', kind: 'text', required: true, lookup: 'location', placeholder: 'SYN-LOC-RCV' },
  { path: 'targetLocation', label: '目标库位', kind: 'text', required: true, lookup: 'location', placeholder: 'SYN-LOC-STORE' },
  { path: 'cartonNumber', label: '整箱箱号（移动整箱填）', kind: 'text', placeholder: '可选' },
  { path: 'toUnit', label: '转入单位（顺带折合可选）', kind: 'text', placeholder: '可选' },
  { path: 'toQuantity', label: '转入数量（可选）', kind: 'integer', placeholder: '可选' },
  { path: 'inboundBatchId', label: '指定入库批次（可选）', kind: 'text' },
  { path: 'batchNumber', label: '批号（可选）', kind: 'text' },
  { path: 'expiryDate', label: '失效日（可选）', kind: 'date' },
  { path: 'labelAlias', label: '贴标别名（可选）', kind: 'text' },
  { path: 'note', label: '备注（可选）', kind: 'text' },
];

export const workFactOutcomeLabels: Record<string, string> = {
  CONFIRMED: '已确认并入账',
  CORRECTED: '已更正出库确认的时点',
  DIVERTED_TO_UNCLAIMED: '预报已取消或完结，已自动转待认领到货（未入账）',
  RECORDED: '已记录',
  REGISTERED: '已登记并入账',
  SHORTAGE_REGISTERED: '已登记短少观察（待批准调减）',
  POSTED: '已入账',
  PENDING: '账上库存不足，指令留在待处理',
  PICKED: '已拣货移入待出库',
  CHECKED: '复核通过，可打包',
  CHECK_MISMATCH: '复核数量与已拣不符：这一次复核已记下，出库单仍停在已拣，不能打包',
  PACKED: '已打包',
  SUBMITTED: '已打包并提交本产品网络委托',
  OWNERSHIP_UNRESOLVED: '打包已记下，但没提交本产品网络委托：这位货主的生产归属还没确定',
  ADMISSION_PAUSED: '打包已记下，但没提交本产品网络委托：这位货主所在范围的准入暂停着',
  OTHER_PRODUCTION_AUTHORITY: '打包已记下，但没提交本产品网络委托：这位货主的生产由别的权威方承接',
  INPUT_NOT_ACCEPTED: '打包已记下，但没提交本产品网络委托：委托的标识或内容没有被受理',
  WITHHELD: '放行条件还没满足，这一笔没有确认出库；条件备齐后再确认，货已经交出去的勾选「离线已交出」照记',
  COUNTED: '退货已入账',
  UNCLAIMED: '未认出货主，已转待认领到货',
  EXISTING_RESULT: '同一事实标识已记录且内容相同（重放）',
  CONTENT_CONFLICT: '同一事实标识已存在不同内容，未覆盖',
  CONTENT_INVALID: '内容不合，一行都没落',
  NOT_ACCEPTED: '未受理，请检查标出的字段',
  NOT_CONFIGURED: '未配置：所需规则、代办授予或凭证还没登记',
  WAREHOUSE_NOT_FOUND: '这个仓还没登记',
  WAREHOUSE_NOT_ACCEPTING: '这个仓不受理此作业（如代理仓）',
  PRE_ADVICE_NOT_FOUND: '找不到指定的入库预报',
  PRE_ADVICE_MISMATCH: '仓或入库形态与预报不符',
  CARTON_NOT_FOUND: '整箱账上找不到这只箱',
  UNAVAILABLE: '库存不足或被占用，未入账',
  STATUS_MISMATCH: '当前单据状态不允许此作业',
  WAVE_NOT_FOUND: '找不到对应的任务或出库请求',
};

export function workFactProblemNote(code: string): string {
  switch (code) {
    case 'DEVICE_NOT_IN_USE':
      return '所选设备号不在本租户设备册、已停用，或未绑定到当前仓库。请先在「扫描设备」确认在用设备。';
    case 'OUTSIDE_ADMISSION_SCOPE':
      return '该仓或该类作业事实不在治理登记的生产权威区间内，未落库。';
    case 'WAREHOUSE_FULFILLMENT_NOT_ENABLED':
      return '本部署尚未启用仓配履约开关（IDP_PARCEL_WAREHOUSE_FULFILLMENT_ENABLED）。';
    case 'MALFORMED_REQUEST':
      return '报文读不成或带了不允许的字段（设备号与操作者走请求头，不能放正文），未提交。';
    case 'NO_ANSWER_FORMED':
      return '系统暂时没能处理，结果还不确定；请保持同一事实标识重试。';
    default:
      return `这次没有成功（错误码 ${code}）。`;
  }
}

export const workFactSpecs: readonly WorkFactSpec[] = [
  // —— 一、入库与退货收货 ——
  {
    id: 'receipt-goods',
    category: 'inbound',
    label: '商品收货确认',
    title: '按商品收货确认入账',
    endpoint: '/warehouse-fulfillment-receipt-confirmations',
    fixedPayload: { shape: 'BY_GOODS' },
    usesDeviceSequence: false,
    usesOccurredAt: true,
    snapshotHint: '扫描入库预报与实收商品条码并选定收货暂存库位（未停用）。确认即入账，自动形成多收/错货/破损差异。',
    form: {
      fields: [
        ...commonHeaderFields(true, false),
        { path: 'preAdviceId', label: '入库预报号', kind: 'text', required: true, placeholder: 'SYN-IPA-01' },
        { path: 'unclaimedArrivalId', label: '引用已认领待认领号（可选）', kind: 'text', placeholder: 'SYN-UCA-01' },
        {
          path: 'lines',
          label: '实收商品明细',
          rowLabel: '收货行',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'observed', label: '扫描码 / 商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
            { path: 'stockState', label: '收货状态', kind: 'select', required: true, options: stockStateOptions },
            { path: 'quantity', label: '实收数量', kind: 'integer', required: true, placeholder: '10' },
            { path: 'locationCode', label: '收货暂存库位', kind: 'text', required: true, lookup: 'receiving-location', placeholder: 'SYN-LOC-RCV' },
            { path: 'batchNumber', label: '批号（跟踪批号必填）', kind: 'text' },
            { path: 'expiryDate', label: '失效日（跟踪效期必填）', kind: 'date' },
            { path: 'labelAlias', label: '贴标别名（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },
  {
    id: 'receipt-carton',
    category: 'inbound',
    label: '整箱收货确认',
    title: '按整箱收货确认入账',
    endpoint: '/warehouse-fulfillment-receipt-confirmations',
    fixedPayload: { shape: 'BY_CARTON' },
    usesDeviceSequence: false,
    usesOccurredAt: true,
    snapshotHint: '扫描整箱箱号并选定在用收货暂存库位，入整箱库存账。',
    form: {
      fields: [
        ...commonHeaderFields(true, false),
        { path: 'preAdviceId', label: '入库预报号', kind: 'text', required: true, placeholder: 'SYN-IPA-01' },
        { path: 'unclaimedArrivalId', label: '引用已认领待认领号（可选）', kind: 'text' },
        {
          path: 'lines',
          label: '实收整箱明细',
          rowLabel: '整箱',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'cartonNumber', label: '预报箱号', kind: 'text', required: true, placeholder: 'SYN-CARTON-01' },
            { path: 'locationCode', label: '收货暂存库位', kind: 'text', required: true, lookup: 'receiving-location', placeholder: 'SYN-LOC-RCV' },
            { path: 'warehouseCartonNumber', label: '仓贴箱号（可选）', kind: 'text' },
            { path: 'damaged', label: '外箱破损', kind: 'boolean', omitWhenFalse: true },
          ],
        },
      ],
    },
  },
  {
    id: 'warehouse-measurement',
    category: 'inbound',
    label: '仓测尺寸重量',
    title: '登记仓测尺寸与重量',
    endpoint: '/warehouse-fulfillment-warehouse-measurements',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '记录商品某一包装层级或在库一箱的实测长宽高与重量。数值按十进制字符串送出。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        {
          path: 'targetKind',
          label: '测量对象种类',
          kind: 'select',
          required: true,
          options: [
            { value: 'GOODS_LEVEL', label: '商品包装层级（GOODS_LEVEL）' },
            { value: 'CARTON', label: '在库整箱（CARTON）' },
          ],
        },
        { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
        { path: 'goodsCode', label: '商品编码（商品层级必填）', kind: 'text', placeholder: 'SYN-GOODS-01' },
        { path: 'packagingLevel', label: '包装层级（商品层级必填）', kind: 'text', placeholder: '件' },
        { path: 'cartonNumber', label: '箱号（在库整箱必填）', kind: 'text', placeholder: 'SYN-CARTON-01' },
        { path: 'dimensions.length', label: '长', kind: 'decimal', required: true, placeholder: '20' },
        { path: 'dimensions.width', label: '宽', kind: 'decimal', required: true, placeholder: '15' },
        { path: 'dimensions.height', label: '高', kind: 'decimal', required: true, placeholder: '10' },
        { path: 'dimensions.unit', label: '尺寸单位', kind: 'text', required: true, placeholder: 'CM' },
        { path: 'weight.value', label: '重量', kind: 'decimal', required: true, placeholder: '1.25' },
        { path: 'weight.unit', label: '重量单位', kind: 'text', required: true, placeholder: 'KG' },
      ],
      validate: (_values, payload) => {
        if (payload.targetKind === 'GOODS_LEVEL' && (!payload.goodsCode || !payload.packagingLevel)) {
          return '测量商品包装层级时请填商品编码与包装层级';
        }
        if (payload.targetKind === 'CARTON' && !payload.cartonNumber) {
          return '测量在库整箱时请填箱号';
        }
        return null;
      },
    },
  },
  {
    id: 'unclaimed-arrival',
    category: 'inbound',
    label: '待认领到货登记',
    title: '一线登记无主/待认领到货',
    endpoint: '/warehouse-fulfillment-unclaimed-arrival-registrations',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '没有对应预报或认不出货主的到货先在此记下观察与存放处，不入库存账。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        {
          path: 'kind',
          label: '到货种类',
          kind: 'select',
          required: true,
          options: [
            { value: 'INBOUND', label: '入库到货（INBOUND）' },
            { value: 'RETURN', label: '退件到货（RETURN）' },
          ],
        },
        { path: 'observation', label: '现场观察描述', kind: 'text', required: true, placeholder: '无箱标纸箱 2 箱，外箱字迹模糊' },
        { path: 'storageLocation', label: '暂存位置（可选）', kind: 'text', placeholder: 'SYN-LOC-RCV' },
        { path: 'ownerClue', label: '货主线索（可选）', kind: 'text' },
        { path: 'preAdviceClue', label: '预报线索（可选）', kind: 'text' },
      ],
    },
  },
  {
    id: 'return-receipt',
    category: 'inbound',
    label: '退货到仓入库',
    title: '退件到仓扫描收货',
    endpoint: '/warehouse-fulfillment-return-receipts',
    usesDeviceSequence: false,
    usesOccurredAt: true,
    snapshotHint: '扫描退件单号入在用退货暂存库位；认不出货主时自动转待认领到货，认领后可带 arrivalId 再入账。',
    form: {
      fields: [
        ...commonHeaderFields(true, false),
        { path: 'location', label: '退货暂存库位', kind: 'text', required: true, lookup: 'return-staging-location', placeholder: 'SYN-LOC-RET' },
        { path: 'trackingNumber', label: '退件单号 / 物流单号', kind: 'text', required: true, placeholder: 'TRK-RET-01' },
        { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
        { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
        { path: 'stockState', label: '库存状态', kind: 'select', required: true, options: stockStateOptions },
        { path: 'quantity', label: '实收数量', kind: 'integer', required: true, placeholder: '1' },
        { path: 'arrivalId', label: '已认领待认领号（可选）', kind: 'text' },
      ],
    },
  },

  // —— 二、库内上架、移库、转换与加工 ——
  {
    id: 'internal-move',
    category: 'internal',
    label: '上架 / 移库 / 补货',
    title: '库内移动（上架、移库、拣选位补货）',
    endpoint: '/warehouse-fulfillment-internal-moves',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '目标库位与库区必须在用。取得走的部分当场入账，不足或占用部分自动形成待批准作业差异。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        { path: 'lines', label: '移动明细', rowLabel: '移动行', kind: 'rows', required: true, rowFields: baseInternalLineFields },
      ],
    },
  },
  {
    id: 'form-conversion',
    category: 'internal',
    label: '形态转换',
    title: '商品包装形态转换（如箱拆件）',
    endpoint: '/warehouse-fulfillment-form-conversions',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '按商品当前修订的包装折合数把源单位转为目标单位，两头折成整数部分入账。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        {
          path: 'lines',
          label: '形态转换明细',
          rowLabel: '转换行',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
            { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '源单位', kind: 'text', required: true, placeholder: '箱' },
            { path: 'stockState', label: '库存状态', kind: 'select', required: true, options: stockStateOptions },
            { path: 'quantity', label: '源数量', kind: 'integer', required: true, placeholder: '1' },
            { path: 'sourceLocation', label: '源库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
            { path: 'targetLocation', label: '目标库位', kind: 'text', required: true, placeholder: 'SYN-LOC-PICK' },
            { path: 'toUnit', label: '目标单位', kind: 'text', required: true, placeholder: '件' },
            { path: 'toQuantity', label: '目标数量', kind: 'integer', required: true, placeholder: '12' },
            { path: 'inboundBatchId', label: '指定入库批次（可选）', kind: 'text' },
            { path: 'note', label: '备注（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },
  {
    id: 'condition-conversion',
    category: 'internal',
    label: '状态转换',
    title: '库内库存状态转换（如良品转次品）',
    endpoint: '/warehouse-fulfillment-condition-conversions',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '从源库位取走原状态库存，以目标状态入目标库位（目标库位与库区须在用且相容）。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        {
          path: 'lines',
          label: '状态转换明细',
          rowLabel: '转换行',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
            { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
            { path: 'stockState', label: '源状态', kind: 'select', required: true, options: stockStateOptions },
            { path: 'toState', label: '目标状态', kind: 'select', required: true, options: stockStateOptions },
            { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '1' },
            { path: 'sourceLocation', label: '源库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
            { path: 'targetLocation', label: '目标库位', kind: 'text', required: true, placeholder: 'SYN-LOC-DEF' },
            { path: 'inboundBatchId', label: '指定入库批次（可选）', kind: 'text' },
            { path: 'note', label: '备注（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },
  {
    id: 'shortage-registration',
    category: 'internal',
    label: '短少观察登记',
    title: '登记库内短少观察',
    endpoint: '/warehouse-fulfillment-shortage-registrations',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '只记现场短少观察并生成待处理作业差异，不直接改库存账；须由有批准授予的人批准后调减。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        {
          path: 'lines',
          label: '短少观察行',
          rowLabel: '短少行',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
            { path: 'sourceLocation', label: '所在库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
            { path: 'goodsCode', label: '商品编码（整箱留空）', kind: 'text', placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '计量单位（整箱留空）', kind: 'text', placeholder: '件' },
            { path: 'stockState', label: '库存状态（整箱留空）', kind: 'select', options: stockStateOptions },
            { path: 'quantity', label: '短少数量（整箱留空）', kind: 'integer', placeholder: '1' },
            { path: 'cartonNumber', label: '缺箱箱号（整箱缺箱填）', kind: 'text' },
            { path: 'note', label: '现场说明（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },
  {
    id: 'processing-execution',
    category: 'internal',
    label: '加工 / 拆分执行',
    title: '执行加工或拆分指令',
    endpoint: '/warehouse-fulfillment-processing-executions',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '按加工/拆分指令填写消耗（OUT）与产出（IN）明细，产出库位与库区须在用；库存不足整笔留在待加工。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        { path: 'instructionId', label: '加工/拆分指令号', kind: 'text', required: true, placeholder: 'INS-01' },
        { path: 'doneQuantity', label: '完成数量', kind: 'integer', required: true, placeholder: '1' },
        {
          path: 'lines',
          label: '消耗与产出明细',
          rowLabel: '执行行',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'direction', label: '方向', kind: 'select', required: true, options: directionOptions },
            { path: 'locationCode', label: '库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
            { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
            { path: 'stockState', label: '库存状态', kind: 'select', required: true, options: stockStateOptions },
            { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '2' },
            { path: 'inboundBatchId', label: '指定入库批次（可选）', kind: 'text' },
            { path: 'labelAlias', label: '贴标别名（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },
  {
    id: 'relabel-execution',
    category: 'internal',
    label: '商品换标执行',
    title: '执行商品换标指令',
    endpoint: '/warehouse-fulfillment-relabel-executions',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '填写被换标库存的 OUT 行，系统按指令目标贴标自动同笔换标入账并沿用原入库批次。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        { path: 'instructionId', label: '换标指令号', kind: 'text', required: true, placeholder: 'INS-01' },
        { path: 'doneQuantity', label: '完成数量', kind: 'integer', required: true, placeholder: '10' },
        {
          path: 'lines',
          label: '换标取出明细',
          rowLabel: '换标行',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'direction', label: '方向（填 OUT）', kind: 'select', required: true, options: [{ value: 'OUT', label: '取出换标（OUT）' }] },
            { path: 'locationCode', label: '所在库位', kind: 'text', required: true, placeholder: 'SYN-LOC-STORE' },
            { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
            { path: 'stockState', label: '库存状态', kind: 'select', required: true, options: stockStateOptions },
            { path: 'quantity', label: '数量', kind: 'integer', required: true, placeholder: '10' },
            { path: 'labelAlias', label: '原贴标（可选）', kind: 'text' },
            { path: 'inboundBatchId', label: '指定入库批次（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },
  {
    id: 'unboxing',
    category: 'internal',
    label: '整箱开箱',
    title: '整箱开箱清点入商品账',
    endpoint: '/warehouse-fulfillment-unboxings',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '扫描在库整箱并录入实点商品与在用目标库位（空箱可不填实点行），入库时间沿用原箱。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        { path: 'ownerAccount', label: '货主客户账户', kind: 'text', required: true, placeholder: 'SYN-OWNER-01' },
        { path: 'cartonNumber', label: '整箱箱号', kind: 'text', required: true, placeholder: 'SYN-CARTON-01' },
        {
          path: 'lines',
          label: '开箱实点明细（空箱可留空）',
          rowLabel: '实点行',
          kind: 'rows',
          rowFields: [
            { path: 'goodsCode', label: '商品编码（不认识可留空填 observed）', kind: 'text', placeholder: 'SYN-GOODS-01' },
            { path: 'observed', label: '未知货观察（无商品编码必填）', kind: 'text' },
            { path: 'unit', label: '计量单位', kind: 'text', placeholder: '件' },
            { path: 'stockState', label: '库存状态', kind: 'select', options: stockStateOptions },
            { path: 'quantity', label: '实点数量', kind: 'integer', required: true, placeholder: '5' },
            { path: 'locationCode', label: '目标入库库位（须在用）', kind: 'text', placeholder: 'SYN-LOC-STORE' },
            { path: 'batchNumber', label: '批号（可选）', kind: 'text' },
            { path: 'expiryDate', label: '失效日（可选）', kind: 'date' },
            { path: 'labelAlias', label: '贴标别名（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },

  // —— 三、盘点清点 ——
  {
    id: 'count-entry',
    category: 'count',
    label: '盘点清点录入',
    title: '录入库位盘点实点结果',
    endpoint: '/warehouse-fulfillment-count-entries',
    usesDeviceSequence: true,
    usesOccurredAt: true,
    snapshotHint: '针对已开始的盘点任务录入指定库位的实点数量；空库位可不加明细行（自动送空数组）。',
    form: {
      fields: [
        ...commonHeaderFields(true, true),
        { path: 'countId', label: '盘点单号', kind: 'text', required: true, placeholder: 'CNT-01' },
        { path: 'locationCode', label: '清点库位', kind: 'text', required: true, lookup: 'location', placeholder: 'SYN-LOC-STORE' },
        {
          path: 'lines',
          label: '实点明细（空库位可不加行）',
          rowLabel: '清点行',
          kind: 'rows',
          emitEmptyArray: true,
          rowFields: [
            { path: 'ownerAccount', label: '货主客户账户（未知货留空）', kind: 'text', placeholder: 'SYN-OWNER-01' },
            { path: 'goodsCode', label: '商品编码（未知货留空）', kind: 'text', placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '计量单位', kind: 'text', placeholder: '件' },
            { path: 'stockState', label: '库存状态', kind: 'select', options: stockStateOptions },
            { path: 'quantity', label: '实点数量（可填 0）', kind: 'integer', required: true, allowZero: true, placeholder: '10' },
            { path: 'observed', label: '未知货观察（无商品编码必填）', kind: 'text' },
            { path: 'batchNumber', label: '批号（可选）', kind: 'text' },
            { path: 'expiryDate', label: '失效日（可选）', kind: 'date' },
            { path: 'labelAlias', label: '贴标别名（可选）', kind: 'text' },
          ],
        },
      ],
    },
  },

  // —— 四、出库拣货、复核、打包、确认与回库 ——
  {
    id: 'pick',
    category: 'outbound',
    label: '拣货确认',
    title: '按拣货任务拣货移入待出库',
    endpoint: '/warehouse-fulfillment-picks',
    usesDeviceSequence: false,
    usesOccurredAt: false,
    snapshotHint: '扫描拣货任务号与实拣数量，移入在用的出库暂存库位（OUTBOUND_STAGING）。少拣自动记短拣差异。',
    form: {
      fields: [
        ...commonHeaderFields(false, false),
        { path: 'taskId', label: '拣货任务号', kind: 'text', required: true, placeholder: 'PKT-01', hint: '可从「出库与波次」波次右栏复制' },
        { path: 'quantity', label: '实拣数量', kind: 'integer', required: true, placeholder: '4' },
        { path: 'targetLocation', label: '目标待出库库位', kind: 'text', required: true, lookup: 'outbound-staging-location', placeholder: 'SYN-LOC-OUT' },
      ],
    },
  },
  {
    id: 'outbound-check',
    category: 'outbound',
    label: '出库复核',
    title: '出库复核清点',
    endpoint: '/warehouse-fulfillment-outbound-checks',
    usesDeviceSequence: false,
    usesOccurredAt: false,
    snapshotHint: '核对出库请求的实点总数；与已拣数量一致才进入可打包状态。',
    form: {
      fields: [
        ...commonHeaderFields(false, false),
        { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
        { path: 'observed', label: '复核实点总数', kind: 'integer', required: true, allowZero: true, placeholder: '4' },
      ],
    },
  },
  {
    id: 'pack',
    category: 'outbound',
    label: '打包称重',
    title: '打包称重量方并提交委托',
    endpoint: '/warehouse-fulfillment-packs',
    usesDeviceSequence: false,
    usesOccurredAt: false,
    snapshotHint: '各包裹总件数须等于已拣总数。本产品网络打包后自动提交委托（预取过的序号 1 沿用原委托并在称重不同时提出资料修订）。',
    form: {
      fields: [
        ...commonHeaderFields(false, false),
        { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
        {
          path: 'parcels',
          label: '装箱包裹明细',
          rowLabel: '包裹',
          kind: 'rows',
          required: true,
          rowFields: [
            { path: 'sequence', label: '包裹序号', kind: 'integer', required: true, placeholder: '1' },
            { path: 'packagingMaterial', label: '包材编码', kind: 'text', required: true, lookup: 'packaging', placeholder: 'SYN-BOX-01' },
            { path: 'weight', label: '称重重量', kind: 'decimal', required: true, placeholder: '1.25' },
            { path: 'weightUnit', label: '重量单位', kind: 'text', required: true, placeholder: 'KG' },
            { path: 'length', label: '长', kind: 'decimal', required: true, placeholder: '30' },
            { path: 'width', label: '宽', kind: 'decimal', required: true, placeholder: '20' },
            { path: 'height', label: '高', kind: 'decimal', required: true, placeholder: '15' },
            { path: 'dimensionUnit', label: '尺寸单位', kind: 'text', required: true, placeholder: 'CM' },
            { path: 'goodsCode', label: '商品编码', kind: 'text', required: true, placeholder: 'SYN-GOODS-01' },
            { path: 'unit', label: '计量单位', kind: 'text', required: true, placeholder: '件' },
            { path: 'quantity', label: '装入数量', kind: 'integer', required: true, placeholder: '4' },
          ],
        },
      ],
    },
  },
  {
    id: 'outbound-confirmation',
    category: 'outbound',
    label: '出库确认',
    title: '包裹离仓出库确认',
    endpoint: '/warehouse-fulfillment-outbound-confirmations',
    usesDeviceSequence: false,
    usesOccurredAt: true,
    snapshotHint: '在线模式校验放行条件（面单本体、接受与更正态）；离线已交出的勾选离线，照记并标出缺口。消耗库存并交给全程追踪。',
    form: {
      fields: [
        ...commonHeaderFields(true, false),
        { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
        { path: 'parcelSequence', label: '包裹序号', kind: 'integer', required: true, placeholder: '1' },
        { path: 'offline', label: '离线已交出（照记并标注缺失放行条件）', kind: 'boolean', omitWhenFalse: true },
        { path: 'externalNumber', label: '外部运单/面单号（可选）', kind: 'text' },
        { path: 'validFrom', label: '放行有效起点（可选）', kind: 'datetime' },
        { path: 'validUntil', label: '放行有效终点（可选）', kind: 'datetime' },
      ],
    },
  },
  {
    id: 'pickup-handover',
    category: 'outbound',
    label: '自提交付',
    title: '自提出库现场交付确认',
    endpoint: '/warehouse-fulfillment-pickup-handovers',
    usesDeviceSequence: false,
    usesOccurredAt: true,
    snapshotHint: '发运方式为自提（PICKUP）时使用，核对提货方与自提凭证后确认离仓。',
    form: {
      fields: [
        ...commonHeaderFields(true, false),
        { path: 'requestId', label: '出库请求号', kind: 'text', required: true, placeholder: 'OBR-01' },
        { path: 'parcelSequence', label: '包裹序号', kind: 'integer', required: true, placeholder: '1' },
        { path: 'pickupParty', label: '提货方', kind: 'text', placeholder: '提货人或车队' },
        { path: 'pickupCredential', label: '自提凭证', kind: 'text', placeholder: '提货码或签收单号' },
        { path: 'offline', label: '离线已交出', kind: 'boolean', omitWhenFalse: true },
      ],
    },
  },
  {
    id: 'stock-return',
    category: 'outbound',
    label: '截单回库',
    title: '把截单/拒收后的已拣货移回可分配库位',
    endpoint: '/warehouse-fulfillment-returns',
    usesDeviceSequence: false,
    usesOccurredAt: false,
    snapshotHint: '完成截单或委托被拒生成的回库任务，把货从待出库移回在用的可分配库位。',
    form: {
      fields: [
        ...commonHeaderFields(false, false),
        { path: 'returnId', label: '回库任务号', kind: 'text', required: true, placeholder: 'OBR-01/PKT-01' },
        { path: 'target', label: '目标可分配库位（须在用）', kind: 'text', required: true, lookup: 'location', placeholder: 'SYN-LOC-STORE' },
      ],
    },
  },
];
