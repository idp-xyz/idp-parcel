import { useMemo } from 'react';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import { useMayMakeDecision, useMayWriteRegistry } from '../use-operator-grants';
import {
  postWarehouseAction,
  warehouseActionOutcomeLabels,
  type WarehouseActionSpec,
} from './actions';
import { problemNote } from './presentation';

/**
 * 按操作者持有的授予筛选仓配页上的写动作。明确没有那一种授予的动作不摆出。
 */
export function useWarehouseActionTargets(specs: readonly WarehouseActionSpec[]): RegistrationTarget[] {
  const mayWriteRegistry = useMayWriteRegistry();
  const mayGoods = useMayMakeDecision('GOODS_RECORD_ON_BEHALF');
  const mayPreAdvice = useMayMakeDecision('INBOUND_PRE_ADVICE_RECORD_ON_BEHALF');
  const mayPreAdviceClose = useMayMakeDecision('INBOUND_PRE_ADVICE_CLOSE');
  const mayDiscrepancy = useMayMakeDecision('RECEIPT_DISCREPANCY_RECORD_ON_BEHALF');
  const mayClaim = useMayMakeDecision('UNCLAIMED_ARRIVAL_CLAIM');
  const mayDispose = useMayMakeDecision('UNCLAIMED_ARRIVAL_DISPOSITION');
  const mayReturn = useMayMakeDecision('RETURN_RECORD_ON_BEHALF');
  const mayRestore = useMayMakeDecision('CONDITION_RESTORE');
  const mayProcess = useMayMakeDecision('PROCESSING_INSTRUCTION');
  const mayRelabel = useMayMakeDecision('RELABEL_INSTRUCTION');
  const mayFreeze = useMayMakeDecision('STOCK_FREEZE');
  const mayCount = useMayMakeDecision('STOCK_COUNT');
  const mayAdjustSubmit = useMayMakeDecision('STOCK_ADJUSTMENT_SUBMISSION');
  const mayAdjustApprove = useMayMakeDecision('STOCK_ADJUSTMENT_APPROVAL');
  const mayOutbound = useMayMakeDecision('OUTBOUND_REQUEST_RECORD_ON_BEHALF');
  const mayWave = useMayMakeDecision('WAVE_RELEASE');
  const mayConfirmCorrect = useMayMakeDecision('OUTBOUND_CONFIRMATION_CORRECTION');

  return useMemo(() => {
    const allowed = (grant: WarehouseActionSpec['grant']): boolean => {
      switch (grant) {
        case 'registry':
          return mayWriteRegistry;
        case 'GOODS_RECORD_ON_BEHALF':
          return mayGoods;
        case 'INBOUND_PRE_ADVICE_RECORD_ON_BEHALF':
          return mayPreAdvice;
        case 'INBOUND_PRE_ADVICE_CLOSE':
          return mayPreAdviceClose;
        case 'RECEIPT_DISCREPANCY_RECORD_ON_BEHALF':
          return mayDiscrepancy;
        case 'UNCLAIMED_ARRIVAL_CLAIM':
          return mayClaim;
        case 'UNCLAIMED_ARRIVAL_DISPOSITION':
          return mayDispose;
        case 'RETURN_RECORD_ON_BEHALF':
          return mayReturn;
        case 'CONDITION_RESTORE':
          return mayRestore;
        case 'PROCESSING_INSTRUCTION':
          return mayProcess;
        case 'RELABEL_INSTRUCTION':
          return mayRelabel;
        case 'STOCK_FREEZE':
          return mayFreeze;
        case 'STOCK_COUNT':
          return mayCount;
        case 'STOCK_ADJUSTMENT_SUBMISSION':
          return mayAdjustSubmit;
        case 'STOCK_ADJUSTMENT_APPROVAL':
          return mayAdjustApprove;
        case 'OUTBOUND_REQUEST_RECORD_ON_BEHALF':
          return mayOutbound;
        case 'WAVE_RELEASE':
          return mayWave;
        case 'OUTBOUND_CONFIRMATION_CORRECTION':
          return mayConfirmCorrect;
        default:
          return true;
      }
    };

    return specs
      .filter((spec) => allowed(spec.grant))
      .map((spec) => ({
        id: spec.id,
        label: spec.label,
        title: spec.title,
        endpoint: `POST ${spec.endpoint}`,
        snapshotHint: spec.snapshotHint,
        submit: (snapshot: unknown) => postWarehouseAction(spec.endpoint, snapshot),
        outcomeLabels: warehouseActionOutcomeLabels,
      }));
  }, [
    specs,
    mayWriteRegistry,
    mayGoods,
    mayPreAdvice,
    mayPreAdviceClose,
    mayDiscrepancy,
    mayClaim,
    mayDispose,
    mayReturn,
    mayRestore,
    mayProcess,
    mayRelabel,
    mayFreeze,
    mayCount,
    mayAdjustSubmit,
    mayAdjustApprove,
    mayOutbound,
    mayWave,
    mayConfirmCorrect,
  ]);
}

export function WarehouseActionPanel({
  moduleId,
  targets,
}: {
  moduleId: string;
  targets: RegistrationTarget[];
}) {
  if (targets.length === 0) {
    return (
      <div className="p-6 type-body text-idpxyz-textMuted">
        当前操作者没有这一页所需的能力面或运营决定授予，没有可执行的写动作。
      </div>
    );
  }
  return <MultiRegistrationPanel moduleId={moduleId} targets={targets} problemNote={problemNote} />;
}
