import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import {
  configureVocabularyWarnings,
  knownStatusCodes,
  labelFor,
  scopedStatusVocabulary,
  statusEntryFor,
  statusVocabulary,
  toneFor,
  toneForLabel,
} from './status';

// 本文件钉的是用语词典的三条纪律：已知码全部有中文、不漏英文；查不到的码原样显示并只告警一次；分册覆盖只在那本册里生效。

const tones = new Set(['good', 'info', 'warn', 'bad', 'muted']);

// Covers: 每个已知码都译成了中文——词里不含任何英文字母、不为空，色调是五档之一。英文混在中文词里（如「已 DELIVERED」）
// 同样算泄漏。
test('已知码全部有中文词，无英文泄漏，色调五档之一', () => {
  ok(knownStatusCodes.length > 80, `词典只收了 ${knownStatusCodes.length} 个码`);
  const check = (code: string, label: string, tone: string) => {
    ok(label.trim() !== '', `${code} 的词为空`);
    equal(/[A-Za-z]/.test(label), false, `${code} 的词「${label}」含英文`);
    ok(tones.has(tone), `${code} 的色调 ${tone} 不在五档里`);
  };
  for (const [code, entry] of Object.entries(statusVocabulary)) check(code, entry.label, entry.tone);
  for (const [scope, table] of Object.entries(scopedStatusVocabulary)) {
    for (const [code, entry] of Object.entries(table)) check(`${scope}/${code}`, entry.label, entry.tone);
  }
});

// Covers: 设计源第四节点名的几组译法。
test('设计源点名的译法', () => {
  equal(labelFor('SUBMITTED'), '待受理');
  equal(labelFor('ACCEPTED'), '已受理');
  equal(labelFor('WITHDRAWN'), '已撤回');
  equal(labelFor('REJECTED'), '已拒绝');
  equal(labelFor('REGISTERED'), '待生效');
  equal(labelFor('EFFECTIVE'), '已生效');
  equal(labelFor('DEACTIVATED'), '已停用');
  equal(labelFor('CANDIDATE'), '待确认');
  equal(labelFor('ESTIMATED'), '预估');
  equal(labelFor('CONFIRMED'), '已确认');
  equal(labelFor('DELIVERED'), '已签收');
  equal(labelFor('PICKED_UP'), '已揽收');
  equal(labelFor('EFFECTIVE_DELIVERY'), '已签收');
  equal(labelFor('external-result-release-full'), '已放行');
  equal(labelFor('node-intake'), '已收寄');
  equal(labelFor('ADMITTED'), '可放行');
});

// Covers: 色调按设计原则四——待处理琥珀、推进中蓝、成立绿、失败红、已结束灰。
test('色调按设计原则四分档', () => {
  equal(toneFor('SUBMITTED'), 'warn');
  equal(toneFor('IN_TRANSIT'), 'info');
  equal(toneFor('DELIVERED'), 'good');
  equal(toneFor('REJECTED'), 'bad');
  equal(toneFor('WITHDRAWN'), 'muted');
});

// Covers: 同一个词被几个码共用时色调一致，否则按词反查（toneForLabel）会随遍历顺序漂。
test('同词同色', () => {
  const toneByLabel = new Map<string, string>();
  const tables = [statusVocabulary, ...Object.values(scopedStatusVocabulary)];
  for (const table of tables) {
    for (const [code, entry] of Object.entries(table)) {
      const seen = toneByLabel.get(entry.label);
      if (seen !== undefined) equal(entry.tone, seen, `「${entry.label}」在 ${code} 上是 ${entry.tone}，别处是 ${seen}`);
      toneByLabel.set(entry.label, entry.tone);
    }
  }
  equal(toneForLabel('已签收'), 'good');
  equal(toneForLabel('不存在的词'), 'muted');
});

// Covers: 查不到的码原样作词、色调中性；装配点注入的告警每码只响一次；关掉告警后不再响。
test('查不到的码原样显示、每码只告警一次', () => {
  const warnings: string[] = [];
  configureVocabularyWarnings((code) => warnings.push(code));
  deepEqual(statusEntryFor('NEVER_SEEN'), { label: 'NEVER_SEEN', tone: 'muted' });
  equal(labelFor('NEVER_SEEN'), 'NEVER_SEEN');
  equal(toneFor('NEVER_SEEN'), 'muted');
  deepEqual(warnings, ['NEVER_SEEN']);
  configureVocabularyWarnings(null);
  equal(labelFor('ANOTHER_UNKNOWN'), 'ANOTHER_UNKNOWN');
  deepEqual(warnings, ['NEVER_SEEN']);
});

// Covers: 分册覆盖只在那本册里生效，别的册与不传册都落总表。
test('分册覆盖按册生效', () => {
  equal(labelFor('REGISTERED', 'work-device'), '在用');
  equal(labelFor('REGISTERED'), '待生效');
  equal(labelFor('REGISTERED', 'pricing-evaluation'), '待生效');
  equal(labelFor('PENDING', 'pricing-evaluation'), '待判断');
  equal(labelFor('REFUSED', 'handover'), '已拒收');
  equal(toneFor('OPEN', 'consolidation'), 'info');
});
