// 界面用语词典 · 单据编号（设置 → 单据编号，ADR-0355 决定五、六、八）。
//
// 规则册读口与预览口交回的全是码：类型代码（`SHIPMENT`）、重置周期（`MONTHLY`）、日期时区来路、登记拒因
// （`TEMPLATE_REPEATS_ACROSS_PERIODS`）。运营看的是「运单」「每月从 1 重排」「格式缺了年」。这里收随产品发布的界面词，
// 和 status.ts 一样一处定义；本册只追加条目，不改别的册。
//
// 码的出处是 internal/platform/docnumber 与它的 HTTP 口（docnumberhttp 包注释是契约）；各上下文改了自己那一类的声明
// （例如 04、05 核定某类其实有开立方业务时区）时，界面词不用跟着改——词按码给，来路按读口答的来路说。
//
// 查不到的码**不在这里猜**：调用方（pages/document-numbering/rows.ts）把它如实露出并说「界面还没有这一类的叫法」，
// 不把码当成中文词端上屏，也不悄悄丢掉一行。

/** 一类单据在界面上的叫法：单据名与它的号叫什么。ADR-0355 决定十一：委托的单据号在界面上叫「运单号」。 */
export interface DocumentTypeWords {
  name: string;
  numberName: string;
}

/** 十一类单据（ADR-0355 决定二）。单据名取管理台各页已在用的叫法，不另造。 */
export const documentTypeWords: Readonly<Record<string, DocumentTypeWords>> = {
  SHIPMENT: { name: '运单', numberName: '运单号' },
  CUSTOMER_STATEMENT: { name: '客户对账单', numberName: '对账单号' },
  STATEMENT_DISPUTE: { name: '对账单争议', numberName: '争议号' },
  AUDITED_PAYABLE: { name: '审核应付', numberName: '应付单号' },
  EXCEPTION_CASE: { name: '异常案件', numberName: '案件号' },
  CLAIM: { name: '客户索赔', numberName: '索赔号' },
  CUSTOMS_CASE: { name: '清关单', numberName: '清关单号' },
  DELIVERY_PRE_ADVICE: { name: '客户送货预报', numberName: '预报号' },
  DELIVERY_HANDOVER_NOTE: { name: '送货交接单', numberName: '交接单号' },
  BOOKING: { name: '订舱', numberName: '订舱号' },
  TRANSPORT_COMMISSION: { name: '运输委托', numberName: '委托号' },
};

/** 重置周期（docnumber.ResetPeriod 的封闭五值）：列上的短词与详情里的一句。 */
export const resetPeriodWords: Readonly<Record<string, { short: string; sentence: string }>> = {
  DAILY: { short: '按日', sentence: '流水每天从 1 起重新数' },
  MONTHLY: { short: '按月', sentence: '流水每月从 1 起重新数' },
  QUARTERLY: { short: '按季', sentence: '流水每季从 1 起重新数' },
  YEARLY: { short: '按年', sentence: '流水每年从 1 起重新数' },
  CONTINUOUS: { short: '不重置', sentence: '流水一直往下数，不重新从 1 起' },
};

/**
 * 开立方业务时区的那一方叫什么（ADR-0355 决定七点名网点与结算账户）。读口只答「按开立方业务时区」，不答开立方是谁；
 * 这里按类型给一句人话，表里没有的类型说「开立方」——声明改了也不至于说错。
 */
export const issuerWordsByType: Readonly<Record<string, string>> = {
  CUSTOMER_STATEMENT: '结算账户',
  DELIVERY_PRE_ADVICE: '网点',
  DELIVERY_HANDOVER_NOTE: '网点',
};

export const genericIssuerWord = '开立方';

/**
 * 登记拒因（docnumberhttp 的 reasonCodes 一表，登记口与预览口共用）：每条说清为什么不能登、怎么改。
 * `MALFORMED_REQUEST` 也在这一表：登记口拿它答「格式或生效时刻没填」。
 */
