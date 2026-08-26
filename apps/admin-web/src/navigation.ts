import type { ElementType } from 'react';
import {
  AlertTriangle,
  Anchor,
  BadgeCheck,
  BadgeDollarSign,
  Banknote,
  Boxes,
  Building2,
  Calculator,
  ClipboardCheck,
  Coins,
  Eye,
  FileText,
  FlaskConical,
  FolderOpen,
  Globe,
  Handshake,
  LayoutDashboard,
  Library,
  LineChart,
  Megaphone,
  Milestone,
  Network,
  PackageOpen,
  PackageSearch,
  PackageX,
  Radar,
  Receipt,
  Route,
  Scale,
  Send,
  ShieldAlert,
  ShieldCheck,
  SlidersHorizontal,
  Stamp,
  Ticket,
  TrendingUp,
  Truck,
  UsersRound,
  Warehouse,
} from 'lucide-react';
import type { NavigationSection } from '@idpxyz/ui-workspace';

// 导航按 CONTEXT-MAP 的业务价值链分区：受理 → 商业配置 → 计价 → 网络路由 →
// 作业履约 → 关务 → 追踪异常 → 结算核算 → 代收清分 → 试点治理。每个条目锚定
// 一个限界上下文的所有权范围（moduleInfoById 记主责与出处）；未接线条目落
// UnwiredModule 诚实占位——完整版图先立，页面按闸门与优先级逐个补。
// 不虚构无文档出处的模块；新增条目前先确认它有同等级出处。
// 实时现场作业（扫描/点验/装卸）不在本管理台：那属一线作业端（ADR-0021），
// 这里的作业类条目只是治理与查阅面。
export const navigationSections: NavigationSection[] = [
  {
    title: '总览',
    items: [{ id: 'workbench', label: '工作台', icon: 'workbench' }],
  },
  {
    // 主数据区集中全部「身份、目录与登记式对象」的查阅入口——操作员按
    // 「数据去哪找」的心智一处找齐。分区是呈现分组：各条目的主责上下文与
    // 出处仍由 moduleInfoById 逐条表达，跨上下文集中不改变所有权，也不改变
    // 写入纪律（登记一律走各自受控入口，本区页面只查阅）。排序按
    // 组织 → 参与方 → 合同协议 → 产品渠道 → 策略 → 价格 → 网络 → 关务 → 追踪异常。
    // 追踪异常那三条是目录（规则与口径），不塞进追踪异常区的案件页：目录是规则，
    // 案件不是——案件页空着是三堵墙拦的，混进目录会让人以为墙降了
    // （票 admin-web-page-wiring-frontier/02 的导航裁决）。
    title: '主数据',
    items: [
      { id: 'group-legal-entities', label: '集团与法人', icon: 'group-legal-entities' },
      { id: 'business-parties', label: '业务参与方', icon: 'business-parties' },
      { id: 'party-contracts', label: '客户与合同', icon: 'party-contracts' },
      { id: 'supplier-agreements', label: '供应商协议', icon: 'supplier-agreements' },
      { id: 'service-products', label: '服务产品与渠道', icon: 'service-products' },
      { id: 'channel-product-catalog', label: '渠道产品目录', icon: 'channel-product-catalog' },
      { id: 'commercial-policies', label: '商业规则与策略', icon: 'commercial-policies' },
      { id: 'price-card-catalog', label: '价卡目录', icon: 'price-card-catalog' },
      { id: 'reference-series', label: '计价参考序列', icon: 'reference-series' },
      { id: 'network-catalog', label: '网络目录', icon: 'network-catalog' },
      { id: 'service-areas', label: '服务区域与覆盖', icon: 'service-areas' },
      { id: 'customs-ports-paths', label: '口岸与申报路径', icon: 'customs-ports-paths' },
      { id: 'compliance-rules', label: '合规规则库', icon: 'compliance-rules' },
      { id: 'tracking-judgment-rules', label: '追踪判断规则', icon: 'tracking-judgment-rules' },
      { id: 'disclosure-policies', label: '披露与通知策略', icon: 'disclosure-policies' },
      { id: 'claim-prerequisites', label: '索赔资格与授权', icon: 'claim-prerequisites' },
    ],
  },
  {
    title: '委托受理',
    items: [
      { id: 'shipment-request', label: '提交与撤回', icon: 'shipment-request' },
      { id: 'shipment-request-inquiry', label: '委托查阅', icon: 'shipment-request-inquiry' },
      { id: 'acceptance-review', label: '接受前人工复核', icon: 'acceptance-review' },
      { id: 'label-transactions', label: '面单交易', icon: 'label-transactions' },
      { id: 'cancel-parcel', label: '取消与收寄后处置', icon: 'cancel-parcel' },
    ],
  },
  {
    title: '计价',
    items: [
      { id: 'pricing-evaluation', label: '价格评价', icon: 'pricing-evaluation' },
    ],
  },
  {
    title: '网络与路由',
    items: [
      { id: 'route-plans', label: '路由计划与改路', icon: 'route-plans' },
    ],
  },
  {
    title: '作业与履约',
    items: [
      { id: 'node-operations-review', label: '节点作业查阅', icon: 'node-operations-review' },
      { id: 'transport-fulfillment-review', label: '运输履约查阅', icon: 'transport-fulfillment-review' },
    ],
  },
  {
    title: '关务合规',
    items: [
      { id: 'customs-cases', label: '关务案件与申报', icon: 'customs-cases' },
      { id: 'customs-restrictions', label: '合规限制与监管税费', icon: 'customs-restrictions' },
    ],
  },
  {
    title: '追踪与异常',
    items: [
      { id: 'tracking-projection', label: '全程追踪', icon: 'tracking-projection' },
      { id: 'exception-triage', label: '异常分诊与处置协调', icon: 'exception-triage' },
      { id: 'exception-cases', label: '异常案件', icon: 'exception-cases' },
      { id: 'claims-recovery', label: '索赔与追偿', icon: 'claims-recovery' },
    ],
  },
  {
    title: '结算与核算',
    items: [
      { id: 'charges-billing', label: '费用与计费', icon: 'charges-billing' },
      { id: 'reconciliation', label: '对账单', icon: 'reconciliation' },
      { id: 'settlement-application', label: '收付款核销', icon: 'settlement-application' },
      { id: 'operating-metrics', label: '经营核算', icon: 'operating-metrics' },
    ],
  },
  {
    title: '代收与清分',
    items: [{ id: 'cod-ledger', label: '代收分户账', icon: 'cod-ledger' }],
  },
  {
    title: '试点治理',
    items: [
      { id: 'stage-admission', label: '阶段决定与暂停恢复', icon: 'stage-admission' },
    ],
  },
  {
    // 演示区与业务区分开列：预览页只演示模板长相（隔离合成 S），不承载业务语义。
    title: '演示',
    items: [
      { id: 'template-preview', label: '模板预览（合成 S）', icon: 'template-preview' },
    ],
  },
];

