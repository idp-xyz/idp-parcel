import type { ReactNode } from 'react';
import { Skeleton, SkeletonCard, SkeletonEventList, SkeletonTable } from '@idpxyz/ui-patterns';
import {
  LoadingState,
  EmptyState,
  ErrorState,
  UnconfiguredState,
  type UnconfiguredFacts,
} from '../components/states';
import { skeletonOf, type LoadingShape } from './loading-shape';

export type { LoadingShape } from './loading-shape';

// 页面四态的判别联合：模板只认 kind，不自己猜「没数据算不算空」——
// 空态与未配置态的区分是业务判断（闸门未放行 ≠ 暂无数据），必须由调用方给出。
export type TemplateViewState =
  | { kind: 'ready' }
  | { kind: 'loading'; shape?: LoadingShape; cols?: number; rows?: number }
  | { kind: 'empty'; title?: string; description?: string }
  | { kind: 'error'; title?: string; description?: string; onRetry?: () => void }
  | { kind: 'unconfigured'; title?: string; description?: string; facts?: UnconfiguredFacts };

export type { UnconfiguredFacts } from '../components/states';

export interface StateSlotProps {
  state: Exclude<TemplateViewState, { kind: 'ready' }>;
  /** 逐态覆盖默认渲染；状态组件的默认呈现不合本页需要时，调用方可先用它绕行。 */
  override?: Partial<Record<'loading' | 'empty' | 'error' | 'unconfigured', () => ReactNode>>;
}

/**
 * 按形状出骨架（形状 → 件的映射在 loading-shape.ts，这里只摆）。`table` 的 SkeletonTable 自己是个 tbody，得套在 table
 * 里才立得住；行列数不传就用 ui-patterns 的默认（列数模板不知道，真实列数归知道列定义的那一方传）。`detail` 是头区
 * 一行加两块卡片，像对象页的头 + 概要。外层一律 role="status"，与 LoadingState 同一语义：还没答案，不承诺进度。
 */
export function LoadingSkeleton({ shape, cols, rows }: { shape?: LoadingShape; cols?: number; rows?: number }) {
  switch (skeletonOf(shape)) {
    case 'table':
      return (
        <div className="w-full overflow-hidden" role="status" aria-label="加载中">
          <table className="w-full">
            <SkeletonTable rows={rows} cols={cols} />
          </table>
        </div>
      );
    case 'event-list':
      return (
        <div className="w-full max-w-[880px] px-6" role="status" aria-label="加载中">
          <SkeletonEventList rows={rows} />
        </div>
      );
    case 'header-and-cards':
      return (
        <div className="w-full max-w-[720px] px-6 space-y-4" role="status" aria-label="加载中">
          <Skeleton className="h-5 w-1/3" />
          <div className="grid grid-cols-2 gap-3">
            <SkeletonCard />
            <SkeletonCard />
          </div>
        </div>
      );
    case 'pulse-block':
      return <LoadingState />;
  }
}

// 三个页面模板共享的四态渲染槽。状态组件从 ../components/states 按名导入；
// 集中在这一个文件里，组件契约变动时只改这里。
export function StateSlot({ state, override }: StateSlotProps) {
  switch (state.kind) {
    case 'loading':
      return (
        <>
          {override?.loading ? (
            override.loading()
          ) : (
            <LoadingSkeleton shape={state.shape} cols={state.cols} rows={state.rows} />
          )}
        </>
      );
    case 'empty':
      return (
        <>
          {override?.empty ? (
            override.empty()
          ) : (
            <EmptyState title={state.title} description={state.description} />
          )}
        </>
      );
    case 'error':
      return (
        <>
          {override?.error ? (
            override.error()
          ) : (
            <ErrorState
              title={state.title}
              description={state.description}
              onRetry={state.onRetry}
            />
          )}
        </>
      );
    case 'unconfigured':
      return (
        <>
          {override?.unconfigured ? (
            override.unconfigured()
          ) : (
            <UnconfiguredState
              title={state.title}
              description={state.description}
              facts={state.facts}
            />
          )}
        </>
      );
  }
}
