import { afterEach, test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { CatalogueListBody, LocationRecord, ZoneRecord } from '../api';
import { createStagingVerifier, liveStagingVerifier, MAX_STAGING_PAGES, type StagingVerdict } from './pick-staging-check';

const WAREHOUSE = 'SYN-WH-01';

function loc(warehouseCode: string, locationCode: string, zoneCode: string, deactivated = false): LocationRecord {
  return { warehouseCode, locationCode, revision: 1, zoneCode, deactivated, registrant: 'op', registeredAt: '2026-11-25T08:00:00Z' };
}

function zone(warehouseCode: string, zoneCode: string, usage: string, deactivated = false): ZoneRecord {
  return { warehouseCode, zoneCode, revision: 1, name: `库区 ${zoneCode}`, usage, deactivated, registrant: 'op', registeredAt: '2026-11-25T08:00:00Z' };
}

interface Registry {
  locations: LocationRecord[];
  zones: ZoneRecord[];
}

// 一个小租户：待出库、存储两种库区，各种不能用的库位各一个，另一个仓里也有库位
const small: Registry = {
  locations: [
    loc('SYN-WH-01', 'SYN-LOC-OUT-01', 'SYN-ZONE-OUT'),
    loc('SYN-WH-01', 'SYN-LOC-OUT-02', 'SYN-ZONE-OUT'),
    loc('SYN-WH-01', 'SYN-LOC-STORE-01', 'SYN-ZONE-STORE'),
    loc('SYN-WH-01', 'SYN-LOC-OLD', 'SYN-ZONE-OUT', true),
    loc('SYN-WH-01', 'SYN-LOC-LOST', 'SYN-ZONE-GONE'),
    loc('SYN-WH-01', 'SYN-LOC-ZDEAD', 'SYN-ZONE-DEAD'),
    loc('SYN-WH-02', 'SYN-LOC-OTHER-WH', 'SYN-ZONE-OUT'),
  ],
  zones: [
    zone('SYN-WH-01', 'SYN-ZONE-OUT', 'OUTBOUND_STAGING'),
    zone('SYN-WH-01', 'SYN-ZONE-STORE', 'STORAGE'),
    zone('SYN-WH-01', 'SYN-ZONE-DEAD', 'OUTBOUND_STAGING', true),
    zone('SYN-WH-02', 'SYN-ZONE-OUT', 'OUTBOUND_STAGING'),
  ],
};

interface Call {
  family: string | undefined;
  conditions: CatalogueConditions;
  after: string | null;
}

type Answer = ApiResult<CatalogueListBody>;

// 读口声明的筛选维：库位册是 warehouseCode、zoneCode、deactivated，库区册是 warehouseCode、deactivated
// （ports.LocationPages 与 ports.ZonePages）；不认的键、排序维与选择器，整个请求答 400。
const declaredFilters: Record<string, readonly string[]> = {
  location: ['warehouseCode', 'zoneCode', 'deactivated'],
  zone: ['warehouseCode', 'deactivated'],
};

const badRequest: Answer = { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' };

/** 字面包含、不分大小写：读口的 q 在读面上就是这样比的（cataloguepage.Binding 的 ILIKE）。 */
function containsAny(keyword: string | undefined, columns: readonly string[]): boolean {
  if (keyword === undefined || keyword === '') return true;
  return columns.some((column) => column.toLowerCase().includes(keyword.toLowerCase()));
}

function inFilter(conditions: CatalogueConditions, dimension: string, value: string): boolean {
  const allowed = conditions.filters?.[dimension];
  return allowed === undefined || allowed.length === 0 || allowed.includes(value);
}

/**
 * 照 Go 端的读法造的名册读口：筛选、q 的包含匹配、先仓后码的次序、按页大小切页并给游标。`fail` 给了就让指定的一次（第几次从 1 数）
 * 答别的。造出来的读口收到没声明过的维一概答 400，所以走得通本身就说明只用了声明过的维。
 */
function fakeCatalogue(registry: Registry, options: { pageSize?: number; fail?: (call: Call, nth: number) => Answer | undefined } = {}) {
  const pageSize = options.pageSize ?? 200;
  const calls: Call[] = [];
  const read = async (conditions: CatalogueConditions, after: string | null): Promise<Answer> => {
    const family = conditions.selectors?.family;
    const call: Call = { family, conditions, after };
    calls.push(call);
    const failed = options.fail?.(call, calls.length);
    if (failed !== undefined) return failed;
    if (family !== 'location' && family !== 'zone') return badRequest;
    const strangeSelector = Object.keys(conditions.selectors ?? {}).some((key) => key !== 'family');
    const strangeFilter = Object.keys(conditions.filters ?? {}).some((key) => !declaredFilters[family].includes(key));
    if (strangeSelector || strangeFilter || conditions.sort !== undefined) return badRequest;

    const rows: Array<LocationRecord | ZoneRecord> =
      family === 'location'
        ? registry.locations
            .filter(
              (row) =>
                inFilter(conditions, 'warehouseCode', row.warehouseCode) &&
                inFilter(conditions, 'zoneCode', row.zoneCode) &&
                containsAny(conditions.q, [row.warehouseCode, row.locationCode, row.zoneCode]),
            )
            .sort((left, right) => (left.warehouseCode + '\u0000' + left.locationCode < right.warehouseCode + '\u0000' + right.locationCode ? -1 : 1))
        : registry.zones
            .filter((row) => inFilter(conditions, 'warehouseCode', row.warehouseCode) && containsAny(conditions.q, [row.warehouseCode, row.zoneCode, row.name]))
            .sort((left, right) => (left.warehouseCode + '\u0000' + left.zoneCode < right.warehouseCode + '\u0000' + right.zoneCode ? -1 : 1));
    const start = after === null ? 0 : Number(after);
    const page = { size: pageSize, next: start + pageSize < rows.length ? String(start + pageSize) : null, total: rows.length };
    const slice = rows.slice(start, start + pageSize);
    return {
      kind: 'outcome',
      status: 200,
      body:
        family === 'location'
          ? { outcome: 'LOCATIONS_LISTED', locations: slice as LocationRecord[], page }
          : { outcome: 'ZONES_LISTED', zones: slice as ZoneRecord[], page },
    };
  };
  return { read, calls };
}

async function verify(registry: Registry, code: string, options: Parameters<typeof fakeCatalogue>[1] & { maxPages?: number } = {}) {
  const { read, calls } = fakeCatalogue(registry, options);
  const verdict = await createStagingVerifier(read, options.maxPages).verify(WAREHOUSE, code);
  return { verdict, calls };
}

function rejectedReason(verdict: StagingVerdict): string {
  equal(verdict.kind, 'rejected', JSON.stringify(verdict));
  return verdict.kind === 'rejected' ? verdict.reason : '';
}

function unverifiedReason(verdict: StagingVerdict): string {
  equal(verdict.kind, 'unverified', JSON.stringify(verdict));
  return verdict.kind === 'unverified' ? verdict.reason : '';
}

// —— 核得过 ——

test('库位在这个仓、没停用，库区没停用且用途是待出库，才核得过；只发两次读：库位册与它所在的库区册', async () => {
  const { verdict, calls } = await verify(small, 'SYN-LOC-OUT-01');
  deepEqual(verdict, { kind: 'ok' });
  deepEqual(
    calls.map((call) => call.conditions),
    [
      { selectors: { family: 'location' }, filters: { warehouseCode: [WAREHOUSE] }, q: 'SYN-LOC-OUT-01' },
      { selectors: { family: 'zone' }, filters: { warehouseCode: [WAREHOUSE] }, q: 'SYN-ZONE-OUT' },
    ],
  );
  deepEqual(
    calls.map((call) => call.after),
    [null, null],
  );
});

test('不靠名册第一页：库位在第一页之外（含这个码的库位超过一页）也核得过，沿游标往下翻', async () => {
  // 450 个库位的码里都含 S-1，排在它前面；要找的 S-1 在第三页
  const decoys = Array.from({ length: 449 }, (_, index) => loc(WAREHOUSE, `A-S-1-${String(index).padStart(3, '0')}`, 'SYN-ZONE-OUT'));
  const registry: Registry = { locations: [...decoys, loc(WAREHOUSE, 'S-1', 'SYN-ZONE-OUT')], zones: small.zones };
  const { verdict, calls } = await verify(registry, 'S-1');
  deepEqual(verdict, { kind: 'ok' });
  deepEqual(
    calls.map((call) => [call.family, call.after]),
    [
      ['location', null],
      ['location', '200'],
      ['location', '400'],
      ['zone', null],
    ],
  );
});

test('码是仓编码的一部分时整个仓的库位都带出来，仍认得出完全相同的那一个', async () => {
  // 仓编码 SYN-WH-01 含 01：q=01 对仓里每个库位都成立，要找的 01 排在 450 个 00-xxx 后面
  const decoys = Array.from({ length: 450 }, (_, index) => loc(WAREHOUSE, `00-${String(index).padStart(3, '0')}`, 'SYN-ZONE-OUT'));
  const registry: Registry = { locations: [...decoys, loc(WAREHOUSE, '01', 'SYN-ZONE-OUT')], zones: small.zones };
  const { verdict, calls } = await verify(registry, '01');
  deepEqual(verdict, { kind: 'ok' });
  equal(calls.filter((call) => call.family === 'location').length, 3);
});

test('租户的仓、库区、库位再多也核得过：几千个库位、几百个库区，只发定向的读', async () => {
  const locations: LocationRecord[] = [];
  const zones: ZoneRecord[] = [];
  for (let warehouse = 1; warehouse <= 30; warehouse += 1) {
    const code = `BIG-WH-${String(warehouse).padStart(2, '0')}`;
    for (let z = 1; z <= 20; z += 1) {
      const zoneCode = `Z${String(z).padStart(2, '0')}`;
      zones.push(zone(code, zoneCode, z === 20 ? 'OUTBOUND_STAGING' : 'STORAGE'));
      for (let l = 1; l <= 10; l += 1) locations.push(loc(code, `${zoneCode}-L${String(l).padStart(2, '0')}`, zoneCode));
    }
  }
  const registry = { locations, zones };
  const reader = fakeCatalogue(registry);
  const verifier = createStagingVerifier(reader.read);
  deepEqual(await verifier.verify('BIG-WH-17', 'Z20-L07'), { kind: 'ok' });
  const firstConditions = reader.calls.map((call) => call.conditions);
  deepEqual(firstConditions[0], { selectors: { family: 'location' }, filters: { warehouseCode: ['BIG-WH-17'] }, q: 'Z20-L07' });
  // 一共 6000 个库位、600 个库区；一次核只翻到要找的那一页，不把名册整本读回来
  ok(reader.calls.length <= 3, String(reader.calls.length));
});

test('库位码逐字保存、分大小写：读口按不分大小写带出的别的库位不算数', async () => {
  const registry: Registry = {
    locations: [loc(WAREHOUSE, 'syn-loc-out-01', 'SYN-ZONE-STORE', true), loc(WAREHOUSE, 'SYN-LOC-OUT-010', 'SYN-ZONE-STORE')],
    zones: small.zones,
  };
  // 只有大小写不同的、与更长的码：都不是 SYN-LOC-OUT-01
  const missing = await verify(registry, 'SYN-LOC-OUT-01');
  ok(rejectedReason(missing.verdict).includes('登记册里没有库位 SYN-LOC-OUT-01'));
  // 完全相同的那一个在，就认它，不被前面带出来的认错
  const both = await verify({ locations: [...registry.locations, loc(WAREHOUSE, 'SYN-LOC-OUT-01', 'SYN-ZONE-OUT')], zones: small.zones }, 'SYN-LOC-OUT-01');
  deepEqual(both.verdict, { kind: 'ok' });
});

test('别的仓里有同码的库位，不影响设备绑的这个仓里的那个', async () => {
  const registry: Registry = {
    locations: [loc('SYN-WH-00', 'SAME-01', 'SYN-ZONE-STORE', true), loc(WAREHOUSE, 'SAME-01', 'SYN-ZONE-OUT')],
    zones: small.zones,
  };
  const { verdict, calls } = await verify(registry, 'SAME-01');
  deepEqual(verdict, { kind: 'ok' });
  // 这个仓里认出来了，不必再问别的仓
  equal(calls.filter((call) => call.family === 'location').length, 1);
});

// —— 核不过：五种，各说各的，次序与后端一致 ——

test('库位属于别的仓：说明是哪个仓、设备绑的是哪个仓；这时才去问别的仓', async () => {
  const { verdict, calls } = await verify(small, 'SYN-LOC-OTHER-WH');
  const reason = rejectedReason(verdict);
  ok(reason.includes('SYN-WH-02') && reason.includes('SYN-WH-01') && reason.includes('SYN-LOC-OTHER-WH'), reason);
  deepEqual(
    calls.map((call) => call.conditions),
    [
      { selectors: { family: 'location' }, filters: { warehouseCode: [WAREHOUSE] }, q: 'SYN-LOC-OTHER-WH' },
      { selectors: { family: 'location' }, q: 'SYN-LOC-OTHER-WH' },
    ],
  );
});

test('库位登记在别的仓而别的仓的行在第一页之外，照样认出它在哪个仓', async () => {
  const decoys = Array.from({ length: 450 }, (_, index) => loc('SYN-WH-02', `A-ELSE-${String(index).padStart(3, '0')}`, 'SYN-ZONE-OUT'));
  const registry: Registry = { locations: [...decoys, loc('SYN-WH-03', 'ELSE', 'SYN-ZONE-OUT')], zones: small.zones };
  const { verdict } = await verify(registry, 'ELSE');
  ok(rejectedReason(verdict).includes('SYN-WH-03'));
});

test('登记册里没有这个库位：这个仓里问完没有、别的仓里也问完没有，才这样说', async () => {
  const { verdict, calls } = await verify(small, 'NOPE-01');
  const reason = rejectedReason(verdict);
  ok(reason.includes('NOPE-01') && reason.includes('登记册里没有'), reason);
  deepEqual(
    calls.map((call) => call.family),
    ['location', 'location'],
  );
});

test('登记册里没有：包含匹配带出一串别的库位时，翻完才这样说', async () => {
  const decoys = Array.from({ length: 450 }, (_, index) => loc(WAREHOUSE, `A-S-1-${String(index).padStart(3, '0')}`, 'SYN-ZONE-OUT'));
  const { verdict, calls } = await verify({ locations: decoys, zones: small.zones }, 'S-1');
  ok(rejectedReason(verdict).includes('登记册里没有库位 S-1'));
  // 这个仓里翻了三页，别的仓（一样是这 450 个）再翻三页，都翻完了
  equal(calls.length, 6);
});

test('库位已停用：这样说，不再去读库区', async () => {
  const { verdict, calls } = await verify(small, 'SYN-LOC-OLD');
  const reason = rejectedReason(verdict);
  ok(reason.includes('SYN-LOC-OLD') && reason.includes('已停用'), reason);
  equal(calls.filter((call) => call.family === 'zone').length, 0);
});

test('找不到库位所在的库区、库区已停用、库区用途不是待出库，各说各的', async () => {
  const lost = rejectedReason((await verify(small, 'SYN-LOC-LOST')).verdict);
  ok(lost.includes('找不到') && lost.includes('SYN-ZONE-GONE'), lost);

  const dead = rejectedReason((await verify(small, 'SYN-LOC-ZDEAD')).verdict);
  ok(dead.includes('库区') && dead.includes('SYN-ZONE-DEAD') && dead.includes('已停用'), dead);

  const store = rejectedReason((await verify(small, 'SYN-LOC-STORE-01')).verdict);
  ok(store.includes('存储') && store.includes('待出库') && store.includes('SYN-ZONE-STORE'), store);
});

test('库区在册的找法也不靠第一页：库区多、而含这个库区码的行超过一页', async () => {
  // 库区码 ZX 的库区排在 450 个名称里带 ZX 的库区后面
  const decoys = Array.from({ length: 450 }, (_, index) => zone(WAREHOUSE, `A-ZX-${String(index).padStart(3, '0')}`, 'STORAGE'));
  const registry: Registry = { locations: [loc(WAREHOUSE, 'LOC-ZX', 'ZX')], zones: [...decoys, zone(WAREHOUSE, 'ZX', 'OUTBOUND_STAGING')] };
  const { verdict, calls } = await verify(registry, 'LOC-ZX');
  deepEqual(verdict, { kind: 'ok' });
  equal(calls.filter((call) => call.family === 'zone').length, 3);
});

test('次序与后端核的次序一致：先库位（在这个仓、没停用），再库区（在、没停用、用途）', async () => {
  // 库位停用与库区停用同时成立：说库位
  const both = await verify(
    { locations: [loc(WAREHOUSE, 'BOTH', 'SYN-ZONE-DEAD', true)], zones: small.zones },
    'BOTH',
  );
  ok(rejectedReason(both.verdict).includes('库位 BOTH 已停用'));
  // 库区既停用、用途又不对：先说停用
  const dead = await verify(
    { locations: [loc(WAREHOUSE, 'DEADSTORE', 'ZDS')], zones: [zone(WAREHOUSE, 'ZDS', 'STORAGE', true)] },
    'DEADSTORE',
  );
  const reason = rejectedReason(dead.verdict);
  ok(reason.includes('已停用') && !reason.includes('用途'), reason);
});

// —— 没核成：读口没问成，不能说成「登记册里没有」 ——

const failures: Array<[string, Answer, string]> = [
  ['网络没通', { kind: 'transport', message: '断了' }, '网络没通（断了）'],
  ['系统没答出来', { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' }, '系统暂时没答出来'],
  ['接入被拒：没有授予', { kind: 'notGranted' }, '没有授予'],
  ['接入被拒：需要登录', { kind: 'loginRequired' }, '需要登录'],
  ['接入被拒：接入渠道未配置', { kind: 'unconfigured' }, '接入渠道未配置'],
];

test('读口没问成（没通、没答、接入被拒）：说没核成和原因，不说成登记册里没有', async () => {
  for (const [name, failure, expected] of failures) {
    // 三处读都可能没问成：库位册、（这个仓里没有时）别的仓的库位册、库区册
    const scenarios: Array<[string, string, (call: Call, nth: number) => Answer | undefined]> = [
      ['库位册', 'SYN-LOC-OUT-01', (call) => (call.family === 'location' ? failure : undefined)],
      ['别的仓的库位册', 'NOPE-01', (_call, nth) => (nth === 2 ? failure : undefined)],
      ['库区册', 'SYN-LOC-OUT-01', (call) => (call.family === 'zone' ? failure : undefined)],
    ];
    for (const [where, code, fail] of scenarios) {
      const { verdict } = await verify(small, code, { fail });
      const reason = unverifiedReason(verdict);
      ok(reason.includes(expected), `${name}/${where}: ${reason}`);
      ok(!reason.includes('登记册里没有'), `${name}/${where}: ${reason}`);
    }
  }
});

test('读口没问成时说是读哪一册没成', async () => {
  const location = await verify(small, 'SYN-LOC-OUT-01', { fail: (call) => (call.family === 'location' ? failures[0][1] : undefined) });
  ok(unverifiedReason(location.verdict).includes('读库位名册没成'));
  const zoneRead = await verify(small, 'SYN-LOC-OUT-01', { fail: (call) => (call.family === 'zone' ? failures[0][1] : undefined) });
  ok(unverifiedReason(zoneRead.verdict).includes('读库区名册没成'));
});

test('翻页翻到一半读口没问成：同样是没核成，不是没有', async () => {
  const decoys = Array.from({ length: 450 }, (_, index) => loc(WAREHOUSE, `A-S-1-${String(index).padStart(3, '0')}`, 'SYN-ZONE-OUT'));
  const { verdict, calls } = await verify({ locations: decoys, zones: small.zones }, 'S-1', {
    fail: (_call, nth) => (nth === 2 ? failures[0][1] : undefined),
  });
  ok(unverifiedReason(verdict).includes('网络没通'));
  equal(calls.length, 2);
});

test('读口的答复不成名册的样子（没有行、没有页）：说没核成，不当成没有', async () => {
  const noRows: Answer = { kind: 'outcome', status: 200, body: { outcome: 'LOCATIONS_LISTED', page: { size: 200, next: null, total: 0 } } };
  const first = await verify(small, 'SYN-LOC-OUT-01', { fail: () => noRows });
  ok(unverifiedReason(first.verdict).includes('不是名册的样子'));
  const noPage: Answer = { kind: 'outcome', status: 200, body: { outcome: 'LOCATIONS_LISTED', locations: [] } as unknown as CatalogueListBody };
  const second = await verify(small, 'SYN-LOC-OUT-01', { fail: () => noPage });
  ok(unverifiedReason(second.verdict).includes('不是名册的样子'));
});

test('含这个码的行多得翻不完：到了页数上限还没认出来，说没核成，不说没有', async () => {
  const decoys = Array.from({ length: MAX_STAGING_PAGES * 200 + 1 }, (_, index) => loc(WAREHOUSE, `A-S-1-${String(index).padStart(5, '0')}`, 'SYN-ZONE-OUT'));
  const { verdict, calls } = await verify({ locations: decoys, zones: small.zones }, 'S-1');
  const reason = unverifiedReason(verdict);
  ok(reason.includes(`翻了 ${MAX_STAGING_PAGES} 页还没认出`), reason);
  ok(!reason.includes('登记册里没有'), reason);
  equal(calls.length, MAX_STAGING_PAGES);
});

test('页数上限可以指定：翻到上限的那一页里认出来就核得过', async () => {
  const decoys = Array.from({ length: 5 }, (_, index) => loc(WAREHOUSE, `A-S-1-${index}`, 'SYN-ZONE-OUT'));
  const registry: Registry = { locations: [...decoys, loc(WAREHOUSE, 'S-1', 'SYN-ZONE-OUT')], zones: small.zones };
  // 页大小 2：S-1 在第三页
  const enough = await verify(registry, 'S-1', { pageSize: 2, maxPages: 3 });
  deepEqual(enough.verdict, { kind: 'ok' });
  const notEnough = await verify(registry, 'S-1', { pageSize: 2, maxPages: 2 });
  ok(unverifiedReason(notEnough.verdict).includes('翻了 2 页'));
});

test('码比读口的检索词上限（100 个字符）还长：不去问，说没核成；刚好 100 个字符照常问', async () => {
  const long = 'L'.repeat(101);
  const tooLong = await verify(small, long);
  const reason = unverifiedReason(tooLong.verdict);
  ok(reason.includes('101') && reason.includes('100'), reason);
  equal(tooLong.calls.length, 0);

  const edge = await verify(small, 'L'.repeat(100));
  ok(rejectedReason(edge.verdict).includes('登记册里没有'));
  equal(edge.calls.length > 0, true);
});

test('码里的字符按字符数、不按 UTF-16 码元数：一百个补充平面字符不算超长', async () => {
  const astral = '𠮷'.repeat(100);
  const { calls } = await verify(small, astral);
  equal(calls.length > 0, true);
});

// —— 接真读口：请求的路径与键 ——

const realFetch = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = realFetch;
});

function answerJson(routes: (url: string) => unknown): string[] {
  const urls: string[] = [];
  globalThis.fetch = (async (input: RequestInfo | URL) => {
    const url = String(input);
    urls.push(url);
    return new Response(JSON.stringify(routes(url)), { status: 200, headers: { 'Content-Type': 'application/json' } });
  }) as typeof fetch;
  return urls;
}

test('接真读口：库位册按仓筛、q 填码，库区册按仓筛、q 填库位所在的库区；除此之外没有别的键', async () => {
  const urls = answerJson((url) =>
    url.startsWith('/warehouse-fulfillment-locations')
      ? { outcome: 'LOCATIONS_LISTED', locations: [loc(WAREHOUSE, 'SYN-LOC-OUT-01', 'SYN-ZONE-OUT')], page: { size: 200, next: null, total: 1 } }
      : { outcome: 'ZONES_LISTED', zones: [zone(WAREHOUSE, 'SYN-ZONE-OUT', 'OUTBOUND_STAGING')], page: { size: 200, next: null, total: 1 } },
  );
  deepEqual(await liveStagingVerifier().verify(WAREHOUSE, 'SYN-LOC-OUT-01'), { kind: 'ok' });
  deepEqual(urls, [
    '/warehouse-fulfillment-locations?warehouseCode=SYN-WH-01&q=SYN-LOC-OUT-01',
    '/warehouse-fulfillment-zones?warehouseCode=SYN-WH-01&q=SYN-ZONE-OUT',
  ]);
});

test('接真读口：翻页带上游标，别的仓那一问不带仓筛', async () => {
  const urls = answerJson((url) =>
    url.includes('after=CUR-2')
      ? { outcome: 'LOCATIONS_LISTED', locations: [], page: { size: 200, next: null, total: 201 } }
      : { outcome: 'LOCATIONS_LISTED', locations: [loc(WAREHOUSE, 'X-S-1', 'SYN-ZONE-OUT')], page: { size: 200, next: 'CUR-2', total: 201 } },
  );
  const verdict = await liveStagingVerifier().verify(WAREHOUSE, 'S-1');
  ok(rejectedReason(verdict).includes('登记册里没有'));
  deepEqual(urls, [
    '/warehouse-fulfillment-locations?warehouseCode=SYN-WH-01&q=S-1',
    '/warehouse-fulfillment-locations?warehouseCode=SYN-WH-01&q=S-1&after=CUR-2',
    '/warehouse-fulfillment-locations?q=S-1',
    '/warehouse-fulfillment-locations?q=S-1&after=CUR-2',
  ]);
});

test('接真读口：码里的特殊字符原样交给读口，由读口按字面包含处理', async () => {
  const urls = answerJson(() => ({ outcome: 'LOCATIONS_LISTED', locations: [], page: { size: 200, next: null, total: 0 } }));
  await liveStagingVerifier().verify(WAREHOUSE, '50%_A\\B 1');
  equal(urls[0], '/warehouse-fulfillment-locations?warehouseCode=SYN-WH-01&q=50%25_A%5CB+1');
});

test('接真读口：网络断了是没核成', async () => {
  globalThis.fetch = (async () => {
    throw new Error('Failed to fetch');
  }) as typeof fetch;
  const verdict = await liveStagingVerifier().verify(WAREHOUSE, 'SYN-LOC-OUT-01');
  const reason = unverifiedReason(verdict);
  ok(reason.includes('Failed to fetch'), reason);
});