export const registrationRejectionWords: Readonly<Record<string, string>> = {
  DOCUMENT_TYPE_NOT_DECLARED: '系统里没有这一类单据；请刷新页面后重试，仍不行请联系产品支持。',
  TEMPLATE_UNPARSEABLE:
    '格式里有认不出的写法：花括号里只能写下面列出的部件（如 {yyyy}、{seq:6}），花括号要成对；格式也不能是空的。',
  TEMPLATE_WITHOUT_SEQUENCE: '格式里没有流水号。每个号都要带一段流水（{seq} 或加扰流水），只靠日期或随机数会出重复的号。',
  TEMPLATE_NEEDS_UNSUPPLIED_VALUE:
    '格式用了开立时给不出取值的部件（网点、货主、区域、法人或校验位）。这些部件会渲染成空的一段，号就少了一截，所以不能登记。',
  TEMPLATE_UNRENDERABLE:
    '按这个格式出的号不能用：格式最多 64 个字符、出来的号最多 40 个字符，号里也不能有空格或换行。',
  TEMPLATE_REPEATS_ACROSS_PERIODS:
    '这个格式会在不同周期出同一个号：流水每个周期从 1 重新数，而格式里缺了比重置周期更粗的日期部件（例如按月重排却不带年、' +
    '按日重排却不带月或年），下个周期就会出和上个周期一样的号。把年（以及月）加进格式即可。',
  SCRAMBLE_SECRET_NOT_CONFIGURED:
    '格式用了加扰流水，但本公司还没有配置加扰密钥。密钥由部署方在实例配置里提供，配好之前请用普通流水。',
  TIME_ZONE_NOT_RECOGNIZED: '认不出这个时区名，请从列表里选一个。',
  MALFORMED_REQUEST: '提交的内容不完整或写法不对：格式与生效时刻都要填。',
};

/** 两个登记口在 201 / 200 上答的三种结果（docnumber.RegistrationOutcome）。 */
export const registrationOutcomeWords: Readonly<Record<string, string>> = {
  VERSION_REGISTERED: '已登记',
  VERSION_EXISTING: '册上已有完全相同的一版，没有重复登记',
  EFFECTIVE_FROM_TAKEN: '这个生效时刻已经登过另一版；规则只增不改，请换一个生效时刻',
};

/** 模板部件的一条说明：写法、叫法、例子与说明。 */
export interface TemplateTokenGuide {
  /** 填进格式里的写法，例如 `{yyyy}`。带 N 的写法里 N 由插入时选的位数替换。 */
  token: string;
  label: string;
  example: string;
  note?: string;
}

/**
 * 给填格式的人看的部件说明（ADR-0355 决定五）。只列登记得上的部件；网点、货主、区域、法人与校验位登不上
 * （开立时给不出取值，见 TEMPLATE_NEEDS_UNSUPPLIED_VALUE），不列。花括号以外的字原样出现在号里。
 */
export const templateTokenGuide: readonly TemplateTokenGuide[] = [
  { token: '{yyyy}', label: '年', example: '2026' },
  { token: '{yy}', label: '年（两位）', example: '26' },
  { token: '{mm}', label: '月', example: '11' },
  { token: '{dd}', label: '日', example: '02' },
  { token: '{q}', label: '季度', example: '4', note: '一年四季，写成 1 到 4' },
  {
    token: '{seq:N}',
    label: '流水号',
    example: '{seq:5} → 00042',
    note: '冒号后的数字是最少位数，不足补零；数满了照常进位，不截断',
  },
  {
    token: '{seqx:N}',
    label: '加扰流水',
    example: '{seqx:6} → 803571',
    note: '冒号后的数字是位数；外人从号上看不出先后与单量，要本公司先配置加扰密钥',
  },
];

/** 流水按格式里最细的日期部件重排——填格式的人最容易忽略的一条。 */
export const resetRuleSentence =
  '流水按格式里最细的日期部件重新从 1 数：有「日」每天、有「月」每月、有「季」每季、只有「年」每年，没有日期部件就一直往下数。';

/** 格式里每个部件在「格式」一格里的叫法（含登不上的那几种：老版本或产品默认里出现时也要读得懂）。 */
export const templateTokenWords: Readonly<Record<string, string>> = {
  yyyy: '年',
  yy: '年',
  mm: '月',
  dd: '日',
  q: '季',
  hh: '时',
  mi: '分',
  ss: '秒',
  ms: '毫秒',
  fy: '财年',
  fyyy: '财年',
  seq: '流水',
  seq36: '字母数字流水',
  seqx: '加扰流水',
  seqx36: '字母数字加扰流水',
  rand: '随机数',
  type: '类型代码',
  site: '网点',
  owner: '货主',
  region: '区域',
  le: '法人',
  check: '校验位',
};
