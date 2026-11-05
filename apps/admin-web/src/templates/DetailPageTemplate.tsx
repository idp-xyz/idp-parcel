import { Fragment, useEffect, useLayoutEffect, useRef, useState, type ReactNode, type RefObject } from 'react';
import { Check, ChevronRight, Copy } from 'lucide-react';
import {
  PageHeader,
  PageHeaderContent,
  PageHeaderTitle,
  PageHeaderDescription,
  PageHeaderActions,
} from '@idpxyz/ui-patterns';
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Timeline,
} from '@idpxyz/ui-primitives';
import { StateSlot, type TemplateViewState, type StateSlotProps } from './state-slot';
import type { DetailField, AuditEntry } from './types';
import {
  resolveWorkspaceTabs,
  type ResolvedWorkspaceTab,
  type WorkspaceTabId,
  type WorkspaceTabInput,
} from './workspace-tabs';
import { WorkspaceTabLabel } from './workspace-tab-icons';
import {
  activeSectionAt,
  bottomSpacerHeight,
  headerCollapsedAfterScroll,
  sectionElementId,
  sectionExpanded,
  sectionScrollTop,
  type SectionPosition,
} from './object-page-sections';
import { TabCountBadge } from '../components/TabCountBadge';

/** 业务区块：一个区块渲染成一个 Panel（Card），内容由调用方组装。 */
export interface DetailSection {
  id: string;
  title: string;
  description?: string;
  content: ReactNode;
}

/**
 * 对象头区第二行的一格元信息（如「最后更新」「提交方」「修订」）。只显调用方给的：
 * 手册把「负责人」「风险」也列在头区，本仓没有这两样的领域来源，模板不为它们留格（票面裁决 2）。
 */
export interface DetailMeta {
  label: string;
  value: ReactNode;
}

/**
 * 指标带的一格。value 只放对象自身从读口原样来的值（件数、修订号、金额已确认与否），
 * 不放派生 KPI——目录读口一次拉全量且有截断上限，从截断列表数出来的数是假的（spec「不做」第一条）。
 * value 收 ReactNode 与 `status` / `DetailField.value` 同一约定：要带色调的值由调用方放徽章，
 * 色调 → 徽章变体的翻译只在 `domain/status.tsx` 一处，模板不另立一张表。
 */
export interface DetailSummaryStat {
  label: string;
  value: ReactNode;
}

/** 调用方给的一签：id 只能取稳定命名，词与顺序由 workspace-tabs 的固定表钉住。 */
export type DetailWorkspaceTab = WorkspaceTabInput<ReactNode>;

