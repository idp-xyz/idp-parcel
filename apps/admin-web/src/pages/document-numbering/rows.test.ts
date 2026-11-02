import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import type { DocumentNumberingReadBody, DocumentTypeRecord } from './api';
import {
  countRuleRows,
  dateZoneOf,
  documentTypeNameOf,
  effectiveFromProblem,
  failureWords,
  filterRuleRows,
  insertToken,
  nextMonthStartWallTime,
  numberingZoneStateOf,
  previewViewOf,
  registrantWords,
  ruleHistoryOf,
  ruleRegistrationAnswerOf,
  ruleRowsOf,
  templateProblem,
  templateReading,
  templateSegmentsOf,
  timeZoneHistoryOf,
  timeZoneOptionsOf,
  timeZoneRegistrationAnswerOf,
  tokenWithDigits,
  wallTimeIn,
  zoneWords,
} from './rows';

// 本文件钉「设置 → 单据编号」页的纯逻辑：格式读给人听、行与来源、日期时区那一句、历次版本的处境、表单的本地检查与
// 各口答复的话。答复形状取 docnumberhttp 包注释的契约。

const at = '2026-11-02T08:00:00Z';

function typeRecord(overrides: Partial<DocumentTypeRecord> = {}): DocumentTypeRecord {
  return {
    documentType: 'SHIPMENT',
    defaultTemplate: 'P{yyyy}{mm}{dd}{seq:6}',
    source: 'PRODUCT_DEFAULT',
    template: 'P{yyyy}{mm}{dd}{seq:6}',
    resetPeriod: 'DAILY',
    dateTimeZone: { source: 'TENANT_NUMBERING_TIME_ZONE', numberingTimeZoneStatus: 'CONFIGURED', numberingTimeZone: 'Asia/Shanghai' },
    sample: 'P20261102000001',
    versions: [],
    ...overrides,
  };
}

function readBody(overrides: Partial<DocumentNumberingReadBody> = {}): DocumentNumberingReadBody {
  return {
    outcome: 'DOCUMENT_NUMBERING_READ',
    at,
    numberingTimeZone: { status: 'CONFIGURED', timeZone: 'Asia/Shanghai', effectiveFrom: '2026-01-01T00:00:00Z' },
    numberingTimeZoneVersions: [],
    documentTypes: [typeRecord()],
    ...overrides,
  };
}

// Covers: 十一类单据都有业务叫法，委托的号叫「运单号」（ADR-0355 决定十一）；词典没有的类型如实露码、不冒充中文。
test('单据类型的叫法：十一类都有，查不到的如实标出', () => {
  for (const code of [
    'SHIPMENT',
    'CUSTOMER_STATEMENT',
    'STATEMENT_DISPUTE',
    'AUDITED_PAYABLE',
    'EXCEPTION_CASE',
    'CLAIM',
    'CUSTOMS_CASE',
    'DELIVERY_PRE_ADVICE',
    'DELIVERY_HANDOVER_NOTE',
    'BOOKING',
    'TRANSPORT_COMMISSION',
  ]) {
    const name = documentTypeNameOf(code);
    ok(name.known, `${code} 没有叫法`);
    equal(/[A-Za-z]/.test(name.name + name.numberName), false, `${code} 的叫法含英文`);
  }
  equal(documentTypeNameOf('SHIPMENT').numberName, '运单号');
  deepEqual(documentTypeNameOf('NEW_KIND'), { name: 'NEW_KIND', numberName: '单据号', known: false });
});

// Covers: 格式拆成字面与部件读给人听；相邻日期部件并成一组；没写位数的加扰流水按平台的 6 位读；认不出的部件原样露出、不判错。
test('格式读给人听', () => {
  equal(templateReading('P{yyyy}{mm}{dd}{seq:6}'), 'P · 年 月 日 · 6 位流水');
  equal(templateReading('STMT-{yyyy}{mm}-{seq:5}'), 'STMT- · 年 月 · - · 5 位流水');
  equal(templateReading('S{yy}{mm}{seqx}'), 'S · 年（两位） 月 · 6 位加扰流水');
  equal(templateReading('Q{q}-{seq}'), 'Q · 季 · - · 流水');
  const segments = templateSegmentsOf('A{nope}{seq:3}');
  deepEqual(segments[1], { kind: 'token', raw: '{nope}', label: '{nope}', date: false, known: false });
  deepEqual(templateSegmentsOf(''), []);
});

