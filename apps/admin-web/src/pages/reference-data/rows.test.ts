import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { moduleInfoById } from '../../navigation';
import type { CountryRecord, CurrencyRecord, UnitRecord } from './api';
import {
  countByMinorUnitScale,
  countByRegion,
  countByUnitCategory,
  countSummary,
  filterCountries,
  filterCurrencies,
  filterUnits,
  minorUnitScaleText,
  referenceViewState,
  regionLabel,
  shortUnitName,
  unitCategoryLabel,
  unitConversionText,
} from './rows';

const countries: CountryRecord[] = [
  { code: 'CN', nameZh: '中国', nameEn: 'China', region: 'ASIA' },
  { code: 'DE', nameZh: '德国', nameEn: 'Germany', region: 'EUROPE' },
  { code: 'HK', nameZh: '中国香港', nameEn: 'Hong Kong', region: 'ASIA' },
  { code: 'US', nameZh: '美国', nameEn: 'United States', region: 'NORTH_AMERICA' },
];

const currencies: CurrencyRecord[] = [
  { code: 'CNY', nameZh: '人民币', nameEn: 'Chinese Yuan Renminbi', symbol: '¥', minorUnitScale: 2 },
  { code: 'JPY', nameZh: '日元', nameEn: 'Japanese Yen', symbol: '¥', minorUnitScale: 0 },
  { code: 'KWD', nameZh: '科威特第纳尔', nameEn: 'Kuwaiti Dinar', symbol: 'KD', minorUnitScale: 3 },
  { code: 'USD', nameZh: '美元', nameEn: 'United States Dollar', symbol: '$', minorUnitScale: 2 },
];

const units: UnitRecord[] = [
  { code: 'KG', nameZh: '千克', category: 'WEIGHT', baseUnit: 'KG', factorToBase: '1' },
  { code: 'LB', nameZh: '磅', category: 'WEIGHT', baseUnit: 'KG', factorToBase: '0.45359237' },
  { code: 'M3', nameZh: '立方米', category: 'VOLUME', baseUnit: 'M3', factorToBase: '1' },
  { code: 'CBM', nameZh: '立方米（CBM 写法）', category: 'VOLUME', baseUnit: 'M3', factorToBase: '1' },
  { code: 'PCS', nameZh: '件', category: 'COUNT', baseUnit: 'PCS', factorToBase: '1' },
  { code: 'DZ', nameZh: '打', category: 'COUNT', baseUnit: 'PCS', factorToBase: '12' },
];

test('大洲与单位类别经词典译成中文，集外取值原样显示', () => {
  equal(regionLabel('NORTH_AMERICA'), '北美洲');
  equal(regionLabel('ASIA'), '亚洲');
  equal(unitCategoryLabel('WEIGHT'), '重量');
  equal(unitCategoryLabel('COUNT'), '件数');
  equal(regionLabel('ATLANTIS'), 'ATLANTIS');
});

test('国家检索按码或中英文名称、不分大小写，大洲筛选与检索叠加', () => {
  deepEqual(filterCountries(countries, { search: '', region: 'ALL' }).map((row) => row.code), ['CN', 'DE', 'HK', 'US']);
  deepEqual(filterCountries(countries, { search: 'cn', region: 'ALL' }).map((row) => row.code), ['CN']);
  deepEqual(filterCountries(countries, { search: '中国', region: 'ALL' }).map((row) => row.code), ['CN', 'HK']);
  deepEqual(filterCountries(countries, { search: 'united', region: 'ALL' }).map((row) => row.code), ['US']);
  deepEqual(filterCountries(countries, { search: '欧洲', region: 'ALL' }).map((row) => row.code), ['DE']);
  deepEqual(filterCountries(countries, { search: '中国', region: 'EUROPE' }).map((row) => row.code), []);
  deepEqual(filterCountries(countries, { search: '  ', region: 'ASIA' }).map((row) => row.code), ['CN', 'HK']);
});

test('大洲计数按固定次序、只列真出现过的，集外的新取值排在已知之后', () => {
  deepEqual(countByRegion(countries), [
    { value: 'ASIA', label: '亚洲', count: 2 },
    { value: 'EUROPE', label: '欧洲', count: 1 },
    { value: 'NORTH_AMERICA', label: '北美洲', count: 1 },
  ]);
  const withNew = [...countries, { code: 'ZZ', nameZh: '新地', nameEn: 'New', region: 'MARS' }];
  const regions = countByRegion(withNew);
  equal(regions[regions.length - 1].value, 'MARS');
});

