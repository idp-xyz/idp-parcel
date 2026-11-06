import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  externalReferenceText,
  identificationChangeLines,
  partyIdentificationCellsOf,
  partyIdentificationSearchTerms,
} from './business-party-identification';

const identified = {
  identityLayerRegistered: true,
  registrationCountry: 'CN',
  lifetimeRegistrationNumbers: [{ typeCode: 'USCC', number: '91000000000000009X' }],
  externalReferences: [{ system: 'ERP', purpose: 'CUSTOMER', code: 'C-0001' }],
};

// Covers: 没带号的修订国家为 null、号为空；带了按目录查法译类型名，查不到按码显；用途译成中文。
test('识别信息的读出格', () => {
  const none = partyIdentificationCellsOf({});
  equal(none.country, null);
  deepEqual(none.numbers, []);
  const cells = partyIdentificationCellsOf(identified, (country, code) => (country === 'CN' && code === 'USCC' ? '统一社会信用代码' : undefined));
  equal(cells.country, 'CN');
  deepEqual(cells.numbers, [{ typeName: '统一社会信用代码', number: '91000000000000009X' }]);
  deepEqual(cells.references, [{ system: 'ERP', purpose: '客户编码', code: 'C-0001' }]);
  equal(partyIdentificationCellsOf(identified).numbers[0].typeName, 'USCC');
  equal(externalReferenceText({ system: 'ERP', purpose: 'SUPPLIER', code: 'V-1' }), 'ERP 供应商编码 V-1');
});

// Covers: 检索命中号与外部编码；没带号时只交外部编码。
test('识别信息的检索词', () => {
  deepEqual(partyIdentificationSearchTerms(identified), ['91000000000000009X', 'C-0001', 'ERP C-0001']);
  deepEqual(partyIdentificationSearchTerms({ externalReferences: [{ system: 'ERP', purpose: 'PARTY', code: 'P-1' }] }), ['P-1', 'ERP P-1']);
});

// Covers: 修订记录逐条说号与编码的增删；首笔对 null 比；带更正依据另起一条。
test('两笔修订之间识别信息的变化', () => {
  deepEqual(identificationChangeLines(null, identified), ['注册号新增 CN USCC 91000000000000009X', '外部编码新增 ERP 客户编码 C-0001']);
  deepEqual(
    identificationChangeLines(identified, {
      identityLayerRegistered: true,
      registrationCountry: 'CN',
      lifetimeRegistrationNumbers: [{ typeCode: 'USCC', number: '91000000000000010X' }],
      identityCorrectionBasis: 'SYN-FIX',
      externalReferences: [{ system: 'ERP', purpose: 'CUSTOMER', code: 'C-0001' }],
    }),
    ['注册号新增 CN USCC 91000000000000010X', '注册号去掉 CN USCC 91000000000000009X', '身份更正依据 SYN-FIX'],
  );
  deepEqual(identificationChangeLines(identified, identified), []);
});
