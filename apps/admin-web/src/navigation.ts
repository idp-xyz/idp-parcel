import type { ElementType } from 'react';
import {
  AlertTriangle,
  Anchor,
  BadgeCheck,
  BadgeDollarSign,
  Banknote,
  BookOpen,
  Boxes,
  Building2,
  Calculator,
  CalendarClock,
  ClipboardCheck,
  Coins,
  Eye,
  FileClock,
  FileText,
  FlaskConical,
  FolderOpen,
  Globe,
  Handshake,
  History,
  LayoutDashboard,
  Landmark,
  Library,
  LineChart,
  Megaphone,
  Milestone,
  Network,
  PackageOpen,
  PackageSearch,
  PackageCheck,
  PackageX,
  Radar,
  Receipt,
  Route,
  Scale,
  Send,
  ShieldAlert,
  ShieldCheck,
  Signpost,
  SlidersHorizontal,
  Split,
  Stamp,
  Star,
  TabletSmartphone,
  Ticket,
  TrendingUp,
  Truck,
  UsersRound,
  Warehouse,
} from 'lucide-react';
import type { NavigationSection } from '@idpxyz/ui-workspace';

// 导航按运营工作分区（改版第一步，设计源「三、新的信息架构」）：工作台 → 运单 → 客户 → 合作伙伴 → 产品与价格 →
// 线路与运力 → 清关 → 异常与索赔 → 账务 → 设置。此前按限界上下文的价值链分区，页名说的是系统里的对象（委托、投影、
// 参与方）；现在页名与分区说的是运营一线的话，所有权一个字没变——每页的主责上下文仍由 moduleInfoById 的 owner 逐条表达，
// 分区只是呈现分组，不改变所有权，也不改变写入纪律。
//
// 页 id 一律不动：page-registry 的登记、hash 地址、保存视图、最近对象都按 id 认页。
// 不虚构无文档出处的模块；新增条目前先确认它有同等级出处。实时现场作业（扫描 / 点验 / 装卸）不在本管理台：那属一线作业端
// （ADR-0021），这里的作业类条目只是治理与查阅面。
export const navigationSections: NavigationSection[] = [
  {
    // 「我的工作」两条目（最近打开、保存的视图）并进工作台一组：三条都是管理台自身的事实，不发请求（黄金标准 Rule 6
    // 要 My Work 在业务导航之上，并进第一组仍满足）。
    title: '工作台',
    items: [
      { id: 'workbench', label: '工作台', icon: 'workbench' },
      { id: 'recent-objects', label: '最近打开', icon: 'recent-objects' },
      { id: 'saved-views', label: '保存的视图', icon: 'saved-views' },
    ],
  },
  {
    // 运单一组收下一票货从下单到签收路上的全部查阅与处置面：委托、追踪、路由、节点、运输都是同一票货的不同侧面，
    // 一线找货的入口是运单号而不是四个系统对象各开一页（设计原则一）。
    title: '运单',
    items: [
      { id: 'shipment-request-inquiry', label: '运单', icon: 'shipment-request-inquiry' },
      { id: 'tracking-projection', label: '物流动态', icon: 'tracking-projection' },
      { id: 'shipment-request', label: '新建与撤回', icon: 'shipment-request' },
      { id: 'acceptance-review', label: '待受理复核', icon: 'acceptance-review' },
      // 处置紧挨复核：同是接受判断任务上的等待态与续办角色（ADR-0132 决定二），但决定集不同——复核完成后决定可以是接受，
      // 处置只能选去向；两页分开，不让一项`业务限制`借复核那条路走到接受。
      { id: 'authorized-disposition', label: '受限运单处置', icon: 'authorized-disposition' },
      // 释放拒收处理处理的是已越过业务决定的资金占用（ADR-0293），不是对委托的判断，与处置分页、授予也分开。
      { id: 'control-release-refusals', label: '资金释放待处理', icon: 'control-release-refusals' },
      { id: 'cancel-parcel', label: '取消运单', icon: 'cancel-parcel' },
      // 收寄后处置的运营一侧：客户渠道的取消签在上一页，这里是有权主体形成处置决定、登记待人工处理项（ADR-0332、ADR-0341）。
      { id: 'post-intake-disposition', label: '货物处置', icon: 'post-intake-disposition' },
      { id: 'label-transactions', label: '面单', icon: 'label-transactions' },
      // 择优决定是 parcel-shipment 形成的只追加判断记录（票 label-channel/14），挨着消费它的面单；对象引用里的映射本体在
      // 合作伙伴区的渠道产品页，这里只透引用。
      { id: 'channel-selection-decisions', label: '渠道择优', icon: 'channel-selection-decisions' },
      { id: 'route-plans', label: '路线规划', icon: 'route-plans' },
      { id: 'node-operations-review', label: '网点作业', icon: 'node-operations-review' },
      { id: 'transport-fulfillment-review', label: '运输班次', icon: 'transport-fulfillment-review' },
      // 唯一一个带写面的作业条目：不是现场作业（ADR-0021），是所有者对外部承运轨迹事实的显式判断（ADR-0102 决定三）。
      { id: 'effective-time-judgment', label: '外部轨迹时间确认', icon: 'effective-time-judgment' },
    ],
  },
  {
    title: '客户',
    items: [
      { id: 'party-contracts', label: '客户与合同', icon: 'party-contracts' },
      { id: 'commercial-policies', label: '商业规则', icon: 'commercial-policies' },
    ],
  },
  {
    // 业务参与方册底下还是同一本，入口按角色称呼（设计源第四节）：客户那一半从「客户与合同」进，承运商、代理、清关行、
    // 渠道从这里进；没有角色的叫往来单位。
    title: '合作伙伴',
    items: [
      { id: 'business-parties', label: '往来单位', icon: 'business-parties' },
      { id: 'supplier-agreements', label: '供应商协议', icon: 'supplier-agreements' },
      { id: 'channel-product-catalog', label: '渠道产品', icon: 'channel-product-catalog' },
    ],
  },
  {
    title: '产品与价格',
    items: [
      { id: 'service-products', label: '服务产品', icon: 'service-products' },
      { id: 'price-card-catalog', label: '价格表', icon: 'price-card-catalog' },
      { id: 'reference-series', label: '汇率与燃油费率', icon: 'reference-series' },
      { id: 'pricing-evaluation', label: '计费记录', icon: 'pricing-evaluation' },
      { id: 'pricing-estimate', label: '计价试算', icon: 'pricing-estimate' },
    ],
  },
  {
    title: '线路与运力',
    items: [
      { id: 'network-catalog', label: '网络与网点', icon: 'network-catalog' },
      { id: 'service-areas', label: '服务区域', icon: 'service-areas' },
      { id: 'customs-ports-paths', label: '口岸与申报路径', icon: 'customs-ports-paths' },
    ],
  },
  {
    title: '清关',
    items: [
      { id: 'customs-cases', label: '清关单', icon: 'customs-cases' },
      { id: 'customs-restrictions', label: '合规限制与税费', icon: 'customs-restrictions' },
      { id: 'compliance-rules', label: '合规规则', icon: 'compliance-rules' },
    ],
  },
  {
    title: '异常与索赔',
    items: [
      { id: 'exception-triage', label: '异常处理', icon: 'exception-triage' },
      { id: 'exception-cases', label: '异常案件', icon: 'exception-cases' },
      { id: 'claims-recovery', label: '索赔与追偿', icon: 'claims-recovery' },
      { id: 'claim-prerequisites', label: '索赔资格与授权', icon: 'claim-prerequisites' },
    ],
  },
  {
    title: '账务',
    items: [
      { id: 'charges-billing', label: '客户费用', icon: 'charges-billing' },
      { id: 'reconciliation', label: '对账单', icon: 'reconciliation' },
      { id: 'settlement-application', label: '收付款核销', icon: 'settlement-application' },
      { id: 'cod-ledger', label: '代收货款', icon: 'cod-ledger' },
      { id: 'operating-metrics', label: '经营核算', icon: 'operating-metrics' },
    ],
  },
  {
    // 设置收公司自己的主体与各类规则、设备、审批；试点治理只给管理员；演示只演示模板长相（隔离合成 S），放最末。
    title: '设置',
    items: [
      { id: 'group-legal-entities', label: '公司主体', icon: 'group-legal-entities' },
      { id: 'tracking-judgment-rules', label: '追踪规则', icon: 'tracking-judgment-rules' },
      { id: 'disclosure-policies', label: '通知与披露', icon: 'disclosure-policies' },
      // 作业设备册是登记式目录，不是现场作业，也不做设备管理（ADR-0149 越权风险点 2）。
      { id: 'work-devices', label: '扫描设备', icon: 'work-devices' },
      { id: 'pending-publications', label: '审批与发布', icon: 'pending-publications' },
      // 基础数据是产品内置的只读目录（ADR-0335），不是公司自己登记的配置，所以放在审批之后、管理员专用的试点治理之前。
      { id: 'reference-data', label: '基础数据', icon: 'reference-data' },
      { id: 'stage-admission', label: '试点治理', icon: 'stage-admission' },
      { id: 'template-preview', label: '演示', icon: 'template-preview' },
    ],
  },
];