// Covers: 读口答复成行；来源、重置与样例照搬；筛选按来源与检索词（检索认单据名、号名、样例与格式读法）。
test('行与筛选', () => {
  const body = readBody({
    documentTypes: [
      typeRecord(),
      typeRecord({
        documentType: 'CUSTOMER_STATEMENT',
        source: 'TENANT_RULE',
        template: 'ST{yyyy}{mm}{seq:4}',
        effectiveFrom: '2026-10-01T00:00:00Z',
        resetPeriod: 'MONTHLY',
        dateTimeZone: { source: 'ISSUER_BUSINESS_TIME_ZONE', numberingTimeZoneStatus: 'CONFIGURED', numberingTimeZone: 'Asia/Shanghai' },
        sample: 'ST2026110001',
      }),
    ],
  });
  const rows = ruleRowsOf(body);
  equal(rows[0].type.name, '运单');
  equal(rows[0].reset.short, '按日');
  equal(rows[1].reset.short, '按月');
  equal(filterRuleRows(rows, { search: '', source: 'TENANT_RULE' }).length, 1);
  equal(filterRuleRows(rows, { search: '对账单号', source: 'ALL' })[0].code, 'CUSTOMER_STATEMENT');
  equal(filterRuleRows(rows, { search: 'p2026', source: 'ALL' })[0].code, 'SHIPMENT');
  equal(filterRuleRows(rows, { search: '流水', source: 'PRODUCT_DEFAULT' }).length, 1);
  deepEqual(countRuleRows(rows), { total: 2, tenantRule: 1, productDefault: 1, withoutNumber: 0 });
});

// Covers: 日期所用时区——开立方没有业务时区的类型在单据编号时区未配置时「不带号」（ADR-0355 决定七）；开立方有业务时区的
// 说清是网点还是结算账户，未配置时不算整类不带号。
test('日期所用时区那一句', () => {
  const tenantMissing = dateZoneOf(typeRecord({ dateTimeZone: { source: 'TENANT_NUMBERING_TIME_ZONE', numberingTimeZoneStatus: 'NOT_CONFIGURED' } }));
  equal(tenantMissing.withoutNumber, true);
  equal(tenantMissing.short, '单据编号时区未配置');
  ok(tenantMissing.sentence.includes('不带单据号'));

  const issuer = dateZoneOf(
    typeRecord({
      documentType: 'DELIVERY_PRE_ADVICE',
      dateTimeZone: { source: 'ISSUER_BUSINESS_TIME_ZONE', numberingTimeZoneStatus: 'NOT_CONFIGURED' },
    }),
  );
  equal(issuer.withoutNumber, false);
  equal(issuer.short, '网点的业务时区');
  equal(dateZoneOf(typeRecord({ documentType: 'CUSTOMER_STATEMENT', dateTimeZone: { source: 'ISSUER_BUSINESS_TIME_ZONE', numberingTimeZoneStatus: 'CONFIGURED', numberingTimeZone: 'UTC' } })).short, '结算账户的业务时区');
  equal(dateZoneOf(typeRecord({ documentType: 'LATER_KIND', dateTimeZone: { source: 'ISSUER_BUSINESS_TIME_ZONE', numberingTimeZoneStatus: 'CONFIGURED', numberingTimeZone: 'UTC' } })).short, '开立方的业务时区');

  const rows = ruleRowsOf(
    readBody({
      numberingTimeZone: { status: 'NOT_CONFIGURED' },
      documentTypes: [typeRecord({ sample: undefined, dateTimeZone: { source: 'TENANT_NUMBERING_TIME_ZONE', numberingTimeZoneStatus: 'NOT_CONFIGURED' } })],
    }),
  );
  equal(countRuleRows(rows).withoutNumber, 1);
});

