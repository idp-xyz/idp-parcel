import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { entrySourceLine, entrySourceWord, manualTrace } from './provenance';

// 来源两格只由读面的留痕在不在场决定：在场即人工录入并带出谁与原件，缺席即接口送达。

test('留痕缺席读作接口送达，不带任何人名与原件', () => {
  equal(entrySourceWord(undefined), '接口送达');
  equal(entrySourceLine(undefined), '接口送达');
});

test('留痕在场读作人工录入，带出录入人短名与原件', () => {
  const entry = { enteredBy: 'https://idp.syn.example/realms/ops#SYN-OP-7', original: '海关放行通知书.pdf' };
  equal(entrySourceWord(entry), '人工录入');
  equal(entrySourceLine(entry), '人工录入｜SYN-OP-7 录入，原件：海关放行通知书.pdf');
});

test('各环节用自己的动词', () => {
  equal(manualTrace('SYN-OP-2', '通话记录 11-13', '告知'), 'SYN-OP-2 告知，原件：通话记录 11-13');
});
