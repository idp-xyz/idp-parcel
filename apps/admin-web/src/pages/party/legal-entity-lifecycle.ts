// 法人详情栏生命周期条的判读（票 admin-web-group-legal-entities/15）。三段取 CONTEXT 的身份生命周期原词
// （已登记 → 已生效 → 已停用），纯函数，node:test 钉着；详情栏只负责摆。

import type { GroupLegalEntityRecord } from './api';
import type { IdentityStatusCode } from './presentation';

export type LifecycleStageState = 'done' | 'current' | 'skipped' | 'pending';

export interface LifecycleStage {
  code: IdentityStatusCode;
  state: LifecycleStageState;
}

const STAGES: readonly IdentityStatusCode[] = ['REGISTERED', 'EFFECTIVE', 'DEACTIVATED'];

/**
 * 三段各处什么态。状态码不在三格之内时答 null，详情栏不画这条——不把新答案硬塞进旧三段。
 *
 * 已停用时「已生效」那段按时点判：生效自早于停用时点才算走过，否则是跳过——登记后未到生效就停用的法人从没生效过，
 * 画成走过就是说假话。时点解析不了时按走过，与服务端状态码给的顺序一致。
 */
export function legalEntityLifecycle(row: GroupLegalEntityRecord): LifecycleStage[] | null {
  const current = STAGES.indexOf(row.status as IdentityStatusCode);
  if (current === -1) return null;
  return STAGES.map((code, index) => {
    if (index === current) return { code, state: 'current' };
    if (index > current) return { code, state: 'pending' };
    if (code === 'EFFECTIVE' && row.status === 'DEACTIVATED' && !effectiveBeforeDeactivation(row)) {
      return { code, state: 'skipped' };
    }
    return { code, state: 'done' };
  });
}

/** 两段之间的连线画成「走过」：前一段走过、后一段走过或正在——连到被跳过的那段不算走过。 */
export function lifecycleConnectorReached(from: LifecycleStage, to: LifecycleStage): boolean {
  return from.state === 'done' && (to.state === 'done' || to.state === 'current');
}

function effectiveBeforeDeactivation(row: GroupLegalEntityRecord): boolean {
  if (row.deactivatedAt === undefined) return true;
  const effective = Date.parse(row.effectiveFrom);
  const deactivated = Date.parse(row.deactivatedAt);
  if (Number.isNaN(effective) || Number.isNaN(deactivated)) return true;
  return effective < deactivated;
}
