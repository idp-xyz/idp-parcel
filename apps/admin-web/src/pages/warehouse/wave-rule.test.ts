import { deepEqual, equal, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { buildGuidedPayload } from './guided-forms';
import { allWarehouseGuidedForms } from './guided-schemas';
import { asWarehouseAction } from './actions';
import {
  allocationExceptionNote,
  maxRequestsPerWaveRuleRun,
  mintWaveRuleRunId,
  nextWaveRuleRevision,
  releaseExceptionsText,
  usableWaveRules,
  warehousesWithWaveRules,
  waveRulePreviewConditions,
  waveRulePreviewText,
  waveRuleRunNote,
  waveRuleSummary,
  type WaveRuleRecord,
} from './wave-rule';

const rule = (ruleCode: string, more: Partial<WaveRuleRecord> = {}): WaveRuleRecord => ({
  warehouseCode: 'WH-1',
  ruleCode,
  revision: 1,
  name: `规则 ${ruleCode}`,
  profile: 'ANY',
  maxRequests: 20,
  batchPick: false,
  deactivated: false,
  operator: 'op',
  registeredAt: '2026-11-27T00:00:00Z',
  ...more,
});

// Covers: 一条规则圈什么、怎么放说成一句话；三个圈单条件都没带时说这个仓已预占的单都圈。
test('规则说成一句话：不带的条件不提，都不带就是都圈', () => {
  equal(waveRuleSummary(rule('A')), '这个仓已预占的出库单都圈，一个波次最多 20 张，逐单拣');
  equal(
    waveRuleSummary(rule('B', { ownerAccount: 'OWNER-1', dispatch: 'PICKUP', profile: 'SINGLE_UNIT', maxRequests: 50 })),
    '货主 OWNER-1，到仓自提，只圈单品单件，一个波次最多 50 张，逐单拣',
  );
  equal(waveRuleSummary(rule('C', { profile: 'MULTI_UNIT', batchPick: true })), '只圈不是单品单件的，一个波次最多 20 张，合并拣货');
  equal(waveRuleSummary(rule('D', { maxLines: 60, maxUnits: 200 })), '这个仓已预占的出库单都圈，一个波次最多 20 张、60 行、200 件，逐单拣');
  // 限了行数或件数时波次会切得更碎，估计只说得出至少几个
  ok(waveRulePreviewText(rule('D', { maxUnits: 200 }), 45).includes('放成至少 3 个波次'));
});

// Covers: 一个仓能用的规则不含停用的、不含别的仓的；有可用规则的仓才进下拉。
test('能用的规则：这个仓的、没停用的', () => {
  const rules = [rule('B'), rule('A'), rule('OLD', { deactivated: true }), rule('X', { warehouseCode: 'WH-2', deactivated: true }), rule('Y', { warehouseCode: 'WH-3' })];
  deepEqual(usableWaveRules(rules, 'WH-1').map((item) => item.ruleCode), ['A', 'B']);
  deepEqual(usableWaveRules(rules, 'WH-2'), []);
  deepEqual(warehousesWithWaveRules(rules), ['WH-1', 'WH-3']);
  equal(nextWaveRuleRevision(rules, 'WH-1', 'A'), 2);
  equal(nextWaveRuleRevision(rules, 'WH-1', 'NEW'), 1);
});

// Covers: 点之前的估计用出库单列表读口问，条件与后端圈单的逐条对应：已预占、这个仓，带了的条件才筛。
test('估计圈得到几张的条件与规则逐条对应', () => {
  deepEqual(waveRulePreviewConditions(rule('A')), { filters: { status: ['RESERVED'], warehouseCode: ['WH-1'] } });
  deepEqual(waveRulePreviewConditions(rule('B', { ownerAccount: 'OWNER-1', dispatch: 'PICKUP', profile: 'SINGLE_UNIT' })), {
    filters: { status: ['RESERVED'], warehouseCode: ['WH-1'], ownerAccount: ['OWNER-1'], dispatch: ['PICKUP'], singleUnit: ['true'] },
  });
  deepEqual(waveRulePreviewConditions(rule('C', { profile: 'MULTI_UNIT' })).filters?.singleUnit, ['false']);
});

// Covers: 估计的话：几张、放成几个波次；超过一次的上限说要再点一次；一张都没有、总数读不出各说各的。
test('估计的话如实说', () => {
  ok(waveRulePreviewText(rule('A'), 45).includes('圈得到 45 张已预占的出库单，全都分配得出的话放成 3 个波次'));
  ok(waveRulePreviewText(rule('A'), 20).includes('放成 1 个波次'));
  const many = waveRulePreviewText(rule('A', { maxRequests: 200 }), maxRequestsPerWaveRuleRun + 30);
  ok(many.includes('放成 3 个波次') && many.includes('剩下的 30 张要再点一次'));
  ok(waveRulePreviewText(rule('A'), 0).includes('没有圈得到的出库单'));
  ok(waveRulePreviewText(rule('A'), null).includes('没读出'));
});

// Covers: 执行号是规则码加到秒的时刻。
test('执行号', () => {
  equal(mintWaveRuleRunId('SMALL-AM', new Date('2026-11-27T01:02:03Z')), 'SMALL-AM-20261127-010203');
});

// Covers: 答复说成的话：放出了几个波次几张；跳过的、没轮到的如实带出并转成提醒；重放说之前成过；
// 没有可放的、规则没登记、已停用、未配置、撞号各说各的；不认识的结果不当成成波。
test('成波之后的话', () => {
  const clean = waveRuleRunNote({ outcome: 'ALLOCATED', waves: [{ waveId: 'R-01', requests: 2 }, { waveId: 'R-02', requests: 1 }], released: 3, skipped: 0, remaining: 0 });
  deepEqual(clean, { released: true, tone: 'success', text: '放出了 2 个波次，共 3 张出库单。' });
  const partial = waveRuleRunNote({ outcome: 'ALLOCATED', waves: [{ waveId: 'R-01', requests: 2 }], released: 2, skipped: 1, remaining: 4 });
  equal(partial.tone, 'warning');
  ok(partial.released && partial.text.includes('有 1 张圈到了但分配不出') && partial.text.includes('还有 4 张这一次没轮到'));
  const replay = waveRuleRunNote({ outcome: 'EXISTING_RESULT', waves: [{ waveId: 'R-01', requests: 2 }], released: 2 });
  ok(replay.released && replay.text.includes('之前已经成过了') && replay.text.includes('1 个波次，共 2 张'));
  const none = waveRuleRunNote({ outcome: 'NOTHING_TO_RELEASE', waves: [], released: 0, skipped: 0 });
  deepEqual(none, { released: false, tone: 'warning', text: '没有圈到合条件的已预占的出库单，一个波次也没放出。' });
  const allSkipped = waveRuleRunNote({ outcome: 'NOTHING_TO_RELEASE', skipped: 2 });
  ok(!allSkipped.released && allSkipped.text.includes('一个波次也没放出') && allSkipped.text.includes('有 2 张圈到了但分配不出'));
  for (const [outcome, word] of [
    ['WAVE_RULE_NOT_FOUND', '没有登记过这条规则'],
    ['WAVE_RULE_DEACTIVATED', '已经停用'],
    ['NOT_CONFIGURED', '未配置'],
    ['CONTENT_CONFLICT', '撞了'],
  ] as const) {
    const note = waveRuleRunNote({ outcome });
    ok(!note.released && note.tone === 'danger' && note.text.includes(word), outcome);
  }
  const strange = waveRuleRunNote({ outcome: 'SOMETHING_NEW' });
  ok(!strange.released && strange.tone === 'danger' && strange.text.includes('SOMETHING_NEW'));
});

// Covers: 放波次勾了「放行能放的」之后没放进波次的那几张各说缘由；行上「放不出」那一句写明是哪一次跳过的；
// 放波次表单勾了才带 partial。
test('放行能放的：例外逐张说缘由，行上写明是哪一次跳过的', () => {
  equal(releaseExceptionsText(undefined), '');
  equal(releaseExceptionsText([]), '');
  const text = releaseExceptionsText([
    { requestId: 'OBR-1', reason: 'NOT_ALLOCATABLE' },
    { requestId: 'OBR-2', reason: 'NOT_RESERVED' },
    { requestId: 'OBR-3', reason: 'NOT_FOUND' },
    { requestId: 'OBR-4', reason: 'NEW_WORD' },
  ]);
  ok(text.startsWith('没放进波次的 4 张：OBR-1（分配不出）、OBR-2（不是已预占）、OBR-3（没有这张单，或不属于这个仓）、OBR-4（NEW_WORD）'));
  equal(asWarehouseAction({ outcome: 'ALLOCATED', exceptions: [{ requestId: 'OBR-1', reason: 'NOT_ALLOCATABLE' }] }).cause?.includes('OBR-1（分配不出）'), true);
  equal(asWarehouseAction({ outcome: 'ALLOCATED' }).cause, undefined);

  equal(allocationExceptionNote(undefined), null);
  const byRule = allocationExceptionNote({ source: 'RULE', reference: 'RUN-1', recordedAt: '2026-11-27T00:00:00Z' });
  ok(byRule?.label === '上次放不出' && byRule.detail.includes('按规则成波（RUN-1）'));
  ok(allocationExceptionNote({ source: 'WAVE', reference: 'WAVE-9', recordedAt: '2026-11-27T00:00:00Z' })?.detail.includes('放波次 WAVE-9'));

  const form = allWarehouseGuidedForms['wave-release'];
  const plain = buildGuidedPayload(form, { waveId: 'W-1', warehouse: 'WH-1', requests: 'OBR-1' });
  ok(plain.ok && !('partial' in plain.payload));
  const partial = buildGuidedPayload(form, { waveId: 'W-1', warehouse: 'WH-1', requests: 'OBR-1', partial: true });
  ok(partial.ok && partial.payload.partial === true);
});

// Covers: 登记规则的表单装出来的报文：没填的圈单条件不带键（不带才是不限，空串后端不收），没勾的两格不带。
test('登记规则的报文：不限的条件不带键', () => {
  const form = allWarehouseGuidedForms['wave-rule'];
  ok(form !== undefined);
  const plain = buildGuidedPayload(form, { warehouse: 'WH-1', ruleCode: 'A', revision: '1', name: '早班小件', maxRequests: '20' });
  ok(plain.ok);
  deepEqual(plain.ok ? plain.payload : null, { warehouse: 'WH-1', ruleCode: 'A', revision: 1, name: '早班小件', maxRequests: 20 });
  const full = buildGuidedPayload(form, {
    warehouse: 'WH-1',
    ruleCode: 'A',
    revision: '2',
    name: '早班小件',
    maxRequests: '20',
    maxLines: '60',
    maxUnits: '200',
    ownerAccount: 'OWNER-1',
    dispatch: 'PICKUP',
    profile: 'SINGLE_UNIT',
    batchPick: true,
    deactivated: true,
  });
  deepEqual(full.ok ? full.payload : null, {
    warehouse: 'WH-1',
    ruleCode: 'A',
    revision: 2,
    name: '早班小件',
    maxRequests: 20,
    maxLines: 60,
    maxUnits: 200,
    ownerAccount: 'OWNER-1',
    dispatch: 'PICKUP',
    profile: 'SINGLE_UNIT',
    batchPick: true,
    deactivated: true,
  });
  const missing = buildGuidedPayload(form, { warehouse: 'WH-1', ruleCode: 'A', revision: '1', name: '早班小件' });
  ok(!missing.ok);
});
