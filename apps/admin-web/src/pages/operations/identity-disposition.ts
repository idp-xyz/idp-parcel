// 待识别实物页的「身份冲突处置」：按实物看各链当前有效版本、判此刻是否冲突、拼处置载荷、读处置答复（票 no-reception-identity/06；
// UC-NO-004、ADR-0210）。
//
// 单独成模块而不写在页面里，理由同 identity-recheck：最容易错的几格是纯函数——一件实物有几条链时「冲突」怎么判、处置候选从哪几版
// 来、「关联对象另有归属」的回显怎么带出须连带的实物、哪几格答复说的是没落版而不是失败。页面组件要 React 与 DOM，判读拿 node:test
// 就能钉住。

import { formatInstant } from '../catalogue-view';
import type {
  AssociatedUnitBody,
  CoDisposedUnit,
  IdentityDispositionRequest,
  IdentityDispositionResponseBody,
} from './identity-disposition-api';
import { labelOf, receptionKindLabels } from './presentation';
import type { IdentityDispositionRecord, IdentityReviewHead, IdentityReviewUnit } from './records-api';

/** 处置结果两种（domain IdentityDispositionResult）。 */
export const dispositionResultLabels: Record<string, string> = {
  DESIGNATED: '指定一件',
  NONE_OF_CANDIDATES: '都不是',
};

/** 处置口结果代数七格（application DispositionOutcome）。 */
export const dispositionOutcomeLabels: Record<string, string> = {
  IDENTITY_CONFLICT_DISPOSED: '身份冲突已处置——逐链追加了处置版',
  EXISTING_RESULT: '同一处置已在册——未再追加',
  ALREADY_DISPOSED: '这一版已被另一份处置接续——未覆盖',
  VERSION_SUPERSEDED: '所处置的版本已换代——未落版',
  NO_IDENTITY_CONFLICT: '无可处置的冲突——未落版',
  NOT_A_CANDIDATE: '指定的包裹不在处置候选之内——未落版',
  ASSOCIATED_ELSEWHERE: '指定的包裹此刻关联在别的实物上——未落版',
};

/** 一件实物此刻的身份冲突状况：照服务端同一读法，合起来判冲突、算处置候选。 */
export interface IdentityConflictState {
  conflicted: boolean;
  /** 各链当前有效版本上的候选与关联合起来的那一组，按标识排序；它们都出自 parcel-shipment 的身份核对答复。 */
  candidates: string[];
  /** 此刻某条链的链头是处置版时，那一份处置文档。 */
  disposition?: IdentityDispositionRecord;
}

/**
 * 判一件实物此刻是否身份冲突：有一版链头记了身份冲突，或带关联的链头指向不同的包裹（ADR-0210 决定五，与方向依据的身份一格同一
 * 读法）。服务端照样会判、照样答一格「无可处置的冲突」——页面先说清楚，只是省一次必然不落版的写。
 */
export function identityConflictStateOf(unit: IdentityReviewUnit): IdentityConflictState {
  const associations = new Set(unit.heads.flatMap((head) => (head.association ? [head.association] : [])));
  const conflicted = unit.heads.some((head) => head.identityConflict) || associations.size > 1;
  const candidates = [
    ...new Set(unit.heads.flatMap((head) => [...head.candidates, ...(head.association ? [head.association] : [])])),
  ].sort();
  const disposition = unit.heads.find((head) => head.disposition)?.disposition;
  return { conflicted, candidates, disposition };
}

/** 「处置」动作此刻能不能发起；能时带默认指名的那一版与可选的候选，不能时带一句为什么。 */
export type DispositionAvailability =
  | { kind: 'available'; disposedVersion: string; versions: string[]; candidates: string[] }
  | { kind: 'unavailable'; reason: string };

export function dispositionAvailabilityOf(unit: IdentityReviewUnit): DispositionAvailability {
  const state = identityConflictStateOf(unit);
  if (!state.conflicted) {
    return {
      kind: 'unavailable',
      reason: state.disposition ? '冲突已处置，此刻不判身份冲突' : '此刻不判身份冲突，无可处置的冲突',
    };
  }
  const versions = unit.heads.map((head) => head.judgmentVersion).filter((version) => version !== '');
  const target = unit.heads.find((head) => head.identityConflict && head.judgmentVersion) ?? unit.heads.find((head) => head.judgmentVersion);
  if (!target) return { kind: 'unavailable', reason: '没有判断版本，无从指名' };
  return { kind: 'available', disposedVersion: target.judgmentVersion, versions, candidates: state.candidates };
}

