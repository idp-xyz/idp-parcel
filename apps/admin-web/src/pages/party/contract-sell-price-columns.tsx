import { useAnswer } from '../../query/answers';
import type { WorkbenchColumn } from '../../templates';
import { readSellPriceScope } from '../pricing/api';
import { pricingAnswersDomain } from '../pricing/pricing-writes';
import type { CustomerContractRecord } from './api';
import { sellPriceArrangementText, sellPriceScopeMark } from './customer-contract-form';

// 客户合同的「销售价格来源」在目录表与详情里是两列（票 commercial-four-module-requirements/20）。
//
// 目录表一页合同每一行都渲染这一列，所以它只写文字、不问计价。逐个来源标出此刻有没有在用的卡是详情才做的事：并成一列
// 的话，列表每一行、每个来源都要发一次请求，而票面要的只是「详情页标得出来」。

/** 目录表那一列：只把合同声明的来源按顺序写成一行字。 */
export const sellPriceListColumn: WorkbenchColumn<CustomerContractRecord> = {
  key: 'sell-price',
  header: '销售价格来源',
  render: (row) => <span className="type-body leading-4">{sellPriceArrangementText(row.sellPriceArrangement)}</span>,
};

/** 详情栏与对象页那一列：逐个来源带标。 */
export const sellPriceDetailColumn: WorkbenchColumn<CustomerContractRecord> = {
  ...sellPriceListColumn,
  render: (row) => <SellPriceSourcesDetail arrangement={row.sellPriceArrangement} />,
};

/** 详情用的列：与目录表同一组、同一顺序，只把「销售价格来源」换成逐个来源带标的那一列。按列本身认，不按键名认。 */
export function detailColumnsOf(listColumns: WorkbenchColumn<CustomerContractRecord>[]): WorkbenchColumn<CustomerContractRecord>[] {
  return listColumns.map((column) => (column === sellPriceListColumn ? sellPriceDetailColumn : column));
}

/**
 * 按合同声明的顺序逐个写出来源，每个来源旁标此刻有没有在用的卡——写错的范围名与卡还没发的范围在这里看得出来
 * （ADR-0380 越权风险点 3）。没有来源列表的合同（没有这一格，或显式不计销售价）只写文字，一次也不问计价。
 */
function SellPriceSourcesDetail({ arrangement }: { arrangement?: CustomerContractRecord['sellPriceArrangement'] }) {
  const sources = arrangement?.noSellPriceBasis ? [] : (arrangement?.sources ?? []);
  if (sources.length === 0) {
    return <span className="type-body leading-4">{sellPriceArrangementText(arrangement)}</span>;
  }
  return (
    <ol className="space-y-1">
      {sources.map((source, index) => (
        <li key={source.pricingScope} className="type-body leading-4">
          <span className="text-idpxyz-textMuted">{index + 1}. </span>
          <span className="font-mono">{source.pricingScope}</span>
          <SellPriceScopeMark scope={source.pricingScope} />
        </li>
      ))}
    </ol>
  );
}

/**
 * 一个来源此刻有没有在用的卡。一个来源一个组件：读走查询缓存，同一个范围名同时出现在几处时只问一次；键归计价域，发了
 * 新卡之后随计价的写一起作废。每次打开详情都重问（fresh）——这一格说的是「此刻」，不拿上一次问到的顶。
 */
function SellPriceScopeMark({ scope }: { scope: string }) {
  const { answer } = useAnswer([pricingAnswersDomain, 'sell-price-scope', scope], () => readSellPriceScope(scope), { fresh: true });
  const mark =
    answer === null ? sellPriceScopeMark(undefined) : answer.kind === 'outcome' ? sellPriceScopeMark(answer.body.cards) : '在用的卡读不到';
  return <span className="text-idpxyz-textMuted"> · {mark}</span>;
}
