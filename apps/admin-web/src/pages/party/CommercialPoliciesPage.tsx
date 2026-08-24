import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['commercial-policies'];

/**
 * 商业规则与政策版本列表行。字段取 party-commercial CONTEXT.md「商业版本」
 * 定义与「商业规则与政策版本」生命周期；接线前没有任何实例数据。
 */
export interface CommercialPolicyVersionRow {
  /** 版本标识：商业版本具有稳定身份、对象类型、适用范围、有效区间、来源与批准依据。 */
  id: string;
  /**
   * 对象类型：接单规则包 / 接受前财务控制策略 / 商业价格政策 / 结算政策 /
   * 信用政策 / 税务分类依据 / 客户服务规则。一列共览不是合并——各对象保持
   * 独立身份，不因此合并为一份「大配置」，本页只是查阅面。
   */
  policyKind: string;
  /** 适用对象与范围：责任法人、客户或供应商相对方、服务/费用/关务范围等，由各对象自行声明。 */
  appliesTo: string;
  /** 版本。 */
  version: string;
  /** 有效区间：重叠候选不是「同时生效」，而是必须阻断解析的适用冲突。 */
  validity: string;
  /** 状态：草稿 / 已发布 / 已生效 / 已到期 / 已退役 / 已替代。发布后正文不可覆盖。 */
  status: string;
  /** 来源与批准依据。 */
  basis: string;
}

const columns: ListColumn<CommercialPolicyVersionRow>[] = [
  { id: 'id', header: '版本标识', className: 'font-mono', render: (row) => row.id },
  { id: 'policy-kind', header: '对象类型', render: (row) => row.policyKind },
  { id: 'applies-to', header: '适用对象与范围', render: (row) => row.appliesTo },
  { id: 'version', header: '版本', align: 'center', className: 'w-[56px] font-mono', render: (row) => row.version },
  { id: 'validity', header: '有效区间', render: (row) => row.validity },
  { id: 'status', header: '状态', align: 'center', render: (row) => row.status },
  { id: 'basis', header: '来源与批准依据', render: (row) => row.basis },
];

/**
 * 商业规则与策略（party-commercial）。行对象是商业规则与政策的不可覆盖版本。
 * 同一解析键和商业选择锚点下每种必需商业依据必须唯一适用：零候选是无适用
 * 依据、多候选是适用冲突、权威读取失败是解析未决——任何一种都不得由系统
 * 任选一条、默认通过或伪装成确定性业务拒绝；绑定缺失、过期、区间重叠或
 * 引用未决时不使用默认价。查阅面，不设登记与发布动作。
 */
export function CommercialPoliciesPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<CommercialPolicyVersionRow>
      title={info.title}
      description={info.owner}
      // 筛选维度（接线时实装进 filters 槽）：对象类型（接单规则包/接受前财务控制
      // 策略/商业价格政策/结算政策/信用政策/税务分类依据/客户服务规则，行注释的
      // 封闭清单）、状态（草稿/已发布/已生效/已到期/已退役/已替代）。
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索对象类型 / 适用对象',
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
