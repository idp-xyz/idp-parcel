import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['pricing-evaluation'];

/**
 * 价格评价列表行。字段取 parcel-pricing CONTEXT.md「价格评价」定义与评价必带轴：
 * 每次评价必须明确租户、主要业务范围、价格方向、计算目的、计价基准时点和版本清单。
 * 选中事实、费用行、解释与版本清单全文属详情区，接线时另行呈现。
 */
export interface PricingEvaluationRow {
  /** 评价引用。回放形成新的评价引用，不复用原评价 ID，原评价不发生状态迁移。 */
  evaluationRef: string;
  /** 评价对象：已受理包裹或试算对象；对象种类进入评价语义摘要，呈现时不得省略种类。 */
  subject: string;
  /** 价格方向（BUY / SELL / INTERNAL）。方向隔离：一个方向的结论不推导另一方向。 */
  direction: string;
  /** 计算目的。 */
  purpose: string;
  /** 计价基准时点。 */
  basisTime: string;
  /** 结果：已完成 / 待判断 / 冲突 / 不可计价 / 未形成——四种非完成结果不得互相冒充。 */
  outcome: string;
  /**
   * 结算币种金额，仅已完成评价有值。不可计价不含金额与费用行，这一列必须留空——
   * 「不得以金额为零的已完成评价表达不可计价」是 CONTEXT 硬句，展示层同样不得用 0 顶替。
   */
  settlementAmount: string;
  /** 版本清单引用。重放必须使用原版本清单，不读取当前最新版本替代。 */
  manifest: string;
}

const columns: ListColumn<PricingEvaluationRow>[] = [
  { id: 'ref', header: '评价引用', className: 'font-mono', render: (row) => row.evaluationRef },
  { id: 'subject', header: '评价对象', render: (row) => row.subject },
  { id: 'direction', header: '价格方向', align: 'center', className: 'w-[88px] font-mono', render: (row) => row.direction },
  { id: 'purpose', header: '计算目的', className: 'font-mono', render: (row) => row.purpose },
  { id: 'basis', header: '计价基准时点', render: (row) => row.basisTime },
  { id: 'outcome', header: '结果', align: 'center', render: (row) => row.outcome },
  { id: 'amount', header: '结算币种金额', align: 'right', className: 'font-mono', render: (row) => row.settlementAmount },
  { id: 'manifest', header: '版本清单', className: 'font-mono', render: (row) => row.manifest },
];

/**
 * 价格评价：已保存评价的查阅面，服务 CONTEXT 点名的争议复核与回放场景。
 * 页面没有「重算 / 修改」动作——已完成评价不可变，回放以原版本清单形成新的评价引用；
 * 接线时若提供回放入口，它属「发起一次新评价」而非修改本行，交互届时按该语义另定。
 */
export function PricingEvaluationsPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<PricingEvaluationRow>
      title={info.title}
      description={info.owner}
      // 筛选维度（接线时实装进 filters 槽）：价格方向（BUY/SELL/INTERNAL 封闭三向，
      // 方向隔离）、计算目的、结果（已完成/待判断/冲突/不可计价/未形成——封闭五格，
      // 四种非完成结果不得互相冒充）、计价基准时点窗口。评价引用与对象经搜索。
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索评价引用 / 评价对象',
      }}
      columns={columns}
      // 接线前无实例：评价用例（evaluate_pricing）尚未接入任何进程，没有运行时评价可查。
      rows={[]}
      rowKey={(row) => row.evaluationRef}
      pagination={{
        page,
        pageSize,
        total: 0,
        onPageChange: setPage,
        onPageSizeChange: setPageSize,
      }}
      viewState={{
        kind: 'unconfigured',
        title: '计价模块尚未接线',
        description: '评价由计价用例按版本清单在进程内形成；该用例与本页的查询端点当前都未接入进程，本页不发请求、不含合成数据与未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '计价用例（evaluate_pricing）与查询端点接入进程并经 ADR-0017 准入闸门放行后接线',
        },
      }}
    />
  );
}
