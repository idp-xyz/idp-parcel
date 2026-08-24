import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['party-contracts'];

/**
 * 客户合同版本列表行。字段取 party-commercial CONTEXT.md「货主客户账户」
 * 「客户合同版本」定义与商业版本共同不变量；接线前没有任何实例数据。
 */
export interface CustomerContractRow {
  /** 合同版本标识。 */
  id: string;
  /** 货主客户账户：必须明确关联其客户参与方；不是运营法人、经营组织或登录用户。 */
  customerAccount: string;
  /** 责任法人：合同版本必须明确关联责任法人参与方。 */
  legalEntity: string;
  /** 服务范围。 */
  serviceScope: string;
  /**
   * 引用规则包与策略：每个用于新委托接受的客户合同版本必须明确引用适用
   * 接单规则包和接受前财务控制策略；规则或策略缺失不得被解释为允许接受。
   */
  ruleReferences: string;
  /** 版本：客户合同独立于服务产品版本演进，新服务产品版本不自动修改既有合同。 */
  version: string;
  /** 适用期间：每个合同版本只在其明确适用期内用于接受新的委托或形成新的商业决定。 */
  validity: string;
  /** 状态：草稿 / 已发布 / 已生效 / 已到期 / 已替代。发布后正文不可覆盖。 */
  status: string;
}

const columns: ListColumn<CustomerContractRow>[] = [
  { id: 'id', header: '合同版本标识', className: 'font-mono', render: (row) => row.id },
  { id: 'customer-account', header: '货主客户账户', render: (row) => row.customerAccount },
  { id: 'legal-entity', header: '责任法人', render: (row) => row.legalEntity },
  { id: 'service-scope', header: '服务范围', render: (row) => row.serviceScope },
  { id: 'rule-references', header: '引用规则包与策略', render: (row) => row.ruleReferences },
  { id: 'version', header: '版本', align: 'center', className: 'w-[56px] font-mono', render: (row) => row.version },
  { id: 'validity', header: '适用期间', render: (row) => row.validity },
  { id: 'status', header: '状态', align: 'center', render: (row) => row.status },
];

/**
 * 客户与合同（party-commercial）。行对象是客户合同版本：合同版本到期或被
 * 后续版本替代，不改变已经接受委托所保存的合同依据。查阅面，不设登记与
 * 发布动作——商业版本的草稿与发布生命周期归受控登记通道。
 */
export function PartyContractsPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<CustomerContractRow>
      title={info.title}
      description={info.owner}
      // 筛选维度（接线时实装进 filters 槽）：责任法人、状态（草稿/已发布/已生效/
      // 已到期/已替代，商业版本生命周期封闭词）、适用期间时点。
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索货主客户账户 / 责任法人',
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
        description: '业务端点按 ADR-0017 的准入闸门尚未放行，本页不发请求、不含未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '对应查询端点经 ADR-0017 准入闸门放行后接线',
        },
      }}
    />
  );
}