// Covers: 历次版本新的在上、最底下垫产品默认；此刻生效 / 未生效 / 已替代与服务端同一判法（生效时刻不晚于此刻的最后一版）。
test('历次规则版本的处境', () => {
  const record = typeRecord({
    source: 'TENANT_RULE',
    template: 'B{yyyy}{mm}{dd}{seq:5}',
    versions: [
      { template: 'A{yyyy}{mm}{dd}{seq:5}', effectiveFrom: '2026-09-01T00:00:00Z', registeredBy: 'OPERATOR/https://issuer.example#ops-1', registeredAt: '2026-08-30T00:00:00Z' },
      { template: 'B{yyyy}{mm}{dd}{seq:5}', effectiveFrom: '2026-10-01T00:00:00Z', registeredBy: 'OPERATOR/https://issuer.example#ops-1', registeredAt: '2026-09-30T00:00:00Z' },
      { template: 'C{yyyy}{mm}{dd}{seq:5}', effectiveFrom: '2026-12-01T00:00:00Z', registeredBy: 'OPERATOR/https://issuer.example#ops-2', registeredAt: '2026-11-01T00:00:00Z' },
    ],
  });
  const history = ruleHistoryOf(record, at);
  deepEqual(
    history.map((entry) => [entry.template[0], entry.standing]),
    [
      ['C', 'NOT_YET_EFFECTIVE'],
      ['B', 'IN_FORCE'],
      ['A', 'SUPERSEDED'],
      ['P', 'SUPERSEDED'],
    ],
  );
  equal(history[3].productDefault, true);

  // 本公司登的版本都还没到生效时刻：产品默认此刻生效。
  const future = ruleHistoryOf(typeRecord({ versions: [record.versions[2]] }), at);
  deepEqual(future.map((entry) => entry.standing), ['NOT_YET_EFFECTIVE', 'IN_FORCE']);
});

// Covers: 单据编号时区——已配置答中文名；未配置但已登了将来才生效的一版，如实说出那一版。
test('单据编号时区此刻的处境', () => {
  const configured = numberingZoneStateOf(readBody());
  equal(configured.configured, true);
  equal(configured.currentZone, 'Asia/Shanghai');
  ok(configured.current && configured.current !== '');

  const upcoming = numberingZoneStateOf(
    readBody({
      numberingTimeZone: { status: 'NOT_CONFIGURED' },
      numberingTimeZoneVersions: [{ timeZone: 'Asia/Shanghai', effectiveFrom: '2026-12-01T00:00:00Z', registeredBy: 'x', registeredAt: at }],
    }),
  );
  deepEqual(upcoming, { configured: false, upcoming: { zone: 'Asia/Shanghai', effectiveFrom: '2026-12-01T00:00:00Z' } });
  deepEqual(numberingZoneStateOf(readBody({ numberingTimeZone: { status: 'NOT_CONFIGURED' } })), { configured: false });

  const history = timeZoneHistoryOf(
    [
      { timeZone: 'UTC', effectiveFrom: '2026-01-01T00:00:00Z', registeredBy: 'x', registeredAt: at },
      { timeZone: 'Asia/Shanghai', effectiveFrom: '2026-06-01T00:00:00Z', registeredBy: 'x', registeredAt: at },
    ],
    at,
  );
  deepEqual(history.map((entry) => [entry.timeZone, entry.standing]), [
    ['Asia/Shanghai', 'IN_FORCE'],
    ['UTC', 'SUPERSEDED'],
  ]);
});

// Covers: 登记人——受控登记命令不露命令名，操作者取短名。
test('登记人的叫法', () => {
  equal(registrantWords('CONTROLLED-CLI/parcel-document-numbering-register'), '受控登记命令');
  equal(registrantWords('OPERATOR/https://dex.example/dex#syn-operator-1'), 'syn-operator-1');
});

