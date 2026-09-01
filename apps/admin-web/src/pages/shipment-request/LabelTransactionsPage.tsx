import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { StatusBadgeFor, domainStatusTones, type DomainStatus } from '../../domain/status';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  CONTINUED_ATTEMPT_BASIS_EMPTY_HISTORY,
  listLabelTransactions,
  type ApiResult,
  type LabelTransactionRow,
  type LabelTransactionsListResponseBody,
} from './api';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['label-transactions'];

/**
 * 面单交易查阅面（已接线：GET /label-transactions）。词取 parcel-shipment CONTEXT.md
 * 「面单交易」「面单交易包裹结果」「面单交易定案」「面单继续尝试决定」原词。
 *
 * 行粒度是交易 × 包裹，摊开在服务端读侧完成（ADR-0084 决定七）：一笔交易可覆盖多个包裹、
 * 一个包裹可因重试/替代/换单关联多笔交易，且「多包裹面单交易可以具有共同交易结果，也必须
 * 分别保存每个包裹的业务结果」——交易级与包裹级结果分列即这条硬句的表形，任何一级都推断
 * 不出另一级。
 *
 * 本页无关闭/重开动作入口：受控关闭与重开是授权业务角色形成的追加式决定（请求方、实际
 * 决定方、授权依据快照缺一不可），查阅面不取得决定权；页面也不设「作废 / 退款」入口——
 * 查询、重打、渠道作废、替换和渠道退款是不同业务动作，各走其责任入口。
 */

// 交易级结果的词表：服务端交的是 LabelTransactionState 的封闭枚举原词，这里译回 CONTEXT
// 的中文原词。认不得的值原样示出而不落进任何一格——静默归格会让某天新增的一种结果在页面上
// 冒充另一种，而那正是「不能由一个层次覆盖另一个层次」要防的同一类错。
const transactionResultWords: Record<string, string> = {
  ESTABLISHED: '已建立',
  SUBMITTED_TO_CHANNEL: '已提交渠道',
  RESULT_UNCERTAIN: '结果不确定',
  SUCCEEDED: '成功',
  PARTIALLY_SUCCEEDED: '部分成功',
  FAILED: '失败',
};

const followUpWords: Record<string, string> = {
  CHANNEL_VOID: '渠道作废',
  CHANNEL_REFUND: '渠道退款',
  REPLACEMENT: '替代',
};

const priorLinkWords: Record<string, string> = {
  RETRY: '重试',
  REPLACEMENT: '替代',
};

function wordOr(table: Record<string, string>, value: string): string {
  return table[value] ?? value;
}

// 词表词（结果待确认/已定案/受控关闭）按共享词表着色；词表没收录的词（如
// 继续尝试判断的「开放」）原样示文，不猜色调。断言只桥接类型边界，词同源于
// CONTEXT 原词。
function toneWordOrText(value: string) {
  return value in domainStatusTones ? (
    <StatusBadgeFor status={value as DomainStatus} />
  ) : (
    value
  );
}

/**
 * 包裹级结果的呈现。三态而不是两态：**结果未回**与**未受理**必须分开——把前者显示成
 * 后者，就是在页面上把「结果不确定」按失败处理，CONTEXT 明禁。
 */
function parcelResultText(row: LabelTransactionRow): string {
  if (!row.hasParcelResult) return '结果未回';
  const base = row.parcelAccepted
    ? `已受理${row.parcelIdentifier ? ` · ${row.parcelIdentifier}` : ''}`
    : `未受理${row.parcelResultReason ? ` · ${row.parcelResultReason}` : ''}`;
  if (row.followUpKinds.length === 0) return base;
  // 后续动作与原结果并列而不改写它：渠道作废与「当初有没有被受理」是两件事。
  return `${base}（${row.followUpKinds.map((kind) => wordOr(followUpWords, kind)).join('、')}）`;
}

