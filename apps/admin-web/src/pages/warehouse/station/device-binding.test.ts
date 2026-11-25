import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  ACTIVE_DEVICE_KEY,
  advanceDeviceSequence,
  bindDevice,
  deviceSequenceKey,
  readActiveDeviceId,
  readDeviceSequence,
  registeredDeviceConditions,
  warehouseDevicesOf,
  writeActiveDeviceId,
  type StorageLike,
} from './device-binding';

function memory(initial: Record<string, string> = {}): StorageLike & { data: Map<string, string> } {
  const data = new Map(Object.entries(initial));
  return {
    data,
    getItem: (key) => data.get(key) ?? null,
    setItem: (key, value) => {
      data.set(key, value);
    },
  };
}

const broken: StorageLike = {
  getItem: () => {
    throw new Error('存储被禁用');
  },
  setItem: () => {
    throw new Error('存储被禁用');
  },
};

// 键与旧作业台页逐字相同：两边读写同一台设备与同一份序号。改这两个字面量就是让两边各记各的。
test('键与旧作业台页逐字相同', () => {
  equal(ACTIVE_DEVICE_KEY, 'idp.parcel.warehouse.activeDevice');
  equal(deviceSequenceKey('SYN-PDA-WH01-01'), 'idp.parcel.warehouse.deviceSeq.SYN-PDA-WH01-01');
});

test('绑定的设备：没记过是空串，记下后读得回', () => {
  const store = memory();
  equal(readActiveDeviceId(store), '');
  writeActiveDeviceId(store, 'SYN-PDA-WH01-01');
  equal(readActiveDeviceId(store), 'SYN-PDA-WH01-01');
  equal(store.data.get('idp.parcel.warehouse.activeDevice'), 'SYN-PDA-WH01-01');
});

test('旧页写下的绑定，作业台读得到；反过来也一样', () => {
  const store = memory({ 'idp.parcel.warehouse.activeDevice': 'DEV-A', 'idp.parcel.warehouse.deviceSeq.DEV-A': '41' });
  equal(readActiveDeviceId(store), 'DEV-A');
  equal(readDeviceSequence(store, 'DEV-A'), 41);
  equal(advanceDeviceSequence(store, 'DEV-A'), 42);
  equal(store.data.get('idp.parcel.warehouse.deviceSeq.DEV-A'), '42');
});

test('设备序号：没记过、记坏了、不是正数都按 0 算', () => {
  const store = memory({
    [deviceSequenceKey('BAD')]: 'abc',
    [deviceSequenceKey('ZERO')]: '0',
    [deviceSequenceKey('NEGATIVE')]: '-3',
    [deviceSequenceKey('OK')]: '7',
  });
  equal(readDeviceSequence(store, 'NEW'), 0);
  equal(readDeviceSequence(store, 'BAD'), 0);
  equal(readDeviceSequence(store, 'ZERO'), 0);
  equal(readDeviceSequence(store, 'NEGATIVE'), 0);
  equal(readDeviceSequence(store, 'OK'), 7);
});

test('设备号为空时序号恒为 0，也不碰存储', () => {
  let touched = false;
  const spy: StorageLike = {
    getItem: () => {
      touched = true;
      return '9';
    },
    setItem: () => {
      touched = true;
    },
  };
  equal(readDeviceSequence(spy, ''), 0);
  equal(touched, false);
});

test('推进序号：不给目标就加一，给了比现在大的就推到那里，不大的仍加一', () => {
  const store = memory();
  equal(advanceDeviceSequence(store, 'D'), 1);
  equal(advanceDeviceSequence(store, 'D'), 2);
  equal(advanceDeviceSequence(store, 'D', 10), 10);
  equal(advanceDeviceSequence(store, 'D', 5), 11);
  equal(advanceDeviceSequence(store, 'D', 11), 12);
  equal(readDeviceSequence(store, 'D'), 12);
});

test('各设备的序号互不相干', () => {
  const store = memory();
  advanceDeviceSequence(store, 'A');
  advanceDeviceSequence(store, 'A');
  advanceDeviceSequence(store, 'B');
  equal(readDeviceSequence(store, 'A'), 2);
  equal(readDeviceSequence(store, 'B'), 1);
});

test('存储读不了、写不了，或根本没有存储：不报错，序号照样交回这一笔用的值', () => {
  equal(readActiveDeviceId(broken), '');
  writeActiveDeviceId(broken, 'D');
  equal(readDeviceSequence(broken, 'D'), 0);
  equal(advanceDeviceSequence(broken, 'D'), 1);
  equal(advanceDeviceSequence(broken, 'D', 8), 8);

  equal(readActiveDeviceId(null), '');
  writeActiveDeviceId(null, 'D');
  equal(readDeviceSequence(null, 'D'), 0);
  equal(advanceDeviceSequence(null, 'D'), 1);
});

test('读设备名册的筛法与旧页相同：只要登记在用的', () => {
  deepEqual(registeredDeviceConditions, { filters: { status: ['REGISTERED'] } });
});

const warehouseDevice = (deviceId: string, warehouse = 'SYN-WH-01') => ({ deviceId, workScopeKind: 'WAREHOUSE', workScopeRef: warehouse });
const nodeDevice = (deviceId: string) => ({ deviceId, workScopeKind: 'NODE', workScopeRef: 'SYN-NODE-01' });

test('只留作业范围是仓库的设备', () => {
  deepEqual(
    warehouseDevicesOf([nodeDevice('N-1'), warehouseDevice('W-1'), warehouseDevice('W-2')]).map((device) => device.deviceId),
    ['W-1', 'W-2'],
  );
});

test('没有可用的仓库设备：说明去登记，不替人造一台', () => {
  deepEqual(bindDevice([], ''), { kind: 'none' });
  deepEqual(bindDevice([nodeDevice('N-1')], 'N-1'), { kind: 'none' });
});

test('只有一台直接用；本机记着的不是它就要重记', () => {
  const only = warehouseDevice('W-1');
  deepEqual(bindDevice([only], ''), { kind: 'bound', device: only, remember: true });
  deepEqual(bindDevice([only], 'W-9'), { kind: 'bound', device: only, remember: true });
  deepEqual(bindDevice([only], 'W-1'), { kind: 'bound', device: only, remember: false });
});

test('有几台：本机记着的那台还在册就用它', () => {
  const first = warehouseDevice('W-1');
  const second = warehouseDevice('W-2', 'SYN-WH-02');
  deepEqual(bindDevice([first, second], 'W-2'), { kind: 'bound', device: second, remember: false });
});

test('有几台而本机没记着、或记着的已不在册：让人选，不替人挑第一台', () => {
  const first = warehouseDevice('W-1');
  const second = warehouseDevice('W-2', 'SYN-WH-02');
  deepEqual(bindDevice([first, second], ''), { kind: 'choose', devices: [first, second] });
  deepEqual(bindDevice([first, second], 'W-9'), { kind: 'choose', devices: [first, second] });
});

test('挑设备时先去掉非仓库设备：一台仓库设备加一台别的范围的设备，仍算只有一台', () => {
  const only = warehouseDevice('W-1');
  deepEqual(bindDevice([nodeDevice('N-1'), only], ''), { kind: 'bound', device: only, remember: true });
});
