import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ApiResult, EnablementAnswerBody, EnablementRecord } from './api';
import {
  ENABLEMENT_SOURCE_LIMIT,
  actionFor,
  actionLabel,
  confirmMessage,
  enablementByCode,
  enablementLabel,
  enablementOutcomeNote,
  enablementScopeNote,
  nextRegistration,
  sourceProblem,
  standingCell,
  standingOf,
} from './enablement';

const china: EnablementRecord = {
  code: 'CN',
  standing: 'ENABLED',
  effectiveFrom: '2026-06-01T00:00:00Z',
  latestRevision: 2,
  history: [
    { revision: 1, status: 'DISABLED', effectiveFrom: '2026-05-01T00:00:00Z', registeredBy: 'idp#a', recordedAt: '2026-05-01T00:00:00Z' },
    { revision: 2, status: 'ENABLED', effectiveFrom: '2026-06-01T00:00:00Z', source: 'MEMO-2', registeredBy: 'idp#b', recordedAt: '2026-06-01T00:00:00Z' },
  ],
};
const singapore: EnablementRecord = {
  code: 'SG',
  standing: 'NOT_CONFIGURED',
  latestRevision: 1,
  history: [{ revision: 1, status: 'ENABLED', effectiveFrom: '2030-01-01T00:00:00Z', registeredBy: 'idp#a', recordedAt: '2026-06-01T00:00:00Z' }],
};
const byCode = enablementByCode([china, singapore]);

test('没登记过的码就是未配置，不是启用也不是停用', () => {
  equal(standingOf(byCode, 'CN'), 'ENABLED');
  equal(standingOf(byCode, 'US'), 'NOT_CONFIGURED');
  // 册上有行但全是未来生效：读回已经导出为未配置，页面照读回走。
  equal(standingOf(byCode, 'SG'), 'NOT_CONFIGURED');
});

test('状态词说人话，集外取值原样显示', () => {
  equal(enablementLabel('ENABLED'), '已启用');
  equal(enablementLabel('DISABLED'), '已停用');
  equal(enablementLabel('NOT_CONFIGURED'), '未配置');
  equal(enablementLabel('SOMETHING_NEW'), 'SOMETHING_NEW');
});

test('页头那句话说清未登记不受限制', () => {
  ok(enablementScopeNote.includes('未登记'));
  ok(enablementScopeNote.includes('不受限制'));
  for (const banned of ['修订', '服务端', '修订号']) ok(!enablementScopeNote.includes(banned));
});

test('已启用的只能停用，其余都是启用', () => {
  equal(actionFor('ENABLED'), 'DISABLE');
  equal(actionFor('DISABLED'), 'ENABLE');
  equal(actionFor('NOT_CONFIGURED'), 'ENABLE');
  equal(actionLabel('ENABLE'), '启用');
  equal(actionLabel('DISABLE'), '停用');
});

test('下一笔登记接册上最新的号；首笔是 1', () => {
  const now = new Date('2026-07-01T12:00:00Z');
  deepEqual(nextRegistration('COUNTRY', 'US', undefined, 'ENABLE', '', now), {
    kind: 'COUNTRY',
    code: 'US',
    revision: 1,
    status: 'ENABLED',
    effectiveFrom: '2026-07-01T12:00:00.000Z',
  });
  const next = nextRegistration('COUNTRY', 'CN', china, 'DISABLE', ' MEMO-3 ', now);
  equal(next.revision, 3);
  equal(next.status, 'DISABLED');
  equal(next.source, 'MEMO-3');
});

test('生效时点取此刻与册上最晚生效时点中较晚的：本机钟慢也不会早于上一笔', () => {
  const slowClock = new Date('2026-05-15T00:00:00Z');
  equal(nextRegistration('COUNTRY', 'CN', china, 'DISABLE', '', slowClock).effectiveFrom, '2026-06-01T00:00:00.000Z');
  // 册上有未来生效的行：后一笔要不早于它，否则登记册会拒。
  equal(nextRegistration('COUNTRY', 'SG', singapore, 'DISABLE', '', new Date('2026-07-01T00:00:00Z')).effectiveFrom, '2030-01-01T00:00:00.000Z');
});

