import { test } from 'node:test';
import { deepEqual, equal, match } from 'node:assert/strict';
import type { LegalEntityRevisionRecord } from './api';
import {
  legalEntityRevisionTimeline,
  revisionHistoryNote,
} from './legal-entity-revisions';

// 本文件钉抽屉「修订历史」区的判读（票 admin-web-group-legal-entities/03 第 5 条）：每笔显修订号、依据、
// 生效自、登记时间，停用那笔标出；序照读口交回的修订号升序，页面不重排也不做 diff。

function revision(over: Partial<LegalEntityRevisionRecord>): LegalEntityRevisionRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    legalEntityId: 'SYN-LE-01',
    partyId: 'SYN-PARTY-OPERATOR-01',
    revision: 1,
    basis: 'SYN-REG-BASIS-LE-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-09-16T04:27:55Z',
    ...over,
  };
}

// 时刻格式化由页面注入（moment.ts 的 formatInstant 依赖装配点配置的时区）；测试里用一个打标记的替身，
// 好看出哪几格真的过了格式化、哪几格被原样漏出去。
const tagged = (iso: string) => `[${iso}]`;

const chain: LegalEntityRevisionRecord[] = [
  revision({}),
  revision({
    revision: 2,
    partyId: 'SYN-PARTY-OPERATOR-02',
    basis: 'SYN-REG-BASIS-LE-01-R2',
    registeredAt: '2026-09-16T05:00:00Z',
  }),
  revision({
    revision: 3,
    partyId: 'SYN-PARTY-OPERATOR-02',
    basis: 'SYN-REG-BASIS-LE-01-R2',
    deactivatedAt: '2026-06-01T00:00:00Z',
    deactivationBasis: 'SYN-DEACT-01',
    registeredAt: '2026-09-16T06:00:00Z',
  }),
];

// Covers: 一笔一项、序不动；标题带修订号，停用那一笔标出`已停用`（CONTEXT 原词）且色调换成 warning，
// 其余两笔标题不带停用字样、色调 default。
test('每笔修订一项，停用那笔标出', () => {
  const items = legalEntityRevisionTimeline(chain, tagged);
  deepEqual(
    items.map((item) => item.id),
    ['r1', 'r2', 'r3'],
  );
  equal(items[0].title, 'r1');
  equal(items[1].title, 'r2');
  equal(items[2].title, 'r3 · 已停用');
  deepEqual(
    items.map((item) => item.variant),
    ['default', 'default', 'warning'],
  );
});

// Covers: 每笔显依据、生效自、参与方身份；停用那笔另显停用时点与停用依据，未停用的笔不带任何停用字样
// （键不在场就不说，不拿空串推）。三个时刻都经注入的格式化函数，一个也不原样漏出去。
test('描述逐笔列依据、生效自、参与方身份，停用两件只在停用那笔', () => {
  const items = legalEntityRevisionTimeline(chain, tagged);
  equal(
    items[0].description,
    '依据 SYN-REG-BASIS-LE-01 · 生效自 [2026-01-02T00:00:00Z] · 参与方身份 SYN-PARTY-OPERATOR-01',
  );
  equal(/停用/.test(items[0].description), false);
  equal(/停用/.test(items[1].description), false);
  equal(
    items[2].description,
    '依据 SYN-REG-BASIS-LE-01-R2 · 生效自 [2026-01-02T00:00:00Z] · 参与方身份 SYN-PARTY-OPERATOR-02' +
      ' · 停用于 [2026-06-01T00:00:00Z]（依据 SYN-DEACT-01）',
  );
  deepEqual(
    items.map((item) => item.timestamp),
    ['登记于 [2026-09-16T04:27:55Z]', '登记于 [2026-09-16T05:00:00Z]', '登记于 [2026-09-16T06:00:00Z]'],
  );
  for (const item of items) {
    // 原始 ISO 串不得裸露在任何一格：裸露即某个时刻没过格式化。
    equal(/(^|[^[])\d{4}-\d{2}-\d{2}T/.test(item.description + item.timestamp), false);
  }
});

// Covers: 空数组不抛也不造项——法人不在册是读口交回的如实答案（票 03 裁 200 + []），页面另用一句话说。
test('空修订链交回空项，注释句分得开「没有」与「没问到」', () => {
  deepEqual(legalEntityRevisionTimeline([], tagged), []);
  match(revisionHistoryNote(0), /没有.*修订/);
  match(revisionHistoryNote(1), /1 笔/);
  match(revisionHistoryNote(3), /3 笔/);
});
