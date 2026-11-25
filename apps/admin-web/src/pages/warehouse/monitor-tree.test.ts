import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { monitorFilters, monitorNodes, outboundLines, taskLinesFromDetail, waveLines } from './monitor-tree';

test('出库单行能打开单据，也能钻进拣货任务', () => {
  const lines = outboundLines([{ requestId: 'OBR-1', documentNumber: 'OB-1', ownerAccount: 'O', warehouseCode: 'WH', status: 'ALLOCATED', dispatch: 'PICKUP' }]);
  equal(lines[0].cells[0], 'OB-1');
  equal(lines[0].drillOutbound, 'OBR-1');
  equal(lines[0].hash?.includes('warehouse-outbound'), true);
});

test('波次行展开任务，任务上没有货主，不再钻库存格', () => {
  const lines = waveLines([{
    waveId: 'W-1',
    warehouseCode: 'WH',
    requests: ['OBR-1'],
    tasks: [{ taskId: 'T-1', locationCode: 'L-1', goodsCode: 'G-1', quantity: 2, picked: 0, status: 'OPEN' }],
  }]);
  equal(lines[0].drillTasks?.[0].cells[1], 'L-1');
  equal(lines[0].drillTasks?.[0].drillStock, undefined);
});

test('出库单详情里的任务带上货主，才能再钻库存格', () => {
  const lines = taskLinesFromDetail([{ taskId: 'T-1', locationCode: 'L-1', goodsCode: 'G-1', quantity: 1, picked: 1, status: 'PICKED' }], 'OWNER', 'WH');
  equal(lines[0].drillStock?.owner, 'OWNER');
  equal(lines[0].drillStock?.goods, 'G-1');
  equal(taskLinesFromDetail([{ taskId: 'T-1', locationCode: 'L-1', goodsCode: 'G-1', quantity: 1, picked: 1, status: 'PICKED' }], '  ', 'WH')[0].drillStock, undefined);
});

test('没声明的筛选维不发出去，状态不在词表里也不发', () => {
  const wave = monitorNodes.find((item) => item.id === 'waves');
  const unclaimed = monitorNodes.find((item) => item.id === 'unclaimed');
  deepEqual(monitorFilters(wave!, 'WH', 'OWNER', 'OPEN'), { warehouseCode: ['WH'] });
  deepEqual(monitorFilters(unclaimed!, ' WH ', 'OWNER', 'UNCLAIMED'), { warehouseCode: ['WH'], status: ['UNCLAIMED'] });
  deepEqual(monitorFilters(unclaimed!, '', '', 'NOPE'), {});
});
