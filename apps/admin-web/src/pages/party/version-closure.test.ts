import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { KnownVersion } from './known-versions';
import {
  closableKeys,
  closureAnswerOf,
  closureCauseText,
  closureConfirmLines,
  closureIncomplete,
  closureKeyOf,
  closureLocalProblems,
  closureModeOf,
  closureOldOf,
  closureOldProblem,
  closurePayloadOf,
  closureSuccessors,
  initialClosureDraft,
  type ClosureTarget,
} from './version-closure';

const target: ClosureTarget = {
  kind: 'CUSTOMER_CONTRACT',
  kindLabel: '客户合同',
  objectId: 'CC-1',
  version: 'v1',
  scope: 'SCOPE-A',
};

function version(over: Partial<KnownVersion> & Pick<KnownVersion, 'version' | 'effectiveStartsAt'>): KnownVersion {
  return { objectId: 'CC-1', scope: 'SCOPE-A', status: 'EFFECTIVE', ...over };
}

const v1 = version({ version: 'v1', effectiveStartsAt: '2026-01-01T00:00:00Z' });
const v2 = version({ version: 'v2', effectiveStartsAt: '2026-06-01T00:00:00Z' });
const v3 = version({ version: 'v3', effectiveStartsAt: '2026-06-01T00:00:00Z' });
const v1Same = version({ version: 'v1b', effectiveStartsAt: '2026-01-01T00:00:00Z' });

test('旧版按对象、范围与版号在重读的册面里认出来；不在册面上就是 null', () => {
  equal(closureOldOf([v2, v1], target), v1);
  equal(closureOldOf([v2], target), null);
  equal(closureOldOf([{ ...v1, scope: 'SCOPE-B' }], target), null);
});

test('后继：同对象同范围、在用、起点不早于旧版；晚的走截止、同起点的走退役', () => {
  const other = { ...version({ version: 'v9', effectiveStartsAt: '2026-06-01T00:00:00Z' }), objectId: 'CC-2' };
  const otherScope = { ...version({ version: 'v8', effectiveStartsAt: '2026-06-01T00:00:00Z' }), scope: 'SCOPE-B' };
  const retired = version({ version: 'v7', effectiveStartsAt: '2026-06-01T00:00:00Z', status: 'RETIRED' });
  const earlier = version({ version: 'v0', effectiveStartsAt: '2025-01-01T00:00:00Z' });
  const found = closureSuccessors(v1, [v1, v2, v1Same, other, otherScope, retired, earlier]);
  deepEqual(found.map((row) => row.version), ['v1b', 'v2']);
  equal(closureModeOf(v1, v2), 'cut');
  equal(closureModeOf(v1, v1Same), 'retirement');
});

test('后继同起点时新版号在前', () => {
  deepEqual(closureSuccessors(v1, [v2, v3]).map((row) => row.version), ['v3', 'v2']);
});

test('起点不早于旧版终点的版本与旧版本来不重叠，不是后继', () => {
  const closed = { ...v1, effectiveEndsAt: '2026-06-01T00:00:00Z' };
  deepEqual(closureSuccessors(closed, [closed, v2]), []);
  deepEqual(closureSuccessors({ ...v1, effectiveEndsAt: '2026-06-01T00:00:01Z' }, [v2]).map((row) => row.version), ['v2']);
});

test('旧版不在册面或不在用，就没有可收尾的', () => {
  equal(closureOldProblem(null)?.includes('不在册面'), true);
  equal(closureOldProblem({ ...v1, status: 'RETIRED' })?.includes('不是「在用」'), true);
  equal(closureOldProblem(v1), null);
});

test('草稿：后继只有一个时选上，多于一个时留空让人选；更正时刻按显示时区取打开时的此刻', () => {
  const now = new Date('2026-10-26T00:30:00Z');
  const single = initialClosureDraft([v2], now, 'Asia/Shanghai');
  equal(single.successor, 'v2');
  equal(single.correctedAt, '2026-10-26T08:30:00');
  equal(initialClosureDraft([v2, v3], now, 'Asia/Shanghai').successor, '');
  equal(initialClosureDraft([], now, 'Asia/Shanghai').successor, '');
});

test('本地问题只报填错的：更正时刻编不成时刻才报，空着的必填格由 closureIncomplete 管', () => {
  const tz = 'Asia/Shanghai';
  deepEqual(closureLocalProblems({ successor: '', reference: '', correctedAt: '' }, 'cut', tz), {});
  deepEqual(
    Object.keys(closureLocalProblems({ successor: 'v2', reference: 'R-1', correctedAt: 'abc' }, 'cut', tz)),
    ['correctedAt'],
  );
  deepEqual(closureLocalProblems({ successor: 'v2', reference: 'R-1', correctedAt: 'abc' }, 'retirement', tz), {});
  deepEqual(closureLocalProblems({ successor: 'v2', reference: 'R-1', correctedAt: '2026-10-26T08:30:00' }, 'cut', tz), {});
});

