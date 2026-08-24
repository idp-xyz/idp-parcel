import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['compliance-rules'];

/**
 * 合规规则库列表行。字段取 customs-compliance CONTEXT.md 两处原词：
 * 「合规判断与规则版本」——自动判断带规则版本、依据、决定方式和责任角色；
 * 「外部结果与规则时效」硬句——关务规则版本必须记录适用辖区、法定生效区间
 * 和规则声明的适用时点，案件创建时间、消息到达时间或系统当前时间不能统一
 * 替代规则的法定适用时点。
 *
 * 本页是规则版本的查阅面。规则登记走受控登记口（parcel-customs-register，
 * 六本案件配置登记册；解释规则已按辖区与法定生效区间版本化），不进在线面；
 * 规则正文属实例半边（PAR-CUS-01/02 待提供），本页不含任何预设规则。
 */
export interface ComplianceRuleVersionRow {
  /** 规则引用（版本化标识；新规则形成新版本，原判断及其当时依据保持不变）。 */
  ruleRef: string;
  /** 判断事项：禁限运、商品归类、原产地、申报价值、监管条件、监管凭证适用性等（「合规判断」词条列举）。 */
  matter: string;
  /** 适用辖区（硬句三维之一）。 */
  jurisdiction: string;
  /** 法定生效区间（硬句三维之二；终点缺席表示尚无终点，换版时由后继起点落定）。 */
  statutoryValidity: string;
  /** 规则声明的适用时点（硬句三维之三：解析用哪个业务时点，由规则自己声明）。 */
  declaredApplicabilityInstant: string;
  /** 决定方式：自动形成或授权角色人工形成——两类判断都必须保存决定方式。 */
  decisionMode: string;
  /** 责任角色。 */
  responsibleRole: string;
  /** 追溯边界：新规则不默认追溯，也不默认影响全部未关闭案件（CONTEXT 原句）。 */
  retroactivityBoundary: string;
}

// 适用辖区、法定生效区间与适用时点三列并排——硬句要求三维齐备，缺任何一维的
// 规则版本在选择侧都答不出「该用哪一版」；这三列不是元数据装饰，是选版的键。
const columns: ListColumn<ComplianceRuleVersionRow>[] = [
  { id: 'rule', header: '规则引用', className: 'font-mono', render: (row) => row.ruleRef },
  { id: 'matter', header: '判断事项', render: (row) => row.matter },
  { id: 'jurisdiction', header: '适用辖区', className: 'font-mono', render: (row) => row.jurisdiction },
  { id: 'validity', header: '法定生效区间', className: 'font-mono', render: (row) => row.statutoryValidity },
  { id: 'instant', header: '声明的适用时点', render: (row) => row.declaredApplicabilityInstant },
  { id: 'mode', header: '决定方式', align: 'center', className: 'w-[88px]', render: (row) => row.decisionMode },
  { id: 'role', header: '责任角色', render: (row) => row.responsibleRole },
  { id: 'retro', header: '追溯边界', render: (row) => row.retroactivityBoundary },
];

/**
 * 合规规则库（customs-compliance）。行对象是版本化的关务规则登记。页面没有
 * 「登记 / 换版」动作——登记是治理动作走受控登记口；换版即登记更晚起点的新版，
 * 历史区间不接受追改，原判断实际采用的规则版本随判断永久可查。
 */
export function ComplianceRulesPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ComplianceRuleVersionRow>
      title={info.title}
      description={`${info.owner}——新规则不默认追溯，原判断与其当时采用的规则版本保持不变`}
      // 筛选维度（接线时实装进 filters 槽）：判断事项、适用辖区、决定方式（自动/
      // 人工）、法定生效区间窗口。规则引用经搜索。
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索规则引用',
      }}
      columns={columns}
      // 接线前无实例：规则正文属实例半边（PAR-CUS-01/02 待提供），不造合成行。
      rows={[]}
      rowKey={(row) => `${row.ruleRef}@${row.jurisdiction}`}
      pagination={{
        page,
        pageSize,
        total: 0,
        onPageChange: setPage,
        onPageSizeChange: setPageSize,
      }}
      viewState={{
        kind: 'unconfigured',
        title: '关务合规模块尚未接线',
        description: '规则登记走受控登记口（parcel-customs-register），不经在线端点；本页是查阅面，其查询端点尚未建，不发请求、不含未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '规则库查询端点建成并经 ADR-0017 准入闸门放行后接线；登记动作留在受控登记口',
        },
      }}
    />
  );
}
