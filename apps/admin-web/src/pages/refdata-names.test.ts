import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import type { ApiResult, CountryListResponseBody, CurrencyListResponseBody } from './reference-data/api';
import {
  countryLabel,
  currencyLabel,
  currentRefdataNames,
  emptyRefdataNames,
  refdataNamesFromAnswers,
  setCurrentRefdataNames,
} from './refdata-names';

const countriesOk: ApiResult<CountryListResponseBody> = {
  kind: 'outcome',
  status: 200,
  body: {
    outcome: 'COUNTRIES_LISTED',
    countries: [
      { code: 'CN', nameZh: '中国', nameEn: 'China', region: 'ASIA' },
      { code: 'SG', nameZh: '新加坡', nameEn: 'Singapore', region: 'ASIA' },
    ],
  },
};

const currenciesOk: ApiResult<CurrencyListResponseBody> = {
  kind: 'outcome',
  status: 200,
  body: {
    outcome: 'CURRENCIES_LISTED',
    currencies: [
      { code: 'CNY', nameZh: '人民币', nameEn: 'Chinese Yuan Renminbi', symbol: '¥', minorUnitScale: 2 },
      { code: 'THB', nameZh: '泰铢', nameEn: 'Thai Baht', symbol: '฿', minorUnitScale: 2 },
    ],
  },
};

test('码译成“名称（码）”，国家与币种各查各的表', () => {
  const { names } = refdataNamesFromAnswers(countriesOk, currenciesOk);
  equal(countryLabel('CN', names), '中国（CN）');
  equal(countryLabel('SG', names), '新加坡（SG）');
  equal(currencyLabel('CNY', names), '人民币（CNY）');
  equal(currencyLabel('THB', names), '泰铢（THB）');
  // 两个码空间互不串：CN 不是币种，CNY 不是国家。
  equal(currencyLabel('CN', names), 'CN');
  equal(countryLabel('CNY', names), 'CNY');
});

test('查不到原样显示：目录外的码、空码、大小写不符的码', () => {
  const { names } = refdataNamesFromAnswers(countriesOk, currenciesOk);
  equal(countryLabel('SYN-COUNTRY-01', names), 'SYN-COUNTRY-01');
  equal(currencyLabel('ZZZ', names), 'ZZZ');
  equal(countryLabel('', names), '');
  equal(countryLabel('cn', names), 'cn');
});

test('目录没读回来时一律按码显示，不把读不到当成目录里没有', () => {
  equal(countryLabel('CN', emptyRefdataNames), 'CN');
  const refused = refdataNamesFromAnswers({ kind: 'notGranted' } as never, { kind: 'unconfigured' } as never);
  equal(refused.complete, false);
  equal(countryLabel('CN', refused.names), 'CN');
  equal(currencyLabel('CNY', refused.names), 'CNY');
});

test('只读回一本时用读回的那本，并标成不完整（不缓存、下次再问）', () => {
  const partial = refdataNamesFromAnswers(countriesOk, { kind: 'transport', message: 'down' });
  equal(partial.complete, false);
  equal(countryLabel('CN', partial.names), '中国（CN）');
  equal(currencyLabel('CNY', partial.names), 'CNY');
  ok(refdataNamesFromAnswers(countriesOk, currenciesOk).complete);
});

test('不传名表时查当前已读回的名表', () => {
  equal(countryLabel('CN'), 'CN');
  setCurrentRefdataNames(refdataNamesFromAnswers(countriesOk, currenciesOk).names);
  try {
    equal(countryLabel('CN'), '中国（CN）');
    equal(currencyLabel('THB'), '泰铢（THB）');
  } finally {
    setCurrentRefdataNames(emptyRefdataNames);
  }
  equal(currentRefdataNames(), emptyRefdataNames);
});
