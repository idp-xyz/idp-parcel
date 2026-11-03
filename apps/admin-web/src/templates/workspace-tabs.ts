// 对象工作区的签：固定表与归并（票 admin-web-ux-alignment/04）。这里不引 React 也不引 @idpxyz 原语，
// 好让 run-tests 的 CommonJS 发射能直接加载它——渲染归 DetailPageTemplate，这里只管「有哪几个签、叫什么、谁在谁前」。

/**
 * 手册「对象工作区（Workspace）统一规范」Main Content Tabs 的稳定命名，渲染顺序即此序；调用方传进来的顺序不算数。
 * 稳定 id 只在这里写一遍——WorkspaceTabId 从它派生，不会出现「类型有、表里没有」的缝。
 */
export const workspaceTabOrder = [
  'summary',
  'contacts',
  'profile-versions',
  'identity-revisions',
  'timeline',
  'related',
  'exceptions',
  'documents',
  'audit',
] as const;

/** 调用方只能给固定表中的 id，签名与顺序统一维护。 */
export type WorkspaceTabId = (typeof workspaceTabOrder)[number];

/** id → 签上的词。词由这里钉住，调用方改不了。 */
export const workspaceTabLabels: Record<WorkspaceTabId, string> = {
  summary: '概要',
  contacts: '联系人',
  'profile-versions': '资料版本',
  // 身份登记的版本链在界面上叫「登记记录」（spec 公司主体 UX「统一用语」）：签名只说这是一串登记，
  // 「身份」「修订」两个领域词留给 CONTEXT 与技术详情。
  'identity-revisions': '登记记录',
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
  /** 编辑签切走时保留草稿与查询状态；未声明的签沿用卸载行为。 */
  keepMounted?: boolean;
  /**
   * 只对「分区」版式有意义（票 company-subject-ux/13）：给了就是可折叠的分区，为真默认折起——折起时只显标题行与
   * `summary`，内容等展开才挂载，重区块不白取数。分签版式不看这两格。
   */
  collapsed?: boolean;
  /** 折起时标题行后的一句摘要（「第 3 版 · 2026-11-01 起生效 · 共 3 版」）。 */
  summary?: Content;
}

/** 归并后要渲染的一签：内容按序摊平，模板内容在前。 */
export interface ResolvedWorkspaceTab<Content> {
  id: WorkspaceTabId;
  label: string;
  contents: Content[];
  count?: number;
  keepMounted?: boolean;
  collapsed?: boolean;
  summary?: Content;
}

/**
 * 模板自身内容与调用方内容按 id 合、按固定序排。
 *
 * - 模板内容（基本信息 / 区块 → 概要，审计留痕 → 审计）在前，调用方同 id 内容接在其后。
 * - 没有任何内容的 id 不出签——签是导航，点不进的导航是死路（票面裁决 1）。
 * - 调用方同一 id 传多次就依次接上；count 取第一个给了的，不相加：计数是调用方对那一签的陈述，
 *   模板不替它做算术。summary 同 count 取第一个给了的；collapsed 同 keepMounted——给了就算可折叠，有一个要折起就折起：
 *   折起只是少挂一段内容，展开才挂；两个调用方一个要折一个不要时，挂少的那一边不会错挂。
 */
export function resolveWorkspaceTabs<Content>(
  templateContents: Partial<Record<WorkspaceTabId, readonly Content[]>>,
  callerTabs: readonly WorkspaceTabInput<Content>[] | undefined,
): ResolvedWorkspaceTab<Content>[] {
  const resolved: ResolvedWorkspaceTab<Content>[] = [];
  for (const id of workspaceTabOrder) {
    const contents: Content[] = [...(templateContents[id] ?? [])];
    let count: number | undefined;
    let keepMounted: boolean | undefined;
    let collapsed: boolean | undefined;
    let summary: Content | undefined;
    for (const tab of callerTabs ?? []) {
      if (tab.id !== id) continue;
      contents.push(tab.content);
      if (count === undefined) count = tab.count;
      if (tab.keepMounted) keepMounted = true;
      if (tab.collapsed !== undefined) collapsed = (collapsed ?? false) || tab.collapsed;
      if (summary === undefined) summary = tab.summary;
    }
    if (contents.length === 0) continue;
    resolved.push({ id, label: workspaceTabLabels[id], contents, count, keepMounted, collapsed, summary });
  }
  return resolved;
}
