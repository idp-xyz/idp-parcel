// 右侧检查器的内容契约与归并（票 admin-web-workspace-form/02 第 1 条）。这里不引 React 也不引 @idpxyz 原语，
// 好让 run-tests 的 CommonJS 发射能直接加载它——渲染归 InspectorPanel，这里只管「有哪几节、叫什么、谁在谁前、什么不许」。
//
// 出处：蓝图（idp-ui@6751fb2 docs/oms_ui_ux_blueprint_v_1.md）10.8 Inspector Panel 的六节减掉 Notes——本仓没有备注读口，
// 一个永远空的节不是「禁用态 + 说明」能交代的；13 节约束「不成为第二张页、长表单不进、宽度稳定、必要时折叠节」落成
// 概要 ≤ 8 格与固定节序两条硬规则。参照 apps/myshop-web/src/shell/RightSidebar.tsx 按对象种类分派渲染器、每个由折叠节组成，
// 但它的 `addToast('操作成功')` 假快捷操作一条不搬：没有端点的动作必须给 disabledReason，不允许 onRun 空转（spec 红线）。

/**
 * 五节的稳定命名，渲染顺序即此序；调用方传进来的顺序不算数。五个 id 只在这里写一遍——InspectorSectionKind 从它派生，
 * 多一个少一个都不会出现「类型有、表里没有」的缝（与 templates/workspace-tabs.ts 同一手法）。
 */
export const inspectorSectionOrder = ['summary', 'status', 'actions', 'related', 'audit'] as const;

export type InspectorSectionKind = (typeof inspectorSectionOrder)[number];

/** id → 节标题。词由这里钉住，调用方改不了。 */
export const inspectorSectionLabels: Record<InspectorSectionKind, string> = {
  summary: '概要',
  status: '状态',
  actions: '快速动作',
  related: '关联对象',
  audit: '审计',
};

/** 默认展开的节：看一眼要看到的三节开着，关联与审计折起来（蓝图 13 节「必要时折叠节」）。 */
export const inspectorSectionDefaultOpen: Record<InspectorSectionKind, boolean> = {
  summary: true,
  status: true,
  actions: true,
  related: false,
  audit: false,
};

/** 概要格数上限（蓝图 13.2「不成为第二张页」）：超过它的内容属详情页。 */
export const INSPECTOR_SUMMARY_LIMIT = 8;

/** 本页供内容、还没选中行时的一句空态；不放假内容。 */
export const INSPECTOR_EMPTY_NOTE = '在列表里单击一行，这里显示它的概要';

/** 本页不往检查器里交内容时的空态。 */
export const INSPECTOR_IDLE_NOTE = '本页没有要在检查器里显示的内容';

/**
 * 空态句按本页供不供内容二选一：栏对所有页常驻，而「在列表里单击一行」只在接了检查器的列表页上为真——
 * spec 红线「留位只允许禁用态 + 说明」要的是一句为真的说明。
 */
export function inspectorEmptyNote(pageOffersContent: boolean): string {
  return pageOffersContent ? INSPECTOR_EMPTY_NOTE : INSPECTOR_IDLE_NOTE;
}

/** 内容违约时栏里那一段错误的标题；违反了哪条由 InspectorContractError 的说明补上。 */
export const INSPECTOR_CONTRACT_ERROR_TITLE = '这一行的检查器内容不合契约，没有显示';

/** 键值一格；值是已排好版的字，取自行里已有字段，不发第二个请求。 */
export interface InspectorField {
  label: string;
  value: string;
  /** 标识、时刻一类等宽显示。 */
  mono?: boolean;
}

/** 状态簇的一枚：word 是 CONTEXT 原词（domain/status 的词表词按词表着色与定层，词表外的原样示文，不猜色调）。 */
export interface InspectorStatusItem {
  label: string;
  word: string;
}

/**
 * 快速动作。有端点的给 onRun；没有的**必须**给 disabledReason（禁用态 + 说明）；两者都没有是契约错误——那是一个看起来能按、
 * 按了什么都不发生的按钮，resolveInspectorSections 对它抛，不静默放行。两者都给时按禁用处理（说明在场就不该能按）。
 */
export interface InspectorAction {
  label: string;
  onRun?: () => void;
  disabledReason?: string;
}

/** 关联对象：点即写 hash，与各模块页写地址的形一致。 */
export interface InspectorRelatedLink {
  label: string;
  hash: string;
}

export type InspectorSection =
  | { kind: 'summary'; fields: InspectorField[] }
  | { kind: 'status'; items: InspectorStatusItem[] }
  | { kind: 'actions'; actions: InspectorAction[] }
  | { kind: 'related'; links: InspectorRelatedLink[] }
  | { kind: 'audit'; fields: InspectorField[] };

export interface InspectorContent {
  title: string;
  /** 标识一类的第二行，等宽显示。 */
  subtitle?: string;
  sections: InspectorSection[];
}

/** 归并后要渲染的一节。 */
export interface ResolvedInspectorSection {
  kind: InspectorSectionKind;
  label: string;
  defaultOpen: boolean;
  section: InspectorSection;
}

/** 契约错误：调用方给的内容违反本文件的硬规则。它是编程错误不是运行时状况，所以抛而不是回一个空节。 */
export class InspectorContractError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'InspectorContractError';
  }
}

function isEmptySection(section: InspectorSection): boolean {
  switch (section.kind) {
    case 'summary':
    case 'audit':
      return section.fields.length === 0;
    case 'status':
      return section.items.length === 0;
    case 'actions':
      return section.actions.length === 0;
    case 'related':
      return section.links.length === 0;
  }
}

