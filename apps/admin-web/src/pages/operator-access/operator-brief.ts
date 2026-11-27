// 操作者页把册上的授予说成人话。这五组只是这一页的说法，与验收矩阵里的岗位分组一致，册上没有角色这一行。
// 组外的决定种类归「其他」。看得见的客户和仓只认生效中的数据可见范围；没有就是看不见，* 才是全部。

import type { OperatorGrantRecord, OperatorRecord } from './api';
import { grantStandingAt, workScopeKindLabels } from './presentation';

export interface DecisionBundle {
  id: string;
  label: string;
  kinds: readonly string[];
}

export const decisionBundles: readonly DecisionBundle[] = [
  {
    id: 'acceptance',
    label: '客服受理',
    kinds: [
      'MANUAL_REVIEW_COMPLETION',
      'ACTIVE_REJECTION',
      'AUTHORIZED_DISPOSITION',
      'CONTROLLED_CLOSURE',
      'CONTROLLED_REOPENING',
      'IDENTITY_CONFLICT_DISPOSITION',
      'PENDING_IDENTITY_RECHECK',
      'POST_INTAKE_MANUAL_ITEM_HANDLING',
      'POST_INTAKE_SERVICE_DISPOSITION',
      'CONTROL_RELEASE_REFUSAL_HANDLING',
      'MANUAL_MEASUREMENT_ENTRY',
      'CONDITION_RESTORE',
      'DELIVERY_HANDOVER_CONFIRMATION',
    ],
  },
  {
    id: 'dispatch',
    label: '调度',
    kinds: [
      'LOAD_ASSIGNMENT',
      'PARTICIPATION_TERMINATION',
      'SEGMENT_CLOSURE',
      'FULFILLMENT_SEGMENT_COMMISSION_ADOPTION',
      'FULFILLMENT_SEGMENT_COMMISSION_ADOPTION_WITHDRAWAL',
      'TRANSPORT_COMMISSION_SUBMISSION',
      'TRANSPORT_COMMISSION_CANCELLATION',
      'TRANSPORT_COMMISSION_AGREEMENT_REVISION',
      'BOOKING_REQUEST_SUBMISSION',
      'BOOKING_REQUEST_CANCELLATION',
      'CAPACITY_RESERVATION',
      'CAPACITY_RELEASE',
      'CARRIER_FIRST_EFFECTIVE_PICKUP_JUDGMENT',
      'AUTHORIZED_REROUTE',
      'ALTERNATE_JOURNEY_START',
      'EFFECTIVE_TIME_JUDGMENT',
      'DISPATCH_TASK_REGISTRATION',
      'DELIVERY_DISPATCH_REASK',
      'DELIVERY_ATTEMPT_RESULT_CONFIRMATION',
      'PICKUP_ATTEMPT_RESULT_CONFIRMATION',
      'NODE_COLLABORATION_ACCEPTANCE',
      'TRANSPORT_CHARGE_OCCURRENCE_REGISTRATION',
      'TRANSPORT_CHARGE_OCCURRENCE_INVALIDATION',
    ],
  },
  {
    id: 'customs',
    label: '关务',
    kinds: [
      'CUSTOMS_CASE_ESTABLISHMENT',
      'CUSTOMS_CASE_CLOSURE',
      'CUSTOMS_DECLARATION_SUBMISSION',
      'CUSTOMS_SUBMISSION_AUTHORIZATION',
      'REGULATORY_CREDENTIAL_REGISTRATION',
      'REGULATORY_DISPOSITION_ACCEPTANCE',
      'RELEASE_GATE_VERIFICATION',
      'INTERNAL_RESTRICTION_ESTABLISHMENT',
      'INTERNAL_RESTRICTION_DECISION',
    ],
  },
  {
    id: 'finance',
    label: '财务',
    kinds: [
      'STATEMENT_PUBLICATION',
      'STATEMENT_DISPUTE_REGISTRATION',
      'STATEMENT_DISPUTE_RESOLUTION',
      'SUPPLIER_BILL_RECEPTION',
      'SUPPLIER_BILL_AUDIT',
      'SUPPLIER_BILL_DISPUTE_DECISION',
      'CHARGE_CONFIRMATION',
      'CHARGE_ADJUSTMENT_REGISTRATION',
      'EXTERNAL_FUNDS_FACT_ADOPTION',
    ],
  },
  {
    id: 'warehouse',
    label: '仓配',
    kinds: [
      'WAVE_RELEASE',
      'STOCK_IMPORT_SUBMISSION',
      'STOCK_COUNT',
      'STOCK_FREEZE',
      'STOCK_ADJUSTMENT_SUBMISSION',
      'STOCK_ADJUSTMENT_APPROVAL',
      'INBOUND_PRE_ADVICE_RECORD_ON_BEHALF',
      'INBOUND_PRE_ADVICE_CLOSE',
      'UNCLAIMED_ARRIVAL_CLAIM',
      'UNCLAIMED_ARRIVAL_DISPOSITION',
      'GOODS_RECORD_ON_BEHALF',
      'RECEIPT_DISCREPANCY_RECORD_ON_BEHALF',
      'OUTBOUND_REQUEST_RECORD_ON_BEHALF',
      'OUTBOUND_CONFIRMATION_CORRECTION',
      'RETURN_RECORD_ON_BEHALF',
      'PROCESSING_INSTRUCTION',
      'RELABEL_INSTRUCTION',
    ],
  },
];

