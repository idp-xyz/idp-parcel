import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { goodsMeasurementRows, goodsStockRows, revisionPrefill, sizeWord } from './goods-profile';

const box = { length: '30', width: '20', height: '10', unit: 'cm' };
const kilo = (value: string) => ({ value, unit: 'kg' });

test('尺寸与重量连成一句，缺哪样就不写哪样，都缺是空串', () => {
  equal(sizeWord(box, kilo('1.2')), '30×20×10 cm · 1.2 kg');
  equal(sizeWord(box, undefined), '30×20×10 cm');
  equal(sizeWord(undefined, kilo('1.2')), '1.2 kg');
  equal(sizeWord(undefined, undefined), '');
});

test('申报与仓测按层级对在一行：申报的先后在前，只有仓测记录的层级排后面', () => {
  const rows = goodsMeasurementRows({
    declaredMeasurements: [
      { level: '件', dimensions: box, weight: kilo('1.2') },
      { level: '箱', weight: kilo('14') },
    ],
    warehouseMeasurements: [
      {
        level: '件',
        measured: true,
        current: { warehouseCode: 'WH1', operator: 'OP1', occurredAt: '2026-11-20T08:00:00Z', dimensions: box, weight: kilo('1.3') },
      },
      { level: '托盘', measured: false },
    ],
  });
  deepEqual(rows, [
    {
      level: '件',
      declared: '30×20×10 cm · 1.2 kg',
      measured: true,
      current: '30×20×10 cm · 1.3 kg',
      measuredBy: { warehouseCode: 'WH1', operator: 'OP1', occurredAt: '2026-11-20T08:00:00Z' },
    },
    { level: '箱', declared: '14 kg', measured: false, current: '' },
    { level: '托盘', declared: '', measured: false, current: '' },
  ]);
});

test('没仓测过的层级，申报里没有时用读口附的货主声明参照顶上；申报里有就以申报为准', () => {
  const rows = goodsMeasurementRows({
    declaredMeasurements: [{ level: '件', weight: kilo('1') }],
    warehouseMeasurements: [
      { level: '件', measured: false, declared: { weight: kilo('9') } },
      { level: '箱', measured: false, declared: { dimensions: box } },
    ],
  });
  equal(rows[0].declared, '1 kg');
  equal(rows[1].declared, '30×20×10 cm');
});

test('什么都没登记就没有行', () => {
  deepEqual(goodsMeasurementRows({}), []);
});

test('库存行按商品编码逐字对，不是包含匹配', () => {
  const summary = [
    { goodsCode: 'G1', warehouse: 'WH1' },
    { goodsCode: 'G10', warehouse: 'WH1' },
    { goodsCode: 'G1', warehouse: 'WH2' },
  ];
  deepEqual(goodsStockRows(summary, 'G1'), [summary[0], summary[2]]);
  deepEqual(goodsStockRows(undefined, 'G1'), []);
});

test('代录新修订基于当前修订号，名称、单位与跟踪开关从当前修订带出', () => {
  deepEqual(
    revisionPrefill({ ownerAccount: 'OWNER-1', goodsCode: 'G1', revision: 3, name: '示例', baseUnit: '件', trackBatch: true, trackExpiry: false }),
    { ownerAccount: 'OWNER-1', goodsCode: 'G1', basisRevision: '3', name: '示例', baseUnit: '件', trackBatch: true, trackExpiry: false },
  );
});
