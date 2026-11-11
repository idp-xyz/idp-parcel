import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { emptyGoodsLine, type GoodsLineForm } from './goods-form';
import {
  fillSameEachBox,
  packingCheckOf,
  packingDraftsOf,
  packingErrorsOf,
  packingProblemsOf,
  withoutBlankGoodsLines,
  withoutPackingBox,
  withoutPackingLine,
  withPackingCell,
} from './packing-form';

// 本文件钉下单页的装箱矩阵(ADR-0373 决定三,票 declared-goods-lines/05):要么不报、报就报全;交给服务端的行号是去空行之后的
// 第几行;格里的 0 当没装;各箱之和按十进制精确相加。

const line = (quantity: string, description = '货'): GoodsLineForm => ({ ...emptyGoodsLine, descriptionZh: description, quantity });

test('一格都没填就是整票清单:不报分配,也没有要提示的', () => {
  const lines = [line('48')];
  equal(packingDraftsOf(lines, [], 3), undefined);
  deepEqual(packingProblemsOf(lines, [['0'], [''], []], 3), []);
});

test('同款每箱 N 件:整列重填指定的箱,别的行不动;交出的行号跳过空行', () => {
  const lines = [line('48', 'T恤'), { ...emptyGoodsLine }, line('2', '帽子')];
  let matrix = withPackingCell([], 0, 2, '2');
  matrix = fillSameEachBox(matrix, 0, '24', 0, 1);
  deepEqual(packingDraftsOf(lines, matrix, 2), [
    [
      { line: 1, quantity: '24' },
      { line: 2, quantity: '2' },
    ],
    [{ line: 1, quantity: '24' }],
  ]);
  deepEqual(packingProblemsOf(lines, matrix, 2), []);
});

test('报了装箱就要报全:逐行和对不上、没填数量的行、什么都没装的箱各说一句', () => {
  const lines = [line('48'), line('')];
  const matrix = [['24', '1'], ['20'], [], ['0']];
  deepEqual(packingProblemsOf(lines, matrix, 4), [
    '第 1 行报了 48,各箱加起来是 44',
    '第 2 行没填数量,报了装箱就要填',
    '第 3 箱、第 4 箱什么都没装;报了装箱,每箱至少装一行',
  ]);
});

test('各箱之和按十进制精确相加,0.1 + 0.2 就是 0.3', () => {
  const check = packingCheckOf([line('0.3')], [['0.1'], ['0.2']], 2);
  deepEqual(check.lines, [{ line: 0, declared: '0.3', packed: '0.3', balanced: true }]);
  equal(packingCheckOf([line('1.50')], [['1.5']], 1).lines[0].balanced, true);
});

test('格要填数字:格上标键,问题清单里点名第几箱第几行;去掉一箱或一行,矩阵跟着收', () => {
  const lines = [line('2'), line('3')];
  deepEqual(packingErrorsOf(lines, [['1x', '3']], 1), {
    'k0.0': '填数字',
    packing: '第 1 箱第 1 行填的不是数字;第 1 行报了 2,各箱加起来是 0',
  });
  deepEqual(withoutPackingBox([['1'], ['2'], ['3']], 1), [['1'], ['3']]);
  deepEqual(withoutPackingLine([['1', '2', '3'], ['4', '5']], 1), [['1', '3'], ['4']]);
});

test('提交前收掉空行,矩阵同步去掉那几列;一行都没填时留一张空卡', () => {
  const lines = [line('2', 'A'), { ...emptyGoodsLine }, line('3', 'C')];
  deepEqual(withoutBlankGoodsLines(lines, [['1', '9', '2']]), { lines: [lines[0], lines[2]], matrix: [['1', '2']] });
  const blank = withoutBlankGoodsLines([{ ...emptyGoodsLine }, { ...emptyGoodsLine }], [['', '']]);
  equal(blank.lines.length, 1);
  deepEqual(blank.matrix, [[]]);
});

test('快捷填法只改那一列的指定几箱,别的箱与别的行不动', () => {
  deepEqual(fillSameEachBox([['5', '1'], ['5']], 1, '2', 1, 2), [['5', '1'], ['5', '2'], ['', '2']]);
});
