import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['service-products'];

/**
 * 服务产品版本列表行。字段取 party-commercial CONTEXT.md「服务产品」
 * 「服务产品版本」「产品—渠道映射」「渠道账号使用授权」定义；
 * 接线前没有任何实例数据。
 */
export interface ServiceProductVersionRow {
  /** 服务产品版本标识。 */
  id: string;
  /** 服务产品：运营企业面向货主客户定义和销售的国际小包服务。 */
  product: string;
  /**
   * 服务形态：网络服务产品 / 面单渠道服务。网络服务产品是服务产品的一种
   * 服务形态，不是与服务产品、渠道产品并列的第三类产品对象；面单渠道服务
   * 不虚构运营企业已经收寄包裹。
   */
  serviceForm: string;
  /** 适用范围。 */
  scope: string;
  /** 产品—渠道映射：定义候选范围，不代表某次交易已经选择或使用了其中一个渠道产品。 */
  channelMappings: string;
  /** 渠道账号使用授权：必须显式、可撤销并可追溯；取得账号凭据不等于取得业务使用授权。 */
  accountAuthorizations: string;
  /** 版本：已发布版本不得原地覆盖，后续修改通过新版本表达。 */
  version: string;
  /** 有效区间。 */
  validity: string;
  /**
   * 状态：草稿 / 已发布 / 已退役。退役只停止参与新的商业选择，不终止已接受的
   * 委托；监管或安全暂停与本生命周期正交，单独记录依据与范围，不得从退役推断。
   */
  status: string;
}

const columns: ListColumn<ServiceProductVersionRow>[] = [
  { id: 'id', header: '服务产品版本标识', className: 'font-mono', render: (row) => row.id },
  { id: 'product', header: '服务产品', render: (row) => row.product },
  { id: 'service-form', header: '服务形态', align: 'center', render: (row) => row.serviceForm },
  { id: 'scope', header: '适用范围', render: (row) => row.scope },
  { id: 'channel-mappings', header: '产品—渠道映射', render: (row) => row.channelMappings },
  { id: 'account-authorizations', header: '渠道账号使用授权', render: (row) => row.accountAuthorizations },
  { id: 'version', header: '版本', align: 'center', className: 'w-[56px] font-mono', render: (row) => row.version },
  { id: 'validity', header: '有效区间', render: (row) => row.validity },
  { id: 'status', header: '状态', align: 'center', render: (row) => row.status },
];

/**
 * 服务产品与渠道（party-commercial）。行对象是服务产品版本及其映射与授权
 * 引用：一个服务产品版本可以映射一个或多个渠道产品，不得把某个渠道产品
 * 永久写成该服务产品的唯一实现。查阅面，不设登记与发布动作。
 */
export function ServiceProductsPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ServiceProductVersionRow>
      title={info.title}
      description={info.owner}
      // 筛选维度（接线时实装进 filters 槽）：服务形态（网络服务产品/面单渠道服务，
      // CONTEXT 封闭词）、状态（草稿/已发布/已退役——暂停与生命周期正交，不混入状态筛选）。
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索服务产品 / 适用范围',
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