// Covers: 部件按光标插入（有选区替换选区），带 N 的写法按位数落成实写法。
test('插入部件', () => {
  deepEqual(insertToken('P-', 1, 1, '{yyyy}'), { text: 'P{yyyy}-', caret: 7 });
  deepEqual(insertToken('PXX', 1, 3, '{dd}'), { text: 'P{dd}', caret: 5 });
  deepEqual(insertToken('P', 99, 99, '{mm}'), { text: 'P{mm}', caret: 5 });
  equal(tokenWithDigits('{seq:N}', 5), '{seq:5}');
  equal(tokenWithDigits('{seqx:N}', 8), '{seqx:8}');
  equal(tokenWithDigits('{yyyy}', 5), '{yyyy}');
});

// Covers: 表单本地只拦「没填」「与此刻相同」「时刻写不成」三样；格式合不合法交给预览口。
test('表单的本地检查', () => {
  equal(templateProblem('  ', 'P{seq}'), '填一个格式');
  equal(templateProblem('P{seq}', 'P{seq}'), '和此刻用的格式一样，不用再登记');
  equal(templateProblem('P{nope', 'P{seq}'), null);
  equal(effectiveFromProblem('', null), '填从什么时候起用');
  equal(effectiveFromProblem('2026-13-01T00:00', null), '时刻写法不对');
  equal(effectiveFromProblem('2026-12-01T00:00', '2026-11-30T16:00:00Z'), null);
});

// Covers: 墙钟按给定时区算，下月 1 日零点跨年进位。
test('墙钟与下月初', () => {
  const instant = new Date('2026-12-31T17:30:00Z');
  equal(wallTimeIn(instant, 'Asia/Shanghai'), '2027-01-01T01:30');
  equal(wallTimeIn(instant, 'UTC'), '2026-12-31T17:30');
  equal(nextMonthStartWallTime(instant, 'UTC'), '2027-01-01T00:00');
  equal(nextMonthStartWallTime(instant, 'Asia/Shanghai'), '2027-02-01T00:00');
});

// Covers: 预览三种答复各说各的话；拒因经词典译中文（含跨周期自撞），重叠照搬；没形成答案时不露码。
test('预览答复的话', () => {
  const sample = previewViewOf({
    kind: 'outcome',
    status: 200,
    body: {
      outcome: 'PREVIEW_RENDERED',
      sample: 'X202612000001',
      timeZone: 'Asia/Shanghai',
      at: '2026-12-01T00:00:00+08:00',
      resetPeriod: 'MONTHLY',
      overlaps: [{ source: 'PRODUCT_DEFAULT', template: 'P{yyyy}{mm}{dd}{seq:6}' }],
    },
  });
  equal(sample.kind, 'sample');
  if (sample.kind === 'sample') {
    equal(sample.reset.short, '按月');
    equal(sample.overlaps.length, 1);
  }

  const rejected = previewViewOf({ kind: 'outcome', status: 200, body: { outcome: 'PREVIEW_REJECTED', reason: 'TEMPLATE_REPEATS_ACROSS_PERIODS' } });
  equal(rejected.kind, 'rejected');
  if (rejected.kind === 'rejected') {
    ok(rejected.words.includes('不同周期'));
    equal(/[A-Z]{3,}/.test(rejected.words), false);
  }

  const noZone = previewViewOf({ kind: 'outcome', status: 200, body: { outcome: 'NUMBERING_TIME_ZONE_NOT_CONFIGURED', resetPeriod: 'DAILY' } });
  deepEqual(noZone, { kind: 'noZone', reset: { short: '按日', sentence: '流水每天从 1 起重新数' } });

  const failed = previewViewOf({ kind: 'noAnswer', status: 500, code: 'NO_ANSWER_FORMED' });
  equal(failed.kind, 'failed');
  if (failed.kind === 'failed') {
    equal(failed.words.includes('NO_ANSWER_FORMED'), false);
    deepEqual(failed.technical, [
      { label: 'HTTP 状态', value: '500' },
      { label: '问题码', value: 'NO_ANSWER_FORMED' },
    ]);
  }
});

