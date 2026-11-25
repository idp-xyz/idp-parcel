import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { appScreen, parseStationAddress, stationHash } from './station-address';

function summary(hash: string) {
  const address = parseStationAddress(hash);
  return address === null ? null : { station: address.station, query: Object.fromEntries(address.query) };
}

test('`#/station` 是选工位那一页，`#/station/pick` 是拣货工位', () => {
  deepEqual(summary('#/station'), { station: null, query: {} });
  deepEqual(summary('#/station/pick'), { station: 'pick', query: {} });
});

test('末尾的斜杠与空的第二段按选工位那一页算', () => {
  deepEqual(summary('#/station/'), { station: null, query: {} });
  deepEqual(summary('#/station//pick'), { station: null, query: {} });
});

test('查询串照读：工位地址与选工位地址都可以带', () => {
  deepEqual(summary('#/station/pick?wave=SYN-WAVE-03'), { station: 'pick', query: { wave: 'SYN-WAVE-03' } });
  deepEqual(summary('#/station?from=wave'), { station: null, query: { from: 'wave' } });
  deepEqual(summary('#/station/pick?wave=W%2F1+%E4%B8%80'), { station: 'pick', query: { wave: 'W/1 一' } });
});

test('带多余的段仍是作业台的地址，只取前两段', () => {
  deepEqual(summary('#/station/pick/extra'), { station: 'pick', query: {} });
  deepEqual(summary('#/station/pick/extra/more?wave=W-1'), { station: 'pick', query: { wave: 'W-1' } });
});

test('没有斜杠的写法与外壳认标签的写法一致', () => {
  deepEqual(summary('#station/pick'), { station: 'pick', query: {} });
});

test('只是前缀相同的别的地址不算作业台', () => {
  equal(parseStationAddress('#/stationery'), null);
  equal(parseStationAddress('#/stationery/pick'), null);
  equal(parseStationAddress('#/station-x'), null);
  equal(parseStationAddress('#/stations'), null);
  equal(parseStationAddress('#/warehouse-workstation'), null);
  equal(parseStationAddress('#/warehouse-outbound/station'), null);
});

test('空地址与别的页都不算', () => {
  equal(parseStationAddress(''), null);
  equal(parseStationAddress('#'), null);
  equal(parseStationAddress('#/'), null);
  equal(parseStationAddress('#/workbench'), null);
  equal(parseStationAddress('#/warehouse-outbound'), null);
});

test('第一段按外壳认标签的同一种写法先解码；解不开的不算', () => {
  deepEqual(summary('#/%73tation/pick'), { station: 'pick', query: {} });
  equal(parseStationAddress('#/%E0%A4%A'), null);
});

test('第二段解不开码时原样留着，不把整个地址判成别的页', () => {
  const address = parseStationAddress('#/station/%E0%A4%A');
  ok(address !== null);
  equal(address.station, '%E0%A4%A');
});

test('造地址：选工位、工位、带波次；空的查询项不写', () => {
  equal(stationHash(null), '#/station');
  equal(stationHash('pick'), '#/station/pick');
  equal(stationHash('pick', { wave: 'SYN-WAVE-03' }), '#/station/pick?wave=SYN-WAVE-03');
  equal(stationHash('pick', { wave: undefined }), '#/station/pick');
  equal(stationHash('pick', { wave: '' }), '#/station/pick');
});

test('造出来的地址读得回原样，波次号里的特殊字符照编码', () => {
  const hash = stationHash('pick', { wave: 'W/1 一&x=2' });
  deepEqual(summary(hash), { station: 'pick', query: { wave: 'W/1 一&x=2' } });
});

test('外壳的地址落外壳，不看部署开关', () => {
  deepEqual(appScreen('#/warehouse-outbound', true), { kind: 'shell' });
  deepEqual(appScreen('#/warehouse-outbound', false), { kind: 'shell' });
  deepEqual(appScreen('', true), { kind: 'shell' });
  deepEqual(appScreen('#/stationery', false), { kind: 'shell' });
});

test('作业台地址上开关开着，或还不知道，照常渲染工位', () => {
  const screen = appScreen('#/station/pick?wave=W-1', true);
  equal(screen.kind, 'station');
  if (screen.kind !== 'station') return;
  equal(screen.address.station, 'pick');
  equal(screen.address.query.get('wave'), 'W-1');
});

test('作业台地址上开关明确关着，摆「没有启用」的说明，不渲染工位', () => {
  deepEqual(appScreen('#/station', false), { kind: 'station-not-enabled' });
  deepEqual(appScreen('#/station/pick', false), { kind: 'station-not-enabled' });
});
