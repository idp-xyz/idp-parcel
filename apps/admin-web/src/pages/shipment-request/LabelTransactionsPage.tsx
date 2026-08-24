import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { StatusBadgeFor, domainStatusTones, type DomainStatus } from '../../domain/status';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['label-transactions'];

/**
 * 面单交易查阅面。词取 parcel-shipment CONTEXT.md「面单交易」「面单交易包裹结果」
 * 「面单交易定案」「面单继续尝试决定」原词。
 *
 * 行粒度是交易 × 包裹：一笔交易可覆盖多个包裹、一个包裹可因重试/替代/换单关联
 * 多笔交易，且「多包裹面单交易可以具有共同交易结果，也必须分别保存每个包裹的
 * 业务结果」——交易级与包裹级结果分列即这条硬句的表形，任何一级都推断不出另一级。
 *
 * 本页无关闭/重开动作入口：受控关闭与重开是授权业务角色形成的追加式决定
 * （请求方、实际决定方、授权依据快照缺一不可），查阅面不取得决定权；
 * 页面也不设「作废 / 退款」入口——查询、重打、渠道作废、替换和渠道退款是
 * 不同业务动作，各走其责任入口。
 */
export interface LabelTransactionRow {
  /** 面单交易标识。 */
  transactionId: string;
  /** 覆盖包裹（当前行的明确包裹）。 */
  parcelId: string;
  /** 渠道账号持有人。与渠道服务方是不同交易角色，代理关系不自动合并（CONTEXT-MAP 核心区分）。 */
  accountHolder: string;
  /** 渠道服务方。 */
  channelServicer: string;
  /** 合同与结算相对方。 */
  settlementCounterparty: string;
  /** 交易级结果。交易级失败不能推导包裹失败。 */
  transactionResult: string;
  /**
   * 包裹级结果（面单交易包裹结果）：该包裹是否被受理、取得的包裹级标识及适用
   * 渠道作废、替代或渠道退款结果；不能由整笔交易结果或其他包裹结果推断。
   */
  parcelResult: string;
  /**
   * 定案状态：结果待确认 / 已定案。定案只说明交易级及相关包裹级结果已经明确、
   * 不再待确认——定下的可能是失败；渠道退款、对账、运营结算不属于定案条件。
   */
  settlementStatus: string;
  /**
   * 包裹级继续尝试判断：开放 / 受控关闭。受控关闭只表示不允许把该包裹纳入
   * 边界后的新重试、替代或换单，不修改交易结果、不使既有面单失效，
   * 也不直接形成包裹终局。
   */
  continueAttemptJudgment: string;
  /** 业务时间（交易建立的业务发生时间）。 */
  businessTime: string;
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

const columns: ListColumn<LabelTransactionRow>[] = [
  {
    id: 'transaction-id',
    header: '面单交易',
    className: 'w-[160px]',
    render: (row) => <span className="font-mono text-[12px] text-idpxyz-accent">{row.transactionId}</span>,
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
  { id: 'transaction-result', header: '交易级结果', render: (row) => row.transactionResult },
  { id: 'parcel-result', header: '包裹级结果', render: (row) => row.parcelResult },
  {
    id: 'settlement-status',
    header: '定案状态',
    align: 'center',
    className: 'w-[112px]',
    render: (row) => toneWordOrText(row.settlementStatus),
  },
  {
    id: 'continue-attempt',
    header: '继续尝试判断',
    align: 'center',
    className: 'w-[112px]',
    render: (row) => toneWordOrText(row.continueAttemptJudgment),
  },
  {
    id: 'business-time',
    header: '业务时间',
    className: 'w-[150px]',
    render: (row) => <span className="font-mono text-[12px]">{row.businessTime}</span>,
  },
];

export function LabelTransactionsPage() {
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<LabelTransactionRow>
      title={info.title}
      // 页头携带定案语义的硬句：定案 ≠ 成功，受控关闭 ≠ 终局。
      description={`${info.owner}——定案只说明结果不再待确认（定下的可能是失败）；受控关闭只拒绝边界后的新尝试，不使既有面单失效，也不直接形成包裹终局。`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '搜索面单交易标识 / 包裹标识',
      }}
      columns={columns}
      // 接线前无实例：行数据与总数届时由 parcel-shipment 应用端口供给。
      rows={[]}
      rowKey={(row) => `${row.transactionId}#${row.parcelId}`}
      pagination={{
        page,
        pageSize,
        total: 0,
        onPageChange: setPage,
        onPageSizeChange: setPageSize,
      }}
      viewState={{
        kind: 'unconfigured',
        title: '面单交易查询端点尚未建立',
        description: `后端现仅提供提交与决定前撤回两个动作端点，面单交易的查询契约待建；本页不发请求、不含合成数据。场景出处：${info.source}`,
      }}
    />
  );
}
