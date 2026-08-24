// 页面模板桶导出。demo.ts 故意不在此导出：合成演示数据只允许演示入口
// 显式按路径引用，出现在这里会让生产页面一行 import 就把假数据带进去。

export {
  ListPageTemplate,
  type ListPageTemplateProps,
  type ListColumn,
  type ListSearchProps,
  type ListPaginationProps,
} from './ListPageTemplate';

export {
  DetailPageTemplate,
  type DetailPageTemplateProps,
  type DetailSection,
} from './DetailPageTemplate';

export {
  ReviewFlowTemplate,
  type ReviewFlowTemplateProps,
  type ReviewQueueItem,
  type ReviewDecision,
  type ReviewDecisionKind,
} from './ReviewFlowTemplate';

export { StateSlot, type TemplateViewState, type StateSlotProps } from './state-slot';
export type { DetailField, AuditEntry } from './types';
