import { test } from 'node:test';
import { deepEqual, equal, notEqual, ok } from 'node:assert/strict';
import { formatInstant } from '../catalogue-view';
import type { ReceptionRecord } from './records-api';
import { receptionRowOf, serviceMarkersCellOf } from './reception-rows';

// 「服务标记」一格的四格加一格（票 operator-channel/44）。最容易错的是三种「没有标记」：已查无标记、
// 读不动、待识别在拼接写法下都是空白，而今天没有方向性门禁，看的人会把空白读成「没有取消、没有终局」。
// 读不动那格尤其要与空白、与「无」分开——它说的是没取得答复，不是答复里什么都没有。

function formed(overrides: Partial<ReceptionRecord> = {}): ReceptionRecord {
  return {
    sourceId: 'SYN-SRC-1',
    kind: 'INTAKE_FORMED',
    unit: 'SYN-UNIT-1',
    node: 'SYN-NODE-1',
    deliveredBy: 'SYN-PARTY-1',
    receivedAt: '2026-10-19T02:00:00Z',
    controlKind: 'NODE_INTAKE',
    controlEstablishedAt: '2026-10-19T02:00:00Z',
    serviceMarkers: [],
    serviceResultCheck: 'CHECKED',
    recordedAt: '2026-10-19T02:00:01Z',
    ...overrides,
  };
}

// 待识别行传输层整键不带核对态，标记也恒为空——这里照答复的真实形状造，不塞一个 undefined 键。
function pending(): ReceptionRecord {
  const { serviceResultCheck: _absent, ...rest } = formed({
    sourceId: 'SYN-SRC-4',
    kind: 'PENDING_IDENTIFICATION',
  });
  return rest;
}

const checkedWithMarkers = formed({
  serviceMarkers: ['SYN-CANCELLED', 'SYN-TERMINAL'],
});
const checkedWithoutMarkers = formed();
const unreadable = formed({ serviceResultCheck: 'UNREADABLE' });

test('已查、有标记：标记照旧用顿号拼接', () => {
  equal(serviceMarkersCellOf(checkedWithMarkers), 'SYN-CANCELLED、SYN-TERMINAL');
});

test('已查、无标记：写明「无取消与终局」，不留空白', () => {
  equal(serviceMarkersCellOf(checkedWithoutMarkers), '无取消与终局');
});

test('读不动：写明服务结果读不动，且点明标记缺席不代表没有取消或终局', () => {
  const cell = serviceMarkersCellOf(unreadable);

  ok(cell.includes('服务结果读不动'), cell);
  ok(cell.includes('不代表没有取消或终局'), cell);
  // 与空白、与「无」、与已查无标记那格同形，这一格就白立了。
  notEqual(cell.trim(), '');
  notEqual(cell, '无');
  ok(!cell.startsWith('无'), `读不动的文字以「无」起头，扫一眼会读成「无标记」：${cell}`);
  notEqual(cell, serviceMarkersCellOf(checkedWithoutMarkers));
});

test('待识别：写「不适用」——身份未定，本就不核对服务结果', () => {
  equal(serviceMarkersCellOf(pending()), '不适用');
});

test('形成行没带核对态：既不冒充「已查无标记」，也不冒充「读不动」', () => {
  const { serviceResultCheck: _absent, ...withoutCheck } = formed();
  const cell = serviceMarkersCellOf(withoutCheck);

  ok(cell.includes('不代表没有取消或终局'), cell);
  ok(!cell.includes('读不动'), `没带核对态不等于读不动：${cell}`);
  notEqual(cell, serviceMarkersCellOf(checkedWithoutMarkers));
});

test('形成行带了页面未收录的核对态：如实露码，同样不下「没有」的结论', () => {
  const cell = serviceMarkersCellOf(formed({ serviceResultCheck: 'SYN-FUTURE-CHECK' }));

  ok(cell.includes('SYN-FUTURE-CHECK'), cell);
  ok(cell.includes('不代表没有取消或终局'), cell);
});

test('标记在场即查到过：核对态缺席时照实列出标记', () => {
  const { serviceResultCheck: _absent, ...withoutCheck } = formed({
    serviceMarkers: ['SYN-CANCELLED'],
  });
  equal(serviceMarkersCellOf(withoutCheck), 'SYN-CANCELLED');
});

test('四格加缺席一格两两不同，且没有一格是空白', () => {
  const { serviceResultCheck: _absent, ...withoutCheck } = formed();
  const cells = [
    checkedWithMarkers,
    checkedWithoutMarkers,
    unreadable,
    pending(),
    withoutCheck,
  ].map(serviceMarkersCellOf);

  equal(new Set(cells).size, cells.length, cells.join(' | '));
  for (const cell of cells) notEqual(cell.trim(), '', cells.join(' | '));
});

test('收寄行整行转写：服务标记一格经同一判读，其余各列照旧', () => {
  deepEqual(receptionRowOf(unreadable), {
    key: 'reception:SYN-SRC-1',
    values: {
      unit: 'SYN-UNIT-1',
      kind: '已形成收寄',
      node: 'SYN-NODE-1',
      deliveredBy: 'SYN-PARTY-1',
      receivedAt: formatInstant('2026-10-19T02:00:00Z'),
      control: '节点收寄 · 控制中',
      serviceMarkers: serviceMarkersCellOf(unreadable),
    },
  });

  const released = receptionRowOf(
    formed({ controlReleasedBy: 'SYN-HANDOVER-1', controlReleasedAt: '2026-10-19T03:00:00Z' }),
  );
  equal(
    released.values.control,
    `节点收寄 · 已转出 SYN-HANDOVER-1 · ${formatInstant('2026-10-19T03:00:00Z')}`,
  );
  equal(receptionRowOf(pending()).values.kind, '待识别');
});
