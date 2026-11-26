import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { ApiResult } from '../../catalogue-api';
import type { CatalogueConditions } from '../../catalogue-query';
import type { WaveListBody, WaveOneBody, WaveRecord, WaveTaskRecord } from '../api';
import { allWavesChoice, cellsOfWaves, compareLocationCodes, loadPickQueue, MAX_WAVE_PAGES, waveChoices, waveOfChoice } from './pick-queue';

function task(taskId: string, locationCode: string, overrides: Partial<WaveTaskRecord> = {}): WaveTaskRecord {
  return {
    taskId,
    requestId: `REQ-${taskId}`,
    locationCode,
    goodsCode: 'SYN-SKU-EARBUD-01',
    unit: '件',
    stockState: 'GOOD',
    quantity: 5,
    picked: 0,
    status: 'OPEN',
    ...overrides,
  };
}

function wave(waveId: string, tasks: WaveTaskRecord[], warehouseCode = 'SYN-WH-01'): WaveRecord {
  return { waveId, warehouseCode, operator: 'op', prefetch: false, status: 'RELEASED', releasedAt: '2026-11-25T08:00:00Z', requests: [], tasks };
}

const scope = { warehouse: 'SYN-WH-01', wave: null };

test('只收待执行的任务：拣完的、短拣的、取消的、迟到的都不在队列里', () => {
  const cells = cellsOfWaves(
    [
      wave('W-1', [
        task('T-1', 'A-01-01'),
        task('T-2', 'A-01-02', { status: 'DONE', picked: 5 }),
        task('T-3', 'A-01-03', { status: 'SHORT', picked: 2 }),
        task('T-4', 'A-01-04', { status: 'CANCELLED' }),
        task('T-5', 'A-01-05', { status: 'LATE' }),
      ]),
    ],
    scope,
  );
  deepEqual(
    cells.map((cell) => cell.taskId),
    ['T-1'],
  );
});

test('只收本仓的波次', () => {
  const cells = cellsOfWaves([wave('W-1', [task('T-1', 'A-01-01')]), wave('W-2', [task('T-2', 'A-01-02')], 'SYN-WH-02')], scope);
  deepEqual(
    cells.map((cell) => cell.taskId),
    ['T-1'],
  );
});

test('每一格带着做这一格要知道的：任务、波次、出库单、库位、商品、单位、库存状态、应拣数量', () => {
  const [cell] = cellsOfWaves([wave('W-9', [task('T-1', 'A-01-01', { quantity: 7, unit: '箱', stockState: 'GOOD' })])], scope);
  deepEqual(cell, {
    taskId: 'T-1',
    waveId: 'W-9',
    requestId: 'REQ-T-1',
    locationCode: 'A-01-01',
    goodsCode: 'SYN-SKU-EARBUD-01',
    unit: '箱',
    stockState: 'GOOD',
    quantity: 7,
  });
});

test('按库位码排序', () => {
  const cells = cellsOfWaves(
    [wave('W-1', [task('T-1', 'B-01-02'), task('T-2', 'A-02-01'), task('T-3', 'A-01-05'), task('T-4', 'A-01-03')])],
    scope,
  );
  deepEqual(
    cells.map((cell) => cell.locationCode),
    ['A-01-03', 'A-01-05', 'A-02-01', 'B-01-02'],
  );
});

test('库位码里的数字按数值比，不按字符：A-1-2 排在 A-1-10 前面', () => {
  const cells = cellsOfWaves([wave('W-1', [task('T-1', 'A-1-10'), task('T-2', 'A-1-2'), task('T-3', 'A-1-9')])], scope);
  deepEqual(
    cells.map((cell) => cell.locationCode),
    ['A-1-2', 'A-1-9', 'A-1-10'],
  );
});