/** 一节里各格的名字；关联对象按地址认。 */
function sectionEntryNames(section: InspectorSection): string[] {
  switch (section.kind) {
    case 'summary':
    case 'audit':
      return section.fields.map((f) => f.label);
    case 'status':
      return section.items.map((i) => i.label);
    case 'actions':
      return section.actions.map((a) => a.label);
    case 'related':
      return section.links.map((l) => l.hash);
  }
}

function firstRepeated(names: string[]): string | null {
  const seen = new Set<string>();
  for (const name of names) {
    if (seen.has(name)) return name;
    seen.add(name);
  }
  return null;
}

/** 同一种节给了多次就按序接起来；接法按节的内容类型各自拼数组。 */
function mergeSections(kind: InspectorSectionKind, parts: InspectorSection[]): InspectorSection {
  switch (kind) {
    case 'summary':
      return { kind, fields: parts.flatMap((p) => (p.kind === 'summary' ? p.fields : [])) };
    case 'audit':
      return { kind, fields: parts.flatMap((p) => (p.kind === 'audit' ? p.fields : [])) };
    case 'status':
      return { kind, items: parts.flatMap((p) => (p.kind === 'status' ? p.items : [])) };
    case 'actions':
      return { kind, actions: parts.flatMap((p) => (p.kind === 'actions' ? p.actions : [])) };
    case 'related':
      return { kind, links: parts.flatMap((p) => (p.kind === 'related' ? p.links : [])) };
  }
}

/**
 * 按固定序归并调用方给的节，校验硬规则：
 * - 顺序固定，调用方的顺序不算数；同一种节给多次按序接起来。
 * - 空节**不渲染**（不出现在结果里），而不是渲染一个空标题——节是导航，点不进的导航是死路。
 * - 概要超过 INSPECTOR_SUMMARY_LIMIT 格抛：多出来的属详情页，截断会静默丢字段、放行会让检查器长成第二张页。
 * - 动作既无 onRun 也无 disabledReason 抛：那是假动作。
 * - 同一节里重名抛（关联对象按地址）：两格同名，读的人分不清哪格是哪格；同一种节给多次接起来时最容易撞上。
 */
export function resolveInspectorSections(content: InspectorContent): ResolvedInspectorSection[] {
  const resolved: ResolvedInspectorSection[] = [];
  for (const kind of inspectorSectionOrder) {
    const parts = content.sections.filter((s) => s.kind === kind);
    if (parts.length === 0) continue;
    const section = mergeSections(kind, parts);
    if (isEmptySection(section)) continue;
    const repeated = firstRepeated(sectionEntryNames(section));
    if (repeated !== null) {
      throw new InspectorContractError(
        `检查器「${content.title}」的「${inspectorSectionLabels[kind]}」节里「${repeated}」出现了不止一次；同一节里一格一个名字`,
      );
    }
    if (section.kind === 'summary' && section.fields.length > INSPECTOR_SUMMARY_LIMIT) {
      throw new InspectorContractError(
        `检查器概要最多 ${INSPECTOR_SUMMARY_LIMIT} 格，「${content.title}」给了 ${section.fields.length} 格；多出来的属详情页`,
      );
    }
    if (section.kind === 'actions') {
      for (const action of section.actions) {
        if (action.onRun === undefined && action.disabledReason === undefined) {
          throw new InspectorContractError(
            `检查器动作「${action.label}」既没有 onRun 也没有 disabledReason：没有端点的动作要给禁用说明，不许空转`,
          );
        }
      }
    }
    resolved.push({ kind, label: inspectorSectionLabels[kind], defaultOpen: inspectorSectionDefaultOpen[kind], section });
  }
  return resolved;
}

/** 面板上一次归并的结果：要么是要渲染的节，要么是一条契约错误的说明。 */
export type InspectorResolution =
  | { kind: 'sections'; sections: ResolvedInspectorSection[] }
  | { kind: 'contractError'; message: string };

/**
 * 面板渲染时用的归并：契约错误接住成说明，别的错误照抛。契约错误仍是编程错误、仍要响亮，但它只关乎这一行的检查器内容——
 * 在渲染里抛出去，React 会卸掉直到最近错误边界的整棵子树，一行内容违约就拖垮了外壳；接住后栏里照样说出违反了哪条，别处不受牵连。
 * 不分开发 / 生产：概要格数随数据变（presentFields 丢空格），开发期样例行过得去的页，生产里全满的那一行才撞上。
 */
export function resolveInspectorForPanel(content: InspectorContent): InspectorResolution {
  try {
    return { kind: 'sections', sections: resolveInspectorSections(content) };
  } catch (error) {
    if (error instanceof InspectorContractError) return { kind: 'contractError', message: error.message };
    throw error;
  }
}

/** 一个动作在面板上是不是禁用态：给了说明就是（即便同时给了 onRun——说明在场就不该能按）。 */
export function inspectorActionDisabled(action: InspectorAction): boolean {
  return action.disabledReason !== undefined;
}

/** 空字符串的格不进检查器：一格「—」是屏幕上的留白记号不是数据，页面组装 fields 时用它过一遍。 */
export function presentFields(fields: (InspectorField | null | undefined)[]): InspectorField[] {
  return fields.filter((f): f is InspectorField => f !== null && f !== undefined && f.value.trim() !== '');
}
