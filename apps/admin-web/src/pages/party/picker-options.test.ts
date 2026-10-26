import { test } from 'node:test';
import { deepEqual, equal, notEqual } from 'node:assert/strict';
import type { ServiceProductListResponseBody } from './api';
import { objectOptionsOf, serviceProductVersionChoices, versionChoiceKey } from './picker-options';

test('一版一行去重成一对象一项，写出各版与在用的那版，按标识排', () => {
  deepEqual(
    objectOptionsOf([
      { objectId: 'SYN-PROD-B', version: 'v1', status: 'RETIRED' },
      { objectId: 'SYN-PROD-A', version: 'v1', status: 'EFFECTIVE' },
      { objectId: 'SYN-PROD-B', version: 'v10', status: 'EFFECTIVE' },
      { objectId: 'SYN-PROD-B', version: 'v2', status: 'RETIRED' },
    ]),
    [
      { value: 'SYN-PROD-A', label: 'SYN-PROD-A · v1 · 在用 v1' },
      { value: 'SYN-PROD-B', label: 'SYN-PROD-B · v10、v2、v1 · 在用 v10' },
    ],
  );
});

test('没有在用的版本就不写在用', () => {
  deepEqual(objectOptionsOf([{ objectId: 'C', version: 'v1', status: 'PUBLISHED' }]), [{ value: 'C', label: 'C · v1' }]);
});

test('对象加版本一起选：一版一项，状态写中文，新版在上', () => {
  const body = {
    products: [
      { objectId: 'P', version: 'v1', status: 'RETIRED' },
      { objectId: 'P', version: 'v2', status: 'EFFECTIVE' },
    ],
  } as unknown as ServiceProductListResponseBody;
  deepEqual(serviceProductVersionChoices(body), [
    { objectId: 'P', version: 'v2', label: 'P · v2 · 已生效' },
    { objectId: 'P', version: 'v1', label: 'P · v1 · 已退役' },
  ]);
});

test('选单键能区分标识里带分隔符的两项', () => {
  notEqual(versionChoiceKey({ objectId: 'A·B', version: 'v1' }), versionChoiceKey({ objectId: 'A', version: 'B·v1' }));
  equal(versionChoiceKey({ objectId: 'A', version: 'v1' }), versionChoiceKey({ objectId: 'A', version: 'v1' }));
});