/** 一版链头的一句话：版本、判断、关联或候选、冲突。 */
export function headSummaryOf(head: IdentityReviewHead): string {
  const parts = [`${head.judgmentVersion || '—'} ${labelOf(receptionKindLabels, head.kind)}`];
  if (head.association) parts.push(`关联 ${head.association}`);
  if (head.candidates.length > 0) parts.push(`候选 ${head.candidates.join('、')}`);
  if (head.identityConflict) parts.push('记身份冲突');
  if (head.disposition) parts.push('处置版');
  return parts.join(' · ');
}

/** 处置文档的一句话：结果、指定的包裹或排除的那一组、连带实物、操作者、依据引用、时刻。 */
export function dispositionSummaryOf(disposition: IdentityDispositionRecord): string {
  const what =
    disposition.result === 'DESIGNATED'
      ? `指定 ${disposition.parcel ?? '—'}${
          (disposition.coDisposedUnits ?? []).length > 0
            ? `，连带 ${(disposition.coDisposedUnits ?? []).map((co) => co.unit).join('、')} 不是它`
            : ''
        }`
      : `都不是（排除 ${(disposition.excluded ?? []).join('、') || '—'}）`;
  return `${what} · 操作者 ${disposition.operator} · 依据 ${disposition.basis} · ${formatInstant(disposition.disposedAt)}`;
}

export interface IdentityReviewRow {
  key: string;
  unit: string;
  values: Readonly<Record<string, string>>;
  disposition: DispositionAvailability;
}

/** 一件实物转写成按实物列的一行：各链当前有效版本列在一起，分得出冲突是否已处置、处置了什么。 */
export function identityReviewRowOf(unit: IdentityReviewUnit): IdentityReviewRow {
  const state = identityConflictStateOf(unit);
  const status = state.conflicted ? '有冲突，待处置' : state.disposition ? '冲突已处置' : '无冲突';
  return {
    key: `identity-review:${unit.unit}`,
    unit: unit.unit,
    values: {
      unit: unit.unit,
      heads: unit.heads.map(headSummaryOf).join('；'),
      status,
      disposition: state.disposition ? dispositionSummaryOf(state.disposition) : '',
    },
    disposition: dispositionAvailabilityOf(unit),
  };
}

/** 复核页处置表单里人填的那几格。 */
export interface DispositionDraft {
  unit: string;
  disposedVersion: string;
  /** 指定的那件包裹；空即「都不是」。 */
  parcel: string;
  basis: string;
  coDisposedUnits: CoDisposedUnit[];
}

/** 拼处置载荷；依据引用空白时不发（服务端照样答坏报文，页面先说）。都不是不带包裹与连带实物。 */
export function dispositionRequestOf(
  draft: DispositionDraft,
): { kind: 'ready'; request: IdentityDispositionRequest } | { kind: 'incomplete'; reason: string } {
  if (draft.basis.trim() === '') return { kind: 'incomplete', reason: '处置依据引用必填' };
  if (draft.parcel === '') {
    return {
      kind: 'ready',
      request: { unit: draft.unit, disposedVersion: draft.disposedVersion, result: 'NONE_OF_CANDIDATES', basis: draft.basis },
    };
  }
  const request: IdentityDispositionRequest = {
    unit: draft.unit,
    disposedVersion: draft.disposedVersion,
    result: 'DESIGNATED',
    parcel: draft.parcel,
    basis: draft.basis,
  };
  if (draft.coDisposedUnits.length > 0) request.coDisposedUnits = draft.coDisposedUnits;
  return { kind: 'ready', request };
}

/**
 * 「关联对象另有归属」的回显带出须连带的实物：每件取它此刻那一版当前有效版本。连带实物的处置候选必须恰好只有指定的那件
 * （ADR-0210 决定四）；有一件还牵连别的包裹，就是它自己另有一桩冲突，要先单独处置那件——这时不给「一并提交」。
 */
export function coDisposedUnitsFromEcho(
  parcel: string,
  associated: AssociatedUnitBody[],
): { kind: 'ready'; units: CoDisposedUnit[] } | { kind: 'blocked'; reason: string } {
  const units: CoDisposedUnit[] = [];
  for (const unit of associated) {
    const candidates = new Set(
      unit.currentVersions.flatMap((version) => [...(version.candidates ?? []), ...(version.association ? [version.association] : [])]),
    );
    if (candidates.size !== 1 || !candidates.has(parcel) || unit.currentVersions.length === 0) {
      return { kind: 'blocked', reason: `${unit.unit} 还牵连别的包裹，先单独处置那件` };
    }
    units.push({ unit: unit.unit, version: unit.currentVersions[0].judgmentVersion });
  }
  return { kind: 'ready', units };
}

