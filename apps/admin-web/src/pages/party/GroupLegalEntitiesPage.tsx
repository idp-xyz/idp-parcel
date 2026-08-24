import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

/**
 * 集团与法人列表行。字段取 party-commercial CONTEXT.md「运营集团租户」「责任法人」
 * 定义与 ADR-0003 的三级边界（集团、法人、客户账户）；接线前没有任何实例数据。
 */
export interface GroupLegalEntityRow {
  /** 对象标识。 */
  id: string;
  /** 运营集团租户：最高业务配置与数据隔离边界，不是责任法人、客户账户或登录组织。 */
  tenant: string;
  /**
   * 对象类型：责任法人 / 经营组织。责任法人不能从集团层级、经营组织或
   * 实际操作人员自动推断，必须显式登记。
   */
  kind: string;
  /** 名称。 */
  name: string;
  /** 业务参与方身份：责任法人同时具有稳定的业务参与方身份，此列呈现其关联。 */
  partyIdentity: string;
}

const columns: ListColumn<GroupLegalEntityRow>[] = [
  { id: 'id', header: '对象标识', className: 'font-mono', render: (row) => row.id },
  { id: 'tenant', header: '运营集团租户', render: (row) => row.tenant },
  { id: 'kind', header: '对象类型', align: 'center', render: (row) => row.kind },
  { id: 'name', header: '名称', render: (row) => row.name },
  { id: 'party-identity', header: '业务参与方身份', render: (row) => row.partyIdentity },
];

/**
 * 集团与法人（party-commercial）。行对象是运营集团租户内的责任法人与经营组织；
 * 集团、法人、客户账户三级边界遵循 ADR-0003，参与方、账户、法人和合同标识
 * 不能互相替代。查阅面，不设登记动作。
 */
export function GroupLegalEntitiesPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<GroupLegalEntityRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索名称 / 业务参与方身份',
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
