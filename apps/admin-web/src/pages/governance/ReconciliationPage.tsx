import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['reconciliation'];

/**
 * 对账单列表行。字段取 settlement-accounting CONTEXT.md「对账单」定义与
 * 「结算账户、对账与争议」规则中发布后固定的属性；接线前没有任何实例数据。
 * 金额是已格式化展示串——格式化归供数方，本页不做币种或精度决策。
 */
export interface StatementRow {
  /** 对账单号，发布后固定。 */
  id: string;
  /** 结算账户（固定责任法人、结算相对方、收付方向、结算币种）。 */
  settlementAccount: string;
  /** 结算周期。 */
  period: string;
  /** 结算币种。 */
  currency: string;
  /** 对账单总额快照；总额必须严格等于所含费用明细结算金额之和。 */
  totalAmount: string;
  /** 单据状态。 */
  docStatus: string;
  /** 对账状态。 */
  reconStatus: string;
  /** 结清状态。 */
  clearStatus: string;
}

// 单据状态、对账状态、结清状态是三列而不是一列合并——CONTEXT.md 明确三者
// 「分别表达」：结算相对方争议不撤销对账单，收付款也不改变对账单内容。
const columns: ListColumn<StatementRow>[] = [
  { id: 'id', header: '对账单号', className: 'font-mono', render: (row) => row.id },
  { id: 'account', header: '结算账户', render: (row) => row.settlementAccount },
  { id: 'period', header: '结算周期', render: (row) => row.period },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[64px]', render: (row) => row.currency },
  { id: 'total', header: '对账单总额', align: 'right', className: 'font-mono', render: (row) => row.totalAmount },
  { id: 'doc', header: '单据状态', align: 'center', render: (row) => row.docStatus },
  { id: 'recon', header: '对账状态', align: 'center', render: (row) => row.reconStatus },
  { id: 'clear', header: '结清状态', align: 'center', render: (row) => row.clearStatus },
];

/**
 * 对账与核销。本页承载对账单（UC-SA-003）视角的列表骨架；核销（收付款分配到
 * 未结项）已按 UC-SA-005 口径独立成页，见同目录 SettlementApplicationPage——
 * 收付款事实与核销关系同对账单分别管理，收付款不改变对账单内容，
 * 故不在本页另立区块。
 */
export function ReconciliationPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<StatementRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索对账单号 / 结算账户',
      }}
      columns={columns}
      // 接线前无实例：行数据与总数届时由 settlement-accounting 应用端口供给。
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
        title: '治理模块尚未接线',
        description: `业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
