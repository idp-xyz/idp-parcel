import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import { outcomeTechnicalRows } from './answer-rows';

test('登记答复的码三格：答复码必在，理由码与在等的原因码缺席时留空', () => {
  deepEqual(outcomeTechnicalRows('REGISTERED'), [
    { label: '答复码', value: 'REGISTERED' },
    { label: '拒绝理由码', value: '' },
    { label: '在等的原因码', value: '' },
  ]);
  deepEqual(outcomeTechnicalRows('REFUSED', 'VERSION_NOT_OVERWRITABLE', 'WAITING_FOR_X').map((row) => row.value), [
    'REFUSED',
    'VERSION_NOT_OVERWRITABLE',
    'WAITING_FOR_X',
  ]);
});
