import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ModuleInfo } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import type {
  EffectiveTimeJudgmentResponseBody,
  ExternalTrackingFactListResponseBody,
  ExternalTrackingFactRecord,
} from './effective-time-judgment-api';
import {
  describeJudgmentAnswer,
  factListViewState,
  factRowsOf,
} from './effective-time-judgment';

// 有效时间判断页的判读（票 label-channel/21）。这一页最容易错的两格：一是把「该源此刻没有待判断的
// 事实」与「接入渠道未配置」演成同一个空态；二是把「已按同值判过」当成失败——它是答案，且带着既有
// 版本的依据，正是票 21 红线要摆出来的那一句。

const info: ModuleInfo = {
  title: '外部轨迹有效时间判断',
  owner: 'transport-fulfillment',
  source: 'CONTEXT.md',
};
const options = { module: info, endpoint: 'GET /transport-fulfillment-external-tracking-facts?source=…&view=…' };
const noRetry = () => {};

const pendingFact: ExternalTrackingFactRecord = {
  fact: 'EXTF-1',
  version: 'EXTV-1',
  source: 'aggregator-a',
  credential: 'carrier-x/1Z999',
  object: 'PCL-1',
  sourceEvent: 'evt-1',
  status: 'DELIVERED',
  occurredAt: '2026-09-04T06:00:00Z',
  receivedAt: '2026-09-04T06:01:30Z',
  effectiveBasis: 'PENDING',
  origin: 'MATERIAL',
  recordedAt: '2026-09-04T06:01:31Z',
};

const judgedByRuleFact: ExternalTrackingFactRecord = {
  ...pendingFact,
  fact: 'EXTF-2',
  version: 'EXTV-2b',
  sourceEvent: undefined,
  effectiveBasis: 'JUDGED_BY_RULE',
  effectiveAt: '2026-09-04T06:10:00Z',
  effectiveRule: 'aggregator-a',
  effectiveRuleVersion: 'ETR-1',
  supersedes: 'EXTV-2a',
  origin: 'JUDGMENT',
};

function outcome<Body>(body: Body): ApiResult<Body> {
  return { kind: 'outcome', status: 200, body };
}

test('待判断行：三个时间只有两个在场，有效时间与依据明细都是缺席不是零值', () => {
  const [row] = factRowsOf({ outcome: 'PENDING_EFFECTIVE_TIME_FACTS_LISTED', facts: [pendingFact] });

  equal(row.key, 'fact:EXTF-1@EXTV-1');
  equal(row.fact, 'EXTF-1');
  equal(row.values.status, 'DELIVERED');
  equal(row.values.effectiveBasis, '待判断');
  equal(row.values.effectiveAt, '—');
  equal(row.values.basisDetail, '');
  equal(row.values.supersedes, '');
  equal(row.values.origin, '素材到达');
  ok(row.values.occurredAt.startsWith('2026-09-04 06:00:00'), row.values.occurredAt);
  ok(row.values.receivedAt.startsWith('2026-09-04 06:01:30'), row.values.receivedAt);
});

test('判断版本行：依据、规则版本与前版原样上列，状态词原词不译', () => {
  const [row] = factRowsOf({
    outcome: 'CURRENT_EXTERNAL_TRACKING_FACTS_LISTED',
    facts: [judgedByRuleFact],
  });

  equal(row.values.effectiveBasis, '按已登记规则判断');
  equal(row.values.basisDetail, 'aggregator-a@ETR-1');
  equal(row.values.supersedes, 'EXTV-2a');
  equal(row.values.origin, '判断形成');
  equal(row.values.sourceEvent, '—');
  ok(row.values.effectiveAt.startsWith('2026-09-04 06:10:00'), row.values.effectiveAt);
});

test('词表没收录的依据词原样示码，不猜词', () => {
  const [row] = factRowsOf({
    outcome: 'CURRENT_EXTERNAL_TRACKING_FACTS_LISTED',
    facts: [{ ...pendingFact, effectiveBasis: 'SOMETHING_NEW' }],
  });
  equal(row.values.effectiveBasis, 'SOMETHING_NEW');
});

test('源未填不发请求，与「问过了、这家源没有待判断的事实」分开措辞', () => {
  const idle = factListViewState(null, '  ', 'pending', 0, noRetry, options);
  equal(idle.kind, 'empty');
  ok(idle.kind === 'empty');
  ok(idle.title?.includes('尚未指定'), idle.title);

  equal(factListViewState(null, 'aggregator-a', 'pending', 0, noRetry, options).kind, 'loading');
});

test('待判断视图空册：点名源，说清它是真答案而不是渠道未配置', () => {
  const body: ExternalTrackingFactListResponseBody = {
    outcome: 'PENDING_EFFECTIVE_TIME_FACTS_LISTED',
    facts: [],
  };
  const state = factListViewState(outcome(body), 'aggregator-a', 'pending', 0, noRetry, options);
  equal(state.kind, 'empty');
  ok(state.kind === 'empty');
  ok(state.title?.includes('aggregator-a'), state.title);
  ok(state.title?.includes('待判断'), state.title);
  ok(state.description?.includes('已判断'), `空态说明要把「全部已判断」这一种空摘出来：${state.description}`);
});

