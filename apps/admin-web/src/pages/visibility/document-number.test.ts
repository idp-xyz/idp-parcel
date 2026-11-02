import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { absenceText, caseNumberName, claimNumberName, primaryReference } from './document-number';

// 本文件钉单据号的呈现：有号显号；没号照旧显内部标识，并按读口给的原因说是上线前开立还是时区未配置，不一律写成前者；
// 号名与无号说法取共用词典。

test('号名取共用词典', () => {
  equal(caseNumberName, '案件号');
  equal(claimNumberName, '索赔号');
});

test('无号原因按读口答的说；读口没带原因（引用别的单据时）只说没有号', () => {
  equal(absenceText('ISSUED_BEFORE_NUMBERING'), '上线前开立，无单据号');
  equal(absenceText('NUMBERING_TIME_ZONE_NOT_CONFIGURED'), '开立时单据编号时区未配置，无单据号');
  equal(absenceText(undefined), '无单据号');
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
