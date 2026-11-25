import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { parseStationAddress } from '../../../shell/station-address';
import {
  legacyWorkstationHash,
  pickStationHash,
  resolveStationRoute,
  stationEntries,
  stationEntryOf,
  stationHomeHash,
} from './stations';

function route(hash: string) {
  const address = parseStationAddress(hash);
  ok(address !== null, `${hash} 应是作业台地址`);
  return resolveStationRoute(address);
}

test('六个工位，次序与名字照屏幕示意；这一笔只有拣货可进', () => {
  deepEqual(
    stationEntries.map((entry) => entry.name),
    ['拣货', '复核打包', '收货', '发货交接', '上架与移库', '盘点清点'],
  );
  deepEqual(
    stationEntries.filter((entry) => entry.ready).map((entry) => entry.id),
    ['pick'],
  );
  equal(new Set(stationEntries.map((entry) => entry.id)).size, 6);
});

test('每个工位都有一句说明，没有英文码', () => {
  for (const entry of stationEntries) {
    ok(entry.summary.length > 0, entry.id);
    ok(!/[A-Z]{3,}/.test(entry.summary), `${entry.id}: ${entry.summary}`);
  }
});

test('地址：选工位、拣货、带波次的拣货；退出作业台回旧页', () => {
  equal(stationHomeHash, '#/station');
  equal(pickStationHash(), '#/station/pick');
  equal(pickStationHash('SYN-WAVE-03'), '#/station/pick?wave=SYN-WAVE-03');
  equal(pickStationHash(undefined), '#/station/pick');
  equal(legacyWorkstationHash, '#/warehouse-workstation');
});

test('选工位地址落到选工位页', () => {
  deepEqual(route('#/station'), { kind: 'home' });
  deepEqual(route('#/station/'), { kind: 'home' });
});

test('拣货地址落到拣货工位；波次号取查询串里的 wave，空白当没给', () => {
  deepEqual(route('#/station/pick'), { kind: 'pick', wave: null });
  deepEqual(route('#/station/pick?wave=SYN-WAVE-03'), { kind: 'pick', wave: 'SYN-WAVE-03' });
  deepEqual(route('#/station/pick?wave='), { kind: 'pick', wave: null });
  deepEqual(route('#/station/pick?wave=%20%20'), { kind: 'pick', wave: null });
  deepEqual(route('#/station/pick?other=1'), { kind: 'pick', wave: null });
});

test('带多余的段也落到拣货工位', () => {
  deepEqual(route('#/station/pick/anything?wave=W-1'), { kind: 'pick', wave: 'W-1' });
});

test('另外五个工位全屏的还没做好：落到说明页，不当成拣货', () => {
  for (const id of ['pack', 'receive', 'dispatch', 'putaway', 'count']) {
    const resolved = route(`#/station/${id}`);
    equal(resolved.kind, 'not-ready', id);
    if (resolved.kind === 'not-ready') equal(resolved.entry.id, id);
  }
});

test('不是工位的词落到「没有这个工位」，带着原词', () => {
  deepEqual(route('#/station/shipping'), { kind: 'unknown', station: 'shipping' });
  deepEqual(route('#/station/PICK'), { kind: 'unknown', station: 'PICK' });
});

test('按词取工位', () => {
  equal(stationEntryOf('pick')?.name, '拣货');
  equal(stationEntryOf('count')?.name, '盘点清点');
  equal(stationEntryOf('nope'), undefined);
  equal(stationEntryOf(null), undefined);
});