test('同一个库位上的几格按任务号定先后，次序稳定', () => {
  const cells = cellsOfWaves([wave('W-1', [task('T-3', 'A-01-01'), task('T-1', 'A-01-01'), task('T-2', 'A-01-01')])], scope);
  deepEqual(
    cells.map((cell) => cell.taskId),
    ['T-1', 'T-2', 'T-3'],
  );
});

test('库位码比较：相等为 0，前后有序', () => {
  equal(compareLocationCodes('A-01-01', 'A-01-01'), 0);
  ok(compareLocationCodes('A-01-01', 'A-01-02') < 0);
  ok(compareLocationCodes('A-01-10', 'A-01-02') > 0);
});

test('指名了波次就只做那个波次，别的波次里的待拣格不进队列', () => {
  const waves = [wave('W-1', [task('T-1', 'A-01-01')]), wave('W-2', [task('T-2', 'A-01-02')])];
  deepEqual(
    cellsOfWaves(waves, { warehouse: 'SYN-WH-01', wave: 'W-2' }).map((cell) => cell.taskId),
    ['T-2'],
  );
  deepEqual(
    cellsOfWaves(waves, { warehouse: 'SYN-WH-01', wave: null }).map((cell) => cell.taskId),
    ['T-1', 'T-2'],
  );
  deepEqual(cellsOfWaves(waves, { warehouse: 'SYN-WH-01', wave: 'W-404' }), []);
});

test('数量不是正整数的任务不收：后端不收小于 1 的实拣数量', () => {
  const cells = cellsOfWaves([wave('W-1', [task('T-0', 'A-01-01', { quantity: 0 }), task('T-1', 'A-01-02', { quantity: 2.5 }), task('T-2', 'A-01-03')])], scope);
  deepEqual(
    cells.map((cell) => cell.taskId),
    ['T-2'],
  );
});

test('同一个任务出现在两页里只算一格', () => {
  const cells = cellsOfWaves([wave('W-1', [task('T-1', 'A-01-01')]), wave('W-1', [task('T-1', 'A-01-01')])], scope);
  equal(cells.length, 1);
});

// —— 读队列：翻页、上限、指名波次 ——

type Page = { waves: WaveRecord[]; next: string | null };

// 没指名波次的用例不该去按号读；真去读了就让用例败在这里。
const neverOne = async (waveId: string): Promise<ApiResult<WaveOneBody>> => {
  throw new Error(`不该按号读 ${waveId}`);
};

function reader(known: WaveRecord[]): { one: (waveId: string) => Promise<ApiResult<WaveOneBody>>; asked: string[] } {
  const asked: string[] = [];
  return {
    asked,
    one: async (waveId) => {
      asked.push(waveId);
      const found = known.find((candidate) => candidate.waveId === waveId);
      return { kind: 'outcome', status: 200, body: found ? { outcome: 'WAVE_FOUND', wave: found } : { outcome: 'WAVE_NOT_FOUND' } };
    },
  };
}

function pager(pages: Page[]): {
  list: (conditions: CatalogueConditions, after: string | null) => Promise<ApiResult<WaveListBody>>;
  seen: Array<{ conditions: CatalogueConditions; after: string | null }>;
} {
  const seen: Array<{ conditions: CatalogueConditions; after: string | null }> = [];
  return {
    seen,
    list: async (conditions, after) => {
      seen.push({ conditions, after });
      const index = after === null ? 0 : Number(after);
      const page = pages[index];
      return {
        kind: 'outcome',
        status: 200,
        body: { outcome: 'WAVES_LISTED', waves: page.waves, page: { size: 200, next: page.next, total: null } },
      };
    },
  };
}

