// 交付范围核对态在管理台上的写法（票 ps-external-mark-relations/13「做什么」第 2 项；ADR-0212 决定五）。
//
// 单独成模块：待识别实物区的「为何待识别」「交付范围核对」「依据 · 此刻归入」三列都要读核对态，词只在这里定一份。最容易错的是把四件事
// 写成一个样——「缺范围」（候选在、没有可验证交付范围）、「范围成立但冲突」、「零候选」与「未核对」——它们候选列可以一样、冲突列可以一样，
// 恢复动作却各不相同：补证据、处置、等委托接受之后重核、等重核问到。

import type { UnidentifiedItemRecord } from './records-api';

/** 核对态封闭三值（domain DeliveryScopeCheckState）。 */
export const deliveryScopeStateLabels: Record<string, string> = {
  VERIFIED: '已证',
  UNVERIFIED: '未证',
  UNCONFIGURED: '未配置',
};

/** 未证的原因封闭五值（domain DeliveryScopeUnverifiedReason），中文取 ADR-0212 决定五的原词。 */
export const deliveryScopeReasonLabels: Record<string, string> = {
  NO_BASIS: '没有依据',
  BASIS_NOT_REGISTERED: '依据不在册',
  BASIS_NOT_APPLICABLE: '依据不适用',
  NUMBER_NOT_PRE_ADVISED: '号码不在预报',
  BINDINGS_NOT_ALL_CONFIRMED: '交接单绑定未全部确认',
};

/** 「依据不适用」的细分封闭四值（domain DeliveryScopeInapplicability）。 */
export const deliveryScopeInapplicabilityLabels: Record<string, string> = {
  NODE_MISMATCH: '节点不符',
  OUTSIDE_WINDOW: '在时窗外',
  WITHDRAWN: '已撤回',
  NOT_A_HANDOVER_MEMBER: '不是交接单成员',
};

/** 依据种类封闭两值（domain DeliveryScopeBasisKind）。 */
export const deliveryScopeBasisKindLabels: Record<string, string> = {
  DELIVERY_PRE_ADVICE: '送站预报',
  DELIVERY_HANDOVER_NOTE: '送站交接单',
};

type ScopeCheckFields = Pick<
  UnidentifiedItemRecord,
  'deliveryScopeCheck' | 'deliveryScopeUnverifiedReason' | 'deliveryScopeInapplicability'
>;

/**
 * 「交付范围核对」一格。没记写明「未记」并说它不是未证——这一版没做范围推导，替它写成未证是编一个没做过的判断；页面未收录的码原样示出。
 */
export function deliveryScopeCheckCellOf(record: ScopeCheckFields): string {
  const state = record.deliveryScopeCheck;
  if (state === undefined || state === '') return '未记：这一版没做范围推导（不等于未证）';
  if (state === 'VERIFIED') return '已证';
  if (state === 'UNCONFIGURED') return '未配置：本租户没登记这一刻生效的预报有效时窗';
  if (state !== 'UNVERIFIED') return `核对态 ${state}（页面未收录）`;
  const reason = record.deliveryScopeUnverifiedReason ?? '';
  const reasonText = deliveryScopeReasonLabels[reason] ?? `原因 ${reason || '未带'}（页面未收录）`;
  const detail = record.deliveryScopeInapplicability;
  if (!detail) return `未证 · ${reasonText}`;
  return `未证 · ${reasonText}（${deliveryScopeInapplicabilityLabels[detail] ?? `${detail}，页面未收录`}）`;
}

// 补证据的两条路（ADR-0212 决定五「范围未证怎样解开」）：交接单得到确认，或实物归入一张能成立范围的交接单，然后经重核。归入是为
// 「来源没带依据、预报没覆盖这个号、原交接单被否认」三种情形立的补救（决定三）；客户事后再交一份预报不会成为这件实物存下的依据。
const ASSIGN_TO_OPEN_NOTE = '把它归入本节点一张开放的送站交接单，绑定全部确认后自动重核';

/**
 * 核对态指向的恢复动作。未配置与未证分开写：前者要租户去登记时窗，后者要补证据（ADR-0212 决定五「未配置与未证分开记」）。已证与没记
 * 不给动作——已证还待识别的，缺的不是范围，看「为何待识别」那一格。
 */
