// 本目录 fetch 出口:代收与清分的分户账查阅端点(GET /collection-subledgers,票
// admin-remainder-mechanism-batch/04)。形状以
// internal/collectionremittance/adapters/http/query_cod_subledgers.go 为准,此处只做
// 镜像不虚构。传输与五格判读收敛在共享 catalogue-api,本文件只保留本上下文的类型与
// 查询函数。

import { exchangeMasterData, type ApiResult } from '../catalogue-api';

export type { ApiResult } from '../catalogue-api';

/**
 * 六个资金位置的派生余额,币种最小单位十进制计数串。余额由分户账记账派生——账上
 * 没有可改写的余额字段;传输用串不用数,是照实转写不是算术值:小数位属币种语义,
 * 本读面不代判精度,页面也不做换算。
 */
export interface SubledgerPositionBalances {
  /** 渠道在途:渠道已代收但尚未回到运营企业。 */
  inTransitAtChannel: string;
  /** 待清分:资金已回但归属尚未落到具体客户应付。 */
  awaitingAllocation: string;
  /** 应付客户:只能由已接受的真实到账经清分形成。 */
  payableToCustomer: string;
  /** 已汇付:已依回汇批次付出(账内终局位置)。 */
  remitted: string;
  /** 短款:实收少于指令、差额经差异事项确认后落此。 */
  shortfall: string;
  /** 溢款:实收多于指令的差额。 */
  surplus: string;
}

/**
 * 一个已形成的回汇批次。分户账键、币种与归集截点在形成时冻结,不接受删除、重开或
 * 改写;state 封闭二值 COLLECTED / HANDED_FOR_PAYMENT,单向推进无回退。
 */
export interface RemittanceBatchRecord {
  batch: string;
  state: string;
  collectedThrough: string;
  formedAt: string;
}

/**
 * 一本代收分户账:四维键(货主客户、责任法人、币种、代收渠道)、保管依据引用、开立
 * 时间、六位置派生余额与引用本账的回汇批次。四维齐备才构成一本账,币种在键上因此
 * 账内不发生换算。
 *
 * remittanceBatches 空数组即「尚无批次」——回汇周期与汇付通道属实例半边,未配置时
 * 如实为空,页面据此显式说「未配置」而不是留白(判读同渠道产品目录的渠道绑定格)。
 */
export interface CodSubledgerRecord {
  customer: string;
  legalEntity: string;
  currency: string;
  channel: string;
  /** party-commercial 代收商业责任依据的标识引用,照登转写。 */
  custodyBasis: string;
  openedAt: string;
  /**
   * 记账笔数,分开「从未记账」与「记账相抵为零」两态——两态在六个零余额上长着同一
   * 张脸。它是行数不是金额,故用数不用串。
   */
  postingCount: number;
  balances: SubledgerPositionBalances;
  remittanceBatches: RemittanceBatchRecord[];
}

export interface CodSubledgerListResponseBody {
  outcome: 'COD_SUBLEDGERS_LISTED';
  subledgers: CodSubledgerRecord[];
}

/**
 * 分户账册只有一本,故本函数不收分派参数——封闭集为一时参数只会造出一个恒定值
 * (判据同 /customs-gate-conditions 那句)。
 */
export function listCodSubledgers(): Promise<ApiResult<CodSubledgerListResponseBody>> {
  return exchangeMasterData<CodSubledgerListResponseBody>('/collection-subledgers');
}