// Covers: 登记口答复——新登、已在册、生效时刻被占三种业务答案，与 400 拒因、无权、未配置的话；码不进句子。
test('登记答复的话', () => {
  const shipment = documentTypeNameOf('SHIPMENT');
  const version = { documentType: 'SHIPMENT', template: 'W{yyyy}{mm}{dd}{seq:6}', effectiveFrom: '2026-12-01T00:00:00Z', registeredBy: 'x', registeredAt: at };
  const registered = ruleRegistrationAnswerOf({ kind: 'outcome', status: 201, body: { outcome: 'VERSION_REGISTERED', version } }, shipment);
  equal(registered.landed, true);
  ok(registered.words.includes('新开立的运单按新格式出运单号'));
  equal(ruleRegistrationAnswerOf({ kind: 'outcome', status: 200, body: { outcome: 'VERSION_EXISTING', version } }, shipment).landed, true);
  const taken = ruleRegistrationAnswerOf({ kind: 'outcome', status: 200, body: { outcome: 'EFFECTIVE_FROM_TAKEN', version } }, shipment);
  equal(taken.landed, false);
  ok(taken.words.includes('换一个生效时刻'));

  const refused = ruleRegistrationAnswerOf({ kind: 'callerProblem', status: 400, code: 'SCRAMBLE_SECRET_NOT_CONFIGURED' }, shipment);
  equal(refused.landed, false);
  ok(refused.words.includes('加扰密钥'));
  deepEqual(refused.technical[1], { label: '问题码', value: 'SCRAMBLE_SECRET_NOT_CONFIGURED' });

  ok(failureWords({ kind: 'notGranted' }, '登记新格式').startsWith('登记新格式：没有授予'));
  ok(failureWords({ kind: 'unconfigured' }, '读取').includes('接入渠道尚未配置'));

  const zone = timeZoneRegistrationAnswerOf({
    kind: 'outcome',
    status: 201,
    body: { outcome: 'VERSION_REGISTERED', version: { timeZone: 'Asia/Shanghai', effectiveFrom: '2026-01-01T00:00:00Z', registeredBy: 'x', registeredAt: at } },
  });
  equal(zone.landed, true);
  ok(zone.words.includes('不补号'));
  ok(timeZoneRegistrationAnswerOf({ kind: 'callerProblem', status: 400, code: 'TIME_ZONE_NOT_RECOGNIZED' }).words.includes('认不出这个时区名'));
});

// Covers: 时区候选按偏移再按名排、补上裸 UTC、不预选；每项留着 IANA 名以分开同名时区。
test('时区候选', () => {
  const offsets: Record<string, number> = { 'Asia/Shanghai': 480, 'Europe/London': 0, 'America/New_York': -300, UTC: 0 };
  const options = timeZoneOptionsOf(['Asia/Shanghai', 'Europe/London', 'America/New_York'], (zone) => `偏移${offsets[zone]}`, (zone) => offsets[zone]);
  deepEqual(
    options.map((option) => option.zone),
    ['America/New_York', 'Europe/London', 'UTC', 'Asia/Shanghai'],
  );
  ok(options.every((option) => option.label.includes(option.zone)));
});

// Covers: 时区的中文叫法按此刻取；认不出的区名与 ICU 只给得出偏移串的区原样交回 IANA 名，不把「GMT-11:00」当名字。
test('时区的中文叫法', () => {
  equal(zoneWords('Asia/Shanghai', new Date('2026-11-02T00:00:00Z')), '中国标准时间');
  equal(zoneWords('Not/AZone'), 'Not/AZone');
  equal(zoneWords(undefined), '—');
  for (const zone of ['Pacific/Midway', 'America/Nome', 'Etc/GMT+5']) {
    const words = zoneWords(zone);
    ok(words === zone || !/^(GMT|UTC)[+-]/.test(words), `${zone} 答了偏移串「${words}」`);
  }
});
