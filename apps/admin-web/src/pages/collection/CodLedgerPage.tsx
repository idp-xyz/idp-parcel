import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listCodSubledgers,
  type CodSubledgerListResponseBody,
  type CodSubledgerRecord,
} from './api';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById,只读引用,不抄第二份。
const info = moduleInfoById['cod-ledger'];

// 行对象是一本代收分户账(票 admin-remainder-mechanism-batch/04):四维键、保管依据、
// 开立时间、六位置派生余额与回汇批次引用,接 GET /collection-subledgers。
//
// 旧骨架里的「代收资金义务」「已接受实收金额」两列随建模落地撤下——义务归代收指令册、
// 实收归代收事实册,都不是分户账行的字段(判据同口岸页撤「所属区域/适用性」那笔:
// 目录事实只有本册登了什么);位置余额已反映被接受的事实经记账落到哪里。「短款/溢款」
// 以资金位置成列,是记账的派生结果,不是差异事项册本身。

/** 回汇批次状态封闭二值的中文词表;读到集外取值时原样显示,不代折成已知格。 */
const batchStateLabels: Record<string, string> = {
  COLLECTED: '已归集',
  HANDED_FOR_PAYMENT: '已交付汇付',
};

// 金额列都是币种最小单位的十进制计数串,照实转写:小数位属币种语义,页面不代判
// 精度也不做换算(币种自有一列)。
function amountCol(
  id: string,
  header: string,
  pick: (row: CodSubledgerRecord) => string,
): ListColumn<CodSubledgerRecord> {
  return { id, header, align: 'right', className: 'font-mono', render: (row) => pick(row) };
}

// 「渠道在途」与「应付客户」两栏表头直接携带硬约束(未实际收到的代收款不得进入
// 可付客户余额),让规则在栏目命名上站住,不靠使用者记规则——沿旧骨架的这条取舍。
const columns: ListColumn<CodSubledgerRecord>[] = [
  { id: 'customer', header: '货主客户', className: 'font-mono', render: (row) => row.customer },
  { id: 'legal-entity', header: '责任法人', className: 'font-mono', render: (row) => row.legalEntity },
  { id: 'channel', header: '代收渠道', className: 'font-mono', render: (row) => row.channel },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[72px] font-mono', render: (row) => row.currency },
  amountCol('in-transit', '渠道在途(未实际收到)', (row) => row.balances.inTransitAtChannel),
  amountCol('awaiting', '待清分', (row) => row.balances.awaitingAllocation),
  amountCol('payable', '应付客户(仅实收后形成)', (row) => row.balances.payableToCustomer),
  amountCol('remitted', '已汇付', (row) => row.balances.remitted),
  amountCol('shortfall', '短款', (row) => row.balances.shortfall),
  amountCol('surplus', '溢款', (row) => row.balances.surplus),
  {
    id: 'posting-count',
    header: '记账笔数',
    align: 'right',
    className: 'font-mono',
    // 「已开立但从未记账」(0 笔)与「记账相抵为零」在六个零余额上长着同一张脸,
    // 笔数是唯一分得开两态的照实转写。
    render: (row) => String(row.postingCount),
  },
  {
    id: 'batches',
    header: '回汇批次',
    // 空数组如实说「未配置」而不是留白——留白读起来像数据缺件,而这格恰恰要说出
    // 「回汇周期与汇付通道属实例半边、尚未配置」这句话(写法同渠道产品目录的渠道
    // 绑定格)。批次在场时逐个列标识与状态,截点归批次详情,本格不展开。
    render: (row) =>
      row.remittanceBatches.length > 0 ? (
        <div className="min-w-44 font-mono text-xs">
          {row.remittanceBatches.map((batch) => (
            <p key={batch.batch}>
              {batch.batch}({batchStateLabels[batch.state] ?? batch.state})
            </p>
          ))}
        </div>
      ) : (
        <span className="text-idpxyz-textMuted">未配置(回汇周期与汇付通道属实例半边,尚无批次)</span>
      ),
  },
  {
    id: 'custody-basis',
    header: '保管依据',
    className: 'font-mono text-xs',
    render: (row) => row.custodyBasis,
  },
  {
    id: 'opened-at',
    header: '开立时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.openedAt),
  },
];

// 行键即分户账主键四维——同一租户内四维组合唯一。
function rowKey(row: CodSubledgerRecord): string {
  return [row.customer, row.legalEntity, row.currency, row.channel].join(':');
}

/**
 * 代收分户账(collection-remittance)。行对象是按(货主客户,责任法人,币种,代收渠道)
 * 隔离的受托保管账:四维齐备才构成一本账,余额由追加式记账派生,账上没有可改写的
 * 余额字段。代收本金是客户的钱,不是经营口径物流收入或普通运营结算余额(COD 服务费
 * 与获准抵扣归 settlement-accounting)。查阅面,不设记账动作——开立与记账走受控 CLI。
 */
export function CodLedgerPage() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CodSubledgerListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listCodSubledgers().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const subledgers = answer?.kind === 'outcome' ? answer.body.subledgers : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做,不下推成查询参数——那要改端点契约。筛选字段即
  // 分户四维(四维之间隔离,不得合并,筛选也按字段各自命中)加保管依据。
  const visibleSubledgers = needle
    ? subledgers.filter((row) =>
        [row.customer, row.legalEntity, row.currency, row.channel, row.custodyBasis].some(
          (value) => value.toLowerCase().includes(needle),
        ),
      )
    : subledgers;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CodSubledgerRecord>
      title={info.title}
      // 页头说明直接携带硬约束,与在途/应付两栏的命名互为呼应。
      description={`${info.owner}——未实际收到的代收款不得进入可付客户余额;金额列为币种最小单位计数`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索货主客户 / 责任法人 / 币种 / 代收渠道',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示:未配置态与错误态下报「0 本」会与状态区
        // 「这不是登记册为空」直接矛盾(README 列表页上列通则第六条)。
        answer?.kind === 'outcome' ? `当前返回 ${subledgers.length} 本分户账` : undefined
      }
      columns={columns}
      rows={visibleSubledgers}
      rowKey={rowKey}
      viewState={catalogueViewState(answer, subledgers.length, retry, {
        module: info,
        endpoint: 'GET /collection-subledgers',
        emptyTitle: '当前租户尚无已开立的代收分户账',
        emptyDescription:
          '读取入口已配置,但分户账登记册为空;真实客户、法人、币种与渠道属实例半边,页面不会预置分户账。开立与记账走 collection-remittance 受控登记 CLI。',
      })}
    />
  );
}
