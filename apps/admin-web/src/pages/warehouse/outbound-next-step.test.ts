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

test('去拣货之后现场的几步还指到旧的作业台页', () => {
  for (const status of ['PICKED', 'CHECKED', 'PACKED', 'AWAITING_RETURN']) {
    const next = outboundNextStep(status, false);
    equal(next?.kind === 'go' ? next.moduleId : null, 'warehouse-workstation', status);
  }
});

test('只有已放波次的才指到拣货工位，波次号对别的状态没有影响', () => {
  for (const status of ['RESERVED', 'PICKED', 'CHECKED', 'PACKED', 'SHIPPED', 'CANCELLED']) {
    equal(outboundNextStep(status, false, 'SYN-WAVE-03')?.kind === 'station', false, status);
  }
  deepEqual(outboundNextStep('PICKED', false, 'SYN-WAVE-03'), outboundNextStep('PICKED', false));
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
