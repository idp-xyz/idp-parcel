import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { Button } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['settlement-application'];

/**
 * 已确认外部收付款列表行。字段取 UC-SA-005 输入语义契约「外部资金事实」与
 * 结果语义契约（未分配 / 部分核销 / 完整核销）；接线前没有任何实例数据。
 * 金额与时间是已格式化展示串——格式化归供数方，本页不做币种、精度或时区决策。
 */
export interface ExternalFundsRow {
  /** 外部资金事实稳定身份。相同身份与版本只采用一次；更正形成新有效版本，本列表呈现当前有效采用结果。 */
  id: string;
  /** 收付方向（收款 / 付款）。 */
  direction: string;
  /** 对方：收款事实的付款方或付款事实的收款方；身份由外部事实保存，本页不推断。 */
  counterparty: string;
  /** 责任法人。 */
  legalEntity: string;
  /** 结算账户。未分配收付款可能尚无唯一合法账户，这一格由供数方如实表达缺口，本页不猜。 */
  settlementAccount: string;
  /** 原币金额。 */
  originalAmount: string;
  /** 原币币种。 */
  currency: string;
  /** 已分配金额：各分配片段带方向金额之和。 */
  allocatedAmount: string;
  /** 未分配金额。已分配与未分配之和必须严格等于原币金额（金额守恒），未分配余额不得被尾差或默认费用吞并。 */
  unallocatedAmount: string;
  /** 分配状态：未分配 / 部分核销 / 已核销。存在歧义时保持未分配，不按金额或到期日猜测归属。 */
  allocationStatus: string;
  /** 外部事实的业务时间；不是系统接收或写入时间。 */
  businessTime: string;
}

// 「已分配金额」「未分配金额」并列成栏而不是只留一个净额——部分核销必须保留
// 剩余未结金额，未分配是第一类结果不是尾差（UC-SA-005 结果语义契约）。
const columns: ListColumn<ExternalFundsRow>[] = [
  { id: 'id', header: '资金事实标识', className: 'font-mono', render: (row) => row.id },
  { id: 'direction', header: '方向', align: 'center', className: 'w-[56px]', render: (row) => row.direction },
  { id: 'counterparty', header: '对方', render: (row) => row.counterparty },
  { id: 'legal-entity', header: '责任法人', render: (row) => row.legalEntity },
  { id: 'account', header: '结算账户', render: (row) => row.settlementAccount },
  { id: 'amount', header: '原币金额', align: 'right', className: 'font-mono', render: (row) => row.originalAmount },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[64px]', render: (row) => row.currency },
  { id: 'allocated', header: '已分配金额', align: 'right', className: 'font-mono', render: (row) => row.allocatedAmount },
  { id: 'unallocated', header: '未分配金额', align: 'right', className: 'font-mono', render: (row) => row.unallocatedAmount },
  { id: 'status', header: '分配状态', align: 'center', render: (row) => row.allocationStatus },
  { id: 'time', header: '业务时间', render: (row) => row.businessTime },
  {
    id: 'actions',
    header: '操作',
    align: 'center',
    // 动作只有「人工分配」：自动核销只在匹配结果唯一时由 UC-SA-005 编排形成，
    // 不提供任何「自动分配」入口，歧义收付款只能经人工分配处置。
    // 数据区未配置态不渲染表格，本动作当前不可达；接线时由应用端口接管，
    // 并仅对未分配 / 部分核销行可用。核销撤销随详情呈现另立，不塞进列表。
    render: () => (
      <Button size="sm" disabled>
        人工分配
      </Button>
    ),
  },
];

/**
 * 收付款核销（UC-SA-005 映射外部收付款并形成运营核销）。
 *
 * 独立成页而不是对账页内区块：核销的行对象是已确认外部收付款及其分配关系，
 * 未结项跨客户运营应收、供应商审核应付、赔付/退款义务、供应商费用贷项、
 * 追偿应收与代垫回收，不从属于对账单；CONTEXT「结算账户、对账与争议」明确
 * 收付款事实与核销关系同对账单分别管理，收付款不改变对账单内容。
 * 分配片段明细（目标金额身份、借贷方向、带方向金额、匹配/抵销依据）与
 * 核销撤销届时按 DetailPageTemplate 另立详情，不在本列表展开。
 */
export function SettlementApplicationPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ExternalFundsRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索资金事实标识 / 结算账户',
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