/**
 * 处置答复的呈现。`tone` 两色：`answered`（形成了的业务答案，含没落版的六格——它们是服务端知道为什么没落版的答案，不是失败）与
 * `refused`（未收录的码）；`landed` 只在追加了处置版时为真，页面据此重读列表。
 */
export interface DispositionNote {
  tone: 'answered' | 'refused';
  landed: boolean;
  headline: string;
  details: string[];
}

export function describeDispositionAnswer(body: IdentityDispositionResponseBody): DispositionNote {
  const headline = `处置口答复：${body.outcome} —— ${labelOf(dispositionOutcomeLabels, body.outcome)}`;
  const target = `${body.unit} 的 ${body.disposedVersion}`;
  const recorded = body.disposition ? [`在册的处置：${dispositionSummaryOf(body.disposition)}`] : [];
  switch (body.outcome) {
    case 'IDENTITY_CONFLICT_DISPOSED':
      return {
        tone: 'answered',
        landed: true,
        headline,
        details: [
          ...(body.disposition ? [`这次处置：${dispositionSummaryOf(body.disposition)}`] : []),
          ...(body.versions ?? []).map(
            (version) =>
              `${version.unit} 追加 ${version.judgmentVersion}（接续 ${version.predecessor ?? '—'}，${labelOf(receptionKindLabels, version.kind)}${
                version.association ? `，关联 ${version.association}` : ''
              }${(version.excluded ?? []).length > 0 ? `，排除 ${(version.excluded ?? []).join('、')}` : ''}）`,
          ),
          '原判断与冲突版照留；业务时间沿用原接收时刻，控制起点不另起',
          body.intentVersion
            ? `收寄意图随 ${body.intentVersion} 交 parcel-shipment；早先交出的不撤回`
            : '没有新交收寄意图（都不是、或这件实物此前已就那件包裹交出过）',
        ],
      };
    case 'EXISTING_RESULT':
      return { tone: 'answered', landed: false, headline, details: [`${target} 已按同一处置接续`, ...recorded] };
    case 'ALREADY_DISPOSED':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [`${target} 已被另一份处置接续，差异请对照在册那一份`, ...recorded],
      };
    case 'VERSION_SUPERSEDED':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [
          `${target} 已不是当前有效版本；此刻各链：${(body.currentVersions ?? []).map((version) => version.judgmentVersion).join('、') || '—'}`,
          '刷新列表后按此刻的版本重提',
        ],
      };
    case 'NO_IDENTITY_CONFLICT':
      return { tone: 'answered', landed: false, headline, details: [`${body.unit} 各链当前有效版本合起来不判身份冲突`] };
    case 'NOT_A_CANDIDATE':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: [`处置候选：${(body.candidates ?? []).join('、') || '—'}；候选之外的包裹不能指定`],
      };
    case 'ASSOCIATED_ELSEWHERE':
      return {
        tone: 'answered',
        landed: false,
        headline,
        details: (body.associatedUnits ?? []).map(
          (unit) => `${unit.unit} 此刻关联着它，当前有效版本 ${unit.currentVersions.map((version) => version.judgmentVersion).join('、') || '—'}`,
        ),
      };
    default:
      // 未收录的 outcome 原样示出：服务端新增一格时，页面宁可显示英文原名，也不把它归进某个既有说法。
      return { tone: 'refused', landed: false, headline, details: [] };
  }
}

/** problem+json 错误码的中文说明。 */
export function dispositionProblemNote(code: string): string {
  switch (code) {
    case 'JUDGMENT_NOT_VISIBLE':
      return '本租户的收寄册上没有这件实物的这一版判断；刷新列表再指名。';
    case 'OUTSIDE_ADMISSION_SCOPE':
      return '这笔生产写不在治理登记的生产权威区间内，要阶段治理去登记区间；重试不会好。';
    case 'MALFORMED_REQUEST':
      return '载荷不是服务端接受的形状（指定一件要带包裹、都不是不带包裹与连带实物、依据引用必填，且不得带身份键）；请检查前端与服务端版本是否一致。';
    case 'METHOD_NOT_ALLOWED':
      return '端点不接受当前 HTTP 方法；请检查前端与服务端版本是否一致。';
    case 'INTAKE_FAILED':
      return '接入面未能形成操作者身份；服务端未读取处置内容。';
    case 'IDENTITY_DEPENDENCY_UNAVAILABLE':
      return '接入身份依赖读不动，可稍后重试。';
    case 'NO_ANSWER_FORMED':
      return '处置未形成答案（落册或意图入队失败，或与同一包裹上的另一笔写撞上），册上未追加，可稍后重试。';
    default:
      return `服务端问题码：${code}`;
  }
}
