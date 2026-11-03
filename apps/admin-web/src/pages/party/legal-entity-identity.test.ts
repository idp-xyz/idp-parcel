import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { RegistrationNumberTypeRecord } from './api';
import { identityLayerCellsOf, lifetimeNumberCells, lifetimeNumbersText, registrationNumberTypeNamesOf } from './legal-entity-identity';

// 本文件钉身份两格的呈现（票 legal-entity-profile/04）：登了就照答复原样示出，没登（修订早于身份层落地）交回 null
// 让页面如实说，不拿空串或占位顶格；类型码经目录译名是调用方交进来的查法，不交就显码（票 company-subject-ux/01 第 3 条）。
// 逐号拆开的 numberCells 给头部胶囊一号一枚用（票 company-subject-ux/07 第 3 条），与拼成一段的 numbers 同一条译名规则。

// Covers: 登了身份层 → 国家码原样、号按「类型码 号」逐个列出，多号用中文分号隔开，次序照答复；逐号拆开的格同一次序。
test('登了身份层：国家与号照答复原样', () => {
  deepEqual(
    identityLayerCellsOf({
      identityLayerRegistered: true,
      registrationCountry: 'CN',
      lifetimeRegistrationNumbers: [{ typeCode: 'USCC', number: '91000000000000001X' }],
    }),
    { country: 'CN', numbers: 'USCC 91000000000000001X', numberCells: [{ typeName: 'USCC', number: '91000000000000001X' }] },
  );
  equal(
    lifetimeNumbersText([
      { typeCode: 'SYN-B', number: '2' },
      { typeCode: 'SYN-A', number: '1' },
    ]),
    'SYN-B 2；SYN-A 1',
  );
  deepEqual(
    lifetimeNumberCells([
      { typeCode: 'SYN-B', number: '2' },
      { typeCode: 'SYN-A', number: '1' },
    ]),
    [
      { typeName: 'SYN-B', number: '2' },
      { typeName: 'SYN-A', number: '1' },
    ],
  );
});

// Covers: identityLayerRegistered 为假 → null（不看其余键）；为真而键缺席 → 空串照答复，不补假值，逐号的格也是空的。
test('没登身份层交回 null，缺键不补', () => {
  equal(identityLayerCellsOf({ identityLayerRegistered: false }), null);
  equal(identityLayerCellsOf({ identityLayerRegistered: false, registrationCountry: 'CN' }), null);
  deepEqual(identityLayerCellsOf({ identityLayerRegistered: true }), { country: '', numbers: '', numberCells: [] });
});

function numberType(over: Partial<RegistrationNumberTypeRecord>): RegistrationNumberTypeRecord {
  return {
    tenantId: 'SYN-TENANT-01',
    countryCode: 'CN',
    typeCode: 'USCC',
    revision: 1,
    typeName: '统一社会信用代码',
    layer: 'IDENTITY',
    formatPattern: '^.{18}$',
    basis: 'SYN-BASIS',
    status: 'EFFECTIVE',
    effectiveFrom: '2026-01-01T00:00:00Z',
    registeredAt: '2026-01-01T00:00:00Z',
    ...over,
  };
}

// Covers: 交了目录查法 → 类型码译成目录里的 typeName，号与分隔照旧；目录里没有的码（历史修订上的旧码）与名为空串的
// 类型原样显码，不抛也不留空；目录按国家分键——同一个码在别国登的名不拿来译本国的号。
test('类型码经目录译名，译不出原样显码', () => {
  const typeNameOf = registrationNumberTypeNamesOf([
    numberType({}),
    numberType({ countryCode: 'US', typeCode: 'EIN', typeName: '雇主识别号' }),
    numberType({ countryCode: 'SG', typeCode: 'USCC', typeName: '新加坡同码异名' }),
    numberType({ typeCode: 'SYN-BLANK', typeName: '' }),
  ]);
  deepEqual(
    identityLayerCellsOf(
      {
        identityLayerRegistered: true,
        registrationCountry: 'CN',
        lifetimeRegistrationNumbers: [
          { typeCode: 'USCC', number: '91000000000000001X' },
          { typeCode: 'SYN-OLD', number: '7' },
          { typeCode: 'SYN-BLANK', number: '8' },
        ],
      },
      typeNameOf,
    ),
    {
      country: 'CN',
      numbers: '统一社会信用代码 91000000000000001X；SYN-OLD 7；SYN-BLANK 8',
      numberCells: [
        { typeName: '统一社会信用代码', number: '91000000000000001X' },
        { typeName: 'SYN-OLD', number: '7' },
        { typeName: 'SYN-BLANK', number: '8' },
      ],
    },
  );
  equal(typeNameOf('US', 'EIN'), '雇主识别号');
  equal(typeNameOf('US', 'USCC'), undefined);
  equal(typeNameOf('SG', 'USCC'), '新加坡同码异名');
  // 没登身份层的行不因交了查法而变：仍是 null。
  equal(identityLayerCellsOf({ identityLayerRegistered: false }, typeNameOf), null);
});

// Covers: 同一国同一码两层都登了时取身份层的名，与目录行的先后无关；已停用的类型照样译。
test('同码两层取身份层，已停用照译', () => {
  const profileFirst = registrationNumberTypeNamesOf([
    numberType({ layer: 'PROFILE', typeName: '资料层同码' }),
    numberType({ typeName: '身份层同码', deactivatedAt: '2026-06-01T00:00:00Z', deactivationBasis: 'SYN-DEACT' }),
  ]);
  const identityFirst = registrationNumberTypeNamesOf([
    numberType({ typeName: '身份层同码' }),
    numberType({ layer: 'PROFILE', typeName: '资料层同码' }),
  ]);
  equal(profileFirst('CN', 'USCC'), '身份层同码');
  equal(identityFirst('CN', 'USCC'), '身份层同码');
  // 只登了资料层的码也译：查法不替调用方判断该不该译，它只答目录里叫什么。
  equal(registrationNumberTypeNamesOf([numberType({ layer: 'PROFILE', typeName: '资料层独有' })])('CN', 'USCC'), '资料层独有');
});
