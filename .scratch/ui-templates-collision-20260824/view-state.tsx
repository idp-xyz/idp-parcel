import type { ReactNode } from 'react';
// 四态组件契约由另一会话交付（components/states），此处按约定名盲写导入；
// 本文件是模板层对该契约的唯一引用点，契约若有出入只需改这里。
// 当前假定的最小 props 面：
//   LoadingState       —— 无必填 props
//   EmptyState         —— { description?: string }
//   ErrorState         —— { description?: string; onRetry?: () => void }
//   UnconfiguredState  —— { description?: string }
import {
  LoadingState,
  EmptyState,
  ErrorState,
  UnconfiguredState,
} from '../components/states';

/**
 * 视图五值状态：四个非就绪态 + ready。
 * 状态一律由页面通过 props 注入，模板不从 rows.length 之类推导——
 * 「空列表」与「未接线」在治理页面是两种不同的真话，推导会把后者伪装成前者。
 */
export type ViewStatus = 'loading' | 'unconfigured' | 'error' | 'empty' | 'ready';

/** 各非就绪态的展示文案与动作，全部可选；不传时由状态组件自身兜底。 */
export interface ViewStateNotes {
  /** error 态的补充说明（如后端返回的错误摘要）。 */
  errorDescription?: string;
  /** error 态的重试动作；不传则不出现重试入口。 */
  onRetry?: () => void;
  /** empty 态的补充说明（如「当前筛选下没有记录」）。 */
  emptyDescription?: string;
  /** unconfigured 态的补充说明（如所依赖的准入闸门/文档出处）。 */
  unconfiguredDescription?: string;
}

/**
 * 状态闸：ready 时渲染 children，否则渲染对应状态组件并居中。
 * 供三个页面模板复用，页面代码不直接接触 components/states。
 */
export function ViewStateGate({
  status,
  notes,
  children,
}: {
  status: ViewStatus;
  notes?: ViewStateNotes;
  children: ReactNode;
}) {
  if (status === 'ready') return <>{children}</>;

  let stateNode: ReactNode;
  switch (status) {
    case 'loading':
      stateNode = <LoadingState />;
      break;
    case 'unconfigured':
      stateNode = <UnconfiguredState description={notes?.unconfiguredDescription} />;
      break;
    case 'error':
      stateNode = <ErrorState description={notes?.errorDescription} onRetry={notes?.onRetry} />;
      break;
    case 'empty':
      stateNode = <EmptyState description={notes?.emptyDescription} />;
      break;
  }

  return <div className="flex-1 flex items-center justify-center overflow-auto">{stateNode}</div>;
}
