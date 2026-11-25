import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { parseStationAddress } from '../../../shell/station-address';
import {
  legacyWorkstationHash,
  packStationHash,
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

test('六个工位，次序与名字照屏幕示意；拣货与复核打包可进', () => {
  deepEqual(
    stationEntries.map((entry) => entry.name),
    ['拣货', '复核打包', '收货', '发货交接', '上架与移库', '盘点清点'],
  );
  deepEqual(
    stationEntries.filter((entry) => entry.ready).map((entry) => entry.id),
    ['pick', 'pack'],
  );
  equal(new Set(stationEntries.map((entry) => entry.id)).size, 6);
});

test('每个工位都有一句说明，没有英文码', () => {
  for (const entry of stationEntries) {
    ok(entry.summary.length > 0, entry.id);
    ok(!/[A-Z]{3,}/.test(entry.summary), `${entry.id}: ${entry.summary}`);
  }
});

test('地址：选工位、拣货、带波次的拣货、复核打包、带出库请求号的复核打包；退出作业台回旧页', () => {
  equal(stationHomeHash, '#/station');
  equal(pickStationHash(), '#/station/pick');
  equal(pickStationHash('SYN-WAVE-03'), '#/station/pick?wave=SYN-WAVE-03');
  equal(pickStationHash(undefined), '#/station/pick');
  equal(packStationHash(), '#/station/pack');
  equal(packStationHash(undefined), '#/station/pack');
  equal(packStationHash(''), '#/station/pack');
  equal(packStationHash('OBR-1'), '#/station/pack?request=OBR-1');
  equal(legacyWorkstationHash, '#/warehouse-workstation');
});

test('出库请求号里有特殊字符也原样往返：写进地址再读出来还是它', () => {
  for (const request of ['OBR-1', 'SYN/REQ 2', 'a&b=c', '出库?1', 'x#y']) {
    const resolved = route(packStationHash(request));
    deepEqual(resolved, { kind: 'pack', request }, request);
  }
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

test('复核打包地址落到复核打包工位；出库请求号取查询串里的 request，空白当没给；别的查询项不影响', () => {
  deepEqual(route('#/station/pack'), { kind: 'pack', request: null });
  deepEqual(route('#/station/pack?request=OBR-1'), { kind: 'pack', request: 'OBR-1' });
  deepEqual(route('#/station/pack?request='), { kind: 'pack', request: null });
  deepEqual(route('#/station/pack?request=%20%20'), { kind: 'pack', request: null });
  deepEqual(route('#/station/pack?wave=W-1'), { kind: 'pack', request: null });
  deepEqual(route('#/station/pack?request=%20OBR-1%20'), { kind: 'pack', request: 'OBR-1' });
  deepEqual(route('#/station/pick?request=OBR-1'), { kind: 'pick', wave: null }, '拣货不认 request');
});

test('另外四个工位全屏的还没做好：落到说明页，不当成拣货或复核打包', () => {
  for (const id of ['receive', 'dispatch', 'putaway', 'count']) {
    const resolved = route(`#/station/${id}`);
    equal(resolved.kind, 'not-ready', id);
    if (resolved.kind === 'not-ready') equal(resolved.entry.id, id);
  }
});

test('不是工位的词落到「没有这个工位」，带着原词', () => {
  deepEqual(route('#/station/shipping'), { kind: 'unknown', station: 'shipping' });
  deepEqual(route('#/station/PICK'), { kind: 'unknown', station: 'PICK' });
  deepEqual(route('#/station/Pack'), { kind: 'unknown', station: 'Pack' });
});

test('按词取工位', () => {
  equal(stationEntryOf('pick')?.name, '拣货');
  equal(stationEntryOf('pack')?.name, '复核打包');
  equal(stationEntryOf('count')?.name, '盘点清点');
  equal(stationEntryOf('nope'), undefined);
  equal(stationEntryOf(null), undefined);
});