test('读队列：按本仓、只问还没拣完的两种状态，从第一页翻到没有下一页，格排好了交回', async () => {
  const source = pager([
    { waves: [wave('W-3', [task('T-3', 'B-01-01')])], next: '1' },
    { waves: [wave('W-2', [task('T-2', 'A-01-01')]), wave('W-1', [task('T-1', 'A-01-02', { status: 'DONE' })])], next: null },
  ]);
  const answer = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: null, list: source.list, one: neverOne });
  equal(answer.kind, 'queue');
  if (answer.kind !== 'queue') return;
  deepEqual(
    answer.queue.cells.map((cell) => cell.taskId),
    ['T-2', 'T-3'],
  );
  equal(answer.queue.wavesRead, 3);
  equal(answer.queue.pagesRead, 2);
  equal(answer.queue.truncated, false);
  equal(answer.queue.waveMissing, false);
  deepEqual(answer.queue.waves, [
    { waveId: 'W-3', open: 1 },
    { waveId: 'W-2', open: 1 },
  ]);
  deepEqual(source.seen.map((call) => call.after), [null, '1']);
  deepEqual(source.seen[0].conditions, { filters: { warehouseCode: ['SYN-WH-01'], status: ['RELEASED', 'PICKING'] } });
});

test('读队列：翻到页数上限还有下一页，就说只看了这些，不假装看全了', async () => {
  const pages: Page[] = Array.from({ length: 10 }, (_, index) => ({
    waves: [wave(`W-${index}`, [task(`T-${index}`, `A-01-0${index}`)])],
    next: index < 9 ? String(index + 1) : null,
  }));
  const source = pager(pages);
  const answer = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: null, list: source.list, one: neverOne });
  equal(answer.kind, 'queue');
  if (answer.kind !== 'queue') return;
  equal(answer.queue.pagesRead, MAX_WAVE_PAGES);
  equal(source.seen.length, MAX_WAVE_PAGES);
  equal(answer.queue.wavesRead, MAX_WAVE_PAGES);
  equal(answer.queue.truncated, true);
  equal(answer.queue.cells.length, MAX_WAVE_PAGES);
});

test('读队列：页数上限可以调，刚好翻完时不算被截断', async () => {
  const source = pager([
    { waves: [wave('W-1', [task('T-1', 'A-01-01')])], next: '1' },
    { waves: [wave('W-2', [task('T-2', 'A-01-02')])], next: null },
  ]);
  const exact = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: null, list: source.list, one: neverOne, maxPages: 2 });
  equal(exact.kind === 'queue' && exact.queue.truncated, false);
  const cut = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: null, list: source.list, one: neverOne, maxPages: 1 });
  equal(cut.kind === 'queue' && cut.queue.truncated, true);
});

test('读队列：指名了波次就按号读那一张，不走列表', async () => {
  const source = pager([{ waves: [wave('W-10', [task('T-10', 'A-01-01')])], next: null }]);
  const known = reader([wave('W-1', [task('T-1', 'A-01-02'), task('T-2', 'A-01-01', { status: 'DONE' })])]);
  const answer = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: 'W-1', list: source.list, one: known.one });
  equal(answer.kind, 'queue');
  if (answer.kind !== 'queue') return;
  deepEqual(
    answer.queue.cells.map((cell) => cell.taskId),
    ['T-1'],
  );
  equal(answer.queue.waveMissing, false);
  equal(answer.queue.truncated, false);
  deepEqual(answer.queue.waves, [{ waveId: 'W-1', open: 1 }]);
  deepEqual(known.asked, ['W-1']);
  equal(source.seen.length, 0);
});

test('读队列：指名的波次没有，或不属于本仓时说没找到，队列是空的', async () => {
  const source = pager([{ waves: [], next: null }]);
  const absent = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: 'W-1', list: source.list, one: reader([]).one });
  equal(absent.kind === 'queue' && absent.queue.waveMissing, true);
  equal(absent.kind === 'queue' && absent.queue.cells.length, 0);

  const elsewhere = reader([wave('W-1', [task('T-1', 'A-01-02')], 'SYN-WH-02')]);
  const answer = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: 'W-1', list: source.list, one: elsewhere.one });
  equal(answer.kind, 'queue');
  if (answer.kind !== 'queue') return;
  equal(answer.queue.waveMissing, true);
  deepEqual(answer.queue.cells, []);
});

