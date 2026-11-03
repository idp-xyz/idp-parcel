// 对象页「分区」版式的纯判读（票 company-subject-ux/13）：照 Fiori Object Page / Dynamic Page 的通则——锚点栏吸顶、
// 各分区单页纵排、头部随滚动收起。DetailPageTemplate 引 ESM-only 的 @idpxyz 原语，run-tests 的 CommonJS 发射加载不了它，
// 要钉的规则抬到这里用 node:test 钉；模板只管量尺寸与摆。这里不引 React，也不碰 DOM。
//
// 坐标约定：下面的「top」都是分区在滚动容器内容坐标里的位置（容器 scrollTop 为 0 时分区顶边到容器顶边的距离），
// 锚点栏在内容流里占头一段、再 sticky 吸顶，所以它的高度既是内容的一部分也是遮住内容的一条。

/** 一个分区在内容坐标里的位置；由模板量出来、按文档顺序给。 */
export interface SectionPosition<Id extends string = string> {
  id: Id;
  top: number;
}

/**
 * 点锚点要滚到的 scrollTop：分区顶边刚好贴在吸顶的锚点栏下沿。锚点栏在内容流里占着头一段，所以减掉的是它自己的高度；
 * 第一个分区在栏下不足一栏高的位置时，退到 0 而不是负数。
 */
export function sectionScrollTop(sectionTop: number, barHeight: number): number {
  return Math.max(0, sectionTop - barHeight);
}

/**
 * 滚动时该高亮哪一条：视口里最靠上的分区，即顶边已经滚到锚点栏下沿（含一像素的容差，平滑滚动停在目标时差零点几像素）
 * 之上的最后一个；还没有任何分区滚上来（在页首）就是第一个。没有分区答 null。
 * 只看「顶边过没过线」而不看「露出多少」：后者在相邻两个分区都露一半时会来回跳。
 */
export function activeSectionAt<Id extends string>(
  sections: readonly SectionPosition<Id>[],
  scrollTop: number,
  barHeight: number,
): Id | null {
  if (sections.length === 0) return null;
  const line = scrollTop + barHeight + 1;
  let active = sections[0].id;
  for (const section of sections) {
    if (section.top <= line) active = section.id;
  }
  return active;
}

/**
 * 末尾垫高多少，让最后一个分区也能滚到锚点栏下沿：没有这一垫，短尾巴分区（折起的、内容少的）永远到不了顶，
 * 点它的锚点就停在半路、高亮也落在前一个分区上（UI5 ObjectPageLayout 用同样的办法在分区容器末尾垫一块）。
 * `contentHeight` 是不含这块垫的内容总高（锚点栏 + 各分区 + 内边距）。够高时垫 0。
 */
export function bottomSpacerHeight(
  lastSectionTop: number,
  barHeight: number,
  clientHeight: number,
  contentHeight: number,
): number {
  return Math.max(0, sectionScrollTop(lastSectionTop, barHeight) + clientHeight - contentHeight);
}

/**
 * 头部收起态随滚动怎么变（Dynamic Page 的 snap）：展开着的头在滚过阈值（meta 行与事实行的高度）后收起；
 * 收起的头只在回到页首时恢复。两边不用同一条线是为了不抖：收起时头矮了、滚动容器随之长高，scrollTop 可能被夹回
 * 阈值以下，用同一条线判会立刻又展开、再收起。
 */
export function headerCollapsedAfterScroll(collapsed: boolean, scrollTop: number, threshold: number): boolean {
  if (collapsed) return scrollTop > 0;
  return scrollTop > threshold;
}

/**
 * 一个分区此刻展不展开：操作者点过就听点过的，没点过听调用方的默认（`collapsed` 为真即默认折起）。
 * 不可折叠的分区（调用方没给 `collapsed`）永远展开，点也点不折。
 */
export function sectionExpanded(collapsedByDefault: boolean | undefined, toggledTo: boolean | undefined): boolean {
  if (collapsedByDefault === undefined) return true;
  return toggledTo ?? !collapsedByDefault;
}

/** 分区元素的 id（`<section id="section-<签 id>">`），锚点栏条目的 aria-controls 与滚动定位都按它找。 */
export function sectionElementId(id: string): string {
  return `section-${id}`;
}