test('没填全：后继、依据必填，截止另要更正时刻，退役不要', () => {
  const full = { successor: 'v2', reference: 'R-1', correctedAt: '2026-10-26T08:30:00' };
  equal(closureIncomplete(full, 'cut'), false);
  equal(closureIncomplete({ ...full, successor: '' }, 'cut'), true);
  equal(closureIncomplete({ ...full, reference: '  ' }, 'retirement'), true);
  equal(closureIncomplete({ ...full, correctedAt: '' }, 'cut'), true);
  equal(closureIncomplete({ ...full, correctedAt: '' }, 'retirement'), false);
  equal(closureIncomplete(full, null), false);
});

test('载荷：截止一节恰一项带更正时刻（换成 RFC 3339），退役一节恰一项不带时刻；不带租户', () => {
  const draft = { successor: 'v2', reference: ' R-1 ', correctedAt: '2026-10-26T08:30:00' };
  const cut = closurePayloadOf(target, 'cut', draft, 'Asia/Shanghai');
  deepEqual(cut, {
    cuts: [
      {
        kind: 'CUSTOMER_CONTRACT',
        scope: 'SCOPE-A',
        objectId: 'CC-1',
        version: 'v1',
        successorVersion: 'v2',
        reference: ' R-1 ',
        correctedAt: '2026-10-26T00:30:00Z',
      },
    ],
  });
  const retirement = closurePayloadOf(target, 'retirement', draft, 'Asia/Shanghai');
  deepEqual(Object.keys(retirement), ['retirements']);
  equal('correctedAt' in (retirement as { retirements: object[] }).retirements[0], false);
  equal('tenantId' in cut, false);
});

test('确认行：截止点取后继生效时点；退役说明为什么不是截止；供应商协议另提醒不查在途委托', () => {
  const draft = { successor: 'v2', reference: 'R-1', correctedAt: '2026-10-26T08:30:00' };
  const cutLines = closureConfirmLines(target, v1, v2, 'cut', draft, 'Asia/Shanghai');
  equal(cutLines.some((line) => line.startsWith('截止点：')), true);
  equal(cutLines.some((line) => line.includes('没有撤回截止的入口')), true);
  equal(cutLines.some((line) => line.includes('在途的运输委托')), false);
  const retireLines = closureConfirmLines(target, v1, v1Same, 'retirement', draft, 'Asia/Shanghai');
  equal(retireLines.some((line) => line.includes('同一起点')), true);
  equal(retireLines.some((line) => line.includes('退役时刻取提交这一刻')), true);
  const agreementLines = closureConfirmLines({ ...target, kind: 'SUPPLIER_AGREEMENT' }, v1, v2, 'cut', draft, 'Asia/Shanghai');
  equal(agreementLines.some((line) => line.includes('在途的运输委托')), true);
});

test('未受理的原因：认得的领域错误给中文并保留原话，认不得的不折成「填写的内容不合要求」', () => {
  const known = closureCauseText(
    "party commercial: the successor's start is not strictly inside the predecessor's selection interval: successor v2 starts at 2026-06-01T00:00:00Z",
  );
  equal(known.text.includes('严格落在旧版现在的选用区间之内'), true);
  equal(known.original?.startsWith('party commercial:'), true);
  const unknown = closureCauseText('party commercial: something new');
  equal(unknown.text, '这一次收尾不成立，没有写入任何内容。');
  equal(unknown.original, 'party commercial: something new');
});

test('答复：已截止、已退役、重放、未受理、没见过的答案各有说法', () => {
  const cut = closureAnswerOf('cut', target, {
    outcome: 'REGISTERED',
    correctedStartsAt: '2026-01-01T00:00:00Z',
    correctedEndsAt: '2026-06-01T00:00:00Z',
  });
  equal(cut.done, true);
  equal(cut.text.startsWith('已截止：v1 此后的选用区间是'), true);
  const retired = closureAnswerOf('retirement', target, {
    outcome: 'REGISTERED',
    successorVersion: 'v1b',
    retiredAt: '2026-10-26T00:30:00Z',
  });
  equal(retired.text.includes('让位给 v1b'), true);
  const replay = closureAnswerOf('cut', target, { outcome: 'ALREADY_REGISTERED' });
  equal(replay.done, true);
  equal(replay.text.includes('已经登记过'), true);
  const refused = closureAnswerOf('cut', target, {
    outcome: 'NOT_ACCEPTED',
    cause: 'party commercial: the predecessor and the successor must both be effective',
  });
  equal(refused.done, false);
  equal(refused.cause?.text.includes('都得是「在用」'), true);
  const strange = closureAnswerOf('cut', target, { outcome: 'SOMETHING' });
  equal(strange.done, false);
  equal(strange.text.includes('SOMETHING'), true);
});

test('可收尾的行：在用且这一页里有后继可让位的版本才有按钮', () => {
  const keys = closableKeys([v1, v2, { ...v1, objectId: 'CC-2' }]);
  equal(keys.has(closureKeyOf(v1)), true);
  equal(keys.has(closureKeyOf(v2)), false);
  equal(keys.has(closureKeyOf({ ...v1, objectId: 'CC-2' })), false);
});
