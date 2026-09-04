import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ModuleInfo } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import type {
  ChannelSelectionDecisionRecord,
  ChannelSelectionDecisionResponseBody,
  TiedChannelSelectionDecisionsResponseBody,
} from './api';
import {
  candidateRowsOf,
  decisionDetailState,
  decisionRowsOf,
  tiedListViewState,
} from './channel-selection-decisions';

// 渠道择优决定页的判读（票 label-channel/23）。最容易错的三格：把「此刻没有并列冲突」与「接入渠道未配置」
// 演成同一个空态；把按标识查不到（终局答案）当成故障重试；把四种出局因由折成一句「不可计价」。

const info: ModuleInfo = {
  title: '渠道择优决定',
  owner: 'parcel-shipment',
  source: 'CONTEXT.md',
};
const options = { module: info, endpoint: 'GET /channel-selection-decisions?view=tied' };
const noRetry = () => {};

const tied: ChannelSelectionDecisionRecord = {
  decisionId: 'CSDN-1',
  scope: 'scope-a',
  mapping: 'mapping-1',
  assembledAsOf: '2026-09-04T18:29:00Z',
  rule: 'COST_ONLY',
  decidedAt: '2026-09-04T18:30:00Z',
  conclusion: 'TIED',
  candidates: [
    { candidate: 'cand-a', evaluation: 'eval-a', outcome: 'TIED' },
    { candidate: 'cand-b', outcome: 'TIED' },
    { candidate: 'cand-c', outcome: 'EXCLUDED', exclusion: 'PENDING_EVIDENCE' },
    { candidate: 'cand-d', outcome: 'EXCLUDED', exclusion: 'RATECARD_EXCLUSION' },
  ],
};

const resolved: ChannelSelectionDecisionRecord = {
  ...tied,
  decisionId: 'CSDN-2',
  decidedAt: '2026-09-04T19:30:00Z',
  conclusion: 'SELECTED',
  selectedCandidate: 'cand-a',
  candidates: [
    { candidate: 'cand-a', evaluation: 'eval-a2', outcome: 'SELECTED' },
    { candidate: 'cand-b', evaluation: 'eval-b2', outcome: 'NOT_SELECTED' },
  ],
};

function outcome<Body>(body: Body): ApiResult<Body> {
  return { kind: 'outcome', status: 200, body };
}

test('并列冲突行：结论译回原词，并列的那几家单独列出，出局与落选的不混进去', () => {
  const body: TiedChannelSelectionDecisionsResponseBody = {
    outcome: 'TIED_CHANNEL_SELECTION_DECISIONS_LISTED',
    decisions: [tied],
  };
  const rows = decisionRowsOf(body);
  equal(rows.length, 1);
  equal(rows[0].key, 'decision:CSDN-1');
  equal(rows[0].decisionId, 'CSDN-1');
  equal(rows[0].values.conclusion, '并列冲突——交人工裁决');
  equal(rows[0].values.rule, '成本单维');
  equal(rows[0].values.tiedCandidates, 'cand-a、cand-b');
  equal(rows[0].values.candidateCount, '4');
  equal(rows[0].values.decidedAt, '2026-09-04 18:30:00 UTC');
  equal(rows[0].values.assembledAsOf, '2026-09-04 18:29:00 UTC');
});

test('逐候选明细：评价引用缺席显 —，出局因由四格各自成句、非出局行留空', () => {
  const rows = candidateRowsOf(tied);
  deepEqual(
    rows.map((row) => row.values),
    [
      { candidate: 'cand-a', outcome: '并列', evaluation: 'eval-a', exclusion: '' },
      { candidate: 'cand-b', outcome: '并列', evaluation: '—', exclusion: '' },
      {
        candidate: 'cand-c',
        outcome: '出局',
        evaluation: '—',
        exclusion: '待判断——补齐事实即可计价',
      },
      {
        candidate: 'cand-d',
        outcome: '出局',
        evaluation: '—',
        exclusion: '不可计价——价卡明确排除',
      },
    ],
  );
  equal(rows[0].key, 'candidate:CSDN-1#cand-a');

  const resolvedRows = candidateRowsOf(resolved);
  equal(resolvedRows[0].values.outcome, '选中');
  equal(resolvedRows[1].values.outcome, '落选');
});

test('词表没收录的词原样示出，不冒充既有一格', () => {
  const rows = candidateRowsOf({
    ...tied,
    candidates: [{ candidate: 'cand-z', outcome: 'WITHDRAWN', exclusion: 'SOMETHING_NEW' }],
  });
  equal(rows[0].values.outcome, 'WITHDRAWN');
  equal(rows[0].values.exclusion, 'SOMETHING_NEW');
});

test('空册是答案：不收窄与收窄各一句，与未配置分开', () => {
  const empty = outcome<TiedChannelSelectionDecisionsResponseBody>({
    outcome: 'TIED_CHANNEL_SELECTION_DECISIONS_LISTED',
    decisions: [],
  });
  const whole = tiedListViewState(empty, undefined, 0, noRetry, options);
  equal(whole.kind, 'empty');
  ok(whole.kind === 'empty' && whole.title === '当前租户此刻没有并列冲突');

  const narrowed = tiedListViewState(empty, { scope: 'scope-a', mapping: 'mapping-1' }, 0, noRetry, options);
  equal(narrowed.kind, 'empty');
  ok(narrowed.kind === 'empty' && narrowed.title === '对象 scope-a / mapping-1 此刻没有并列冲突');

  const unconfigured = tiedListViewState({ kind: 'unconfigured' }, undefined, 0, noRetry, options);
  equal(unconfigured.kind, 'unconfigured');

  const ready = tiedListViewState(
    outcome<TiedChannelSelectionDecisionsResponseBody>({
      outcome: 'TIED_CHANNEL_SELECTION_DECISIONS_LISTED',
      decisions: [tied],
    }),
    undefined,
    1,
    noRetry,
    options,
  );
  equal(ready.kind, 'ready');
  equal(tiedListViewState(null, undefined, 0, noRetry, options).kind, 'loading');
});

test('单份查阅：查不到是终局答案自成一格，未配置与故障各归各格', () => {
  const found = decisionDetailState(
    outcome<ChannelSelectionDecisionResponseBody>({ outcome: 'CHANNEL_SELECTION_DECISION', decision: resolved }),
    'CSDN-2',
  );
  ok(found.kind === 'decision' && found.decision.selectedCandidate === 'cand-a');

  const notVisible = decisionDetailState(
    { kind: 'callerProblem', status: 404, code: 'CHANNEL_SELECTION_DECISION_NOT_VISIBLE' },
    'CSDN-404',
  );
  deepEqual(notVisible, { kind: 'notVisible', decisionId: 'CSDN-404' });

  const malformed = decisionDetailState({ kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' }, 'x');
  ok(malformed.kind === 'error' && !malformed.canRetry);

  const noAnswer = decisionDetailState({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }, 'CSDN-1');
  ok(noAnswer.kind === 'error' && noAnswer.canRetry);

  equal(decisionDetailState({ kind: 'unconfigured' }, 'CSDN-1').kind, 'unconfigured');
  equal(decisionDetailState(null, 'CSDN-1').kind, 'loading');
});
