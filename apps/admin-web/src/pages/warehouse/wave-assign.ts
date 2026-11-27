// 指派拣货人时可选的人（纯 TypeScript，node:test 直接测）。
//
// 拣货人是操作者册上的人，标识是「发行方#sub」——与后端各册上记操作者的是同一个串。名册从操作者读口来；
// 在这个仓有没有此刻有效的「作业事实登记」授予只是提示：没有授予的人派了也拣不了，但授予可以后补，所以不拦着选。
// 名册一页一页读完，设页数上限；翻到上限还没读完时如实说，没列出的人用手填。读不到名册（没有查阅操作者的授予）时也是手填。

import type { ApiResult } from '../catalogue-api';

/** 名册最多翻几页。读口一页的行数由后端定；翻到这里还有下一页就不再翻，界面上说明没列全。 */
export const MAX_ROSTER_PAGES = 20;

type RosterPage = { operators: AssignableOperator[]; page: { next: string | null } };

export type RosterAnswer =
  | { kind: 'roster'; operators: AssignableOperator[]; more: boolean }
  | { kind: 'failed'; failure: Exclude<ApiResult<RosterPage>, { kind: 'outcome' }> };

/** 把操作者名册一页一页读完。头一页就没读成算没读出名册；读到一半没成，前面读到的照用，并说明没列全。 */
export async function loadOperatorRoster(
  list: (after: string | null) => Promise<ApiResult<RosterPage>>,
  maxPages = MAX_ROSTER_PAGES,
): Promise<RosterAnswer> {
  const operators: AssignableOperator[] = [];
  let after: string | null = null;
  for (let page = 0; page < maxPages; page += 1) {
    const answer = await list(after);
    if (answer.kind !== 'outcome') return page === 0 ? { kind: 'failed', failure: answer } : { kind: 'roster', operators, more: true };
    operators.push(...answer.body.operators);
    after = answer.body.page.next;
    if (after === null) return { kind: 'roster', operators, more: false };
  }
  return { kind: 'roster', operators, more: true };
}

export interface AssignableOperator {
  issuer: string;
  subject: string;
  /** 登过的姓名；没登就没有。 */
  name?: string;
  /** 登记这位操作者时写的依据。 */
  basis?: string;
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
      const basis = (operator.basis ?? '').trim();
      const personName = (operator.name ?? '').trim();
      // 登了姓名的摆姓名，sub 跟在后面分开同名的两个人。没登的摆 sub：它往往是发行方给的一串不好认的号，
      // 登记这位操作者时写的依据多半说了他是谁，摆在后面帮人认。
      const name =
        personName !== ''
          ? `${personName} · ${operator.subject}`
          : basis === ''
            ? operator.subject
            : `${operator.subject} · ${basis.length > 24 ? `${basis.slice(0, 24)}…` : basis}`;
      return {
        value: `${operator.issuer}#${operator.subject}`,
        label: granted ? name : `${name}（在这个仓没有作业授予，派了也拣不了）`,
        granted,
      };
    })
    .sort((left, right) => (left.granted === right.granted ? left.value.localeCompare(right.value) : left.granted ? -1 : 1));
}

/**
 * 报文：选了人带上他，选「不派」不带 assignee——不带才是取消指派，空串后端不收。
 * 按库区派时带上库区：只派这个波次里这一个库区的格；不带库区是派整个波次。
 */
export function waveAssignmentPayload(waveId: string, assignee: string, zone = ''): { waveId: string; zoneCode?: string; assignee?: string } {
  const chosen = assignee.trim();
  const scoped = zone.trim() === '' ? { waveId } : { waveId, zoneCode: zone.trim() };
  return chosen === '' ? scoped : { ...scoped, assignee: chosen };
}

export interface WaveZone {
  zoneCode: string;
  /** 这个库区里还等着拣的任务数。 */
  open: number;
  /** 这个库区里的任务总数。 */
  tasks: number;
}

/**
 * 这个波次的任务分在哪几个库区，按库区码排。读口没带库区的任务（库位在册上认不出）不算进任何一个库区——
 * 它们只能照整个波次的指派走。
 */
export function waveZones(tasks: ReadonlyArray<{ zoneCode?: string; status: string }>): WaveZone[] {
  const zones = new Map<string, WaveZone>();
  for (const task of tasks) {
    if (!task.zoneCode) continue;
    const zone = zones.get(task.zoneCode) ?? { zoneCode: task.zoneCode, open: 0, tasks: 0 };
    zone.tasks += 1;
    if (task.status === 'OPEN') zone.open += 1;
    zones.set(task.zoneCode, zone);
  }
  return [...zones.values()].sort((left, right) => left.zoneCode.localeCompare(right.zoneCode));
}