export const sidebarIconMap: Record<string, ElementType> = {
  workbench: LayoutDashboard,
  'group-legal-entities': Building2,
  'business-parties': UsersRound,
  'shipment-request': Send,
  'shipment-request-inquiry': PackageSearch,
  'acceptance-review': ClipboardCheck,
  'label-transactions': Ticket,
  'cancel-parcel': PackageX,
  'party-contracts': Handshake,
  'service-products': Boxes,
  'channel-product-catalog': PackageOpen,
  'supplier-agreements': FileText,
  'commercial-policies': SlidersHorizontal,
  'price-card-catalog': BadgeDollarSign,
  'reference-series': TrendingUp,
  'pricing-evaluation': Calculator,
  'network-catalog': Network,
  'service-areas': Globe,
  'route-plans': Route,
  'node-operations-review': Warehouse,
  'transport-fulfillment-review': Truck,
  'customs-cases': Stamp,
  'compliance-rules': Library,
  'customs-ports-paths': Anchor,
  'customs-restrictions': ShieldAlert,
  'tracking-judgment-rules': Milestone,
  'disclosure-policies': Eye,
  'claim-prerequisites': BadgeCheck,
  'tracking-projection': Radar,
  'exception-triage': AlertTriangle,
  'exception-cases': FolderOpen,
  'claims-recovery': Megaphone,
  'charges-billing': Receipt,
  reconciliation: Scale,
  'settlement-application': Banknote,
  'operating-metrics': LineChart,
  'cod-ledger': Coins,
  'stage-admission': ShieldCheck,
  'template-preview': FlaskConical,
};

export interface ModuleInfo {
  /** 页面标题，与导航条目一致。 */
  title: string;
  /** 主责上下文（领域语言名 + 目录名）。 */
  owner: string;
  /** 场景出处，指向权威文档名，不复述其内容。 */
  source: string;
}

