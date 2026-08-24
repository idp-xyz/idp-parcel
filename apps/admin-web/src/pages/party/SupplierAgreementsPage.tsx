import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['supplier-agreements'];

/**
 * 供应商商业协议版本列表行。字段取 party-commercial CONTEXT.md
 * 「供应商商业协议版本」定义与其生命周期；接线前没有任何实例数据。
 */
export interface SupplierAgreementRow {
  /** 协议版本标识。 */
  id: string;
  /** 责任法人。 */
  legalEntity: string;
  /** 供应商：明确供应商、代理商或其他服务提供方。 */
  supplier: string;
  /** 采购服务范围。 */
  serviceScope: string;
  /** 采购价格条件：销售、采购和法人间价格规则分别表达，一个方向的变化不改写另一方向。 */
  pricingTerms: string;
  /** 结算条件。 */
  settlementTerms: string;
  /** 版本：采购服务、价格、结算或责任条件变化时形成新版本，不覆盖原版本。 */
  version: string;
  /** 适用期间：协议在批准生效后才能用于新的采购决定和供应商预期成本计算。 */
  validity: string;
  /** 状态：到期、终止或被替代后不改变已形成的运输委托、履约事实、账单主张或审核应付依据。 */
  status: string;
}

const columns: ListColumn<SupplierAgreementRow>[] = [
  { id: 'id', header: '协议版本标识', className: 'font-mono', render: (row) => row.id },
  { id: 'legal-entity', header: '责任法人', render: (row) => row.legalEntity },
  { id: 'supplier', header: '供应商', render: (row) => row.supplier },
  { id: 'service-scope', header: '采购服务范围', render: (row) => row.serviceScope },
  { id: 'pricing-terms', header: '采购价格条件', render: (row) => row.pricingTerms },
  { id: 'settlement-terms', header: '结算条件', render: (row) => row.settlementTerms },
  { id: 'version', header: '版本', align: 'center', className: 'w-[56px] font-mono', render: (row) => row.version },
  { id: 'validity', header: '适用期间', render: (row) => row.validity },
  { id: 'status', header: '状态', align: 'center', render: (row) => row.status },
];

/**
 * 供应商协议（party-commercial）。行对象是供应商商业协议版本：它定义可复用的
 * 采购条件、价格和结算责任，不等于一次实际运输委托、订舱、履约事实或供应商
 * 账单——实际委托快照归 transport-fulfillment，成本与应付归 settlement-accounting。
 * 查阅面，不设登记与发布动作。
 */
export function SupplierAgreementsPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<SupplierAgreementRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索供应商 / 责任法人',
      }}
      columns={columns}
      // 接线前无实例：行数据与总数届时由 party-commercial 应用端口供给。
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
        title: '参与方与商业模块尚未接线',
        description: `业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
