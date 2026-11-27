import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { monitorFilters, monitorGroups, monitorNodes, outboundLines, taskLinesFromDetail, waveLines, waveTaskLines } from './monitor-tree';

test('出库单行能打开单据，也能钻进拣货任务', () => {
  const lines = outboundLines([{ requestId: 'OBR-1', documentNumber: 'OB-1', ownerAccount: 'O', warehouseCode: 'WH', status: 'ALLOCATED', dispatch: 'PICKUP' }]);
  equal(lines[0].cells[0], 'OB-1');
  equal(lines[0].drillLabel, '拣货任务');
  equal(lines[0].drillOutbound, 'OBR-1');
  equal(lines[0].hash?.includes('warehouse-outbound'), true);
});

test('波次行记下要再读的波次，任务条数用列表上的计数；读到的任务没有货主，不再钻库存格', () => {
  const lines = waveLines([{ waveId: 'W-1', warehouseCode: 'WH', requests: ['OBR-1'], openTasks: 1, doneTasks: 1, shortTasks: 0, cancelledTasks: 0 }]);
  equal(lines[0].cells[3], '2');
  equal(lines[0].drillWave, 'W-1');
  const tasks = waveTaskLines([{ taskId: 'T-1', locationCode: 'L-1', goodsCode: 'G-1', quantity: 2, picked: 0, status: 'OPEN' }]);
  equal(tasks[0].cells[1], 'L-1');
  equal(tasks[0].drillStock, undefined);
});

test('出库单详情里的任务带上货主，才能再钻库存格', () => {
  const lines = taskLinesFromDetail([{ taskId: 'T-1', locationCode: 'L-1', goodsCode: 'G-1', quantity: 1, picked: 1, status: 'PICKED' }], 'OWNER', 'WH');
  equal(lines[0].drillStock?.owner, 'OWNER');
  equal(lines[0].drillStock?.goods, 'G-1');
  equal(taskLinesFromDetail([{ taskId: 'T-1', locationCode: 'L-1', goodsCode: 'G-1', quantity: 1, picked: 1, status: 'PICKED' }], '  ', 'WH')[0].drillStock, undefined);
});

test('没有列表读口的对象不进树', () => {
  const labels = monitorGroups().flatMap((group) => group.nodes.map((node) => node.label));
  equal(labels.includes('收货记录'), false);
  equal(labels.includes('出库单'), true);
});

test('没声明的筛选维不发出去，状态不在词表里也不发', () => {
  const wave = monitorNodes.find((item) => item.id === 'waves');
  const unclaimed = monitorNodes.find((item) => item.id === 'unclaimed');
  deepEqual(monitorFilters(wave!, 'WH', 'OWNER', 'OPEN'), { warehouseCode: ['WH'] });
  deepEqual(monitorFilters(unclaimed!, ' WH ', 'OWNER', 'UNCLAIMED'), { warehouseCode: ['WH'], status: ['UNCLAIMED'] });
  deepEqual(monitorFilters(unclaimed!, '', '', 'NOPE'), {});
});
