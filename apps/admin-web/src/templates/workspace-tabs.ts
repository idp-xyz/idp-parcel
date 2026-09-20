// 对象工作区的签：固定表与归并（票 admin-web-ux-alignment/04）。这里不引 React 也不引 @idpxyz 原语，
// 好让 run-tests 的 CommonJS 发射能直接加载它——渲染归 DetailPageTemplate，这里只管「有哪几个签、叫什么、谁在谁前」。

/**
 * 手册「对象工作区（Workspace）统一规范」Main Content Tabs 的稳定命名：换产品不换签名。
 * 调用方只能给这六个 id 之一，给不了新签。
 */
export type WorkspaceTabId = 'summary' | 'timeline' | 'related' | 'exceptions' | 'documents' | 'audit';

/** 渲染顺序即此序；调用方传进来的顺序不算数。 */
export const workspaceTabOrder = [
  'summary',
  'timeline',
  'related',
  'exceptions',
  'documents',
  'audit',
] as const satisfies readonly WorkspaceTabId[];

/** id → 签上的词。词由这里钉住，调用方改不了。 */
export const workspaceTabLabels: Record<WorkspaceTabId, string> = {
  summary: '概要',
  timeline: '时间线',
  related: '关联',
  exceptions: '异常',
  documents: '文档',
  audit: '审计',
};

/** 调用方给的一签：只有 id 与内容，可选一个计数（如关联对象几件）。 */
export interface WorkspaceTabInput<Content> {
  id: WorkspaceTabId;
  content: Content;
  count?: number;
}

/** 归并后要渲染的一签：内容按序摊平，模板内容在前。 */
export interface ResolvedWorkspaceTab<Content> {
  id: WorkspaceTabId;
  label: string;
  contents: Content[];
  count?: number;
}

/**
 * 模板自身内容与调用方内容按 id 合、按固定序排。
 *
 * - 模板内容（基本信息 / 区块 → 概要，审计留痕 → 审计）在前，调用方同 id 内容接在其后。
 * - 没有任何内容的 id 不出签——签是导航，点不进的导航是死路（票面裁决 1）。
 * - 调用方同一 id 传多次就依次接上；count 取第一个给了的，不相加：计数是调用方对那一签的陈述，
 *   模板不替它做算术。
 */
export function resolveWorkspaceTabs<Content>(
  templateContents: Partial<Record<WorkspaceTabId, readonly Content[]>>,
  callerTabs: readonly WorkspaceTabInput<Content>[] | undefined,
): ResolvedWorkspaceTab<Content>[] {
  const resolved: ResolvedWorkspaceTab<Content>[] = [];
  for (const id of workspaceTabOrder) {
    const contents: Content[] = [...(templateContents[id] ?? [])];
    let count: number | undefined;
    for (const tab of callerTabs ?? []) {
      if (tab.id !== id) continue;
      contents.push(tab.content);
      if (count === undefined) count = tab.count;
    }
    if (contents.length === 0) continue;
    resolved.push({ id, label: workspaceTabLabels[id], contents, count });
  }
  return resolved;
}
