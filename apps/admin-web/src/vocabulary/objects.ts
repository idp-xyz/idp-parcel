// 界面用语词典 · 对象与字段名词（设计源「四、用语词典」）。
//
// 页名、列名、详情栏标签用一线的词：运单、客户、承运商、清关单、物流动态。系统术语（委托、参与方、修订、依据、投影、
// 租户）留在代码与领域文档里，界面上要用时经这张表翻译，一处定义。键是稳定的英文标识符（给代码引用），值是界面词；
// 领域原词写在注释里，便于对回 CONTEXT。
//
// 这里只收**名词**；状态与枚举码在 status.ts，按钮与句子由各页自己写（主动语态、说它做什么）。

export const objectNouns = {
  // —— 运单（领域：客户委托 / 包裹）——
  shipment: '运单',
  shipmentNumber: '运单号',
  /** 客户自己的单号（领域：来源请求键），排在运单号下面。 */
  customerReference: '客户单号',
  parcel: '包裹',
  /** 领域：声明包裹（件数）。 */
  parcelCount: '件数',
  weight: '重量',
  destination: '目的地',
  recipient: '收件人',
  submittedAt: '提交时间',
  /** 领域：最新标准里程碑。 */
  currentStatus: '当前状态',
  status: '状态',
  progress: '进度',

  // —— 往来方（领域：业务参与方按角色称呼；没有角色时叫往来单位）——
  customer: '客户',
  /** 领域：客户账户标识——编码进技术详情，列上显客户名称。 */
  customerCode: '客户编码',
  customerAccount: '客户账户',
  carrier: '承运商',
  agent: '代理',
  customsBroker: '清关行',
  channel: '渠道',
  supplier: '供应商',
  partner: '合作伙伴',
  counterparty: '往来单位',
  /** 领域：责任法人——我们自己签约、开票的主体。 */
  companyEntity: '公司主体',
  contract: '合同',
  supplierAgreement: '供应商协议',

  // —— 登记与变更（领域：修订 / 登记依据 / 依据引用）——
  changeRecord: '变更记录',
  sourceDocument: '来源单据',
  technicalDetails: '技术详情',
  approval: '审批',
  publication: '发布',

  // —— 产品与价格（领域：价卡 / 计价评价 / 评价引用）——
  serviceProduct: '服务产品',
  priceList: '价格表',
  billing: '计费',
  billingRecord: '计费记录',
  estimate: '计价试算',
  referenceRate: '汇率与燃油费率',

  // —— 账务 ——
  charge: '费用',
  receivable: '应收',
  payable: '应付',
  statement: '对账单',
  settlement: '收付款核销',
  codCollection: '代收货款',
  operatingResult: '经营核算',

  // —— 清关（领域：关务案件 / 申报单元 / 提交版本 / 放行门禁核对）——
  customsCase: '清关单',
  declarationBatch: '报关批次',
  declaration: '申报',
  releaseCheck: '放行检查',
  releaseRecord: '放行记录',
  dutyAndTax: '税费',
  complianceRestriction: '合规限制',

  // —— 物流动态（领域：全程追踪投影 / 派生时间）——
  trackingUpdates: '物流动态',
  updatedAt: '更新时间',
  eta: '预计送达',

  // —— 线路与运力（领域：节点 / 线路 / 班次 / 容量池）——
  site: '网点',
  warehouse: '仓',
  route: '路线',
  schedule: '班次',
  capacity: '舱位',
  serviceArea: '服务区域',
  port: '口岸',
  declarationPath: '申报路径',

  // —— 作业与处置（领域：作业设备册 / 收寄后服务处置决定 / 待人工处理项）——
  scanDevice: '扫描设备',
  disposition: '货物处置',
  todo: '待办事项',

  // —— 异常与索赔 ——
  exception: '异常',
  exceptionCase: '异常案件',
  claim: '索赔',
  recovery: '追偿',
  notification: '通知',
  disclosure: '披露',
} as const satisfies Record<string, string>;

export type ObjectNoun = keyof typeof objectNouns;

export function nounOf(key: ObjectNoun): string {
  return objectNouns[key];
}