export const moduleInfoById: Record<string, ModuleInfo> = {
  // —— 主数据（party-commercial 的身份与关系类所有权）——
  'group-legal-entities': {
    title: '集团与法人',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 运营企业集团及其法人、经营组织和业务身份',
  },
  'business-parties': {
    title: '业务参与方',
    owner: '参与方与商业（party-commercial）',
    source:
      'docs/domain/party-commercial/CONTEXT.md 角色中立的业务参与方身份，以及承运商、承运商代理商、转售商、聚合平台等参与方关系与渠道账号的持有人、有效范围、商业归属',
  },

  // —— 委托受理（parcel-shipment）——
  'shipment-request-inquiry': {
    title: '委托查阅',
    owner: '小包托运（parcel-shipment）',
    source: 'docs/domain/parcel-shipment/CONTEXT.md 委托生命周期与「授权查询作用域」「统一不可见结果」',
  },
  'acceptance-review': {
    title: '接受前人工复核',
    owner: '小包托运（parcel-shipment）',
    source: 'docs/domain/parcel-shipment/CONTEXT.md「适用规则显式要求人工业务判断」',
  },
  'label-transactions': {
    title: '面单交易',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/domain/parcel-shipment/CONTEXT.md 面单交易、交易级与包裹级渠道业务结果、面单交易定案、包裹级关闭或重开请求与决定、渠道角色与责任依据快照',
  },
  'cancel-parcel': {
    title: '取消与收寄后处置',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/application/parcel-shipment/UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md 与 CONTEXT.md「包裹取消决定」「收寄后服务处置决定」',
  },

  // —— 商业配置（party-commercial）——
  'party-contracts': {
    title: '客户与合同',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 客户账户、客户合同与供应商商业协议的独立版本生命周期',
  },
  'service-products': {
    title: '服务产品与渠道',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 服务产品版本、产品—渠道映射与渠道账号业务使用授权',
  },
  'channel-product-catalog': {
    title: '渠道产品目录',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 外部渠道产品目录身份与商业适用性、产品—渠道映射、可复用渠道约束及各自的适用有效期',
  },
  'supplier-agreements': {
    title: '供应商协议',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/CONTEXT-MAP.md party-commercial → transport-fulfillment：供应商商业协议、采购价格与结算条件的版本生命周期',
  },
  'commercial-policies': {
    title: '商业规则与策略',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 接单规则包、接受前财务控制策略、商业价格政策、结算政策与信用政策',
  },

  // —— 计价（parcel-pricing）——
  'price-card-catalog': {
    title: '价卡目录',
    owner: '小包计价（parcel-pricing）',
    source: 'docs/domain/parcel-pricing/CONTEXT.md「定价方案与价表版本」生命周期与价卡发布门禁',
  },
  'reference-series': {
    title: '计价参考序列',
    owner: '小包计价（parcel-pricing）',
    source: 'docs/adr/0013-pricing-owns-versioned-external-reference-series.md 与 CONTEXT.md「计价参考序列」',
  },
  'pricing-evaluation': {
    title: '价格评价',
    owner: '小包计价（parcel-pricing）',
    source: 'docs/domain/parcel-pricing/CONTEXT.md「价格评价」生命周期（争议复核与回放）',
  },

  // —— 网络与路由（network-routing）——
  'network-catalog': {
    title: '网络目录',
    owner: '网络与路由（network-routing）',
    source: 'docs/domain/network-routing/CONTEXT.md 节点网络身份、有向连接、线路与服务日历；登记走受控 CLI（cmd/parcel-network-register）',
  },
  'service-areas': {
    title: '服务区域与覆盖',
    owner: '网络与路由（network-routing）',
    source: 'docs/domain/network-routing/CONTEXT.md 物流节点的版本化服务区域和节点覆盖关系',
  },
  'route-plans': {
    title: '路由计划与改路',
    owner: '网络与路由（network-routing）',
    source: 'docs/domain/network-routing/CONTEXT.md 包裹级路由计划、路由指令、路由偏离判断与受控改路历史',
  },

  // —— 作业与履约（node-operations / transport-fulfillment，管理台只做治理查阅面）——
  'node-operations-review': {
    title: '节点作业查阅',
    owner: '节点作业（node-operations）',
    source: 'docs/domain/node-operations/CONTEXT.md 节点收寄、集运单元、实测与交接证据；实时现场作业属一线作业端（ADR-0021），本页为治理查阅面',
  },
  'transport-fulfillment-review': {
    title: '运输履约查阅',
    owner: '运输履约（transport-fulfillment）',
    source: 'docs/domain/transport-fulfillment/CONTEXT.md 班次、容量池、权威运输交接结果与交付证明',
  },

  // —— 关务合规（customs-compliance）——
  'customs-cases': {
    title: '关务案件与申报',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/customs-compliance/CONTEXT.md 稳定关务案件、申报单元、正式申报资料快照与不可覆盖提交版本',
  },
  'compliance-rules': {
    title: '合规规则库',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/customs-compliance/CONTEXT.md 禁限运、归类、原产地、申报价值、监管凭证适用性等规则化合规判断及其规则版本、依据和决定方式',
  },
  'customs-ports-paths': {
    title: '口岸与申报路径',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/CONTEXT-MAP.md customs-compliance ↔ network-routing：关务提供合规候选区域、口岸、申报路径、限制及解除结果，路由只在合格候选中选择',
  },
  'customs-restrictions': {
    title: '合规限制与监管税费',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/customs-compliance/CONTEXT.md 关务限制及解除、监管核定税费与放行门禁核对',
  },

  // —— 追踪与异常（visibility-exception）——
  'tracking-projection': {
    title: '全程追踪',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 全程追踪投影、标准追踪里程碑、版本化 ETA 与客户隔离视图',
  },
  'exception-triage': {
    title: '异常分诊与处置协调',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 异常案件「进入人工复核」与处置协调',
  },
  'exception-cases': {
    title: '异常案件',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 异常案件的根对象、版本化影响范围、响应周期、受控归并与关闭重开',
  },
  'claims-recovery': {
    title: '索赔与追偿',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 客户异常通知决定、客户索赔项与供应商或保险追偿事项（金额结算归 settlement-accounting）',
  },

  // —— VE 六类规则与策略目录（主数据区呈现，主责仍在 visibility-exception）——
  'tracking-judgment-rules': {
    title: '追踪判断规则',
    owner: '全程追踪与异常（visibility-exception）',
    source:
      'docs/domain/visibility-exception/CONTEXT.md 标准里程碑映射按源上下文与事实类型版本化登记；高可信且命中版本化分诊规则的信号可自动建立或关联案件，低可信先进入分诊',
  },
  'disclosure-policies': {
    title: '披露与通知策略',
    owner: '全程追踪与异常（visibility-exception）',
    source:
      'docs/domain/visibility-exception/CONTEXT.md 客户披露必须使用经批准的最小范围和内容；通知义务能否被满足由客户合同和通知策略另行判断（通知保存目标客户、要求时限和适用渠道）',
  },
  'claim-prerequisites': {
    title: '索赔资格与授权',
    owner: '全程追踪与异常（visibility-exception）',
    source:
      'docs/domain/visibility-exception/CONTEXT.md 按申请人授权、客户账户、合同版本、索赔时限、目标范围、重复关系和最低材料要求判断资格',
  },

  // —— 结算与核算（settlement-accounting）——
  'charges-billing': {
    title: '费用与计费',
    owner: '结算与经营核算（settlement-accounting）',
    source: 'docs/domain/settlement-accounting/CONTEXT.md 费用项目、计费重量采用、费用明细与费用调整的版本与依据',
  },
  reconciliation: {
    title: '对账单',
    owner: '结算与经营核算（settlement-accounting）',
    source: 'docs/domain/settlement-accounting/CONTEXT.md「结算账户、对账与争议」对账单管理',
  },
  'settlement-application': {
    title: '收付款核销',
    owner: '结算与经营核算（settlement-accounting）',
    source:
      'docs/application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md 与 CONTEXT.md「核销」「真实收付映射」',
  },
  'operating-metrics': {
    title: '经营核算',
    owner: '结算与经营核算（settlement-accounting）',
    source: 'docs/domain/settlement-accounting/CONTEXT.md 按明确口径、版本、币种及截至时点派生的经营毛利与经营损失指标',
  },

  // —— 代收与清分（collection-remittance，尚无独立 CONTEXT.md，出处为上下文地图）——
  'cod-ledger': {
    title: '代收分户账',
    owner: '代收与清分（collection-remittance）',
    source: 'docs/domain/CONTEXT-MAP.md collection-remittance：代收资金义务、渠道在途代收款、待清分款、应付客户款与汇付',
  },

  // —— 试点治理（pilotgovernance）——
  'stage-admission': {
    title: '阶段决定与暂停恢复',
    owner: '试点治理（pilotgovernance）',
    source: 'docs/design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md',
  },
};

export const pageTitleById: Record<string, string> = {
  workbench: '工作台',
  'shipment-request': '提交与撤回',
  'template-preview': '模板预览（合成 S）',
  ...Object.fromEntries(
    Object.entries(moduleInfoById).map(([id, info]) => [id, info.title]),
  ),
};
