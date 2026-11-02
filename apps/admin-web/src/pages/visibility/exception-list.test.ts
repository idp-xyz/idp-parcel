import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { configureVocabularyWarnings, labelFor, statusEntryFor } from '../../vocabulary/status';
import {
  casePhaseCodes,
  claimConclusionCodes,
  claimScreenCodes,
  dispositionCancellationCodes,
  dispositionJudgmentCodes,
  handlerLabel,
  notificationMilestoneCodes,
  notificationScope,
  recoveryMilestoneCodes,
  recoveryMilestoneScope,
  triageOutcomeCodes,
} from './case-presentation';
import {
  claimStatus,
  countByStatus,
  dispositionStatus,
  episodeStatus,
  notificationStatus,
  sortRowsBy,
} from './exception-list';
import type { ClaimItemRecord, CustomerNotificationRecord, SignalEpisodeRecord } from './case-api';

// 本文件钉异常与索赔区的两件事：各封闭集的码在词典里全部有中文（含分册），以及行状态取哪一步——还没答的格给派生词，
// 不演成任何一个已答的值。

const hasChinese = (code: string, scope?: string) => {
  const warnings: string[] = [];
  configureVocabularyWarnings((unknown) => warnings.push(unknown));
  const label = labelFor(code, scope);
  configureVocabularyWarnings(null);
  deepEqual(warnings, [], `${scope ?? '总表'}/${code} 查不到中文`);
  equal(/[A-Za-z]/.test(label), false, `${code} 的词「${label}」含英文`);
};

// Covers: 案件、分诊、处置、索赔、通知、追偿各封闭集的码都有中文，查不到的码不上屏。
test('各封闭集的码全部有中文', () => {
  for (const code of casePhaseCodes) hasChinese(code);
  for (const code of triageOutcomeCodes) hasChinese(code);
  for (const code of dispositionJudgmentCodes) hasChinese(code, 'disposition-judgment');
  for (const code of dispositionCancellationCodes) hasChinese(code);
  for (const code of claimScreenCodes) hasChinese(code);
  for (const code of claimConclusionCodes) hasChinese(code);
  for (const code of notificationMilestoneCodes) hasChinese(code, notificationScope);
  for (const code of recoveryMilestoneCodes) hasChinese(code, recoveryMilestoneScope(code));
  for (const code of ['LOSS', 'DAMAGE']) hasChinese(code, 'claim-kind');
  for (const code of ['DELIVERY_FAILED', 'CUSTOMS_HOLD', 'ETA_GAP', 'SOURCE_FACT_CONFLICT', 'DEPARTURE_GATE_BLOCKED']) {
    hasChinese(code, 'exception-signal');
  }
});

// Covers: 同一个码在本区的意思与总表不同的几处，按分册取词。
test('本区分册的译法', () => {
  equal(labelFor('DELIVERED', notificationScope), '已送达');
  equal(labelFor('DELIVERED'), '已签收');
  equal(labelFor('DELIVERY_FAILED', recoveryMilestoneScope('DELIVERY_FAILED')), '送达失败');
  equal(labelFor('SUBMITTED', recoveryMilestoneScope('SUBMITTED')), '已提交');
  equal(labelFor('ACCEPTED', 'disposition-judgment'), '已接受');
  equal(labelFor('MERGED', 'case-conclusion'), '已并入其他案件');
  equal(handlerLabel('CUSTOMS_COMPLIANCE'), '清关');
  equal(handlerLabel('SOMETHING_NEW'), 'SOMETHING_NEW');
});

const episode = (overrides: Partial<SignalEpisodeRecord> = {}): SignalEpisodeRecord => ({
  episodeId: 'EP-1',
  parcel: 'DPCL-1',
  kind: 'DELIVERY_FAILED',
  causalCondition: 'C',
  factBasis: [],
  rule: 'R',
  confidence: 'CARRIER_CONFIRMED',
  hits: 1,
  startedAt: '2026-10-28T10:00:00Z',
  lastHitAt: '2026-10-28T10:00:00Z',
  ...overrides,
});