export function deliveryScopeRecoveryOf(record: ScopeCheckFields): string {
  switch (record.deliveryScopeCheck) {
    case 'UNCONFIGURED':
      return '租户登记预报有效时窗（登记册配置写面）后，人工重核这一版';
    case 'UNVERIFIED':
      break;
    default:
      return '';
  }
  switch (record.deliveryScopeUnverifiedReason) {
    case 'BINDINGS_NOT_ALL_CONFIRMED':
      return '到「送站交接单」区确认那张交接单其余账户的绑定；全部确认后自动重核';
    case 'BASIS_NOT_REGISTERED':
      return `核对出示的预报或交接单标识；或${ASSIGN_TO_OPEN_NOTE}`;
    default:
      return ASSIGN_TO_OPEN_NOTE;
  }
}

/** 「为何待识别」的封闭分格。 */
export type PendingCause =
  | 'UNCHECKED'
  | 'CHECK_NOT_RECORDED'
  | 'CONFLICT_WITHIN_SCOPE'
  | 'CONFLICT'
  | 'ZERO_CANDIDATES'
  | 'SCOPE_MISSING'
  | 'SCOPE_UNCONFIGURED'
  | 'NO_COMPATIBLE_CANDIDATE_IN_SCOPE'
  | 'SCOPE_NOT_RECORDED';

type PendingFields = Pick<UnidentifiedItemRecord, 'identityCheck' | 'identityConflict' | 'candidates'> & ScopeCheckFields;

/**
 * 一件待识别实物为何仍待识别。次序有意：没问到 parcel-shipment 的先说（候选空也不是查无此号）；记冲突的再按有没有范围分（范围成立但冲突
 * 只经处置解开，ADR-0212 决定四）；零候选不看范围（决定四「一个也没有」）；剩下候选在、不记冲突的，才按核对态说缺的是什么。
 */
export function pendingCauseOf(record: PendingFields): { cause: PendingCause; label: string } {
  if (record.identityCheck === 'UNCHECKED') {
    return { cause: 'UNCHECKED', label: '未核对：没问到 parcel-shipment，候选空不代表查无此号' };
  }
  if (record.identityConflict) {
    return record.deliveryScopeCheck === 'VERIFIED'
      ? { cause: 'CONFLICT_WITHIN_SCOPE', label: '范围成立但冲突：范围内仍不止一件或有同号声明，经处置解开' }
      : { cause: 'CONFLICT', label: '身份冲突（没有可验证交付范围）：看冲突来由一列' };
  }
  if (record.candidates.length === 0) {
    return record.identityCheck === 'CHECKED'
      ? { cause: 'ZERO_CANDIDATES', label: '零候选：parcel-shipment 此刻查无标识这个号的包裹' }
      : { cause: 'CHECK_NOT_RECORDED', label: '候选空、未带身份核对态：查没查过不知道' };
  }
  switch (record.deliveryScopeCheck) {
    case 'UNVERIFIED':
      return { cause: 'SCOPE_MISSING', label: '缺范围：有候选，但没有可验证交付范围（未证）' };
    case 'UNCONFIGURED':
      return { cause: 'SCOPE_UNCONFIGURED', label: '缺范围：预报有效时窗未配置' };
    case 'VERIFIED':
      return { cause: 'NO_COMPATIBLE_CANDIDATE_IN_SCOPE', label: '范围成立，但范围内没有相容的候选' };
    default:
      return { cause: 'SCOPE_NOT_RECORDED', label: '有候选、不记冲突，这一版没记范围核对态' };
  }
}

type BasisFields = Pick<UnidentifiedItemRecord, 'sourcePreserved' | 'deliveryScopeBasis' | 'assignedHandoverNote'>;

/**
 * 「依据 · 此刻归入」一格：来源里上报的依据引用与此刻最后归入的交接单，两样都写。来源未保全时那一格依据本无处可存，写「来源未保全」，不写
 * 「来源没带依据」。
 */
export function deliveryScopeBasisCellOf(record: BasisFields): string {
  const basis = record.deliveryScopeBasis;
  const source = !record.sourcePreserved
    ? '来源未保全'
    : basis
      ? `${deliveryScopeBasisKindLabels[basis.kind] ?? basis.kind} ${basis.id}`
      : '来源没带依据';
  const assigned = record.assignedHandoverNote ? `此刻归入 ${record.assignedHandoverNote}` : '未归入交接单';
  return `${source} · ${assigned}`;
}
