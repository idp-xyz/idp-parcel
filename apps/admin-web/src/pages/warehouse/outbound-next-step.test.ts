import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { outboundNextStep, outboundReservationOpen, outboundSettled } from './outbound-next-step';

test('占没占够只在拣完之前问：拣完之后与取消之后预占是 0，不是缺货', () => {
  for (const status of ['RESERVED', 'PARTIALLY_RESERVED', 'PENDING_POLICY', 'ALLOCATED']) equal(outboundReservationOpen(status), true, status);
  for (const status of ['PICKED', 'CHECKED', 'PACKED', 'SHIPPED', 'CANCELLED', 'REJECTED']) equal(outboundReservationOpen(status), false, status);
});

test('已预占的在出库单上就地放波次', () => {
  deepEqual(outboundNextStep('RESERVED', false), { kind: 'wave', label: '放波次' });
});

test('已放波次的去拣货工位：不知道波次是本仓全部波次，知道就带上', () => {
  deepEqual(outboundNextStep('ALLOCATED', false), { kind: 'station', label: '去拣货', hash: '#/station/pick' });
  deepEqual(outboundNextStep('ALLOCATED', false, 'SYN-WAVE-03'), {
    kind: 'station',
    label: '去拣货',
    hash: '#/station/pick?wave=SYN-WAVE-03',
  });
  // 波次号里的特殊字符照编码，地址读得回原样
  deepEqual(outboundNextStep('ALLOCATED', false, 'W/1 一'), {
    kind: 'station',
    label: '去拣货',
    hash: '#/station/pick?wave=W%2F1+%E4%B8%80',
  });
});

test('复核打包之后现场还没搬进全屏的几步还指到旧的作业台页', () => {
  for (const status of ['PACKED', 'AWAITING_RETURN']) {
    const next = outboundNextStep(status, false);
    equal(next?.kind === 'go' ? next.moduleId : null, 'warehouse-workstation', status);
  }
});

test('已拣货的去复核、已复核的去打包，都进复核打包工位；带着出库请求号就直接做这一张，没带就进去等人扫单号', () => {
  deepEqual(outboundNextStep('PICKED', false, undefined, 'OBR-1'), { kind: 'station', label: '去复核', hash: '#/station/pack?request=OBR-1' });
  deepEqual(outboundNextStep('CHECKED', false, undefined, 'OBR-1'), { kind: 'station', label: '去打包', hash: '#/station/pack?request=OBR-1' });
  deepEqual(outboundNextStep('PICKED', false), { kind: 'station', label: '去复核', hash: '#/station/pack' });
  deepEqual(outboundNextStep('CHECKED', false), { kind: 'station', label: '去打包', hash: '#/station/pack' });
  // 请求号里的特殊字符照编码，地址读得回原样
  deepEqual(outboundNextStep('PICKED', false, undefined, 'OBR/1 一'), {
    kind: 'station',
    label: '去复核',
    hash: '#/station/pack?request=OBR%2F1+%E4%B8%80',
  });
});

test('出库请求号只对已拣货与已复核的有用，别的状态不受影响', () => {
  for (const status of ['RESERVED', 'ALLOCATED', 'PACKED', 'SHIPPED', 'CANCELLED', 'AWAITING_RETURN', 'HANDED_OFF']) {
    deepEqual(outboundNextStep(status, false, undefined, 'OBR-1'), outboundNextStep(status, false), status);
  }
});

test('只有已放波次的才指到拣货工位，波次号对别的状态没有影响', () => {
  for (const status of ['RESERVED', 'PICKED', 'CHECKED', 'PACKED', 'SHIPPED', 'CANCELLED']) {
    const next = outboundNextStep(status, false, 'SYN-WAVE-03');
    equal(next?.kind === 'station' && next.hash.startsWith('#/station/pick'), false, status);
  }
  deepEqual(outboundNextStep('PICKED', false, 'SYN-WAVE-03'), outboundNextStep('PICKED', false));
  deepEqual(outboundNextStep('CHECKED', false, 'SYN-WAVE-03', 'OBR-1'), outboundNextStep('CHECKED', false, undefined, 'OBR-1'));
});

test('代理仓的单不指到自营仓的作业台', () => {
  for (const status of ['HANDED_OFF', 'FORWARDED']) {
    const next = outboundNextStep(status, true);
    equal(next?.kind === 'go' ? next.moduleId : null, 'warehouse-agents', status);
  }
  const packed = outboundNextStep('PACKED', true);
  equal(packed?.kind === 'go' ? packed.moduleId : null, 'warehouse-agents');
});

test('卡在预占的两种状态说清在等什么，不摆一个按不动的按钮', () => {
  equal(outboundNextStep('PARTIALLY_RESERVED', false)?.kind, 'wait');
  equal(outboundNextStep('PENDING_POLICY', false)?.kind, 'wait');
});

test('走完或中止的没有下一步，队列上不算在等', () => {
  for (const status of ['SHIPPED', 'CANCELLED', 'REJECTED', 'SPLIT']) {
    equal(outboundNextStep(status, false), null, status);
    equal(outboundSettled(status), true, status);
  }
  equal(outboundSettled('PACKED'), false);
});
