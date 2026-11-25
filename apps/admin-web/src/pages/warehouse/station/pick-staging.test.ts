import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import type { StorageLike } from './device-binding';
import { pickStagingKey, readPickStaging, writePickStaging } from './pick-staging';

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

test('按设备记：这台设备记着的放货位，下次进来读得回', () => {
  const store = memory();
  equal(readPickStaging(store, 'DEV-A'), null);
  writePickStaging(store, 'DEV-A', 'SYN-LOC-OUT-01');
  equal(readPickStaging(store, 'DEV-A'), 'SYN-LOC-OUT-01');
  equal(store.data.get('idp.parcel.station.pick.staging.DEV-A'), 'SYN-LOC-OUT-01');
});

test('各设备各记各的，换了设备不会拿到别的设备的放货位', () => {
  const store = memory();
  writePickStaging(store, 'DEV-A', 'SYN-LOC-OUT-01');
  writePickStaging(store, 'DEV-B', 'SYN-LOC-OUT-02');
  equal(readPickStaging(store, 'DEV-A'), 'SYN-LOC-OUT-01');
  equal(readPickStaging(store, 'DEV-B'), 'SYN-LOC-OUT-02');
  equal(readPickStaging(store, 'DEV-C'), null);
});

test('换放货位就是覆盖', () => {
  const store = memory();
  writePickStaging(store, 'DEV-A', 'SYN-LOC-OUT-01');
  writePickStaging(store, 'DEV-A', 'SYN-LOC-OUT-02');
  equal(readPickStaging(store, 'DEV-A'), 'SYN-LOC-OUT-02');
});

test('空串、没有设备号、存储读不了或写不了：当没记着，不报错', () => {
  equal(readPickStaging(memory({ [pickStagingKey('DEV-A')]: '  ' }), 'DEV-A'), null);
  equal(readPickStaging(memory(), ''), null);
  equal(readPickStaging(null, 'DEV-A'), null);
  const broken: StorageLike = {
    getItem: () => {
      throw new Error('存储被禁用');
    },
    setItem: () => {
      throw new Error('存储被禁用');
    },
  };
  equal(readPickStaging(broken, 'DEV-A'), null);
  writePickStaging(broken, 'DEV-A', 'X');
  writePickStaging(null, 'DEV-A', 'X');
  const store = memory();
  writePickStaging(store, '', 'X');
  equal(store.data.size, 0);
});
