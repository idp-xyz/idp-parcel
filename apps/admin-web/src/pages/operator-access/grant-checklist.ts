import type { OperatorGrantPayload, OperatorGrantRecord } from './api';
import { grantStandingAt } from './presentation';

// 按清单授予：一次提交多笔，每一笔仍是现有的授予登记口。不造角色，也不代填标识——前缀、依据、起点由人给，
// 每一笔的标识是前缀加上这一格的原词，提交前能逐笔看见。已经生效或未到的同一格跳过，已撤销的可以再授。

/** 不带细分就不能授的两格：运营决定要种类，作业事实要范围。 */
const qualifiedFaces = new Set(['OPERATION_DECISION', 'WORK_FACT_REGISTRATION']);

export interface GrantChecklistSelection {
  faces: readonly string[];
  decisionKinds: readonly string[];
  workScopes: readonly { kind: string; reference: string }[];
}

export interface GrantChecklistDraft {
  prefix: string;
  basis: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
  issuer: string;
  subject: string;
}

export interface GrantChecklistPlan {
  payloads: OperatorGrantPayload[];
  skipped: number;
  /** 人还没给齐，或生成的标识与已有的相撞。有这句话就不提交。 */
  problem: string | null;
}

function lineKey(line: {
  capabilityFace: string;
  decisionKind?: string;
  workScope?: { kind: string; reference: string };
}): string {
  if (line.decisionKind) return `D:${line.decisionKind}`;
  if (line.workScope) return `W:${line.workScope.kind}:${line.workScope.reference}`;
  return `F:${line.capabilityFace}`;
}

function suffixOf(line: {
  capabilityFace: string;
  decisionKind?: string;
  workScope?: { kind: string; reference: string };
}): string {
  if (line.decisionKind) return line.decisionKind;
  if (line.workScope) return `${line.workScope.kind}-${line.workScope.reference}`;
  return line.capabilityFace;
}

/** 把勾选收成要登记的载荷。同一格只留一笔。 */
export function planGrantChecklist(
  selection: GrantChecklistSelection,
  draft: GrantChecklistDraft,
  already: readonly OperatorGrantRecord[],
  now: Date,
): GrantChecklistPlan {
  const prefix = draft.prefix.trim();
  const basis = draft.basis.trim();
  const starts = draft.effectiveStartsAt.trim();
  if (prefix === '' || basis === '' || starts === '') {
    return { payloads: [], skipped: 0, problem: '授予标识前缀、依据和生效起点都要填。' };
  }

  const lines: { capabilityFace: string; decisionKind?: string; workScope?: { kind: string; reference: string } }[] = [];
  for (const face of selection.faces) {
    if (face.trim() === '' || qualifiedFaces.has(face)) continue;
    lines.push({ capabilityFace: face });
  }
  for (const kind of selection.decisionKinds) {
    if (kind.trim() === '') continue;
    lines.push({ capabilityFace: 'OPERATION_DECISION', decisionKind: kind });
  }
  for (const scope of selection.workScopes) {
    const kind = scope.kind.trim();
    const reference = scope.reference.trim();
    if (kind === '' || reference === '') {
      return { payloads: [], skipped: 0, problem: '作业范围的种类和引用都要填。' };
    }
    lines.push({ capabilityFace: 'WORK_FACT_REGISTRATION', workScope: { kind, reference } });
  }
  if (lines.length === 0) {
    return { payloads: [], skipped: 0, problem: '还没有勾选要授的。' };
  }

  const covered = new Set<string>();
  const usedIds = new Set<string>();
  for (const grant of already) {
    usedIds.add(grant.grantId);
    const standing = grantStandingAt(grant, now);
    if (standing === 'effective' || standing === 'pending') covered.add(lineKey(grant));
  }

  const payloads: OperatorGrantPayload[] = [];
  const seen = new Set<string>();
  let skipped = 0;
  for (const line of lines) {
    const key = lineKey(line);
    if (seen.has(key)) continue;
    seen.add(key);
    if (covered.has(key)) {
      skipped += 1;
      continue;
    }
    const grantId = `${prefix}-${suffixOf(line)}`;
    if (usedIds.has(grantId)) {
      return { payloads: [], skipped, problem: `授予标识 ${grantId} 已经用过，换一个前缀。` };
    }
    usedIds.add(grantId);
    payloads.push({
      grantId,
      issuer: draft.issuer,
      subject: draft.subject,
      capabilityFace: line.capabilityFace,
      ...(line.decisionKind ? { decisionKind: line.decisionKind } : {}),
      ...(line.workScope ? { workScope: line.workScope } : {}),
      effectiveStartsAt: starts,
      ...(draft.effectiveEndsAt ? { effectiveEndsAt: draft.effectiveEndsAt } : {}),
      basis,
    });
  }
  return { payloads, skipped, problem: null };
}