test('读队列：指名的波次没读成就原样交回失败', async () => {
  const source = pager([{ waves: [], next: null }]);
  const failure = await loadPickQueue({
    warehouse: 'SYN-WH-01',
    wave: 'W-1',
    list: source.list,
    one: async () => ({ kind: 'transport', message: '网络断了' }),
  });
  deepEqual(failure, { kind: 'failed', failure: { kind: 'transport', message: '网络断了' } });
});

test('读队列：读口没答出来就原样交回失败，不交一个空队列冒充「没有要拣的」', async () => {
  const failure = await loadPickQueue({
    warehouse: 'SYN-WH-01',
    wave: null,
    list: async () => ({ kind: 'transport', message: '网络断了' }),
    one: neverOne,
  });
  deepEqual(failure, { kind: 'failed', failure: { kind: 'transport', message: '网络断了' } });

  const half = pager([{ waves: [wave('W-1', [task('T-1', 'A-01-01')])], next: '1' }]);
  let calls = 0;
  const flaky = await loadPickQueue({
    warehouse: 'SYN-WH-01',
    wave: null,
    list: async (conditions, after) => {
      calls += 1;
      return calls === 1 ? half.list(conditions, after) : { kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' };
    },
    one: neverOne,
  });
  equal(flaky.kind, 'failed');
});

test('各波次还剩几格：没有待拣格的波次不列，新的在前', async () => {
  const source = pager([
    {
      waves: [
        wave('W-3', [task('T-3', 'A-01-03', { status: 'DONE' })]),
        wave('W-2', [task('T-2', 'A-01-02'), task('T-4', 'A-01-04')]),
        wave('W-1', [task('T-1', 'A-01-01')]),
      ],
      next: null,
    },
  ]);
  const answer = await loadPickQueue({ warehouse: 'SYN-WH-01', wave: null, list: source.list, one: neverOne });
  equal(answer.kind, 'queue');
  if (answer.kind !== 'queue') return;
  deepEqual(answer.queue.waves, [
    { waveId: 'W-2', open: 2 },
    { waveId: 'W-1', open: 1 },
  ]);
});

// —— 换波次的选项 ——

test('换波次的选项：全部波次在最前，后面是各波次还剩几格，次序照队列给的', () => {
  deepEqual(
    waveChoices(null, [
      { waveId: 'W-2', open: 2 },
      { waveId: 'W-1', open: 1 },
    ]),
    [
      { value: allWavesChoice, label: '全部波次' },
      { value: 'W-2', label: 'W-2（还有 2 格）' },
      { value: 'W-1', label: 'W-1（还有 1 格）' },
    ],
  );
  deepEqual(waveChoices(null, []), [{ value: allWavesChoice, label: '全部波次' }]);
});

test('换波次的选项：地址里指名的波次不在读到的里面时也列出来，排在全部波次后面', () => {
  deepEqual(waveChoices('W-9', [{ waveId: 'W-1', open: 1 }]), [
    { value: allWavesChoice, label: '全部波次' },
    { value: 'W-9', label: 'W-9' },
    { value: 'W-1', label: 'W-1（还有 1 格）' },
  ]);
  // 在读到的里面就不重复列
  equal(waveChoices('W-1', [{ waveId: 'W-1', open: 1 }]).filter((choice) => choice.value === 'W-1').length, 1);
});

test('换波次的选项：选项的值都不是空串（下拉不收空串），选中的值换得回波次号', () => {
  const choices = waveChoices('W-9', [{ waveId: 'W-1', open: 1 }]);
  ok(choices.every((choice) => choice.value !== ''));
  equal(waveOfChoice(allWavesChoice), null);
  equal(waveOfChoice('W-1'), 'W-1');
  equal(waveOfChoice('W-9'), 'W-9');
});
