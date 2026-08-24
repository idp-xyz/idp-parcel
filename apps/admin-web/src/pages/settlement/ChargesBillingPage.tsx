import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['charges-billing'];

/**
 * 费用明细列表行。字段取 settlement-accounting CONTEXT.md「费用明细」定义与
 * 「费用形成与证据」规则里确认费用必须固定的属性；接线前没有任何实例数据。
 * 金额是已格式化展示串——格式化归供数方，本页不做币种或精度决策。
 */
export interface ChargeDetailRow {
  /** 费用明细标识。 */
  id: string;
  /** 费用项目：对经济含义的稳定定义，不包含某次交易实际使用的价格和金额。 */
  feeItem: string;
  /** 主要计费范围：一条费用明细必须且只能有一个。 */
  primaryScope: string;
  /** 收付方向。 */
  direction: string;
  /** 责任法人。 */
  legalEntity: string;
  /** 结算相对方。 */
  counterparty: string;
  /**
   * 阶段：预估 / 暂估 / 确认 / 调整。追加式生命周期，四者分别保留，
   * 不用一个可覆盖金额表达；确认后的变化只能追加调整明细或新版本。
   */
  stage: string;
  /** 版本：确认后出现新事实、源事实更正或规则适用性变化时形成新版本，不覆盖历史结果。 */
  version: string;
  /**
   * 计费重量采用：客户计费重量或供应商计费重量的财务采用结果。
   * 两者分别依据各自合同与计量规则形成，任一方不改写另一方，也不覆盖原始测量。
   */
  billingWeight: string;
  /** 原币金额。 */
  originalAmount: string;
  /**
   * 合同结算币金额。原币、合同结算币与换算依据是从同一个评价采用来的一组，
   * 整组重述不拆散；原币与合同结算币相同时两个金额必须相等。
   */
  settlementAmount: string;
  /**
   * 依据：实际采用的 PricingEvaluation 与采用解释。无法解释命中规则、计算顺序
   * 或责任范围的金额不得成为确认费用。调整行的依据另含调整类型、业务原因、
   * 权威依据与唯一创建用例（见 CONTEXT「调整类型与唯一所有权」表）。
   */
  basis: string;
}

const columns: ListColumn<ChargeDetailRow>[] = [
  { id: 'id', header: '费用明细标识', className: 'font-mono', render: (row) => row.id },
  { id: 'fee-item', header: '费用项目', render: (row) => row.feeItem },
  { id: 'primary-scope', header: '主要计费范围', render: (row) => row.primaryScope },
  { id: 'direction', header: '收付方向', align: 'center', className: 'w-[72px]', render: (row) => row.direction },
  { id: 'legal-entity', header: '责任法人', render: (row) => row.legalEntity },
  { id: 'counterparty', header: '结算相对方', render: (row) => row.counterparty },
  { id: 'stage', header: '阶段', align: 'center', className: 'w-[64px]', render: (row) => row.stage },
  { id: 'version', header: '版本', align: 'center', className: 'w-[56px] font-mono', render: (row) => row.version },
  { id: 'billing-weight', header: '计费重量采用', align: 'right', className: 'font-mono', render: (row) => row.billingWeight },
  { id: 'original-amount', header: '原币金额', align: 'right', className: 'font-mono', render: (row) => row.originalAmount },
  { id: 'settlement-amount', header: '合同结算币金额', align: 'right', className: 'font-mono', render: (row) => row.settlementAmount },
  { id: 'basis', header: '依据', render: (row) => row.basis },
];

/**
 * 费用与计费（settlement-accounting）。行对象是费用明细：预估、暂估、确认与
 * 调整分别保留成行，版本与依据（实际 PricingEvaluation 及采用解释）随行呈现。
 * 本页是查阅面，不设任何调整创建动作——每类调整只能由「调整类型与唯一所有权」
 * 表指定的唯一创建用例形成，页面入口不取得创建权。
 */
export function ChargesBillingPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ChargeDetailRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索费用明细标识 / 费用项目 / 主要计费范围',
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
        title: '结算模块尚未接线',
        description: `业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