export interface DetailPageTemplateProps {
  /** 页面标题（如「托运申报单」）。 */
  title: string;
  /** 业务标识（如申报单号），展示在标题旁，等宽字体便于比对抄录。 */
  identifier?: string;
  /** 标题行右侧的状态展示，通常是 StatusBadge；语义归调用方。 */
  status?: ReactNode;
  description?: string;
  headerActions?: ReactNode;
  /**
   * 对象头区元信息行。对象工作区头区（两行、主编号可复制）随 meta 或 tabs **任一**传入而启用——
   * 传空数组也算传了，首用页正是靠 `tabs={[]}` + meta 切成头区形态；两者都不传才沿用单行头，
   * 另两张详情页因此一字节不变。
   */
  meta?: DetailMeta[];
  /** 指标带（手册 Summary Strip，4–6 格一排）。不传或传空不渲染，不显「—」占位格。 */
  summary?: DetailSummaryStat[];
  /** 「基本信息」区字段。两列栅格排布，字段多时自动换行。 */
  basicFields: DetailField[];
  /** 基本信息区标题，默认「基本信息」。 */
  basicTitle?: string;
  /** 业务区块（如包裹明细、判定翻译结果），按序渲染为独立 Panel。 */
  sections?: DetailSection[];
  /** 审计留痕；不传则不渲染审计区（区别于「传空数组=有区但暂无记录」）。 */
  auditTrail?: AuditEntry[];
  /** 审计区标题，默认「审计留痕」。 */
  auditTitle?: string;
  /**
   * 对象工作区的签。传了（哪怕是空数组）内容区就按签分：基本信息进「概要」顶部、区块跟在其后、
   * 审计留痕进「审计」，调用方同 id 的内容接在模板内容之后；没有内容的签不出（票面裁决 1）。
   * 传了它也同时把页头切成对象工作区头区（与 meta 同一开关，见上）。
   * 不传仍叠 Card——另两张详情页不改，母版与旧形态并存到它们各自的票再换（票面裁决 3）。
   */
  tabs?: DetailWorkspaceTab[];
  /**
   * 当前签（可选受控）。不传由模板自己记，与此前一致；传了由调用方记，签切换经 onActiveTabChange 报回。要它的是
   * 「页头动作要把人带到某一签」那种页面，模板自己记的话调用方够不着。
   * 传进来的签此刻没有内容时照旧退回第一签。
   */
  activeTab?: WorkspaceTabId;
  onActiveTabChange?: (tab: WorkspaceTabId) => void;
  /**
   * 签怎么摆（票 company-subject-ux/13）。`'tabs'`（缺省）是分签：一次只显一签，对象页默认都是它。`'sections'` 是分区，
   * 照 Fiori Object Page：签成锚点栏吸在滚动容器顶上，各签的内容按固定序全部排在一页上纵向滚动，点锚点平滑滚过去、
   * 滚动时按视口里最靠上的分区高亮；头区的 meta 行与指标带随滚动收起（Dynamic Page），标题行与动作留着；
   * `activeTab` / `onActiveTabChange` 在这里的含义是「当前锚点」——外部给的 id 滚到那一节。给主数据类对象（公司主体、
   * 往来单位）：它们的签是几块都要看的资料，不是几种互斥的视图。分区版式下 `tabs` 不传按空数组算。
   * 不传或传 `'tabs'` 时渲染与此前逐字节同：分区版式整条支路在前面提前返回，分签那条支路一字未动。
   */
  layout?: 'tabs' | 'sections';
  /**
   * 可选右侧上下文（手册「可选右侧上下文」）：关联对象、说明一类由调用方组装的内容，宽 280px，
   * 只在 xl 及以上显示——窄屏上它会把主列挤没，藏起来比挤着好。不传不占位。
   * 放进来的东西在窄屏上看不见，所以只放「看不见也不缺」的辅助内容，正文归签。
   */
  aside?: ReactNode;
  /**
   * 页脚动作条（Fiori 对象页的 footer toolbar，票 company-subject-section-edit/03）：对象进入编辑态时「保存 / 取消」钉在
   * 这里，不随内容滚、不被分区挤走；内容归调用方。只在 ready 态渲染——非 ready 态没有东西可保存。不传不渲染，渲染逐字节同。
   */
  footer?: ReactNode;
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
}

