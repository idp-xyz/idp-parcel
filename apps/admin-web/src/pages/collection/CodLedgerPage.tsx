import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { labelFor } from '../../vocabulary/status';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { listCodSubledgers, type CodSubledgerListResponseBody, type CodSubledgerRecord } from './api';
import { TwoLine, formatMoney, technicalRows } from '../settlement/ops-ui';
import { namedRef, usePartyNames, type PartyNames } from '../settlement/party-names';

// 代收货款页（主责上下文 collection-remittance：代收指令、代收事实、代收分户账、回汇批次与差异事项）。行对象是一本代收分户账
// ——按（货主客户，公司主体，币种，代收渠道）四维隔离的受托保管账，接 GET /collection-subledgers。
const info = moduleInfoById['cod-ledger'];

// 列上只放运营盯的钱：应付客户多少、还有多少在渠道手里没回来、回来了还没清到客户头上的、已经汇出去的、差异。
// 「应付客户」这一栏只能由已接受的真实到账经清分形成——未实际收到的代收款不得进入可付客户余额，表头不再带硬句，
// 规则写在这里。已抵扣、保管依据、记账笔数与公司主体进技术详情。

/** 短款记成负、溢款记成正，一格说清差异方向；两边都是零就说「无」。 */
function differenceText(row: CodSubledgerRecord): string {
  const parts: string[] = [];
  if (row.balances.shortfall !== '0') parts.push(`短 ${formatMoney(row.balances.shortfall, '')}`);
  if (row.balances.surplus !== '0') parts.push(`溢 ${formatMoney(row.balances.surplus, '')}`);
  return parts.join('，') || '无';
}

function ledgerTechnicalRows(row: CodSubledgerRecord) {
  return technicalRows([
    { label: '客户账户', value: row.customer },
    { label: '公司主体', value: row.legalEntity },
    { label: '代收渠道', value: row.channel },
    { label: '保管依据', value: row.custodyBasis },
    // 「已开立但从未记账」与「记账相抵为零」在全零余额上长着同一张脸，笔数是唯一分得开两态的照实转写。
    { label: '记账笔数', value: String(row.postingCount) },
    { label: '已抵扣', value: formatMoney(row.balances.offsetApplied, row.currency) },
    {
      label: '回汇批次',
      value: row.remittanceBatches
        .map((batch) => `${batch.batch}：${labelFor(batch.state)}，归集至 ${formatInstant(batch.collectedThrough)}，形成于 ${formatInstant(batch.formedAt)}`)
        .join('；') || undefined,
    },
    { label: '开立时间', value: formatInstant(row.openedAt) },
  ]);
}

function amountCol(id: string, header: string, pick: (row: CodSubledgerRecord) => string): ListColumn<CodSubledgerRecord> {
  return {
    id,
    header,
    align: 'right',
    className: 'whitespace-nowrap font-mono text-xs tabular-nums',
    render: (row) => formatMoney(pick(row), ''),
  };
}

function columns(names: PartyNames): ListColumn<CodSubledgerRecord>[] {
  return [
    {
      id: 'customer',
      header: '客户',
      className: 'whitespace-nowrap',
      render: (row) => {
        const ref = namedRef(names.account(row.customer), row.customer);
        return <TwoLine primary={ref.primary} secondary={ref.secondary} />;
      },
    },
    { id: 'channel', header: '代收渠道', className: 'whitespace-nowrap font-mono text-xs', render: (row) => row.channel },
    { id: 'currency', header: '币种', align: 'center', className: 'font-mono text-xs', render: (row) => row.currency },
    amountCol('payable', '应付客户', (row) => row.balances.payableToCustomer),
    amountCol('in-transit', '渠道在途', (row) => row.balances.inTransitAtChannel),
    amountCol('awaiting', '待清分', (row) => row.balances.awaitingAllocation),
    amountCol('remitted', '已汇付', (row) => row.balances.remitted),
    { id: 'difference', header: '差异', className: 'whitespace-nowrap text-xs', render: differenceText },
    {
      id: 'batches',
      header: '回汇批次',
      // 空数组如实说「未配置」而不是留白：回汇周期与汇付通道属实例半边、尚未配置。批次在场时逐个给状态胶囊。
      render: (row) =>
        row.remittanceBatches.length > 0 ? (
          <div className="flex flex-wrap gap-1">
            {row.remittanceBatches.map((batch) => (
              <StatusPill key={batch.batch} code={batch.state} label={`${batch.batch} · ${labelFor(batch.state)}`} />
            ))}
          </div>
        ) : (
          <span className="text-idpxyz-textMuted">未配置回汇周期</span>
        ),
    },
    {
      id: 'technical',
      header: '',
      className: 'align-top',
      render: (row) => <TechnicalDetails rows={ledgerTechnicalRows(row)} className="max-w-md" />,
    },
  ];
}

// 行键即分户账主键四维——同一租户内四维组合唯一。
function rowKey(row: CodSubledgerRecord): string {
  return [row.customer, row.legalEntity, row.currency, row.channel].join(':');
}

/**
 * 代收货款分户账：四维齐备才构成一本账，余额由追加式记账派生，账上没有可改写的余额字段。代收本金是客户的钱，不是经营
 * 口径物流收入或普通运营结算余额（COD 服务费与获准抵扣归 settlement-accounting）。查阅面，不设记账动作——开立与记账走受控 CLI。
 */
export function CodLedgerPage() {
  const names = usePartyNames();
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
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleSubledgers = needle
    ? subledgers.filter((row) =>
        [row.customer, names.account(row.customer), row.legalEntity, row.currency, row.channel].some((value) =>
          value?.toLowerCase().includes(needle),
        ),
      )
    : subledgers;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CodSubledgerRecord>
      title="代收货款"
      description="替客户代收的货款分户账：收到多少、清到客户头上多少、汇出去多少"
      search={{ value: search, onChange: setSearch, placeholder: '搜客户、渠道、币种' }}
      filterSummary={answer?.kind === 'outcome' ? `${subledgers.length} 本分户账` : undefined}
      columns={columns(names)}
      rows={visibleSubledgers}
      rowKey={rowKey}
      emptyRowsNote="没有匹配的分户账"
      viewState={catalogueViewState(answer, subledgers.length, retry, {
        module: info,
        endpoint: 'GET /collection-subledgers',
        emptyTitle: '还没有代收货款分户账',
        emptyDescription: '客户开通代收货款服务并开立分户账后在这里出现；页面不会预置分户账。',
      })}
    />
  );
}
