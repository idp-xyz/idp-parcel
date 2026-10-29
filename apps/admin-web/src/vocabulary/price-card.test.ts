import { test } from 'node:test';
import { equal, ok } from 'node:assert/strict';
import * as vocabulary from './price-card';

// 价卡内容词册的纪律：每个词都是中文、不漏英文字母。单位与币种例外（开放词表，查不到时原样露码，且表里的词本身仍是中文）。
// 特征与比较词的键对着领域的封闭集合：少一个键，价卡里出现那个特征就会降级成「暂不支持展示」。

test('词册里的每个词都是中文，无英文泄漏', () => {
  const tables: Record<string, Readonly<Record<string, string>>> = {
    featureLabels: vocabulary.featureLabels,
    comparisonLabels: vocabulary.comparisonLabels,
    addressNatureValueLabels: vocabulary.addressNatureValueLabels,
    priceDirectionLabels: vocabulary.priceDirectionLabels,
    priceCardPurposeLabels: vocabulary.priceCardPurposeLabels,
    aggregationLabels: vocabulary.aggregationLabels,
    aggregationSubjectLabels: vocabulary.aggregationSubjectLabels,
    weightUnitLabels: vocabulary.weightUnitLabels,
    lengthUnitLabels: vocabulary.lengthUnitLabels,
    volumeUnitLabels: vocabulary.volumeUnitLabels,
    currencySuffixLabels: vocabulary.currencySuffixLabels,
    roundingModeLabels: vocabulary.roundingModeLabels,
    amountRoundingPointLabels: vocabulary.amountRoundingPointLabels,
    referenceSeriesKindLabels: vocabulary.referenceSeriesKindLabels,
    referenceCatalogueKindLabels: vocabulary.referenceCatalogueKindLabels,
  };
  for (const [name, table] of Object.entries(tables)) {
    for (const [code, word] of Object.entries(table)) {
      ok(word.trim() !== '', `${name}.${code} 的词为空`);
      equal(/[A-Za-z]/.test(word), false, `${name}.${code} 的词「${word}」含英文`);
    }
  }
  for (const entry of Object.values(vocabulary.priceCardStatusLabels)) {
    equal(/[A-Za-z]/.test(entry.label), false, `状态词「${entry.label}」含英文`);
  }
});

// Covers: 判定条件的特征集合闭合（FeatureSource 十一个）、比较运算五个；少一个，对应价卡的条件就读不出来。
test('特征词与比较词覆盖领域的封闭集合', () => {
  for (const source of [
    'LONGEST_SIDE',
    'SECOND_LONGEST_SIDE',
    'LENGTH_AND_GIRTH',
    'VOLUME',
    'ACTUAL_WEIGHT',
    'VOLUMETRIC_WEIGHT',
    'CHARGEABLE_WEIGHT',
    'ZONE',
    'ADDRESS_TYPE',
    'ADDRESS_NATURE',
    'SERVICE_OPTION',
  ]) {
    ok(vocabulary.featureLabels[source], `特征 ${source} 没有中文词`);
  }
  for (const operator of ['GT', 'GE', 'LT', 'LE', 'EQ']) ok(vocabulary.comparisonLabels[operator], `比较 ${operator} 没有中文词`);
});

// Covers: 自身计价重量这一格的界面叫法（ADR-0347 决定一）：限定语写进词里，运营不会把它当最终计费重量。
test('自身计价重量与偏远档位的界面叫法', () => {
  equal(vocabulary.featureLabels.CHARGEABLE_WEIGHT, '计价重量（不含最低计价重量抬高）');
  equal(vocabulary.featureLabels.ADDRESS_TYPE, '偏远档位');
});
