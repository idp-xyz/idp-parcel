import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { assigneeOptions, loadOperatorRoster, waveAssignmentPayload, waveZones, type AssignableOperator } from './wave-assign';

const now = new Date('2026-11-27T02:00:00Z');
const issuer = 'https://issuer.example.invalid';

const grant = (more: Partial<AssignableOperator['grants'][number]> = {}): AssignableOperator['grants'][number] => ({
  capabilityFace: 'WORK_FACT_REGISTRATION',
  workScope: { kind: 'WAREHOUSE', reference: 'WH-1' },
  effectiveStartsAt: '2026-01-01T00:00:00Z',
  ...more,
});

const operator = (subject: string, grants: AssignableOperator['grants']): AssignableOperator => ({ issuer, subject, grants });

// Covers: 可选的拣货人是操作者册上的人，标识是「发行方#sub」；此刻在这个仓有有效的作业事实登记授予的排在前面，
// 没有的照列但写明派了也拣不了——别的仓的授予、别的能力面、已撤销、还没生效、已到期的都不算。
test('可选的拣货人：在这个仓有作业授予的在前，没有的写明', () => {
  const options = assigneeOptions(
    [
      operator('zoe', [grant()]),
      operator('other-warehouse', [grant({ workScope: { kind: 'WAREHOUSE', reference: 'WH-2' } })]),
      operator('decision-only', [grant({ capabilityFace: 'OPERATION_DECISION', workScope: undefined })]),
      operator('revoked', [grant({ revocation: { revokedAt: '2026-06-01T00:00:00Z' } })]),
      operator('not-yet', [grant({ effectiveStartsAt: '2027-01-01T00:00:00Z' })]),
      operator('expired', [grant({ effectiveEndsAt: '2026-06-01T00:00:00Z' })]),
      operator('amy', [grant({ effectiveEndsAt: '2027-01-01T00:00:00Z' })]),
    ],
    'WH-1',
    now,
  );
  deepEqual(
    options.filter((option) => option.granted).map((option) => [option.value, option.label]),
    [
      [`${issuer}#amy`, 'amy'],
      [`${issuer}#zoe`, 'zoe'],
    ],
  );
  deepEqual(
    options.filter((option) => !option.granted).map((option) => option.value),
    ['decision-only', 'expired', 'not-yet', 'other-warehouse', 'revoked'].map((subject) => `${issuer}#${subject}`),
  );
  equal(options.find((option) => option.value.endsWith('#revoked'))?.label, 'revoked（在这个仓没有作业授予，派了也拣不了）');
  // 登记时写的依据摆在 sub 后面帮人认，太长就截
  const named = assigneeOptions([{ ...operator('sub-1', [grant()]), basis: '早班拣货员 小王' }, { ...operator('sub-2', [grant()]), basis: '一'.repeat(30) }], 'WH-1', now);
  equal(named[0].label, 'sub-1 · 早班拣货员 小王');
  equal(named[1].label, `sub-2 · ${'一'.repeat(24)}…`);
  // 登了姓名的摆姓名，sub 跟在后面；依据不再摆
  const withName = assigneeOptions([{ ...operator('sub-1', [grant()]), name: '合成甲', basis: '早班拣货员' }, { ...operator('sub-9', []), name: '合成乙' }], 'WH-1', now);
  equal(withName[0].label, '合成甲 · sub-1');
  equal(withName[1].label, '合成乙 · sub-9（在这个仓没有作业授予，派了也拣不了）');
});

// Covers: 指派的报文：选了人带上他，不派不带 assignee（不带才是取消指派）。
test('指派的报文', () => {
  deepEqual(waveAssignmentPayload('W-1', `${issuer}#amy`), { waveId: 'W-1', assignee: `${issuer}#amy` });
  deepEqual(waveAssignmentPayload('W-1', ''), { waveId: 'W-1' });
  deepEqual(waveAssignmentPayload('W-1', '   '), { waveId: 'W-1' });
  // 按库区派带上库区；取消这个库区的指派是带库区、不带人
  deepEqual(waveAssignmentPayload('W-1', `${issuer}#amy`, 'ZONE-A'), { waveId: 'W-1', zoneCode: 'ZONE-A', assignee: `${issuer}#amy` });
  deepEqual(waveAssignmentPayload('W-1', '', 'ZONE-A'), { waveId: 'W-1', zoneCode: 'ZONE-A' });
});

// Covers: 名册一页一页读完；翻到页数上限还有下一页、或读到一半没成，前面读到的照用并标出没列全；头一页就没读成算没读出。
test('操作者名册读完所有页', async () => {
  const pages: Record<string, { operators: AssignableOperator[]; next: string | null }> = {
    first: { operators: [operator('a', [])], next: 'second' },
    second: { operators: [operator('b', [])], next: 'third' },
    third: { operators: [operator('c', [])], next: null },
  };
  const asked: Array<string | null> = [];
  const list = async (after: string | null) => {
    asked.push(after);
    const page = pages[after ?? 'first'];
    return { kind: 'outcome' as const, status: 200, body: { operators: page.operators, page: { next: page.next } } };
  };
  const whole = await loadOperatorRoster(list);
  ok(whole.kind === 'roster' && !whole.more);
  if (whole.kind === 'roster') deepEqual(whole.operators.map((entry) => entry.subject), ['a', 'b', 'c']);
  deepEqual(asked, [null, 'second', 'third']);

  const capped = await loadOperatorRoster(list, 2);
  ok(capped.kind === 'roster' && capped.more && capped.operators.length === 2);

  let calls = 0;
  const flaky = async (after: string | null) => {
    calls += 1;
    return calls === 1 ? list(after) : ({ kind: 'transport', message: '断了' } as const);
  };
  const partial = await loadOperatorRoster(flaky);
  ok(partial.kind === 'roster' && partial.more && partial.operators.length === 1);

  const failed = await loadOperatorRoster(async () => ({ kind: 'notGranted' }) as const);
  equal(failed.kind, 'failed');
});

// Covers: 波次的任务分在哪几个库区：按库区码排，各自数出还等着拣的与总数；读口没带库区的任务不算进任何库区。
test('波次的任务分在哪几个库区', () => {
  deepEqual(
    waveZones([
      { zoneCode: 'ZONE-B', status: 'OPEN' },
      { zoneCode: 'ZONE-A', status: 'DONE' },
      { zoneCode: 'ZONE-B', status: 'SHORT' },
      { status: 'OPEN' },
      { zoneCode: 'ZONE-A', status: 'OPEN' },
    ]),
    [
      { zoneCode: 'ZONE-A', open: 1, tasks: 2 },
      { zoneCode: 'ZONE-B', open: 1, tasks: 2 },
    ],
  );
  deepEqual(waveZones([{ status: 'OPEN' }]), []);
});
