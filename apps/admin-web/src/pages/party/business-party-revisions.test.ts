import { test } from 'node:test';
import { deepEqual, equal, match } from 'node:assert/strict';
import type { BusinessPartyRevisionRecord } from './api';
import {
  businessPartyRevisionHistoryNote,
  businessPartyRevisionTimeline,
} from './business-party-revisions';

// 本文件钉业务参与方抽屉「修订历史」区的判读（票 admin-web-group-legal-entities/12 第 5 条）：每笔显修订号、
// 名称、依据、生效自、登记时间，停用那笔标出；序照读口交回的修订号升序，页面不重排也不做 diff。
// 名称是本册比法人册多出的一格：两笔并排时「从哪份换到哪份」要能读出来。

function revision(over: Partial<BusinessPartyRevisionRecord>): BusinessPartyRevisionRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    partyId: 'SYN-PARTY-RETIRED-01',
    partyName: '合成已停用参与方',
    revision: 1,
    basis: 'SYN-REG-BASIS-PARTY-RETIRED-01',
    effectiveFrom: '2026-01-02T00:00:00Z',
    registeredAt: '2026-09-16T04:27:55Z',
    ...over,
  };
}

// 时刻格式化由页面注入（moment.ts 的 formatInstant 依赖装配点配置的时区）；测试里用一个打标记的替身，
// 好看出哪几格真的过了格式化、哪几格被原样漏出去。
const tagged = (iso: string) => `[${iso}]`;

// 演示形态照票面完成判据：SYN-PARTY-RETIRED-01 两笔，第 2 笔已停用；第 2 笔顺带换了名称与依据。
const chain: BusinessPartyRevisionRecord[] = [
  revision({}),
  revision({
    revision: 2,
    partyName: '合成已停用参与方（更名）',
    basis: 'SYN-REG-BASIS-PARTY-RETIRED-01-R2',
    deactivatedAt: '2026-06-01T00:00:00Z',
    deactivationBasis: 'SYN-DEACT-PARTY-01',
    registeredAt: '2026-09-16T05:00:00Z',
  }),
];

// Covers: 一笔一项、序不动；标题带修订号，停用那一笔标出`已停用`（CONTEXT 原词）且色调换成 warning，
// 未停用那笔标题不带停用字样、色调 default。
test('每笔修订一项，停用那笔标出', () => {
  const items = businessPartyRevisionTimeline(chain, tagged);
  deepEqual(
    items.map((item) => item.id),
    ['r1', 'r2'],
  );
  equal(items[0].title, 'r1');
  equal(items[1].title, 'r2 · 已停用');
  deepEqual(
    items.map((item) => item.variant),
    ['default', 'warning'],
  );
});

// Covers: 每笔显依据、生效自、名称；停用那笔另显停用时点与停用依据，未停用的笔不带任何停用字样
// （键不在场就不说，不拿空串推）。名称随每一笔在场，两笔并排能读出从哪份换到哪份。
// 三个时刻都经注入的格式化函数，一个也不原样漏出去。
test('描述逐笔列依据、生效自、名称，停用两件只在停用那笔', () => {
  const items = businessPartyRevisionTimeline(chain, tagged);
  equal(
    items[0].description,
    '依据 SYN-REG-BASIS-PARTY-RETIRED-01 · 生效自 [2026-01-02T00:00:00Z] · 名称 合成已停用参与方',
  );
  equal(/停用于/.test(items[0].description), false);
  equal(
    items[1].description,
    '依据 SYN-REG-BASIS-PARTY-RETIRED-01-R2 · 生效自 [2026-01-02T00:00:00Z] · 名称 合成已停用参与方（更名）' +
      ' · 停用于 [2026-06-01T00:00:00Z]（依据 SYN-DEACT-PARTY-01）',
  );
  deepEqual(
    items.map((item) => item.timestamp),
    ['登记于 [2026-09-16T04:27:55Z]', '登记于 [2026-09-16T05:00:00Z]'],
  );
  for (const item of items) {
    // 原始 ISO 串不得裸露在任何一格：裸露即某个时刻没过格式化。
    equal(/(^|[^[])\d{4}-\d{2}-\d{2}T/.test(item.description + item.timestamp), false);
  }
});

// Covers: 空数组不抛也不造项——参与方不在册是读口交回的如实答案（票 12 沿票 03 裁 200 + []），页面另用
// 一句话说，且那句话点名的是「参与方」不是「法人」——两册共用一份判读，注释句却不能共用一个主语。
test('空修订链交回空项，注释句分得开「没有」与「没问到」且主语是参与方', () => {
  deepEqual(businessPartyRevisionTimeline([], tagged), []);
  match(businessPartyRevisionHistoryNote(0), /没有这个参与方的修订/);
  equal(/法人/.test(businessPartyRevisionHistoryNote(0)), false);
  match(businessPartyRevisionHistoryNote(1), /1 笔/);
  match(businessPartyRevisionHistoryNote(2), /2 笔/);
});
