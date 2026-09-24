import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { identityLayerCellsOf, lifetimeNumbersText } from './legal-entity-identity';

// 本文件钉身份两格的呈现（票 legal-entity-profile/04）：登了就照答复原样示出，没登（修订早于身份层落地）交回 null
// 让页面如实说，不拿空串或占位顶格。

// Covers: 登了身份层 → 国家码原样、号按「类型码 号」逐个列出，多号用中文分号隔开，次序照答复。
test('登了身份层：国家与号照答复原样', () => {
  deepEqual(
    identityLayerCellsOf({
      identityLayerRegistered: true,
      registrationCountry: 'CN',
      lifetimeRegistrationNumbers: [{ typeCode: 'USCC', number: '91000000000000001X' }],
    }),
    { country: 'CN', numbers: 'USCC 91000000000000001X' },
  );
  equal(
    lifetimeNumbersText([
      { typeCode: 'SYN-B', number: '2' },
      { typeCode: 'SYN-A', number: '1' },
    ]),
    'SYN-B 2；SYN-A 1',
  );
});

// Covers: identityLayerRegistered 为假 → null（不看其余键）；为真而键缺席 → 空串照答复，不补假值。
test('没登身份层交回 null，缺键不补', () => {
  equal(identityLayerCellsOf({ identityLayerRegistered: false }), null);
  equal(identityLayerCellsOf({ identityLayerRegistered: false, registrationCountry: 'CN' }), null);
  deepEqual(identityLayerCellsOf({ identityLayerRegistered: true }), { country: '', numbers: '' });
});
