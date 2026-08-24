import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['cod-ledger'];

/**
 * 代收分户账列表行。collection-remittance 尚无独立 CONTEXT.md，字段取
 * docs/domain/CONTEXT-MAP.md 该上下文「拥有」清单的原词；接线前没有任何实例数据。
 * 金额是已格式化展示串——格式化归供数方，本页不做币种或精度决策。
 */
export interface CodLedgerRow {
  /** 分户标识。 */
  id: string;
  /** 货主客户。 */
  customer: string;
  /** 责任法人。 */
  legalEntity: string;
  /** 代收渠道。 */
  channel: string;
  /** 币种。分户四维（客户、责任法人、币种、代收渠道）之间隔离与对账，不得合并。 */
  currency: string;
  /** 代收资金义务：依据客户代收服务要求建立。 */
  codObligation: string;
  /** 渠道在途代收款：渠道已报告但运营企业尚未实际收到。 */
  inTransitAmount: string;
  /** 已接受实收金额：关联银行或支付事实后确认的真实到账。 */
  receivedAmount: string;
  /** 待清分款。 */
  pendingClearance: string;
  /**
   * 应付客户款：只能由已接受实收金额经清分形成——未实际收到的代收款
   * 不得提前确认为可付客户余额（CONTEXT-MAP 列为本上下文「不拥有」的权力）。
   */
  payableToCustomer: string;
  /** 客户汇付。 */
  remittance: string;
}

// 「渠道在途代收款」与「应付客户款」是两个必须分开的资金桶：在途表头直接
// 标注「未实际收到」，让硬约束在栏目命名上站住，不靠使用者记规则。
// CONTEXT-MAP 还列有短款与溢款分户结果，届时随分户明细补列，本骨架先立
// 义务 → 在途 → 实收 → 待清分 → 应付客户 → 汇付这条主链。
const columns: ListColumn<CodLedgerRow>[] = [
  { id: 'id', header: '分户标识', className: 'font-mono', render: (row) => row.id },
  { id: 'customer', header: '货主客户', render: (row) => row.customer },
  { id: 'legal-entity', header: '责任法人', render: (row) => row.legalEntity },
  { id: 'channel', header: '代收渠道', render: (row) => row.channel },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[64px]', render: (row) => row.currency },
  { id: 'obligation', header: '代收资金义务', align: 'right', className: 'font-mono', render: (row) => row.codObligation },
  { id: 'in-transit', header: '渠道在途代收款（未实际收到）', align: 'right', className: 'font-mono', render: (row) => row.inTransitAmount },
  { id: 'received', header: '已接受实收金额', align: 'right', className: 'font-mono', render: (row) => row.receivedAmount },
  { id: 'pending-clearance', header: '待清分款', align: 'right', className: 'font-mono', render: (row) => row.pendingClearance },
  { id: 'payable', header: '应付客户款（仅实收后形成）', align: 'right', className: 'font-mono', render: (row) => row.payableToCustomer },
  { id: 'remittance', header: '客户汇付', align: 'right', className: 'font-mono', render: (row) => row.remittance },
];

/**
 * 代收分户账（collection-remittance）。行对象是按客户、责任法人、币种与
 * 代收渠道隔离的代收本金分户；代收本金是客户商品交易资金，不是经营口径
 * 物流收入或普通运营结算余额（COD 服务费与获准抵扣归 settlement-accounting）。
 */
export function CodLedgerPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<CodLedgerRow>
      title={info.title}
      // 页头说明直接携带硬约束，与在途/应付两栏的命名互为呼应。
      description={`${info.owner}——未实际收到的代收款不得进入可付客户余额`}
      // 筛选维度（接线时实装进 filters 槽）：分户四维即筛选四维——货主客户、责任
      // 法人、币种、代收渠道（四维之间隔离与对账，不得合并，筛选也按维分立）。
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索货主客户 / 代收渠道',
      }}
      columns={columns}
      // 接线前无实例：行数据与总数届时由 collection-remittance 应用端口供给。
      rows={[]}
      rowKey={(row) => row.id}
      pagination={{
        page,
        pageSize,
        total: 0,
        onPageChange: setPage,
        onPageSizeChange: setPageSize,
      }}
      viewState={{
        kind: 'unconfigured',
        title: '代收与清分模块尚未接线',
        description: '业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '代收分户账查询端点建成并经 ADR-0017 准入闸门放行后接线',
        },
      }}
    />
  );
}