test('来源单据只拦过长，空白不带', () => {
  equal(sourceProblem(''), null);
  equal(sourceProblem('x'.repeat(ENABLEMENT_SOURCE_LIMIT)), null);
  ok(sourceProblem('单'.repeat(ENABLEMENT_SOURCE_LIMIT + 1))?.includes('200'));
  ok(!('source' in nextRegistration('CURRENCY', 'USD', undefined, 'ENABLE', '   ', new Date())));
});

const outcome = (body: EnablementAnswerBody): ApiResult<EnablementAnswerBody> => ({ kind: 'outcome', status: 200, body });

test('登记答复说人话：登记上了或早已登过算落定，其余把原因摆出来', () => {
  const landed = enablementOutcomeNote(outcome({ outcome: 'REGISTERED' }), 'ENABLE');
  equal(landed.landed, true);
  equal(landed.text, '已启用。');
  const already = enablementOutcomeNote(outcome({ outcome: 'ALREADY_REGISTERED' }), 'DISABLE');
  equal(already.landed, true);
  ok(already.text.includes('早已登记过'));

  const conflict = enablementOutcomeNote(outcome({ outcome: 'CONTENT_CONFLICT' }), 'ENABLE');
  equal(conflict.landed, false);
  ok(conflict.text.includes('刷新'));

  const notInCatalogue = enablementOutcomeNote(outcome({ outcome: 'NOT_ACCEPTED', refusalReason: 'CODE_NOT_IN_CATALOGUE' }), 'ENABLE');
  equal(notInCatalogue.landed, false);
  ok(notInCatalogue.text.includes('目录里没有这个码'));
  const stale = enablementOutcomeNote(outcome({ outcome: 'NOT_ACCEPTED', refusalReason: 'REVISION_OUT_OF_SEQUENCE' }), 'ENABLE');
  ok(stale.text.includes('别人改过'));
});

test('接入拒绝与传输失败也说人话，不露修订号、服务端这类词', () => {
  const answers: ApiResult<EnablementAnswerBody>[] = [
    { kind: 'unconfigured' },
    { kind: 'loginRequired' },
    { kind: 'notGranted' },
    { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' },
    { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' },
    { kind: 'transport', message: 'boom' },
    outcome({ outcome: 'NOT_ACCEPTED', refusalReason: 'INPUT_NOT_WELL_FORMED' }),
    outcome({ outcome: 'NOT_ACCEPTED', refusalReason: 'EFFECTIVE_BEFORE_PRIOR' }),
    outcome({ outcome: 'NOT_ACCEPTED' }),
  ];
  for (const answer of answers) {
    const note = enablementOutcomeNote(answer, 'ENABLE');
    equal(note.landed, false);
    ok(note.text.length > 0);
    for (const banned of ['修订', '服务端', 'revision', 'MALFORMED']) ok(!note.text.includes(banned), `${answer.kind}: ${note.text}`);
  }
  // 服务端随 4xx 给的理由散文原样示出。
  equal(enablementOutcomeNote({ kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST', detail: '这一格不对' }, 'ENABLE').text, '这一格不对');
});

test('确认语说清这一步只登记、不拦别处', () => {
  ok(confirmMessage('ENABLE', '中国').includes('只是登记'));
  ok(confirmMessage('DISABLE', '中国').includes('不会阻止任何业务'));
});

test('状态格：没读到时不假装未配置', () => {
  deepEqual(standingCell(null, byCode, 'CN'), { kind: 'loading' });
  deepEqual(standingCell({ kind: 'notGranted' }, new Map(), 'CN'), { kind: 'unreadable' });
  deepEqual(standingCell({ kind: 'outcome', status: 200, body: { enablements: [china] } }, byCode, 'CN'), { kind: 'standing', standing: 'ENABLED' });
  deepEqual(standingCell({ kind: 'outcome', status: 200, body: { enablements: [china] } }, byCode, 'US'), { kind: 'standing', standing: 'NOT_CONFIGURED' });
});