const bundled = new Set(decisionBundles.flatMap((bundle) => bundle.kinds));

/** 词表里有、五组没收的决定种类，调整时归在「其他」。 */
export function unbundledDecisionKinds(kinds: readonly string[]): string[] {
  return kinds.filter((kind) => !bundled.has(kind));
}

export interface OperatorBrief {
  duty: string;
  visibility: string;
}

function countsNow(grant: OperatorGrantRecord, now: Date): boolean {
  return grantStandingAt(grant, now) === 'effective';
}

function visibilityPhrase(grants: readonly OperatorGrantRecord[], kind: string, noun: string, now: Date): string | null {
  const refs = grants
    .filter((grant) => countsNow(grant, now) && grant.dataVisibility?.kind === kind)
    .map((grant) => grant.dataVisibility?.reference ?? '')
    .filter((reference) => reference !== '');
  if (refs.includes('*')) return `${noun}全部`;
  if (refs.length > 0) return `${noun}只有 ${refs.join('、')}`;
  return null;
}

/** 名单和右栏标题用的两句。只数生效中的授予。 */
export function operatorBrief(operator: OperatorRecord, now: Date): OperatorBrief {
  const live = operator.grants.filter((grant) => countsNow(grant, now));
  const faces = new Set(live.map((grant) => grant.capabilityFace));
  const decisions = new Set(live.map((grant) => grant.decisionKind).filter((kind): kind is string => Boolean(kind)));
  const parts: string[] = [];
  if (faces.has('OPERATOR_ACCESS_ADMINISTRATION')) parts.push('管人');
  if (faces.has('REGISTRY_CONFIGURATION_WRITE')) parts.push('登记册');
  for (const bundle of decisionBundles) {
    const held = bundle.kinds.filter((kind) => decisions.has(kind)).length;
    if (held === 0) continue;
    parts.push(held === bundle.kinds.length ? bundle.label : `${bundle.label} ${held}/${bundle.kinds.length}`);
  }
  const other = [...decisions].filter((kind) => !bundled.has(kind)).length;
  if (other > 0) parts.push(`其他 ${other}`);
  const work = live.filter((grant) => grant.capabilityFace === 'WORK_FACT_REGISTRATION' && grant.workScope);
  if (work.length > 2) {
    parts.push(`现场作业 ${work.length} 处`);
  } else {
    for (const grant of work) {
      const scope = grant.workScope;
      if (!scope) continue;
      const kind = workScopeKindLabels[scope.kind] ?? scope.kind;
      parts.push(`现场作业 ${kind} ${scope.reference}`);
    }
  }
  if (parts.length === 0) parts.push(faces.has('MASTER_DATA_AND_OPERATIONS_READ') ? '只读' : '还不能做业务');
  const customer = visibilityPhrase(operator.grants, 'CUSTOMER_ACCOUNT', '客户', now);
  const warehouse = visibilityPhrase(operator.grants, 'WAREHOUSE', '仓', now);
  const visibility =
    customer === null && warehouse === null ? '看不见业务数据' : [customer ?? '客户没有', warehouse ?? '仓没有'].join(' · ');
  return { duty: parts.join('、'), visibility };
}
