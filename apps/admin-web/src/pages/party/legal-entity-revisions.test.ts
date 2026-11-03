import { test } from 'node:test';
import { deepEqual, equal, match } from 'node:assert/strict';
import type { LegalEntityRevisionRecord } from './api';
import {
  legalEntityRevisionTimeline,
  revisionHistoryNote,
} from './legal-entity-revisions';

// 本文件钉对象页「登记记录」签的判读（票 admin-web-group-legal-entities/03 第 5 条）：每笔显版本号、来源单据、
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
    // 身份层落地之前的旧形状；登了身份层的那几笔由各用例自己覆盖。
    identityLayerRegistered: false,
    ...over,
  };
}

const absentIdentity = ' · 注册地与注册号：这一版登记时还没有这两格';

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

// Covers: 一笔一项、序不动；标题短写「第 N 版」——轴名由「登记记录」签承担，不再写「身份 rN」（spec 公司主体 UX
// 「统一用语」）；停用那一笔标出`已停用`（CONTEXT 原词）且色调换成 warning，其余两笔标题
// 不带停用字样、色调 default。条目 id 不带名：它只作键。
test('每笔修订一项，停用那笔标出', () => {
  const items = legalEntityRevisionTimeline(chain, tagged);
  deepEqual(
    items.map((item) => item.id),
    ['r1', 'r2', 'r3'],
  );
  equal(items[0].title, '第 1 版');
  equal(items[1].title, '第 2 版');
  equal(items[2].title, '第 3 版 · 已停用');
  deepEqual(
    items.map((item) => item.variant),
    ['default', 'default', 'warning'],
  );
});

// Covers: 每笔显来源单据、生效自、往来单位档案；停用那笔另显停用时点与停用来源单据，未停用的笔不带任何停用字样
// （键不在场就不说，不拿空串推）。三个时刻都经注入的格式化函数，一个也不原样漏出去。
test('描述逐笔列来源单据、生效自、往来单位档案，停用两件只在停用那笔', () => {
  const items = legalEntityRevisionTimeline(chain, tagged);
  equal(
    items[0].description,
    '来源单据 SYN-REG-BASIS-LE-01 · 生效自 [2026-01-02T00:00:00Z] · 往来单位档案 SYN-PARTY-OPERATOR-01' + absentIdentity,
  );
  equal(/停用/.test(items[0].description), false);
  equal(/停用/.test(items[1].description), false);
  equal(
    items[2].description,
    '来源单据 SYN-REG-BASIS-LE-01-R2 · 生效自 [2026-01-02T00:00:00Z] · 往来单位档案 SYN-PARTY-OPERATOR-02' +
      absentIdentity +
      ' · 停用于 [2026-06-01T00:00:00Z]（来源单据 SYN-DEACT-01）',
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

// Covers: 登了身份层的笔显国家与号（译名表没读回时国家原样显码）；没登的那笔如实写「这一版登记时还没有这两格」而不是
// 空着；更正来源单据只在更正那一笔在场（票 legal-entity-profile/04「历史修订两格为空时如实写」）。
test('身份两格逐笔示出，旧笔如实写还没有这两格，更正来源单据只在更正那笔', () => {
  const identity = {
    identityLayerRegistered: true,
    registrationCountry: 'CN',
    lifetimeRegistrationNumbers: [{ typeCode: 'USCC', number: '91000000000000001X' }],
  };
  const items = legalEntityRevisionTimeline(
    [
      revision({}),
      revision({ revision: 2, ...identity }),
      revision({
        revision: 3,
        ...identity,
        lifetimeRegistrationNumbers: [{ typeCode: 'USCC', number: '91000000000000002Y' }],
        identityCorrectionBasis: 'SYN-CORR-01',
      }),
    ],
    tagged,
  );
  equal(items[0].description.endsWith(absentIdentity), true);
  equal(
    items[1].description,
    '来源单据 SYN-REG-BASIS-LE-01 · 生效自 [2026-01-02T00:00:00Z] · 往来单位档案 SYN-PARTY-OPERATOR-01' +
      ' · 注册地 CN · 注册号 USCC 91000000000000001X',
  );
  equal(/更正/.test(items[1].description), false);
  equal(
    items[2].description.endsWith(' · 注册号 USCC 91000000000000002Y · 更正来源单据 SYN-CORR-01'),
    true,
  );
});

// Covers: 空数组不抛也不造项——法人不在册是读口交回的如实答案（票 03 裁 200 + []），页面另用一句话说。
test('空修订链交回空项，注释句分得开「没有」与「没问到」', () => {
  deepEqual(legalEntityRevisionTimeline([], tagged), []);
  match(revisionHistoryNote(0), /没有.*登记记录/);
  match(revisionHistoryNote(1), /1 笔登记记录，从早到晚/);
  match(revisionHistoryNote(3), /3 笔登记记录/);
  // 「修订号」是技术详情里的词，计数句不再点它。
  equal(/修订/.test(revisionHistoryNote(3)), false);
});

// Covers: 显到哪一级由时间线定、时区由注入的格式化定（spec「统一用语」：主数据页生效 / 停用到日期，版本历史的登记时刻到
// 分钟）——三个时刻各带 precision 交给格式化函数，页面传 moment.ts 的 formatInstant 就直接生效，不必各页自己记。
test('登记时刻到分钟、生效与停用到日期，经注入的格式化函数', () => {
  const leveled = (iso: string, options?: { precision?: string }) => `${options?.precision ?? 'second'}:${iso}`;
  const items = legalEntityRevisionTimeline(chain, leveled);
  equal(items[0].timestamp, '登记于 minute:2026-09-16T04:27:55Z');
  equal(/生效自 day:2026-01-02T00:00:00Z/.test(items[0].description), true);
  equal(/停用于 day:2026-06-01T00:00:00Z/.test(items[2].description), true);
  equal(/second:/.test(items.map((item) => item.description + item.timestamp).join('')), false);
});

// Covers: 交了注册号类型目录的查法 → 注册号的类型码译成名；不交照码显（上一条用例）；译不出的码原样。
test('注册号类型码经交进来的查法译名', () => {
  const items = legalEntityRevisionTimeline(
    [
      revision({
        identityLayerRegistered: true,
        registrationCountry: 'CN',
        lifetimeRegistrationNumbers: [
          { typeCode: 'USCC', number: '91000000000000001X' },
          { typeCode: 'SYN-OLD', number: '7' },
        ],
      }),
    ],
    tagged,
    (country, typeCode) => (country === 'CN' && typeCode === 'USCC' ? '统一社会信用代码' : undefined),
  );
  equal(items[0].description.endsWith(' · 注册地 CN · 注册号 统一社会信用代码 91000000000000001X；SYN-OLD 7'), true);
});
