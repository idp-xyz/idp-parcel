import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { numberlessNote, primaryReference } from './document-number';

// 本文件钉单据号的呈现：有号显号；没号照旧显内部标识，并按读口给的原因说是上线前开立还是时区未配置，不一律写成前者。

test('无号原因两种情形各说各的，读口没给原因不替它猜', () => {
  equal(numberlessNote('ISSUED_BEFORE_NUMBERING'), '上线前开立，无单据号');
  equal(numberlessNote('NUMBERING_TIME_ZONE_NOT_CONFIGURED'), '开立时单据编号时区未配置，无单据号');
  equal(numberlessNote(undefined), '无单据号');
  equal(numberlessNote('SOMETHING_NEW'), '无单据号');
});

test('主列：有号显号，没号照旧显内部标识并带上原因', () => {
  deepEqual(primaryReference('EX-20261102-0003', 'XCS-01ABC', undefined), { text: 'EX-20261102-0003', numbered: true });
  deepEqual(primaryReference(undefined, 'XCS-01ABC', 'NUMBERING_TIME_ZONE_NOT_CONFIGURED'), {
    text: 'XCS-01ABC',
    numbered: false,
    hint: '开立时单据编号时区未配置，无单据号',
  });
  deepEqual(primaryReference('', 'XCS-01ABC', 'ISSUED_BEFORE_NUMBERING'), {
    text: 'XCS-01ABC',
    numbered: false,
    hint: '上线前开立，无单据号',
  });
});
