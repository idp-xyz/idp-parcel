// 界面用语词典 · 价卡内容（价格表详情栏）。
//
// 价卡登记的规范化文档里全是枚举码与单位码（`LONGEST_SIDE`、`GT`、`KG`、`FIRST_CONTINUE`……）；运营看的是「最长边大于
// 100 厘米」。这里收随产品发布的参考译法，和 status.ts 一样一处定义；本册只追加条目，不改总表。
//
// 键取 domain 的原词（parcel-pricing 的 FeatureSource、ComparisonOperator、PricingWeightMethod……），查不到的码**不在这里
// 猜**——调用方（pages/pricing/price-card-content.ts）把「词典里没有」当成看不懂的结构降级，而不是把码原样端上屏。
// 单位与币种例外：它们是开放词表（租户可以用任何标准单位），查不到时原样显示单位码——码本身就是行业通用写法。

/** 判定条件的特征词（parcel-pricing CONTEXT「特征」：闭合集合）。 */
export const featureLabels: Readonly<Record<string, string>> = {
  LONGEST_SIDE: '最长边',
  SECOND_LONGEST_SIDE: '次长边',
  LENGTH_AND_GIRTH: '长加周长',
  VOLUME: '体积',
  ACTUAL_WEIGHT: '实重',
  VOLUMETRIC_WEIGHT: '体积重',
  // 读的是包裹自身的计价重量，不含条件最低计价重量的抬高（ADR-0347 决定一）；界面把这层限定写进词里，运营才不会
  // 把它当成最终计费用的那个重量。
  CHARGEABLE_WEIGHT: '计价重量（不含最低计价重量抬高）',
  ZONE: '分区',
  // 这一槽承载的是偏远档位（槽名沿用至今，取值是档位码），界面一律叫偏远档位。
  ADDRESS_TYPE: '偏远档位',
  ADDRESS_NATURE: '地址性质',
  SERVICE_OPTION: '服务选项',
};

/** 比较词（ComparisonOperator）。次序运算只配数值特征，EQ 只配类别特征（领域构造期守）。 */
export const comparisonLabels: Readonly<Record<string, string>> = {
  GT: '大于',
  GE: '不小于',
  LT: '小于',
  LE: '不大于',
  EQ: '等于',
};

/** 类别特征是类别、不是数值：同一特征的几个「等于」并列时合成一句「属于」。 */
export const membershipLabel = '属于';

/** 地址性质的封闭取值（ADR-0347 决定四）。 */
export const addressNatureValueLabels: Readonly<Record<string, string>> = {
  COMMERCIAL: '商业',
  RESIDENTIAL: '住宅',
};

/** 价格方向：运营说的是「售价」与「成本」。 */
export const priceDirectionLabels: Readonly<Record<string, string>> = {
  SELL: '售价',
  BUY: '成本',
  INTERNAL: '内部价',
};

/** 计算目的（与价格方向首发一一对应）。 */
export const priceCardPurposeLabels: Readonly<Record<string, string>> = {
  CUSTOMER_CHARGE: '客户收费',
  SUPPLIER_COST: '供应商成本',
  INTERNAL_PRICE: '内部结算价',
};

/** 计价口径（聚合方式）。 */
export const aggregationLabels: Readonly<Record<string, string>> = {
  PER_PACKAGE: '每件计价',
  PER_SHIPMENT: '按票聚合计价',
  PER_MASTER_DOCUMENT: '按主单聚合计价',
};

/** 费用在聚合主体上的计收单位词：「每件固定 5 元」「每票固定 5 元」。 */
export const aggregationSubjectLabels: Readonly<Record<string, string>> = {
  PER_PACKAGE: '每件',
  PER_SHIPMENT: '每票',
  PER_MASTER_DOCUMENT: '每主单',
};

export const weightUnitLabels: Readonly<Record<string, string>> = {
  G: '克',
  KG: '千克',
  OZ: '盎司',
  LB: '磅',
};

export const lengthUnitLabels: Readonly<Record<string, string>> = {
  CM: '厘米',
  IN: '英寸',
};

/** 体积的单位是长度单位的立方（领域里体积阈值的 unit 报的是底层长度单位）。 */
export const volumeUnitLabels: Readonly<Record<string, string>> = {
  CM: '立方厘米',
  IN: '立方英寸',
};

/** 金额后缀：人民币写「元」，其余常见币种写中文名；目录之外的币种原样写码。 */
export const currencySuffixLabels: Readonly<Record<string, string>> = {
  CNY: '元',
  USD: '美元',
  SGD: '新加坡元',
  EUR: '欧元',
  GBP: '英镑',
  JPY: '日元',
  HKD: '港元',
};

/** 取整模式词（RoundingMode）。NONE 不带进位单位。 */
export const roundingModeLabels: Readonly<Record<string, string>> = {
  CEILING: '向上进位到',
  HALF_UP: '四舍五入到',
};
export const roundingNoneLabel = '不进位';

/** 金额取整的应用点（AmountRoundingPoint）。 */
export const amountRoundingPointLabels: Readonly<Record<string, string>> = {
  PER_LINE: '每一项费用',
  TOTAL: '合计',
};

/** 参考数据的种类词：序列（数值型，按期取值）与目录（类别型，邮编前缀映射）。 */
export const referenceSeriesKindLabels: Readonly<Record<string, string>> = {
  FUEL_RATE: '燃油费率序列',
  EXCHANGE_RATE: '汇率序列',
  PUBLISHED_AMOUNT: '按期公布金额序列',
};
export const referenceCatalogueKindLabels: Readonly<Record<string, string>> = {
  ZONE: '分区目录',
  REMOTE_TIER: '偏远档位目录',
};

/** 价表的查档维度（TableKey）：缺省按分区，也可按偏远档位。 */
export const rateKeyNames = {
  zone: '分区',
  remoteTier: '偏远档位',
} as const;

/** 价卡行的状态词（价格表列表的「状态」列）。色调是管理台的呈现判断，不是领域状态本身。 */
export type PriceCardStatusCode = 'IN_FORCE' | 'NOT_YET' | 'EXPIRED' | 'CUT_OFF' | 'RETIRED';

export const priceCardStatusLabels: Readonly<
  Record<PriceCardStatusCode, { label: string; tone: 'good' | 'info' | 'warn' | 'bad' | 'muted' }>
> = {
  IN_FORCE: { label: '生效中', tone: 'good' },
  NOT_YET: { label: '未生效', tone: 'info' },
  EXPIRED: { label: '已过期', tone: 'muted' },
  CUT_OFF: { label: '已截止', tone: 'warn' },
  RETIRED: { label: '已退役', tone: 'muted' },
};
