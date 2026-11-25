import { useMemo } from 'react';
import { useMayMakeDecision } from '../use-operator-grants';
import { approveStockImport, rejectStockImport, submitStockImport } from './api';
import type { GuidedActionTarget } from './GuidedActionPanel';
import { enrichedStockImportForms } from './guided-schemas';
import { stockImportOutcomeLabels, stockImportSnapshotHints } from './presentation';

const submitDecision = 'STOCK_IMPORT_SUBMISSION';
// 批准与驳回挂的是库存调整的批准授予：建账导入与库存调整共用同一套批准职责。
const approvalDecision = 'STOCK_ADJUSTMENT_APPROVAL';

/**
 * 建账导入的三个写动作：提交、批准入账、驳回，按操作者持有的授予筛——明确没有那一种授予的不摆（摆不摆只是呈现，
 * 拒绝仍由服务端判）。列表页与单据页共用这一份，两处的动作与答复译法不会各写一套。
 */
export function useStockImportTargets(): ReadonlyMap<string, GuidedActionTarget> {
  const maySubmit = useMayMakeDecision(submitDecision);
  const mayDecide = useMayMakeDecision(approvalDecision);
  return useMemo(() => {
    const map = new Map<string, GuidedActionTarget>();
    if (maySubmit) {
      map.set('submit', {
        id: 'submit',
        label: '提交建账导入',
        title: '提交建账导入',
        endpoint: 'POST /warehouse-fulfillment-stock-import-submissions',
        snapshotHint: stockImportSnapshotHints.submit,
        submit: submitStockImport,
        outcomeLabels: stockImportOutcomeLabels,
        form: enrichedStockImportForms.submit,
      });
    }
    if (mayDecide) {
      map.set('approve', {
        id: 'approve',
        label: '批准入账',
        title: '批准建账导入并入账',
        endpoint: 'POST /warehouse-fulfillment-stock-import-approvals',
        snapshotHint: stockImportSnapshotHints.approve,
        submit: approveStockImport,
        outcomeLabels: stockImportOutcomeLabels,
        form: enrichedStockImportForms.approve,
      });
      map.set('reject', {
        id: 'reject',
        label: '驳回导入',
        title: '驳回建账导入',
        endpoint: 'POST /warehouse-fulfillment-stock-import-rejections',
        snapshotHint: stockImportSnapshotHints.reject,
        submit: rejectStockImport,
        outcomeLabels: stockImportOutcomeLabels,
        form: enrichedStockImportForms.reject,
      });
    }
    return map;
  }, [maySubmit, mayDecide]);
}