export const sidebarIconMap: Record<string, ElementType> = {
  'recent-objects': History,
  'saved-views': Star,
  workbench: LayoutDashboard,
  'group-legal-entities': Building2,
  'business-parties': UsersRound,
  'shipment-request': Send,
  'shipment-request-inquiry': PackageSearch,
  'acceptance-review': ClipboardCheck,
  'authorized-disposition': Signpost,
  'control-release-refusals': Landmark,
  'label-transactions': Ticket,
  'channel-selection-decisions': Split,
  'cancel-parcel': PackageX,
  'post-intake-disposition': PackageCheck,
  'party-contracts': Handshake,
  'service-products': Boxes,
  'channel-product-catalog': PackageOpen,
  'supplier-agreements': FileText,
  'commercial-policies': SlidersHorizontal,
  'pending-publications': FileClock,
  'price-card-catalog': BadgeDollarSign,
  'reference-series': TrendingUp,
  'pricing-evaluation': Calculator,
  'pricing-estimate': Calculator,
  'network-catalog': Network,
  'service-areas': Globe,
  'route-plans': Route,
  'node-operations-review': Warehouse,
  'transport-fulfillment-review': Truck,
  'effective-time-judgment': CalendarClock,
  'customs-cases': Stamp,
  'compliance-rules': Library,
  'customs-ports-paths': Anchor,
  'customs-restrictions': ShieldAlert,
  'tracking-judgment-rules': Milestone,
  'disclosure-policies': Eye,
  'claim-prerequisites': BadgeCheck,
  'work-devices': TabletSmartphone,
  'reference-data': BookOpen,
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
  /** 页面标题，与导航条目一致；用运营一线的叫法。 */
  title: string;
  /**
   * 给人看的一句话：这页用来做什么。页面副标题取它，不取 owner。类型上可选只是为了让各模块测试里手搭的 ModuleInfo 桩不必
   * 都补这一句；moduleInfoById 里每页都有，navigation.test.ts 钉着。
   */
  summary?: string;
  /** 主责上下文（领域语言名 + 目录名）。给工具与就绪度用，不渲染到页面。 */
  owner: string;
  /** 场景出处，指向权威文档名，不复述其内容。给工具与就绪度用，不渲染到页面。 */
  source: string;
}

