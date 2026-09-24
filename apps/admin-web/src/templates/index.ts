// 页面模板桶导出。demo.ts 故意不在此导出：合成演示数据只允许演示入口
// 显式按路径引用，出现在这里会让生产页面一行 import 就把假数据带进去。

export {
  ListPageTemplate,
  type ListPageTemplateProps,
  type ListColumn,
  type ListSearchProps,
  type ListPaginationProps,
  type ListSortOption,
  type ListSortProps,
  type ListSavedView,
  type ListSavedViewsProps,
} from './ListPageTemplate';
export {
  densityRowPadding,
  filterBarSlots,
  rowInteraction,
  rowKeyOpens,
  type FilterBarSlot,
  type FilterBarSlotId,
  type RowInteraction,
  type RowInteractionInputs,
} from './list-page-structure';

export {
  DetailPageTemplate,
  type DetailPageTemplateProps,
  type DetailSection,
  type DetailMeta,
  type DetailSummaryStat,
  type DetailWorkspaceTab,
} from './DetailPageTemplate';
export {
  resolveWorkspaceTabs,
  workspaceTabLabels,
  workspaceTabOrder,
  type ResolvedWorkspaceTab,
  type WorkspaceTabId,
  type WorkspaceTabInput,
} from './workspace-tabs';

export {
  ReviewFlowTemplate,
  type ReviewFlowTemplateProps,
  type ReviewQueueItem,
  type ReviewDecision,
  type ReviewDecisionKind,
  type ReviewDecisionOption,
} from './ReviewFlowTemplate';

export {
  StateSlot,
  type TemplateViewState,
  type StateSlotProps,
  type LoadingShape,
  type UnconfiguredFacts,
} from './state-slot';
export { resolveBreadcrumb, type TemplateBreadcrumb } from './breadcrumb';
export type { DetailField, AuditEntry } from './types';
export type {
  ListSelectionProps,
  ListBulkActionsProps,
  ListCsvExportProps,
} from './ListPageTemplate';
export {
  clearSelection,
  retainSelectedRows,
  rowsToCsv,
  selectedOnPage,
  selectedRows,
  toggleAllOnPage,
  toggleSelected,
  type CsvCellText,
  type CsvColumn,
  type PageSelectionState,
} from './list-selection';

export {
  INSPECTOR_EMPTY_NOTE,
  INSPECTOR_SUMMARY_LIMIT,
  InspectorContractError,
  inspectorActionDisabled,
  inspectorSectionDefaultOpen,
  inspectorSectionLabels,
  inspectorSectionOrder,
  presentFields,
  resolveInspectorSections,
  type InspectorAction,
  type InspectorContent,
  type InspectorField,
  type InspectorRelatedLink,
  type InspectorSection,
  type InspectorSectionKind,
  type InspectorStatusItem,
  type ResolvedInspectorSection,
  INSPECTOR_CONTRACT_ERROR_TITLE,
  resolveInspectorForPanel,
  type InspectorResolution,
} from './inspector';
export { InspectorProvider, useInspector, type InspectorController } from './inspector-context';
export { InspectorPanel, type InspectorPanelProps } from './InspectorPanel';
