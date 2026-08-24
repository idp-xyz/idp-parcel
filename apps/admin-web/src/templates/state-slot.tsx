import type { ReactNode } from 'react';
import {
  LoadingState,
  EmptyState,
  ErrorState,
  UnconfiguredState,
} from '../components/states';

// 页面四态的判别联合：模板只认 kind，不自己猜「没数据算不算空」——
// 空态与未配置态的区分是业务判断（闸门未放行 ≠ 暂无数据），必须由调用方给出。
export type TemplateViewState =
  | { kind: 'ready' }
  | { kind: 'loading' }
  | { kind: 'empty'; title?: string; description?: string }
  | { kind: 'error'; title?: string; description?: string; onRetry?: () => void }
  | { kind: 'unconfigured'; title?: string; description?: string };

export interface StateSlotProps {
  state: Exclude<TemplateViewState, { kind: 'ready' }>;
  /** 逐态覆盖默认渲染；状态组件的默认呈现不合本页需要时，调用方可先用它绕行。 */
  override?: Partial<Record<'loading' | 'empty' | 'error' | 'unconfigured', () => ReactNode>>;
}

// 三个页面模板共享的四态渲染槽。状态组件从 ../components/states 按名导入；
// 集中在这一个文件里，组件契约变动时只改这里。
export function StateSlot({ state, override }: StateSlotProps) {
  switch (state.kind) {
    case 'loading':
      return <>{override?.loading ? override.loading() : <LoadingState />}</>;
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
            <UnconfiguredState title={state.title} description={state.description} />
          )}
        </>
      );
  }
}
