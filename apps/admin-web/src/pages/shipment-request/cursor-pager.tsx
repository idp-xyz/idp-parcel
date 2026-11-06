import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { cursorPageSummary, cursorPagerControls, type ListCursorPaginationProps } from '../../templates';

// ReviewFlowTemplate 与 WorkbenchPageTemplate 没有 pagination 槽，翻页条由页面自己摆。字句与两钮能不能按取模板导出的
// cursorPageSummary / cursorPagerControls，与 ListPageTemplate 的翻页条同一口径。
export function CursorPager({ pagination }: { pagination: ListCursorPaginationProps }) {
  const { page, size, total, next, onPrevious, onNext } = pagination;
  const { canPrevious, canNext } = cursorPagerControls(page, next);
  return (
    <nav
      aria-label="翻页"
      className="flex items-center justify-between gap-4 type-caption text-idpxyz-textMuted"
      data-cursor-pagination
    >
      <span className="tabular-nums">{cursorPageSummary(page, size, total)}</span>
      <div className="flex items-center gap-1">
        <Button variant="ghost" size="sm" disabled={!canPrevious} onClick={onPrevious}>
          <ChevronLeft className="h-3 w-3" aria-hidden="true" />
          上一页
        </Button>
        <Button variant="ghost" size="sm" disabled={!canNext} onClick={() => next !== null && onNext(next)}>
          下一页
          <ChevronRight className="h-3 w-3" aria-hidden="true" />
        </Button>
      </div>
    </nav>
  );
}

/** 页底一条，边框与留白同 ListPageTemplate 摆翻页条的那一格。 */
export function CursorPagerBar({ pagination }: { pagination: ListCursorPaginationProps }) {
  return (
    <div className="shrink-0 border-t border-idpxyz-border px-4 py-1.5">
      <CursorPager pagination={pagination} />
    </div>
  );
}

/**
 * 翻过第一页之后答出零行，说的是「这一页往后没有了」——队列里的行处理掉之后常见——不是册为空：照收窄办（catalogue-view 的
 * narrowed），页面留在就绪态，翻页条跟着留下，好往回翻；落成空态就连翻页条一起收掉了。
 */
export function pastFirstPage(pagination: ListCursorPaginationProps | undefined): boolean {
  return pagination !== undefined && pagination.page > 1;
}
