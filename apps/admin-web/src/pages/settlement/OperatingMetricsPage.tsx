import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['operating-metrics'];

/**
 * 经营指标列表行。字段取 settlement-accounting CONTEXT.md「经营毛利」定义与
 * 「经营指标、人工调整与系统边界」规则；接线前没有任何实例数据。
 * 金额与时点是已格式化展示串——格式化归供数方，本页不做币种、精度或时区决策。
 */
export interface OperatingMetricRow {
  /** 指标快照标识。 */
  id: string;
  /** 分析范围：包裹、客户、产品、线路等经营归因范围。 */
  scope: string;
  /**
   * 口径：预估 / 已确认 / 已结算。客户与供应商基础按阶段成对采用；
   * 索赔调整后口径必须声明依附哪一基础口径，不能形成混合阶段。
   */
  caliber: string;
  /** 计算版本：经营毛利必须保存计算版本、币种和截至时点。 */
  version: string;
  /** 币种。 */
  currency: string;
  /** 截至时点：账期快照不得被迟到费用静默覆盖。 */
  asOf: string;
  /**
   * 客户侧采用金额。预估口径采用当前有效客户预估费用；已确认口径采用当前有效
   * 客户运营应收；已结算口径只采用 UC-SA-005 形成的运营核销分配范围。
   */
  customerBasis: string;
  /** 供应商侧采用金额：预估口径为当前有效供应商预期成本；已确认口径见下两个拆列。 */
  supplierBasis: string;
  /**
   * 其中：审核应付。与供应商费用贷项按各自借贷方向分别计入一次——
   * 「当前有效审核应付」不得被解释为已经静默净含贷项。
   */
  auditedPayable: string;
  /**
   * 其中：供应商费用贷项。与审核应付两栏并列呈现组成，不预先净额；
   * 预估口径两拆列不适用，由供数方如实留白，不填零冒充。
   */
  supplierCredit: string;
  /** 经营毛利：只能派生，不能直接修改；不等同于法定会计利润。 */
  grossMargin: string;
  /** 经营损失：同经营毛利，只能派生。 */
  operatingLoss: string;
}

// 「其中：审核应付」「其中：供应商费用贷项」拆列并置——组成可见是 CONTEXT 的
// 硬要求：经营口径必须按贷项身份、有效性、借贷方向和截至时点单独采用一次，
// 并保留原应付关系；净付款或资金映射本身不能替代这些关系。
const columns: ListColumn<OperatingMetricRow>[] = [
  { id: 'scope', header: '分析范围', render: (row) => row.scope },
  { id: 'caliber', header: '口径', align: 'center', className: 'w-[72px]', render: (row) => row.caliber },
  { id: 'version', header: '计算版本', align: 'center', className: 'font-mono', render: (row) => row.version },
  { id: 'currency', header: '币种', align: 'center', className: 'w-[64px]', render: (row) => row.currency },
  { id: 'as-of', header: '截至时点', render: (row) => row.asOf },
  { id: 'customer-basis', header: '客户侧采用', align: 'right', className: 'font-mono', render: (row) => row.customerBasis },
  { id: 'supplier-basis', header: '供应商侧采用', align: 'right', className: 'font-mono', render: (row) => row.supplierBasis },
  { id: 'audited-payable', header: '其中：审核应付', align: 'right', className: 'font-mono', render: (row) => row.auditedPayable },
  { id: 'supplier-credit', header: '其中：供应商费用贷项', align: 'right', className: 'font-mono', render: (row) => row.supplierCredit },
  { id: 'gross-margin', header: '经营毛利', align: 'right', className: 'font-mono', render: (row) => row.grossMargin },
  { id: 'operating-loss', header: '经营损失', align: 'right', className: 'font-mono', render: (row) => row.operatingLoss },
];

/**
 * 经营核算（settlement-accounting）。行对象是按明确口径、计算版本、币种与
 * 截至时点派生的经营毛利/经营损失指标快照。指标只能派生不能直接修改，
 * 本页是查阅面，不设任何指标编辑动作。
 */
export function OperatingMetricsPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<OperatingMetricRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索分析范围 / 计算版本',
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