test('币种检索含符号；小数位筛选与人话', () => {
  deepEqual(filterCurrencies(currencies, { search: '日元', scale: 'ALL' }).map((row) => row.code), ['JPY']);
  deepEqual(filterCurrencies(currencies, { search: '¥', scale: 'ALL' }).map((row) => row.code), ['CNY', 'JPY']);
  deepEqual(filterCurrencies(currencies, { search: '', scale: '0' }).map((row) => row.code), ['JPY']);
  deepEqual(filterCurrencies(currencies, { search: 'dollar', scale: '2' }).map((row) => row.code), ['USD']);
  equal(minorUnitScaleText(0), '无小数');
  equal(minorUnitScaleText(2), '2 位小数');
  equal(minorUnitScaleText(3), '3 位小数');
  deepEqual(
    countByMinorUnitScale(currencies).map((entry) => [entry.label, entry.count]),
    [['无小数', 1], ['2 位小数', 2], ['3 位小数', 1]],
  );
});

test('单位检索与类别筛选；类别计数按重量、长度、体积、件数的次序', () => {
  deepEqual(filterUnits(units, { search: '磅', category: 'ALL' }).map((row) => row.code), ['LB']);
  deepEqual(filterUnits(units, { search: '体积', category: 'ALL' }).map((row) => row.code), ['M3', 'CBM']);
  deepEqual(filterUnits(units, { search: 'pcs', category: 'COUNT' }).map((row) => row.code), ['PCS']);
  deepEqual(filterUnits(units, { search: '', category: 'WEIGHT' }).map((row) => row.code), ['KG', 'LB']);
  deepEqual(
    countByUnitCategory(units).map((entry) => [entry.label, entry.count]),
    [['重量', 2], ['体积', 2], ['件数', 2]],
  );
});

test('换算一格说人话：基准单位、同值别名、“1 磅 = 0.45359237 千克”，系数原样不取整', () => {
  const text = (code: string) => unitConversionText(units.find((row) => row.code === code) as UnitRecord, units);
  equal(text('KG'), '基准单位');
  equal(text('LB'), '1 磅 = 0.45359237 千克');
  equal(text('CBM'), '与立方米同值');
  equal(text('DZ'), '1 打 = 12 件');
  // 基准单位不在传入的表里（读口只给了一部分）时退回它的码，不抛。
  equal(unitConversionText(units[1], [units[1]]), '1 磅 = 0.45359237 KG');
  equal(shortUnitName('立方米（CBM 写法）'), '立方米');
  equal(shortUnitName('双（对）'), '双');
});

test('计数句：没筛只报总数，筛过报显示 / 共', () => {
  equal(countSummary(249, 249, '个国家和地区'), '共 249 个国家和地区');
  equal(countSummary(249, 3, '个国家和地区'), '显示 3 / 共 249 个国家和地区');
});

test('页面态：空目录不是“还没登记”，未配置与未授予各自一格', () => {
  const module = moduleInfoById['reference-data'];
  const empty = referenceViewState(
    { kind: 'outcome', status: 200, body: { outcome: 'COUNTRIES_LISTED', countries: [] } },
    0,
    () => undefined,
    module,
    'GET /reference-data/countries',
    '国家和地区',
  );
  equal(empty.kind, 'empty');
  const ready = referenceViewState(
    { kind: 'outcome', status: 200, body: { outcome: 'COUNTRIES_LISTED', countries } },
    countries.length,
    () => undefined,
    module,
    'GET /reference-data/countries',
    '国家和地区',
  );
  equal(ready.kind, 'ready');
  equal(
    referenceViewState({ kind: 'unconfigured' } as never, 0, () => undefined, module, 'GET /reference-data/countries', '国家和地区').kind,
    'unconfigured',
  );
  equal(
    referenceViewState({ kind: 'notGranted' } as never, 0, () => undefined, module, 'GET /reference-data/countries', '国家和地区').kind,
    'notGranted',
  );
  equal(referenceViewState(null, 0, () => undefined, module, 'GET /reference-data/countries', '国家和地区').kind, 'loading');
});