// Covers: 没有分诊结论是「待分诊」，有结论按词典。
test('发作期状态：未分诊与已分诊分开', () => {
  deepEqual(episodeStatus(episode()), { label: '待分诊', tone: 'warn' });
  const triaged = episodeStatus(episode({ triage: { outcome: 'MANUAL_REVIEW', rule: 'R', triagedAt: '2026-10-28T10:00:00Z' } }));
  equal(triaged.code, 'MANUAL_REVIEW');
  equal(triaged.label, statusEntryFor('MANUAL_REVIEW').label);
});

// Covers: 处置请求被替代优先于判断；没有判断是「等对方答复」。
test('处置请求状态', () => {
  equal(dispositionStatus({}).label, '等对方答复');
  equal(dispositionStatus({ judgment: 'ACCEPTED' }).label, '已接受');
  equal(dispositionStatus({ judgment: 'ACCEPTED', supersededBy: 'REQ-2' }).label, '已替代');
});

const claim = (overrides: Partial<ClaimItemRecord> = {}): ClaimItemRecord => ({
  batch: 'B',
  itemId: 'I',
  customer: 'SYN-ACCOUNT-01',
  contract: 'SYN-CONTRACT-01',
  target: 'DPCL-1',
  kind: 'LOSS',
  submittedAt: '2026-10-28T12:00:11Z',
  revision: 1,
  deadlineVersions: 0,
  withdrawn: false,
  ...overrides,
});

// Covers: 索赔状态取最靠后的一判：撤回 > 结论 > 资格 > 待审核。
test('索赔状态取走到的那一步', () => {
  deepEqual(claimStatus(claim()), { label: '待审核', tone: 'warn' });
  equal(claimStatus(claim({ screen: 'ELIGIBLE' })).code, 'ELIGIBLE');
  equal(claimStatus(claim({ screen: 'ELIGIBLE', conclusion: 'FULLY_ESTABLISHED' })).code, 'FULLY_ESTABLISHED');
  equal(claimStatus(claim({ screen: 'ELIGIBLE', withdrawn: true })).label, '已撤回');
});

const notification = (milestones: CustomerNotificationRecord['milestones']): CustomerNotificationRecord => ({
  notificationId: 'N',
  customer: 'SYN-ACCOUNT-01',
  episode: 'EP-1',
  decidedAt: '2026-10-28T10:00:00Z',
  policy: 'P',
  content: 'C',
  deadline: '2026-10-29T10:00:00Z',
  channel: 'EMAIL',
  obligation: 'O',
  milestones,
});

// Covers: 通知进度取登记时刻最晚的节点，送达说「已送达」不说「已签收」；一个节点都没有是「待发送」。
test('通知进度取最后一步', () => {
  equal(notificationStatus(notification([])).label, '待发送');
  const status = notificationStatus(
    notification([
      { milestone: 'DELIVERED', recordedAt: '2026-10-28T11:00:00Z' },
      { milestone: 'CHANNEL_ACCEPTED', recordedAt: '2026-10-28T10:30:00Z' },
    ]),
  );
  equal(status.label, '已送达');
  equal(status.scope, notificationScope);
});

// Covers: 计数按词、按首次出现排；排序按时刻比而不是字符串比。
test('计数与排序', () => {
  const rows = [claim(), claim({ screen: 'ELIGIBLE' }), claim()];
  deepEqual(
    countByStatus(rows, claimStatus).map((item) => [item.label, item.count]),
    [
      ['待审核', 2],
      [labelFor('ELIGIBLE'), 1],
    ],
  );
  const sorted = sortRowsBy(
    [claim({ itemId: 'a', submittedAt: '2026-10-28T12:00:55Z' }), claim({ itemId: 'b', submittedAt: '2026-10-28T12:00:55.9Z' })],
    { key: 'submitted', dir: -1 },
    { submitted: { value: (row) => row.submittedAt, instant: true } },
  );
  ok(sorted[0].itemId === 'b');
});
