import { Fragment, useEffect, useState, type ReactNode } from 'react';
import { Check, Copy } from 'lucide-react';
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

/** 调用方给的一签：id 只能是六个稳定命名之一，词与顺序由 workspace-tabs 的固定表钉住。 */
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
   * 可选右侧上下文（手册「可选右侧上下文」）：关联对象、说明一类由调用方组装的内容，宽 280px，
   * 只在 xl 及以上显示——窄屏上它会把主列挤没，藏起来比挤着好。不传不占位。
   * 放进来的东西在窄屏上看不见，所以只放「看不见也不缺」的辅助内容，正文归签。
   */
  aside?: ReactNode;
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
  aside,
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
            <div key={field.label} className="flex gap-3 text-[12px] leading-5">
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
          <p className="text-[12px] text-idpxyz-textMuted">暂无审计记录。</p>
        ) : (
          <Timeline items={auditTrail} />
        )}
      </CardContent>
    </Card>
  );

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {objectHeader ? (
        <PageHeader className="items-start">
          <PageHeaderContent className="flex-col items-start gap-1">
            <div className="flex min-w-0 max-w-full items-center gap-3">
              <PageHeaderTitle>{title}</PageHeaderTitle>
              {identifier && (
                <span className="inline-flex items-center gap-1">
                  <span className="select-all font-mono text-[12px] text-idpxyz-accent">
                    {identifier}
                  </span>
                  <CopyIdentifierButton text={identifier} />
                </span>
              )}
              {status}
              {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
            </div>
            {meta !== undefined && meta.length > 0 && (
              <dl className="flex flex-wrap gap-x-4 gap-y-0.5 text-[11px] leading-4">
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
              <span className="font-mono text-[12px] text-idpxyz-accent">{identifier}</span>
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
    </div>
  );
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
}: {
  tabs: ResolvedWorkspaceTab<ReactNode>[];
  aside: ReactNode;
}) {
  const [active, setActive] = useState<WorkspaceTabId | undefined>(undefined);
  if (tabs.length === 0) return <div className="flex-1 overflow-auto" />;
  const value = active !== undefined && tabs.some((tab) => tab.id === active) ? active : tabs[0].id;
  return (
    <Tabs
      value={value}
      onValueChange={(next) => setActive(next as WorkspaceTabId)}
      className="flex-1 flex flex-col overflow-hidden gap-0"
    >
      <TabsList className="px-4 shrink-0">
        {tabs.map((tab) => (
          <TabsTrigger key={tab.id} value={tab.id}>
            {tab.label}
            {tab.count !== undefined && (
              <span className="text-[11px] font-normal text-idpxyz-textMuted">{tab.count}</span>
            )}
          </TabsTrigger>
        ))}
      </TabsList>
      <div className="flex-1 overflow-auto">
        <ContentColumns aside={aside}>
          {tabs.map((tab) => (
            <TabsContent key={tab.id} value={tab.id} className="flex flex-col gap-4">
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

// 指标带：结构照 ui-primitives StatCard（小写标签 + 大数），没有直接用它——它的 value 只收
// string | number、p-5 / text-2xl 是 Dashboard 尺度，对象头区下要的是能放徽章的格与更紧的密度。
// auto-fit 栅格让 4 格与 6 格都占满一排，不为凑数留空格。
function SummaryStrip({ stats }: { stats: DetailSummaryStat[] }) {
  return (
    <div className="grid shrink-0 grid-cols-[repeat(auto-fit,minmax(150px,1fr))] gap-3 border-b border-idpxyz-border px-4 py-3">
      {stats.map((stat) => (
        <Card key={stat.label} className="px-4 py-3 shadow-none hover:shadow-none">
          <div className="text-[11px] font-medium uppercase tracking-wider text-idpxyz-textMuted">
            {stat.label}
          </div>
          <div className="mt-1 text-lg font-semibold text-idpxyz-textBright">{stat.value}</div>
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
