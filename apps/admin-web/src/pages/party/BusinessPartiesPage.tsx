import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['business-parties'];

/**
 * 参与方关系列表行。字段取 party-commercial CONTEXT.md「业务参与方」「参与方关系」
 * 定义——关系必须保存双方、角色、方向、适用范围、依据和有效区间；
 * 接线前没有任何实例数据。
 */
export interface PartyRelationRow {
  /** 关系标识。 */
  id: string;
  /** 参与方：具有稳定身份的企业、组织或个人。 */
  party: string;
  /** 相对参与方。 */
  counterparty: string;
  /**
   * 角色：客户、供应商、承运商代理、转售、渠道账号持有等。角色单独成列而不
   * 并入参与方身份——参与方不因一次交易角色被永久定义为货主、代理商或承运商；
   * 承运商代理商是业务关系和交易角色，不是永久企业类型。
   */
  role: string;
  /** 方向。 */
  direction: string;
  /** 适用范围。 */
  scope: string;
  /** 依据。 */
  basis: string;
  /** 版本：关系内容、角色或范围变化形成新关系版本，不原地改写此前有效事实。 */
  version: string;
  /** 有效区间。关系是时态事实，不能仅从名称、品牌或技术账号推断。 */
  validity: string;
  /** 状态：候选关系 / 已生效 / 已到期 / 已撤销 / 已替代。撤销或到期只影响生效边界后的新决定。 */
  status: string;
}

const columns: ListColumn<PartyRelationRow>[] = [
  { id: 'id', header: '关系标识', className: 'font-mono', render: (row) => row.id },
  { id: 'party', header: '参与方', render: (row) => row.party },
  { id: 'counterparty', header: '相对参与方', render: (row) => row.counterparty },
  { id: 'role', header: '角色', align: 'center', render: (row) => row.role },
  { id: 'direction', header: '方向', align: 'center', className: 'w-[64px]', render: (row) => row.direction },
  { id: 'scope', header: '适用范围', render: (row) => row.scope },
  { id: 'basis', header: '依据', render: (row) => row.basis },
  { id: 'version', header: '版本', align: 'center', className: 'w-[56px] font-mono', render: (row) => row.version },
  { id: 'validity', header: '有效区间', render: (row) => row.validity },
  { id: 'status', header: '状态', align: 'center', render: (row) => row.status },
];

/**
 * 业务参与方（party-commercial）。行对象是参与方关系：承运商、承运商代理商、
 * 转售商、聚合平台与渠道账号持有人都以「双方 + 角色 + 有效区间」的时态关系
 * 表达，代理关系不自动合并交易角色。渠道账号持有人、渠道服务方、合同与
 * 结算相对方、底层承运商、实际承运商和责任承担方必须分别表达。
 * 查阅面，不设登记动作。
 */
export function BusinessPartiesPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<PartyRelationRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索参与方 / 角色',
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