const columns: ListColumn<LabelTransactionRow>[] = [
  {
    id: 'transaction-id',
    header: '面单交易',
    className: 'w-[170px]',
    render: (row) => (
      <div className="flex flex-col gap-0.5">
        <span className="font-mono text-[12px] text-idpxyz-accent">{row.transactionId}</span>
        {row.priorTransactionId && (
          <span className="font-mono text-[11px] text-idpxyz-muted">
            {wordOr(priorLinkWords, row.priorLinkKind ?? '')}自 {row.priorTransactionId}
          </span>
        )}
      </div>
    ),
  },
  {
    id: 'parcel-id',
    header: '覆盖包裹',
    className: 'w-[150px]',
    render: (row) => <span className="font-mono text-[12px]">{row.parcelId}</span>,
  },
  { id: 'account-holder', header: '渠道账号持有人', render: (row) => row.accountHolder },
  { id: 'channel-servicer', header: '渠道服务方', render: (row) => row.channelServicer },
  { id: 'counterparty', header: '合同与结算相对方', render: (row) => row.settlementCounterparty },
  {
    id: 'transaction-result',
    header: '交易级结果',
    render: (row) => wordOr(transactionResultWords, row.transactionResult),
  },
  { id: 'parcel-result', header: '包裹级结果', render: (row) => parcelResultText(row) },
  {
    id: 'settlement-status',
    header: '定案状态',
    align: 'center',
    className: 'w-[112px]',
    render: (row) => toneWordOrText(row.finalized ? '已定案' : '结果待确认'),
  },
  {
    id: 'continue-attempt',
    header: '继续尝试判断',
    align: 'center',
    className: 'w-[112px]',
    render: (row) => toneWordOrText(row.continuedAttemptOpen ? '开放' : '受控关闭'),
  },
  {
    id: 'business-time',
    header: '业务时间',
    className: 'w-[170px]',
    render: (row) => <span className="font-mono text-[12px]">{formatInstant(row.establishedAt)}</span>,
  },
];

export function LabelTransactionsPage() {
  const [keyword, setKeyword] = useState('');
  const [reloadToken, setReloadToken] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<LabelTransactionsListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listLabelTransactions().then((result) => {
      if (!cancelled) setAnswer(result);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body?.rows ?? [];
  const needle = keyword.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        [row.transactionId, row.parcelId].some((field) => field.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadToken((token) => token + 1);

  // 页头必须转述服务端给的派生依据：整列「开放」派生自一段真实为空的决定历史（继续尝试
  // 决定登记册另票落地），不说明就会被读成「已核对过关闭册」。
  const continuedAttemptNote =
    body?.continuedAttemptBasis === CONTINUED_ATTEMPT_BASIS_EMPTY_HISTORY
      ? '「继续尝试判断」按规则派生自当前决定历史，而继续尝试决定登记册尚未落地——此刻这段历史真实为空，因此整列派生为「开放」，不代表已核对过关闭册。'
      : '';

  return (
    <ListPageTemplate<LabelTransactionRow>
      title={info.title}
      // 页头携带定案语义的硬句：定案 ≠ 成功，受控关闭 ≠ 终局。
      description={`${info.owner}——定案只说明结果不再待确认（定下的可能是失败）；受控关闭只拒绝边界后的新尝试，不使既有面单失效，也不直接形成包裹终局。${continuedAttemptNote}`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '搜索面单交易标识 / 包裹标识',
      }}
      filterSummary={answer?.kind === 'outcome' ? `共 ${visibleRows.length} 行` : undefined}
      columns={columns}
      rows={visibleRows}
      rowKey={(row) => `${row.transactionId}#${row.parcelId}`}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /label-transactions',
        emptyTitle: '当前租户没有面单交易',
        emptyDescription:
          '读取入口已配置、登记册为空——面单交易由渠道适配器写入，而独立面单渠道服务不进入首发生产，渠道接入落地前本册零行是设计而不是缺陷。',
      })}
    />
  );
}