// 详情页模板：PageHeader + Panel 分区（基本信息 / 业务区块 / 审计区）。
// 非 ready 态替换整个内容区——详情页没有「部分可看」的中间态，
// 半份详情比没有详情更误导复核判断。
export function DetailPageTemplate({
  title,
  identifier,
  status,
  description,
  headerActions,
  meta,
  summary,
  basicFields,
  basicTitle = '基本信息',
  sections,
  auditTrail,
  auditTitle = '审计留痕',
  tabs,
  activeTab,
  onActiveTabChange,
  layout = 'tabs',
  aside,
  footer,
  viewState,
  stateOverride,
}: DetailPageTemplateProps) {
  // 手册「对象工作区」的 Object Header 随 meta 或 tabs 任一启用；旧形态那一支的 JSX
  // 原样保留而不抽成共用片段，是为了让「不传新 prop 时渲染逐字节同」能对着代码读出来。
  const objectHeader = meta !== undefined || tabs !== undefined;

  // 三类模板内容各造一次，叠 Card 与分签两种形态共用同一份元素——两边渲染的是同一个东西，
  // 差别只在装进哪个容器。
  const basicCard = (
    <Card>
      <CardHeader>
        <CardTitle>{basicTitle}</CardTitle>
      </CardHeader>
      <CardContent>
        <dl className="grid grid-cols-1 gap-x-8 gap-y-2.5 sm:grid-cols-2">
          {basicFields.map((field) => (
            <div key={field.label} className="flex gap-3 type-body leading-5">
              <dt className="w-[96px] shrink-0 text-idpxyz-textMuted">{field.label}</dt>
              <dd className="min-w-0 flex-1 break-words text-idpxyz-text">
                {field.value}
              </dd>
            </div>
          ))}
        </dl>
      </CardContent>
    </Card>
  );
  const sectionCards = sections?.map((section) => (
    <Card key={section.id}>
      <CardHeader>
        <CardTitle>{section.title}</CardTitle>
        {section.description && (
          <CardDescription>{section.description}</CardDescription>
        )}
      </CardHeader>
      <CardContent>{section.content}</CardContent>
    </Card>
  ));
  const auditCard = auditTrail && (
    <Card>
      <CardHeader>
        <CardTitle>{auditTitle}</CardTitle>
      </CardHeader>
      <CardContent>
        {auditTrail.length === 0 ? (
          <p className="type-body text-idpxyz-textMuted">暂无审计记录。</p>
        ) : (
          <Timeline items={auditTrail} />
        )}
      </CardContent>
    </Card>
  );

  // 分区版式整条支路在这里提前返回，下面分签与叠 Card 两条支路一字不动——「传 'tabs' 时渲染逐字节同」能对着代码读出来
  // （与上面保留旧形态头区 JSX 是同一个理由）。模板内容归哪一签的口径与下面分签那一处相同：基本信息按有没有字段判、
  // 审计留痕传了空数组仍算有内容。
  if (layout === 'sections') {
    return (
      <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
        <SectionsWorkspace
          title={title}
          identifier={identifier}
          status={status}
          description={description}
          headerActions={headerActions}
          meta={meta}
          summary={summary}
          ready={viewState.kind === 'ready'}
          tabs={resolveWorkspaceTabs<ReactNode>(
            {
              summary: [...(basicFields.length > 0 ? [basicCard] : []), ...(sectionCards ?? [])],
              audit: auditTrail ? [auditCard] : [],
            },
            (tabs ?? []).filter((tab) => hasRenderableContent(tab.content)),
          )}
          aside={aside}
          activeTab={activeTab}
          onActiveTabChange={onActiveTabChange}
          footer={footer}
          stateSlot={
            viewState.kind === 'ready' ? null : (
              <div className="flex-1 flex items-center justify-center overflow-auto">
                <StateSlot state={viewState} override={stateOverride} />
              </div>
            )
          }
        />
      </div>
    );
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {objectHeader ? (
        <PageHeader className="items-start">
          <PageHeaderContent className="flex-col items-start gap-1">
            <div className="flex min-w-0 max-w-full items-center gap-3">
              <PageHeaderTitle>{title}</PageHeaderTitle>
              {identifier && (
                <span className="inline-flex items-center gap-1">
                  <span className="select-all font-mono type-body text-idpxyz-accent">
                    {identifier}
                  </span>
                  <CopyIdentifierButton text={identifier} />
                </span>
              )}
              {status}
              {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
            </div>
            {meta !== undefined && meta.length > 0 && (
              <dl className="flex flex-wrap gap-x-4 gap-y-0.5 type-caption leading-4">
                {meta.map((item) => (
                  <div key={item.label} className="flex gap-1">
                    <dt className="text-idpxyz-textMuted">{item.label}</dt>
                    <dd className="text-idpxyz-text">{item.value}</dd>
                  </div>
                ))}
              </dl>
            )}
          </PageHeaderContent>
          {headerActions && <PageHeaderActions>{headerActions}</PageHeaderActions>}
        </PageHeader>
      ) : (
        <PageHeader>
          <PageHeaderContent>
            <PageHeaderTitle>{title}</PageHeaderTitle>
            {identifier && (
              <span className="font-mono type-body text-idpxyz-accent">{identifier}</span>
            )}
            {status}
            {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
          </PageHeaderContent>
          {headerActions && <PageHeaderActions>{headerActions}</PageHeaderActions>}
        </PageHeader>
      )}

      {viewState.kind === 'ready' && summary !== undefined && summary.length > 0 && (
        <SummaryStrip stats={summary} />
      )}

      {viewState.kind === 'ready' ? (
        tabs !== undefined ? (
          <WorkspaceTabs
            tabs={resolveWorkspaceTabs<ReactNode>(
              {
                // 基本信息是必传 prop，空数组分不出「不适用」与「暂无」，只能按有没有字段判有没有内容；
                // 审计留痕沿旧语义：传了空数组仍是「有区但暂无记录」，那句话本身是内容。
                summary: [...(basicFields.length > 0 ? [basicCard] : []), ...(sectionCards ?? [])],
                audit: auditTrail ? [auditCard] : [],
              },
              tabs.filter((tab) => hasRenderableContent(tab.content)),
            )}
            aside={aside}
            activeTab={activeTab}
            onActiveTabChange={onActiveTabChange}
          />
        ) : (
          <div className="flex-1 overflow-auto">
            <ContentColumns aside={aside}>
              {basicCard}

              {sectionCards}

              {auditCard}
            </ContentColumns>
          </div>
        )
      ) : (
        <div className="flex-1 flex items-center justify-center overflow-auto">
          <StateSlot state={viewState} override={stateOverride} />
        </div>
      )}
      {viewState.kind === 'ready' && hasRenderableContent(footer) ? <FooterBar>{footer}</FooterBar> : null}
    </div>
  );
}

/** 页脚动作条的容器：一道上边线、页面底色，内容由调用方摆（左边说话、右边动作，主动作靠右是调用方的事）。 */
function FooterBar({ children }: { children: ReactNode }) {
  return <div className="shrink-0 border-t border-idpxyz-border bg-idpxyz-bg px-4 py-2.5">{children}</div>;
}

// React 会把 null / undefined / 布尔 / 空串渲染成空，调用方给了这样一签等于没给——
// 归并前先剔掉，免得出一个点进去空白的签（票面裁决 1）。
function hasRenderableContent(node: ReactNode): boolean {
  return node !== null && node !== undefined && typeof node !== 'boolean' && node !== '';
}

// 内容列：不传 aside 时就是今天那一列（960px 居中），传了才在 xl 及以上多出右侧 280px 一列，
// 主列宽度不变、容器随之放宽。两种形态（叠 Card / 分签）都从这里过，右栏只在一处定义。
function ContentColumns({ aside, children }: { aside: ReactNode; children: ReactNode }) {
  if (!hasRenderableContent(aside)) {
    return <div className="mx-auto flex max-w-[960px] flex-col gap-4 p-4">{children}</div>;
  }
  return (
    <div className="mx-auto flex max-w-[960px] gap-4 p-4 xl:max-w-[1256px]">
      <div className="flex min-w-0 flex-1 flex-col gap-4">{children}</div>
      <aside className="hidden w-[280px] shrink-0 flex-col gap-4 xl:flex">{aside}</aside>
    </div>
  );
}

// 分签内容区。签的词与顺序来自 workspace-tabs 的固定表，这里只管装容器。
// 受控而不用 defaultValue：签随内容有无出没，当前签消失时退回第一签，不留一个选中了却没内容的空区。
function WorkspaceTabs({
  tabs,
  aside,
  activeTab,
  onActiveTabChange,
}: {
  tabs: ResolvedWorkspaceTab<ReactNode>[];
  aside: ReactNode;
  activeTab?: WorkspaceTabId;
  onActiveTabChange?: (tab: WorkspaceTabId) => void;
}) {
  const [ownActive, setOwnActive] = useState<WorkspaceTabId | undefined>(undefined);
  const active = activeTab ?? ownActive;
  if (tabs.length === 0) return <div className="flex-1 overflow-auto" />;
  const value = active !== undefined && tabs.some((tab) => tab.id === active) ? active : tabs[0].id;
  return (
    <Tabs
      value={value}
      onValueChange={(next) => {
        setOwnActive(next as WorkspaceTabId);
        onActiveTabChange?.(next as WorkspaceTabId);
      }}
      className="flex-1 flex flex-col overflow-hidden gap-0"
    >
      <TabsList className="px-4 shrink-0">
        {tabs.map((tab) => (
          <TabsTrigger key={tab.id} value={tab.id}>
            {/* 词取归并结果的 label 而不让 WorkspaceTabLabel 自己再查一遍表：签上显什么由 resolveWorkspaceTabs 一处定。 */}
            <WorkspaceTabLabel id={tab.id}>{tab.label}</WorkspaceTabLabel>
            {tab.count !== undefined && (
              tab.id === 'contacts'
                ? <TabCountBadge count={tab.count} />
                : <span className="type-caption font-normal text-idpxyz-textMuted">{tab.count}</span>
            )}
          </TabsTrigger>
        ))}
      </TabsList>
      <div className="flex-1 overflow-auto">
        <ContentColumns aside={aside}>
          {tabs.map((tab) => (
            <TabsContent
              key={tab.id}
              value={tab.id}
              forceMount={tab.keepMounted || undefined}
              className="flex flex-col gap-4 data-[state=inactive]:hidden"
            >
              {tab.contents.map((content, index) => (
                <Fragment key={index}>{content}</Fragment>
              ))}
            </TabsContent>
          ))}
        </ContentColumns>
      </div>
    </Tabs>
  );
}

/**
 * 分区版式（票 company-subject-ux/13）：头区 + 可收起的 meta 行与指标带 + 单页滚动的分区。头区标题行的 JSX 与上面对象工作区
 * 头区那一支重复而不抽成共用片段，理由同文件里保留旧形态头区的那条注释——让「传 'tabs' 时逐字节同」能对着代码读出来。
 * 收起态由滚动容器报上来（WorkspaceSections），这里只管把 meta 行与指标带折起来；非 ready 态头区一律展开——
 * 没有内容可滚，收着的头没有东西能把它放回来。
 */
function SectionsWorkspace({
  title,
  identifier,
  status,
  description,
  headerActions,
  meta,
  summary,
  ready,
  tabs,
  aside,
  activeTab,
  onActiveTabChange,
  footer,
  stateSlot,
}: {
  title: string;
  identifier?: string;
  status?: ReactNode;
  description?: string;
  headerActions?: ReactNode;
  meta?: DetailMeta[];
  summary?: DetailSummaryStat[];
  ready: boolean;
  tabs: ResolvedWorkspaceTab<ReactNode>[];
  aside: ReactNode;
  activeTab?: WorkspaceTabId;
  onActiveTabChange?: (tab: WorkspaceTabId) => void;
  footer?: ReactNode;
  stateSlot: ReactNode;
}) {
  const [collapsed, setCollapsed] = useState(false);
  const metaRef = useRef<HTMLDivElement>(null);
  const summaryRef = useRef<HTMLDivElement>(null);
  // 收起阈值即 meta 行与指标带此刻的高度：只在展开时读（收起时它们量出来是 0，而收起态只问「回没回到页首」）。
  const collapseThresholdOf = () => (metaRef.current?.offsetHeight ?? 0) + (summaryRef.current?.offsetHeight ?? 0);
  const headerCollapsed = collapsed && ready;
  return (
    <>
      <PageHeader className="items-start">
        <PageHeaderContent className="flex-col items-start">
          <div className="flex min-w-0 max-w-full items-center gap-3">
            <PageHeaderTitle>{title}</PageHeaderTitle>
            {identifier && (
              <span className="inline-flex items-center gap-1">
                <span className="select-all font-mono type-body text-idpxyz-accent">
                  {identifier}
                </span>
                <CopyIdentifierButton text={identifier} />
              </span>
            )}
            {status}
            {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
          </div>
          {meta !== undefined && meta.length > 0 && (
            <CollapsibleRows collapsed={headerCollapsed} innerRef={metaRef}>
              {/* 标题行与 meta 行的间距放在可收起块里面而不是 flex gap 上：块收成 0 高时 gap 还会留一道空。 */}
              <dl className="mt-1 flex flex-wrap gap-x-4 gap-y-0.5 type-caption leading-4">
                {meta.map((item) => (
                  <div key={item.label} className="flex gap-1">
                    <dt className="text-idpxyz-textMuted">{item.label}</dt>
                    <dd className="text-idpxyz-text">{item.value}</dd>
                  </div>
                ))}
              </dl>
            </CollapsibleRows>
          )}
        </PageHeaderContent>
        {headerActions && <PageHeaderActions>{headerActions}</PageHeaderActions>}
      </PageHeader>

      {ready && summary !== undefined && summary.length > 0 && (
        <CollapsibleRows collapsed={headerCollapsed} innerRef={summaryRef}>
          <SummaryStrip stats={summary} />
        </CollapsibleRows>
      )}

      {ready ? (
        <WorkspaceSections
          tabs={tabs}
          aside={aside}
          activeTab={activeTab}
          onActiveTabChange={onActiveTabChange}
          headerCollapsed={collapsed}
          onHeaderCollapsedChange={setCollapsed}
          collapseThresholdOf={collapseThresholdOf}
        />
      ) : (
        stateSlot
      )}
      {ready && hasRenderableContent(footer) ? <FooterBar>{footer}</FooterBar> : null}
    </>
  );
}

/**
 * 随头部收起的一块：高度在 1fr 与 0fr 之间过渡、透明度随之，只用 transition、不量高度——grid 行的 fr 过渡让内容块
 * 从真实高度收到 0 而不用先知道它多高（max-height 那条路要么量、要么猜个上限）。里层 min-h-0 + overflow-hidden 是
 * 这个技法的另一半：没有它 grid 项按内容撑开、行收不下去。收起时整块 inert：meta 行里有可复制的号（按钮），
 * 只裁掉高度的话 Tab 键还能落到看不见的按钮上，读屏也还能读到。
 */
function CollapsibleRows({
  collapsed,
  innerRef,
  children,
}: {
  collapsed: boolean;
  innerRef: RefObject<HTMLDivElement | null>;
  children: ReactNode;
}) {
  return (
    <div
      className={`grid w-full shrink-0 transition-[grid-template-rows,opacity] duration-200 ease-out ${
        collapsed ? 'grid-rows-[0fr] opacity-0' : 'grid-rows-[1fr] opacity-100'
      }`}
      inert={collapsed}
    >
      <div ref={innerRef} className="min-h-0 overflow-hidden">
        {children}
      </div>
    </div>
  );
}

/** 一次「滚到某分区」的要求；seq 让同一分区连点两次也再滚一遍。 */
interface ScrollRequest {
  id: WorkspaceTabId;
  smooth: boolean;
  seq: number;
}

function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true;
}

/** 各分区此刻在滚动容器内容坐标里的位置，按文档顺序。 */
function measureSections(scroller: HTMLElement): SectionPosition<WorkspaceTabId>[] {
  const origin = scroller.getBoundingClientRect().top - scroller.scrollTop;
  return Array.from(scroller.querySelectorAll<HTMLElement>('[data-section-id]')).map((element) => ({
    id: element.dataset.sectionId as WorkspaceTabId,
    top: element.getBoundingClientRect().top - origin,
  }));
}

/**
 * 分区内容区：锚点栏（复用分签的 Tabs 原语，所以长得与签一样、也带 tablist 的键盘走法）吸在滚动容器顶上，
 * 各分区按固定序排在一页上。判读（当前锚点、滚动偏移、末尾垫高、头部收起、折叠态）都在 object-page-sections.ts，
 * 这里只量与摆。
 *
 * 锚点栏用受控的 Tabs 却不接 onValueChange，改在每个条目上接 onClick：Radix 只在值变了才报，点已经高亮的那条
 * 就不会再滚回它的顶上；activationMode="manual" 让方向键只移焦点、Enter / 空格才滚。
 *
 * 外部给的 `activeTab`：与上次报回去的不同才算一次新的要求（滚过去、折着的展开）——自己报回去的锚点经调用方 state
 * 回流进来时不能再滚一遍，那会在人滚动途中把分区顶边一次次拽回栏下。首次（挂载时记住的锚点）直接跳、不平滑。
 */
function WorkspaceSections({
  tabs,
  aside,
  activeTab,
  onActiveTabChange,
  headerCollapsed,
  onHeaderCollapsedChange,
  collapseThresholdOf,
}: {
  tabs: ResolvedWorkspaceTab<ReactNode>[];
  aside: ReactNode;
  activeTab?: WorkspaceTabId;
  onActiveTabChange?: (tab: WorkspaceTabId) => void;
  headerCollapsed: boolean;
  onHeaderCollapsedChange: (collapsed: boolean) => void;
  collapseThresholdOf: () => number;
}) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const barRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState<WorkspaceTabId | undefined>(activeTab);
  const [toggled, setToggled] = useState<Partial<Record<WorkspaceTabId, boolean>>>({});
  const [spacer, setSpacer] = useState(0);
  const [scrollRequest, setScrollRequest] = useState<ScrollRequest | null>(null);
  const reportedRef = useRef<WorkspaceTabId | undefined>(undefined);

  const report = (id: WorkspaceTabId) => {
    if (reportedRef.current === id) return;
    reportedRef.current = id;
    onActiveTabChange?.(id);
  };
  const expand = (id: WorkspaceTabId) =>
    setToggled((previous) => (previous[id] === true ? previous : { ...previous, [id]: true }));
  const requestScroll = (id: WorkspaceTabId, smooth: boolean) =>
    setScrollRequest((previous) => ({ id, smooth, seq: (previous?.seq ?? 0) + 1 }));
  const goTo = (id: WorkspaceTabId) => {
    expand(id);
    setActive(id);
    report(id);
    requestScroll(id, true);
  };

  useEffect(() => {
    if (activeTab === undefined || activeTab === reportedRef.current) return;
    const first = reportedRef.current === undefined;
    reportedRef.current = activeTab;
    expand(activeTab);
    setActive(activeTab);
    // 挂载时记住的锚点就是第一个分区的话不动：页面从 0 开，不为把第一个分区顶边贴到栏下而吃掉顶上那点内边距。
    if (!first || activeTab !== tabs[0]?.id) requestScroll(activeTab, !first);
  }, [activeTab]);

  useLayoutEffect(() => {
    const scroller = scrollRef.current;
    if (!scrollRequest || !scroller) return;
    const target = scroller.querySelector<HTMLElement>(`[data-section-id="${scrollRequest.id}"]`);
    if (!target) return;
    const top = target.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;
    scroller.scrollTo({
      top: sectionScrollTop(top, barRef.current?.offsetHeight ?? 0),
      behavior: scrollRequest.smooth && !prefersReducedMotion() ? 'smooth' : 'auto',
    });
  }, [scrollRequest]);

  // 末尾垫高随内容与容器的尺寸重算：分区展开 / 折起、读口答回来内容长高、头部收起让容器长高，都会变。
  useLayoutEffect(() => {
    const scroller = scrollRef.current;
    const content = contentRef.current;
    if (!scroller || !content) return;
    const measure = () => {
      const positions = measureSections(scroller);
      const last = positions[positions.length - 1];
      const barHeight = barRef.current?.offsetHeight ?? 0;
      setSpacer(last ? bottomSpacerHeight(last.top, barHeight, scroller.clientHeight, barHeight + content.offsetHeight) : 0);
    };
    measure();
    if (typeof ResizeObserver === 'undefined') return;
    const observer = new ResizeObserver(measure);
    observer.observe(scroller);
    observer.observe(content);
    return () => observer.disconnect();
  }, [tabs.length]);

  // 卸载（页面态离开 ready）时把头放回来：没有滚动容器就没有人能再报「回到页首」。
  useEffect(() => () => onHeaderCollapsedChange(false), [onHeaderCollapsedChange]);

  const onScroll = () => {
    const scroller = scrollRef.current;
    if (!scroller) return;
    const next = activeSectionAt(measureSections(scroller), scroller.scrollTop, barRef.current?.offsetHeight ?? 0);
    if (next !== null && next !== active) {
      setActive(next);
      report(next);
    }
    const collapsed = headerCollapsedAfterScroll(headerCollapsed, scroller.scrollTop, collapseThresholdOf());
    if (collapsed !== headerCollapsed) onHeaderCollapsedChange(collapsed);
  };

  if (tabs.length === 0) return <div className="flex-1 overflow-auto" />;
  const value = active !== undefined && tabs.some((tab) => tab.id === active) ? active : tabs[0].id;
  return (
    <div ref={scrollRef} onScroll={onScroll} className="flex-1 overflow-auto">
      <div ref={barRef} className="sticky top-0 z-10 bg-idpxyz-editor">
        <Tabs value={value} activationMode="manual" className="gap-0">
          <TabsList className="px-4 shrink-0" aria-label="分区">
            {tabs.map((tab) => (
              <TabsTrigger
                key={tab.id}
                value={tab.id}
                aria-controls={sectionElementId(tab.id)}
                onClick={() => goTo(tab.id)}
              >
                <WorkspaceTabLabel id={tab.id}>{tab.label}</WorkspaceTabLabel>
                {tab.count !== undefined && (
                  tab.id === 'contacts'
                    ? <TabCountBadge count={tab.count} />
                    : <span className="type-caption font-normal text-idpxyz-textMuted">{tab.count}</span>
                )}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </div>
      <div ref={contentRef}>
        <ContentColumns aside={aside}>
          {/* 分区之间比卡之间空得多一些：分区是一级、卡是二级，间距也得分两级。 */}
          <div className="flex flex-col gap-7">
            {tabs.map((tab) => {
              const expanded = sectionExpanded(tab.collapsed, toggled[tab.id]);
              return (
                <section
                  key={tab.id}
                  id={sectionElementId(tab.id)}
                  data-section-id={tab.id}
                  aria-labelledby={`${sectionElementId(tab.id)}-title`}
                  className="flex flex-col gap-3"
                >
                  <SectionHeading
                    tab={tab}
                    expanded={expanded}
                    onToggle={
                      tab.collapsed === undefined
                        ? undefined
                        : () => setToggled((previous) => ({ ...previous, [tab.id]: !expanded }))
                    }
                  />
                  {expanded
                    ? tab.contents.map((content, index) => <Fragment key={index}>{content}</Fragment>)
                    : null}
                </section>
              );
            })}
          </div>
        </ContentColumns>
      </div>
      <div aria-hidden="true" style={{ height: spacer }} />
    </div>
  );
}

/**
 * 分区标题行：词 + 计数，下面一道线横贯整列。它是一级标题、卡的标题条是二级：一级取 15px 半粗（与联系人区的小标题
 * 同一档）、不装进任何底色里，二级（ProfileReadoutCard、CardTitle）更小且在卡自己的框里。可折叠的分区整个标题是一枚按钮，
 * 前面带转向的箭头；折着时标题后面接调用方给的一句摘要。h3：页头的标题是 h2，读屏按标题跳就是一节一节跳。
 */
function SectionHeading({
  tab,
  expanded,
  onToggle,
}: {
  tab: ResolvedWorkspaceTab<ReactNode>;
  expanded: boolean;
  onToggle?: () => void;
}) {
  const count =
    tab.count !== undefined ? (
      <span className="ml-1.5 type-body font-normal tabular-nums text-idpxyz-breadcrumb">{tab.count}</span>
    ) : null;
  return (
    <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 border-b border-idpxyz-border pb-1.5">
      <h3 id={`${sectionElementId(tab.id)}-title`} className="type-section font-semibold leading-6 text-idpxyz-textBright">
        {onToggle ? (
          <button
            type="button"
            aria-expanded={expanded}
            onClick={onToggle}
            className="inline-flex items-center gap-1 rounded hover:text-idpxyz-accent focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent"
          >
            <ChevronRight
              size={16}
              aria-hidden="true"
              className={`shrink-0 text-idpxyz-breadcrumb transition-transform ${expanded ? 'rotate-90' : ''}`}
            />
            {tab.label}
            {count}
          </button>
        ) : (
          <>
            {tab.label}
            {count}
          </>
        )}
      </h3>
      {!expanded && tab.summary !== undefined ? (
        <span className="type-body leading-5 text-idpxyz-breadcrumb">{tab.summary}</span>
      ) : null}
    </div>
  );
}

// 指标带：结构照 ui-primitives StatCard（小写标签 + 大数），没有直接用它——它的 value 只收
// string | number、p-5 / text-2xl 是 Dashboard 尺度，对象头区下要的是能放徽章的格与更紧的密度。
// auto-fit 栅格让 4 格与 6 格都占满一排，不为凑数留空格。
function SummaryStrip({ stats }: { stats: DetailSummaryStat[] }) {
  return (
    <div className="grid shrink-0 grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-3 border-b border-idpxyz-border px-4 py-3">
      {stats.map((stat) => (
        <Card key={stat.label} className="px-4 py-3 shadow-none hover:shadow-none">
          <div className="type-caption font-medium uppercase tracking-wider text-idpxyz-textMuted">
            {stat.label}
          </div>
          <div className="mt-1 type-title leading-7 font-semibold text-idpxyz-textBright">{stat.value}</div>
        </Card>
      ))}
    </div>
  );
}

// 主编号复制：抄录比对是操作员对主编号最常做的事。反馈只换图标一下，不走 Toast——
// 模板不该要求调用方套 ToastProvider。剪贴板 API 只在安全上下文存在，没有就只留 select-all 那条路。
function CopyIdentifierButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 1500);
    return () => clearTimeout(timer);
  }, [copied]);
  return (
    <button
      type="button"
      aria-label="复制主编号"
      title="复制主编号"
      onClick={() => {
        if (!navigator.clipboard) return;
        void navigator.clipboard.writeText(text).then(() => setCopied(true));
      }}
      className="rounded p-0.5 text-idpxyz-textMuted hover:bg-idpxyz-hover hover:text-idpxyz-text"
    >
      {copied ? <Check size={12} /> : <Copy size={12} />}
    </button>
  );
}
