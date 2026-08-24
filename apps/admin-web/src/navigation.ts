import type { ElementType } from 'react';
import {
  LayoutDashboard,
  ClipboardCheck,
  AlertTriangle,
  FlaskConical,
  Scale,
  Send,
  ShieldCheck,
  BadgeDollarSign,
  TrendingUp,
  Calculator,
} from 'lucide-react';
import type { NavigationSection } from '@idpxyz/ui-workspace';

// 导航只列有文档出处的模块：ADR-0018 论证过的桌面治理场景、PN-08 的阶段治理，
// UC-PS-001/005 的提交与撤回（对 parcel-api 真实端点），
// 以及 parcel-pricing 的计价查阅面（CONTEXT.md 与 ADR-0013）；
// 不虚构其他模块；新增条目前先确认它有同等级的文档出处。
export const navigationSections: NavigationSection[] = [
  {
    title: '总览',
    items: [{ id: 'workbench', label: '工作台', icon: 'workbench' }],
  },
  {
    title: '委托',
    items: [{ id: 'shipment-request', label: '提交与撤回', icon: 'shipment-request' }],
  },
  {
    title: '治理与复核',
    items: [
      { id: 'acceptance-review', label: '接受前人工复核', icon: 'acceptance-review' },
      { id: 'exception-triage', label: '异常分诊与处置协调', icon: 'exception-triage' },
      { id: 'reconciliation', label: '对账与核销', icon: 'reconciliation' },
      { id: 'stage-admission', label: '阶段决定与暂停恢复', icon: 'stage-admission' },
    ],
  },
  {
    title: '计价',
    items: [
      { id: 'price-card-catalog', label: '价卡目录', icon: 'price-card-catalog' },
      { id: 'reference-series', label: '计价参考序列', icon: 'reference-series' },
      { id: 'pricing-evaluation', label: '价格评价', icon: 'pricing-evaluation' },
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
  'shipment-request': Send,
  'acceptance-review': ClipboardCheck,
  'exception-triage': AlertTriangle,
  reconciliation: Scale,
  'stage-admission': ShieldCheck,
  'template-preview': FlaskConical,
  'price-card-catalog': BadgeDollarSign,
  'reference-series': TrendingUp,
  'pricing-evaluation': Calculator,
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
  'acceptance-review': {
    title: '接受前人工复核',
    owner: '小包托运（parcel-shipment）',
    source: 'docs/domain/parcel-shipment/CONTEXT.md「适用规则显式要求人工业务判断」',
  },
  'exception-triage': {
    title: '异常分诊与处置协调',
    owner: '全程追踪与异常（visibility-exception）',
    source: 'docs/domain/visibility-exception/CONTEXT.md 异常案件「进入人工复核」与处置协调',
  },
  reconciliation: {
    title: '对账与核销',
    owner: '结算与经营核算（settlement-accounting）',
    source: 'docs/domain/settlement-accounting/CONTEXT.md 对账与金额核销',
  },
  'stage-admission': {
    title: '阶段决定与暂停恢复',
    owner: '试点治理（pilotgovernance）',
    source: 'docs/design/pn-08-end-to-end-pilot-and-stage-admission-development-handoff.md',
  },
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
};

export const pageTitleById: Record<string, string> = {
  workbench: '工作台',
  'shipment-request': '提交与撤回',
  'template-preview': '模板预览（合成 S）',
  ...Object.fromEntries(
    Object.entries(moduleInfoById).map(([id, info]) => [id, info.title]),
  ),
};