export const moduleInfoById: Record<string, ModuleInfo> = {
  // —— 工作台（管理台自身；数据是本机浏览器的事实，不属任何限界上下文）——
  'recent-objects': {
    title: '最近打开',
    summary: '这台电脑上最近打开过的运单、客户等对象，点一下回到它。',
    owner: '管理台自身（apps/admin-web）',
    source:
      '.scratch/admin-web-ux-alignment/spec.md「缺口」表第二档；黄金标准「Left Navigation 黄金标准」Rule 6（My Work：Recent Objects）',
  },
  'saved-views': {
    title: '保存的视图',
    summary: '存下来的筛选条件，一键回到常用的列表视图。',
    owner: '管理台自身（apps/admin-web）',
    source:
      '.scratch/admin-web-ux-alignment/spec.md「缺口」表第二档；黄金标准「Left Navigation 黄金标准」Rule 6（My Work：Saved Views）',
  },

  // —— 运单（parcel-shipment）——
  'shipment-request-inquiry': {
    title: '运单',
    summary: '查看全部运单：谁下的单、几件、现在到哪一步。',
    owner: '小包托运（parcel-shipment）',
    source: 'docs/domain/parcel-shipment/CONTEXT.md 委托生命周期与「授权查询作用域」「统一不可见结果」',
  },
  'acceptance-review': {
    title: '待受理复核',
    summary: '需要人工判断才能受理的运单，在这里完成复核。',
    owner: '小包托运（parcel-shipment）',
    source: 'docs/domain/parcel-shipment/CONTEXT.md「适用规则显式要求人工业务判断」',
  },
  'authorized-disposition': {
    title: '受限运单处置',
    summary: '被规则拦下的运单，由有权限的人决定去向：拒绝，或退回客户补充。',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/domain/parcel-shipment/CONTEXT.md「授权处置」与`等待授权处置`；docs/adr/0132-authorized-disposition-decides-the-destination-of-a-restricted-request-as-its-own-wait-state-and-never-passes.md 决定一（去向封闭两值，放行不在集内）',
  },
  'control-release-refusals': {
    title: '资金释放待处理',
    summary: '预付冻结没能自动释放的运单，核对后在结算侧释放，或标记已处理。',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/adr/0282-a-control-release-the-provider-refuses-is-registered-for-handling-instead-of-compensated.md 决定二（释放未受理册）；docs/adr/0293-a-refused-control-release-is-handled-by-releasing-at-settlement-or-marking-handled-and-the-handling-resumes-the-acceptance-chain.md',
  },
  'post-intake-disposition': {
    title: '货物处置',
    summary: '已收寄后客户要求停止服务的包裹，决定退运、改派或继续，并跟进待办。',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/application/parcel-shipment/UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md；docs/adr/0332-a-post-intake-service-disposition-is-an-append-only-per-parcel-decision-chain-that-asks-object-owners-for-scoped-requests-through-an-outbox-envelope.md；docs/adr/0341-system-executable-disposition-requests-run-in-the-owners-consumers-and-the-rest-become-manual-items-that-parcel-shipment-holds-and-operators-register-results-for.md',
  },
  'label-transactions': {
    title: '面单',
    summary: '向渠道购买面单的每一笔记录及其结果。',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/domain/parcel-shipment/CONTEXT.md 面单交易、交易级与包裹级渠道业务结果、面单交易定案、包裹级关闭或重开请求与决定、渠道角色与责任依据快照',
  },
  'channel-selection-decisions': {
    title: '渠道择优',
    summary: '多个渠道并列、需要人工定夺的运单。',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/product/PILOT-PARAMETER-REGISTER.md PAR-NET-16 择优逐次留痕、并列冲突交人工裁决；docs/domain/CONTEXT-MAP.md「具体渠道选择」归 parcel-shipment；票 label-channel/14 裁决——择优决定作为只追加的决定记录落 PS 侧，引用候选与出局因由、不拷内容',
  },
  'cancel-parcel': {
    title: '取消运单',
    summary: '客户或运营要求取消某件包裹时，在这里发起。',
    owner: '小包托运（parcel-shipment）',
    source:
      'docs/application/parcel-shipment/UC-PS-006-CANCEL-PARCEL-OR-COORDINATE-POST-INTAKE-DISPOSITION.md 与 CONTEXT.md「包裹取消决定」「收寄后服务处置决定」',
  },

  // —— 客户 / 合作伙伴（party-commercial）——
  'party-contracts': {
    title: '客户与合同',
    summary: '客户账户、合同及其版本：谁签了什么、什么时候生效。',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 客户账户、客户合同与供应商商业协议的独立版本生命周期',
  },
  'commercial-policies': {
    title: '商业规则',
    summary: '接单、价格、结算、信用与授权等规则的各个版本。',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 接单规则包、接受前财务控制策略、商业价格政策、结算政策与信用政策',
  },
  'business-parties': {
    title: '往来单位',
    summary: '所有与我们有往来的单位——客户、承运商、代理、清关行、渠道——以及彼此之间的关系。',
    owner: '参与方与商业（party-commercial）',
    source:
      'docs/domain/party-commercial/CONTEXT.md 角色中立的业务参与方身份，以及承运商、承运商代理商、转售商、聚合平台等参与方关系与渠道账号的持有人、有效范围、商业归属',
  },
  'supplier-agreements': {
    title: '供应商协议',
    summary: '与承运商、代理等供应商签的协议及其版本。',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/CONTEXT-MAP.md party-commercial → transport-fulfillment：供应商商业协议、采购价格与结算条件的版本生命周期',
  },
  'channel-product-catalog': {
    title: '渠道产品',
    summary: '渠道平台提供的产品目录，以及我们的服务产品与它们的对应关系。',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 外部渠道产品目录身份与商业适用性、产品—渠道映射、可复用渠道约束及各自的适用有效期',
  },

  // —— 产品与价格（party-commercial / parcel-pricing）——
  'service-products': {
    title: '服务产品',
    summary: '我们对外销售的服务产品及其版本与形态。',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 服务产品版本、产品—渠道映射与渠道账号业务使用授权',
  },
  'price-card-catalog': {
    title: '价格表',
    summary: '对客户与对供应商的价格表版本：导入、批准与发布。',
    owner: '小包计价（parcel-pricing）',
    source: 'docs/domain/parcel-pricing/CONTEXT.md「定价方案与价表版本」生命周期与价卡发布门禁',
  },
  'reference-series': {
    title: '汇率与燃油费率',
    summary: '计价要引用的汇率、燃油费率等外部数据，按期登记。',
    owner: '小包计价（parcel-pricing）',
    source: 'docs/adr/0013-pricing-owns-versioned-external-reference-series.md 与 CONTEXT.md「计价参考序列」',
  },
  'pricing-evaluation': {
    title: '计费记录',
    summary: '每一次计价的结果与依据，用于对账与争议复核。',
    owner: '小包计价（parcel-pricing）',
    source: 'docs/domain/parcel-pricing/CONTEXT.md「价格评价」生命周期（争议复核与回放）',
  },
  'pricing-estimate': {
    title: '计价试算',
    summary: '按价格表试算一件包裹的费用，不写入任何记录。',
    owner: '小包计价（parcel-pricing）',
    source: 'docs/application/parcel-pricing/UC-PP-001-FORM-ESTIMATE-EVALUATIONS.md 与 docs/adr/0152-operator-estimate-is-a-pricing-use-case-computed-not-recorded-and-never-handed-to-settlement.md',
  },

  // —— 线路与运力（network-routing / customs-compliance）——
  'network-catalog': {
    title: '网络与网点',
    summary: '网点、连接、线路与服务日历的登记。',
    owner: '网络与路由（network-routing）',
    source: 'docs/domain/network-routing/CONTEXT.md 节点网络身份、有向连接、线路与服务日历；登记走受控 CLI（cmd/parcel-network-register）',
  },
  'service-areas': {
    title: '服务区域',
    summary: '各网点覆盖哪些区域。',
    owner: '网络与路由（network-routing）',
    source: 'docs/domain/network-routing/CONTEXT.md 物流节点的版本化服务区域和节点覆盖关系',
  },
  'route-plans': {
    title: '路线规划',
    summary: '每件包裹的运输路线怎么定、改过几次。',
    owner: '网络与路由（network-routing）',
    source: 'docs/domain/network-routing/CONTEXT.md 包裹级路由计划、路由指令、路由偏离判断与受控改路历史',
  },
  'customs-ports-paths': {
    title: '口岸与申报路径',
    summary: '可用的口岸与申报路径。',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/CONTEXT-MAP.md customs-compliance ↔ network-routing：关务提供合规候选区域、口岸、申报路径、限制及解除结果，路由只在合格候选中选择',
  },

  // —— 设置 · 作业设备册（共享接入身份；不是限界上下文）——
  'work-devices': {
    title: '扫描设备',
    summary: '网点与场外作业使用的扫描设备登记。',
    owner: '共享接入身份（access identity）',
    source:
      'docs/adr/0149-business-command-faces-split-into-frontline-operator-and-integration-client-families.md 决定二与越权风险点 2',
  },

  // —— 运单 · 作业与履约（node-operations / transport-fulfillment，管理台只做治理查阅面）——
  'node-operations-review': {
    title: '网点作业',
    summary: '网点收寄、集运与交接的作业记录。',
    owner: '节点作业（node-operations）',
    source: 'docs/domain/node-operations/CONTEXT.md 节点收寄、集运单元、实测与交接证据；实时现场作业属一线作业端（ADR-0021），本页为治理查阅面',
  },
  'transport-fulfillment-review': {
    title: '运输班次',
    summary: '班次、舱位与承运商交接的实际情况。',
    owner: '运输履约（transport-fulfillment）',
    source: 'docs/domain/transport-fulfillment/CONTEXT.md 班次、容量池、权威运输交接结果与交付证明',
  },
  'effective-time-judgment': {
    title: '外部轨迹时间确认',
    summary: '承运商回传的轨迹从何时起算，由我们确认。',
    owner: '运输履约（transport-fulfillment）',
    source:
      'docs/domain/transport-fulfillment/CONTEXT.md 外部承运轨迹事实；ADR-0102 决定三——有效时间由所有者显式判断或按该源已登记规则形成，不得默默等于发生时间；本页是判断人的读写面，不解释状态词、不建议时间',
  },

  // —— 清关（customs-compliance）——
  'customs-cases': {
    title: '清关单',
    summary: '每件包裹的清关单、报关批次与海关回执。',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/customs-compliance/CONTEXT.md 稳定关务案件、申报单元、正式申报资料快照与不可覆盖提交版本',
  },
  'compliance-rules': {
    title: '合规规则',
    summary: '禁限运、归类、申报价值等合规判断规则的版本。',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/customs-compliance/CONTEXT.md 禁限运、归类、原产地、申报价值、监管凭证适用性等规则化合规判断及其规则版本、依据和决定方式',
  },
  'customs-restrictions': {
    title: '合规限制与税费',
    summary: '合规限制及其解除、监管核定税费、放行检查。',
    owner: '关务与贸易合规（customs-compliance）',
    source: 'docs/domain/customs-compliance/CONTEXT.md 关务限制及解除、监管核定税费与放行门禁核对',
  },

  // —— 运单 · 物流动态 / 异常与索赔（visibility-exception）——
  'tracking-projection': {
    title: '物流动态',
    summary: '每件包裹现在到哪一步、最近一条动态是什么。',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 全程追踪投影、标准追踪里程碑、版本化 ETA 与客户隔离视图',
  },
  'exception-triage': {
    title: '异常处理',
    summary: '需要人工分诊或协调处置的异常信号。',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 异常案件「进入人工复核」与处置协调',
  },
  'exception-cases': {
    title: '异常案件',
    summary: '已立案的异常：谁在处理、处理到哪一步。',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 异常案件的根对象、版本化影响范围、响应周期、受控归并与关闭重开',
  },
  'claims-recovery': {
    title: '索赔与追偿',
    summary: '客户索赔，以及向供应商或保险的追偿。',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 客户异常通知决定、客户索赔项与供应商或保险追偿事项（金额结算归 settlement-accounting）',
  },

  // —— 设置 · 规则与策略（主责仍在 visibility-exception）——
  'tracking-judgment-rules': {
    title: '追踪规则',
    summary: '承运商回传的事件如何对应到标准状态、什么情况自动立案。',
    owner: '全程追踪与异常（visibility-exception）',
    source:
      'docs/domain/visibility-exception/CONTEXT.md 标准里程碑映射按源上下文与事实类型版本化登记；高可信且命中版本化分诊规则的信号可自动建立或关联案件，低可信先进入分诊',
  },
  'disclosure-policies': {
    title: '通知与披露',
    summary: '向客户披露什么、通知的时限与渠道。',
    owner: '全程追踪与异常（visibility-exception）',
    source:
      'docs/domain/visibility-exception/CONTEXT.md 客户披露必须使用经批准的最小范围和内容；通知义务能否被满足由客户合同和通知策略另行判断（通知保存目标客户、要求时限和适用渠道）',
  },
  'claim-prerequisites': {
    title: '索赔资格与授权',
    summary: '哪些合同覆盖哪些索赔，谁可以代客户提出。',
    owner: '全程追踪与异常（visibility-exception）',
    source:
      'docs/domain/visibility-exception/CONTEXT.md 按申请人授权、客户账户、合同版本、索赔时限、目标范围、重复关系和最低材料要求判断资格',
  },

  // —— 账务（settlement-accounting）——
  'charges-billing': {
    title: '客户费用',
    summary: '每张运单的费用明细、计费重量与调整。',
    owner: '结算与经营核算（settlement-accounting）',
    source: 'docs/domain/settlement-accounting/CONTEXT.md 费用项目、计费重量采用、费用明细与费用调整的版本与依据',
  },
  reconciliation: {
    title: '对账单',
    summary: '发给客户的对账单，以及供应商送来的账单主张。',
    owner: '结算与经营核算（settlement-accounting）',
    source: 'docs/domain/settlement-accounting/CONTEXT.md「结算账户、对账与争议」对账单管理',
  },
  'settlement-application': {
    title: '收付款核销',
    summary: '把实际收到或付出的钱核到对账单与账户上。',
    owner: '结算与经营核算（settlement-accounting）',
    source:
      'docs/application/settlement-accounting/UC-SA-005-MAP-EXTERNAL-FUNDS-AND-APPLY-SETTLEMENT.md 与 CONTEXT.md「核销」「真实收付映射」',
  },
  'operating-metrics': {
    title: '经营核算',
    summary: '按口径派生的经营毛利与经营损失。',
    owner: '结算与经营核算（settlement-accounting）',
    source: 'docs/domain/settlement-accounting/CONTEXT.md 按明确口径、版本、币种及截至时点派生的经营毛利与经营损失指标',
  },

  // —— 账务 · 代收（collection-remittance）——
  'cod-ledger': {
    title: '代收货款',
    summary: '替客户代收的货款、回汇批次与差异。',
    owner: '代收与清分（collection-remittance）',
    source: 'docs/domain/collection-remittance/CONTEXT.md 代收指令、代收事实、代收分户账、回汇批次与差异事项',
  },

  // —— 设置 · 公司主体（party-commercial）——
  'group-legal-entities': {
    title: '公司主体',
    summary: '我们自己用来签约、开票与结算的法人主体。',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 运营企业集团及其法人、经营组织和业务身份',
  },

  // —— 设置 · 审批与发布（party-commercial）——
  'pending-publications': {
    title: '审批与发布',
    summary: '等待批准的合同、价格与规则发布稿。',
    owner: '参与方与商业（party-commercial）',
    source: 'docs/domain/party-commercial/CONTEXT.md 待批准发布与商业版本的批准；docs/adr/0314 待批准载体的读回与摘要绑定',
  },

  // —— 设置 · 基础数据（产品内置目录；不是限界上下文）——
  'reference-data': {
    title: '基础数据',
    summary: '国家和地区、币种、计量单位的标准目录；国家和币种另可登记本公司启用哪些。',
    owner: '产品内置基础数据（platform/refdata；不是限界上下文）；启用范围登记归参与方与商业（party-commercial）',
    source:
      'docs/adr/0335-countries-currencies-and-units-are-product-built-in-read-only-reference-data-released-with-the-code-and-unit-conversion-keeps-precision-and-is-not-yet-wired-into-pricing.md 决定一、三、六、七；启用范围见 docs/adr/0345-tenant-enablement-of-countries-and-currencies-is-a-tenant-value-registered-in-party-commercial-and-new-write-entries-accept-only-catalogue-codes.md',
  },

  // —— 设置 · 试点治理（pilotgovernance）——
  'stage-admission': {
    title: '试点治理',
    summary: '产品实例的阶段决定、暂停与恢复，只给管理员。',
    owner: '试点治理（pilotgovernance）',
    source: 'docs/design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md',
  },
};

export const pageTitleById: Record<string, string> = {
  workbench: '工作台',
  'shipment-request': '新建与撤回',
  'template-preview': '演示',
  ...Object.fromEntries(
    Object.entries(moduleInfoById).map(([id, info]) => [id, info.title]),
  ),
};
