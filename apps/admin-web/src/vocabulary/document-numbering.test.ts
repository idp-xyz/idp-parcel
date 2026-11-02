import { test } from 'node:test';
import { deepEqual, equal, match, ok } from 'node:assert/strict';
import {
  documentNumberAbsenceNote,
  documentTypeWords,
  numberingCollisionNote,
  registrationOutcomeWords,
  registrationRejectionWords,
  resetPeriodWords,
  templateTokenGuide,
} from './document-numbering';
import { labelFor, toneFor } from './status';

// 本文件钉单据编号一册的覆盖面：服务端各封闭集的每个码都有中文，界面词里不漏英文（部件写法如 `{yyyy}` 是填格式的人
// 要照着写的字，不算码）。码表取自 internal/platform/docnumber 与 docnumberhttp 的 reasonCodes。

const lettersOutsideTokens = (text: string) => /[A-Za-z]/.test(text.replace(/\{[a-z0-9:N]+\}/g, ''));

// Covers: 十一类单据（ADR-0355 决定二）都有单据名与号名，且是中文。
test('十一类单据都有中文叫法', () => {
  deepEqual(Object.keys(documentTypeWords).sort(), [
    'AUDITED_PAYABLE',
    'BOOKING',
    'CLAIM',
    'CUSTOMER_STATEMENT',
    'CUSTOMS_CASE',
    'DELIVERY_HANDOVER_NOTE',
    'DELIVERY_PRE_ADVICE',
    'EXCEPTION_CASE',
    'SHIPMENT',
    'STATEMENT_DISPUTE',
    'TRANSPORT_COMMISSION',
  ]);
  for (const [code, words] of Object.entries(documentTypeWords)) {
    equal(/[A-Za-z]/.test(words.name + words.numberName), false, `${code} 的叫法含英文`);
  }
});

// Covers: 重置周期五值、登记口三种结果、登记拒因（含 TEMPLATE_REPEATS_ACROSS_PERIODS 与 MALFORMED_REQUEST）都有中文。
test('各封闭集的码都译了', () => {
  deepEqual(Object.keys(resetPeriodWords).sort(), ['CONTINUOUS', 'DAILY', 'MONTHLY', 'QUARTERLY', 'YEARLY']);
  deepEqual(Object.keys(registrationOutcomeWords).sort(), ['EFFECTIVE_FROM_TAKEN', 'VERSION_EXISTING', 'VERSION_REGISTERED']);
  for (const code of [
    'DOCUMENT_TYPE_NOT_DECLARED',
    'TEMPLATE_UNPARSEABLE',
    'TEMPLATE_WITHOUT_SEQUENCE',
    'TEMPLATE_NEEDS_UNSUPPLIED_VALUE',
    'TEMPLATE_UNRENDERABLE',
    'TEMPLATE_REPEATS_ACROSS_PERIODS',
    'SCRAMBLE_SECRET_NOT_CONFIGURED',
    'TIME_ZONE_NOT_RECOGNIZED',
    'MALFORMED_REQUEST',
  ]) {
    const words = registrationRejectionWords[code];
    ok(words !== undefined && words.trim() !== '', `${code} 没有中文说明`);
    equal(lettersOutsideTokens(words), false, `${code} 的说明「${words}」含英文`);
  }
  for (const words of [...Object.values(resetPeriodWords).flatMap((entry) => [entry.short, entry.sentence]), ...Object.values(registrationOutcomeWords)]) {
    equal(/[A-Za-z]/.test(words), false, `「${words}」含英文`);
  }
});

// Covers: 部件说明覆盖派单点名的年、月、日、季、流水位数与加扰流水，说明是中文。
test('部件说明', () => {
  const tokens = templateTokenGuide.map((entry) => entry.token);
  for (const token of ['{yyyy}', '{mm}', '{dd}', '{q}', '{seq:N}', '{seqx:N}']) ok(tokens.includes(token), `缺 ${token}`);
  for (const entry of templateTokenGuide) {
    equal(lettersOutsideTokens(entry.label + (entry.note ?? '')), false, `${entry.token} 的说明含英文`);
  }
});

// Covers: 规则来源胶囊在词典分册里，产品默认不是警示色。
test('规则来源胶囊', () => {
  equal(labelFor('PRODUCT_DEFAULT', 'document-numbering-source'), '产品默认');
  equal(labelFor('TENANT_RULE', 'document-numbering-source'), '本公司规则');
  equal(toneFor('PRODUCT_DEFAULT', 'document-numbering-source'), 'muted');
});

test('无号原因两种说法分开，认不出的原因码如实露出', () => {
  equal(documentNumberAbsenceNote('ISSUED_BEFORE_NUMBERING'), '上线前开立，无单据号');
  equal(documentNumberAbsenceNote('NUMBERING_TIME_ZONE_NOT_CONFIGURED'), '开立时单据编号时区未配置，无单据号');
  match(documentNumberAbsenceNote('SOMETHING_NEW'), /SOMETHING_NEW/);
});

test('撞号说法带上那一类的号名，认不出类型就泛说单据号', () => {
  match(numberingCollisionNote('BOOKING'), /订舱号与已有的号重复/);
  match(numberingCollisionNote('TRANSPORT_COMMISSION'), /委托号/);
  match(numberingCollisionNote(), /单据号与已有的号重复/);
  match(numberingCollisionNote('BOOKING'), /设置 → 单据编号/);
});
