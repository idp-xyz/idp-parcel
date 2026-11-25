import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { CatalogueListBody, PackagingRecord } from '../api';
import { createPackagingFinder, loadPackagingList, materialOf, MAX_PACKAGING_LIST_PAGES, MAX_PACKAGING_PAGES } from './pack-packaging';

function record(code: string, name: string, over: Partial<PackagingRecord> = {}): PackagingRecord {
  return {
    packagingMaterialCode: code,
    revision: 1,
    name,
    dimensions: { length: '30', width: '20', height: '15', unit: 'CM' },
    weight: { value: '0.2', unit: 'KG' },
    deactivated: false,
    registrant: 'op',
    registeredAt: '2026-11-25T08:00:00Z',
    ...over,
  };
}

type Answer = ApiResult<CatalogueListBody>;

interface Call {
  conditions: CatalogueConditions;
  after: string | null;
}

// 包材册读口声明的筛选维只有 deactivated；选择器只认 family；不认的键整个请求答 400。
const badRequest: Answer = { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' };

const contains = (keyword: string | undefined, columns: readonly string[]): boolean =>
  keyword === undefined || keyword === '' || columns.some((column) => column.toLowerCase().includes(keyword.toLowerCase()));

function fakeRegister(rows: PackagingRecord[], options: { pageSize?: number; fail?: (nth: number) => Answer | undefined } = {}) {
  const pageSize = options.pageSize ?? 200;
  const calls: Call[] = [];
  const read = async (conditions: CatalogueConditions, after: string | null): Promise<Answer> => {
    calls.push({ conditions, after });
    const failed = options.fail?.(calls.length);
    if (failed !== undefined) return failed;
    const strangeSelector = Object.keys(conditions.selectors ?? {}).some((key) => key !== 'family');
    const strangeFilter = Object.keys(conditions.filters ?? {}).some((key) => key !== 'deactivated');
    if (conditions.selectors?.family !== 'packaging' || strangeSelector || strangeFilter || conditions.sort !== undefined) return badRequest;
    const wanted = conditions.filters?.deactivated;
    const matching = rows
      .filter((row) => wanted === undefined || wanted.length === 0 || wanted.includes(String(row.deactivated)))
      .filter((row) => contains(conditions.q, [row.packagingMaterialCode, row.name]))
      .sort((left, right) => (left.packagingMaterialCode < right.packagingMaterialCode ? -1 : 1));
    const start = after === null ? 0 : Number(after);
    const page = { size: pageSize, next: start + pageSize < matching.length ? String(start + pageSize) : null, total: matching.length };
    return { kind: 'outcome', status: 200, body: { outcome: 'PACKAGING_MATERIALS_LISTED', packagingMaterials: matching.slice(start, start + pageSize), page } };
  };
  return { read, calls };
}

const register = [
  record('BOX-S', '小号纸箱'),
  record('BOX-L', '大号纸箱', { dimensions: { length: '60', width: '40', height: '30', unit: 'CM' } }),
  record('BAG-1', '气泡袋', { dimensions: { length: '', width: '', height: '', unit: '' } }),
  record('BOX-OLD', '旧纸箱', { deactivated: true }),
];

async function find(rows: PackagingRecord[], code: string, options: Parameters<typeof fakeRegister>[1] & { maxPages?: number } = {}) {
  const { read, calls } = fakeRegister(rows, options);
  const lookup = await createPackagingFinder(read, options.maxPages).find(code);
  return { lookup, calls };
}

test('包材记录收成工位要的几格：编码、名称、外廓与单位、包材自重的单位、是否停用；没登记外廓就是空串', () => {
  deepEqual(materialOf(register[0]), {
    code: 'BOX-S',
    name: '小号纸箱',
    length: '30',
    width: '20',
    height: '15',
    dimensionUnit: 'CM',
    weightUnit: 'KG',
    deactivated: false,
  });
  const bare = materialOf({ ...register[2], dimensions: undefined, weight: undefined } as unknown as PackagingRecord);
  deepEqual([bare.length, bare.width, bare.height, bare.dimensionUnit, bare.weightUnit], ['', '', '', '', '']);
});

test('扫包材码：认出编码完全相同的那一行；只发一次读，不加筛选（停用的也认得出来）', async () => {
  const { lookup, calls } = await find(register, 'BOX-L');
  equal(lookup.kind, 'found');
  ok(lookup.kind === 'found' && lookup.material.code === 'BOX-L' && lookup.material.length === '60');
  deepEqual(
    calls.map((call) => call.conditions),
    [{ selectors: { family: 'packaging' }, q: 'BOX-L' }],
  );
});

test('扫到已停用的包材：认得出来，并带着停用的标记，让屏上说「已停用」而不是「没有」', async () => {
  const { lookup } = await find(register, 'BOX-OLD');
  ok(lookup.kind === 'found' && lookup.material.deactivated);
});

test('读口按字面包含，要认的是编码完全相同的行：BOX 不是 BOX-S，大小写不同不是同一个编码', async () => {
  equal((await find(register, 'BOX')).lookup.kind, 'absent');
  equal((await find(register, 'box-s')).lookup.kind, 'absent');
});

test('含这个码的行比一页多，目标在后面的页上也认得出', async () => {
  // 按编码排，目标排在最后
  const many = [...Array.from({ length: 5 }, (_, index) => record(`A${index}BOX-9`, '纸箱')), record('BOX-9', '纸箱')];
  const { lookup, calls } = await find(many, 'BOX-9', { pageSize: 2 });
  ok(lookup.kind === 'found' && lookup.material.code === 'BOX-9');
  deepEqual(
    calls.map((call) => call.after),
    [null, '2', '4'],
  );
});

test('没有这个包材：问完了才说没有', async () => {
  const { lookup, calls } = await find(register, 'NOPE-1');
  deepEqual(lookup, { kind: 'absent' });
  equal(calls.length, 1);
});

test('读不成不说成没有，原因带到话里', async () => {
  const down = (await find(register, 'NOPE-1', { fail: () => ({ kind: 'transport', message: '断了' }) })).lookup;
  equal(down.kind, 'unverified');
  ok(down.kind === 'unverified' && down.reason.includes('读包材名册没成') && down.reason.includes('网络没通'));

  const refused = (await find(register, 'NOPE-1', { fail: () => ({ kind: 'notGranted' }) })).lookup;
  equal(refused.kind, 'unverified');

  const shape = (await find(register, 'NOPE-1', { fail: () => ({ kind: 'outcome', status: 200, body: { outcome: 'X' } as unknown as CatalogueListBody }) })).lookup;
  ok(shape.kind === 'unverified' && shape.reason.includes('不是名册的样子'));
});

test('翻到页数上限还没认出就说没问完，不说没有', async () => {
  const many = Array.from({ length: 12 }, (_, index) => record(`BOX-7${String(index).padStart(2, '0')}`, '纸箱'));
  const { lookup, calls } = await find(many, 'BOX-7', { pageSize: 2, maxPages: 3 });
  ok(lookup.kind === 'unverified' && lookup.reason.includes('翻了 3 页'));
  equal(calls.length, 3);
  equal(MAX_PACKAGING_PAGES, 10);
});

test('码比读口收的检索词（100 个字符）还长，一次读都不发', async () => {
  const { lookup, calls } = await find(register, 'X'.repeat(101));
  ok(lookup.kind === 'unverified' && lookup.reason.includes('101'));
  equal(calls.length, 0);
});

// —— 点选用的列表 ——

test('点选列表：只列在用的包材，读口用 deactivated=false 筛；读口不筛时停用的也在这里滤掉', async () => {
  const { read, calls } = fakeRegister(register);
  const answer = await loadPackagingList(read);
  equal(answer.kind, 'list');
  if (answer.kind !== 'list') return;
  deepEqual(
    answer.list.items.map((item) => item.code),
    ['BAG-1', 'BOX-L', 'BOX-S'],
  );
  equal(answer.list.truncated, false);
  deepEqual(
    calls.map((call) => call.conditions),
    [{ selectors: { family: 'packaging' }, filters: { deactivated: ['false'] } }],
  );

  const leaky = async (): Promise<Answer> => ({
    kind: 'outcome',
    status: 200,
    body: { outcome: 'PACKAGING_MATERIALS_LISTED', packagingMaterials: register, page: { size: 200, next: null, total: 4 } },
  });
  const unfiltered = await loadPackagingList(leaky);
  ok(unfiltered.kind === 'list' && !unfiltered.list.items.some((item) => item.deactivated));
});

test('点选列表：沿游标翻页；翻到上限读口还有下一页，如实标出只列了前面一部分', async () => {
  const many = Array.from({ length: 9 }, (_, index) => record(`BOX-${index}`, '纸箱'));
  const full = await loadPackagingList(fakeRegister(many, { pageSize: 4 }).read);
  ok(full.kind === 'list' && full.list.items.length === 9 && !full.list.truncated);

  const cut = await loadPackagingList(fakeRegister(many, { pageSize: 2 }).read, 2);
  ok(cut.kind === 'list' && cut.list.items.length === 4 && cut.list.truncated);
  equal(MAX_PACKAGING_LIST_PAGES, 5);
});

test('点选列表：读不成、答复不成名册的样子，都答没读成并带原因', async () => {
  const down = await loadPackagingList(fakeRegister(register, { fail: () => ({ kind: 'transport', message: '断了' }) }).read);
  ok(down.kind === 'failed' && down.reason.includes('网络没通'));
  const shape = await loadPackagingList(async () => ({ kind: 'outcome', status: 200, body: { outcome: 'X' } as unknown as CatalogueListBody }));
  ok(shape.kind === 'failed' && shape.reason.includes('不是名册的样子'));
});