test('全部当前版视图空册：措辞与待判断空册分开——这家源还没有被认领的轨迹', () => {
  const body: ExternalTrackingFactListResponseBody = {
    outcome: 'CURRENT_EXTERNAL_TRACKING_FACTS_LISTED',
    facts: [],
  };
  const state = factListViewState(outcome(body), 'aggregator-a', 'current', 0, noRetry, options);
  equal(state.kind, 'empty');
  ok(state.kind === 'empty');
  ok(state.title?.includes('当前版'), state.title);
  ok(!state.title?.includes('待判断'), `全部当前版的空册不该说成待判断：${state.title}`);
});

test('有行即就绪；未配置与未形成答案走全站那套 HTTP 代数', () => {
  equal(
    factListViewState(
      outcome({ outcome: 'PENDING_EFFECTIVE_TIME_FACTS_LISTED', facts: [pendingFact] }),
      'aggregator-a',
      'pending',
      1,
      noRetry,
      options,
    ).kind,
    'ready',
  );
  equal(
    factListViewState({ kind: 'unconfigured' }, 'aggregator-a', 'pending', 0, noRetry, options).kind,
    'unconfigured',
  );
  equal(
    factListViewState(
      { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' },
      'aggregator-a',
      'pending',
      0,
      noRetry,
      options,
    ).kind,
    'error',
  );
});

test('已判断：新版本回指前版、依据是所有者显式判断、意图已交出', () => {
  const body: EffectiveTimeJudgmentResponseBody = {
    outcome: 'EFFECTIVE_TIME_JUDGED',
    fact: 'EXTF-1',
    version: 'EXTV-J1',
    source: 'aggregator-a',
    object: 'PCL-1',
    status: 'DELIVERED',
    occurredAt: '2026-09-04T06:00:00Z',
    receivedAt: '2026-09-04T06:01:30Z',
    effectiveBasis: 'JUDGED_EXPLICITLY',
    effectiveAt: '2026-09-04T06:05:00Z',
    supersedes: 'EXTV-1',
    recordedAt: '2026-09-05T12:00:00Z',
  };
  const note = describeJudgmentAnswer(body);
  equal(note.tone, 'answered');
  equal(note.landed, true);
  ok(note.headline.includes('EFFECTIVE_TIME_JUDGED'), note.headline);
  ok(note.details.some((line) => line.includes('EXTV-J1') && line.includes('EXTV-1')), note.details.join('|'));
  ok(note.details.some((line) => line.includes('所有者显式判断')), note.details.join('|'));
  ok(!note.details.some((line) => line.includes('尚未交出')), '意图已交出时不得说欠着');
});

test('已按同值判过：不是失败，摆出既有版本的依据与有效时间（票 21 红线）', () => {
  const body: EffectiveTimeJudgmentResponseBody = {
    outcome: 'ALREADY_JUDGED_AS_GIVEN',
    fact: 'EXTF-1',
    version: 'EXTV-J1',
    effectiveBasis: 'JUDGED_EXPLICITLY',
    effectiveAt: '2026-09-04T06:05:00Z',
    supersedes: 'EXTV-1',
  };
  const note = describeJudgmentAnswer(body);
  equal(note.tone, 'answered');
  equal(note.landed, false);
  ok(note.details.some((line) => line.includes('已经判过') && line.includes('所有者显式判断')), note.details.join('|'));
  ok(note.details.some((line) => line.includes('2026-09-04 06:05:00')), note.details.join('|'));
});

test('意图没交出去：判断照样落地，但要把 handoffReference 摆出来', () => {
  const note = describeJudgmentAnswer({
    outcome: 'EFFECTIVE_TIME_JUDGED',
    fact: 'EXTF-1',
    version: 'EXTV-J1',
    effectiveBasis: 'JUDGED_EXPLICITLY',
    effectiveAt: '2026-09-04T06:05:00Z',
    supersedes: 'EXTV-1',
    handoffReference: 'CONT-h4nd0ff',
  });
  equal(note.landed, true);
  ok(note.details.some((line) => line.includes('尚未交出') && line.includes('CONT-h4nd0ff')), note.details.join('|'));
});

test('未受理：不给重试；未决：带成因与续办引用、可重试', () => {
  const refused = describeJudgmentAnswer({ outcome: 'INPUT_NOT_ACCEPTED' });
  equal(refused.tone, 'refused');
  equal(refused.canRetry, false);

  const undecided = describeJudgmentAnswer({
    outcome: 'JUDGMENT_UNDECIDED',
    undecidedReason: 'TRACKING_FACT_REGISTRY_UNAVAILABLE',
    continuationReference: 'CONT-abc123',
  });
  equal(undecided.tone, 'undecided');
  equal(undecided.canRetry, true);
  ok(undecided.details.some((line) => line.includes('事实登记册读不回来')), undecided.details.join('|'));
  ok(undecided.details.some((line) => line.includes('CONT-abc123')), undecided.details.join('|'));
});

test('词表没收录的结果词原样示出，不归进既有中文说法', () => {
  const note = describeJudgmentAnswer({ outcome: 'SOMETHING_NEW' });
  ok(note.headline.includes('SOMETHING_NEW'), note.headline);
  deepEqual(note.details, []);
});
