// 指派拣货人时可选的人（纯 TypeScript，node:test 直接测）。
//
// 拣货人是操作者册上的人，标识是「发行方#sub」——与后端各册上记操作者的是同一个串。名册从操作者读口来；
// 在这个仓有没有此刻有效的「作业事实登记」授予只是提示：没有授予的人派了也拣不了，但授予可以后补，所以不拦着选。
// 名册只读第一页；人多到一页列不完时，没列出的人用手填。读不到名册（没有查阅操作者的授予）时也是手填。

export interface AssignableOperator {
  issuer: string;
  subject: string;
  grants: ReadonlyArray<{
    capabilityFace: string;
    workScope?: { kind: string; reference: string };
    effectiveStartsAt: string;
    effectiveEndsAt?: string;
    revocation?: unknown;
  }>;
}

export interface AssigneeOption {
  /** 操作者标识「发行方#sub」。 */
  value: string;
  label: string;
  /** 此刻在这个仓有有效的作业事实登记授予。 */
  granted: boolean;
}

const workFactFace = 'WORK_FACT_REGISTRATION';

function grantedAt(operator: AssignableOperator, warehouse: string, now: Date): boolean {
  return operator.grants.some((grant) => {
    if (grant.capabilityFace !== workFactFace || grant.revocation) return false;
    if (grant.workScope?.kind !== 'WAREHOUSE' || grant.workScope.reference !== warehouse) return false;
    if (new Date(grant.effectiveStartsAt).getTime() > now.getTime()) return false;
    return grant.effectiveEndsAt === undefined || new Date(grant.effectiveEndsAt).getTime() > now.getTime();
  });
}

/** 可选的拣货人：在这个仓有作业授予的在前，各自按 sub 排。 */
export function assigneeOptions(operators: readonly AssignableOperator[], warehouse: string, now: Date = new Date()): AssigneeOption[] {
  return operators
    .map((operator) => {
      const granted = grantedAt(operator, warehouse, now);
      return {
        value: `${operator.issuer}#${operator.subject}`,
        label: granted ? operator.subject : `${operator.subject}（在这个仓没有作业授予，派了也拣不了）`,
        granted,
      };
    })
    .sort((left, right) => (left.granted === right.granted ? left.value.localeCompare(right.value) : left.granted ? -1 : 1));
}

/** 报文：选了人带上他，选「不派」不带 assignee——不带才是取消指派，空串后端不收。 */
export function waveAssignmentPayload(waveId: string, assignee: string): { waveId: string; assignee?: string } {
  const chosen = assignee.trim();
  return chosen === '' ? { waveId } : { waveId, assignee: chosen };
}
