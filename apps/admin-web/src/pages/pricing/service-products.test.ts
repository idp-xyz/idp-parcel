import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  canonicalServiceProducts,
  declaresServiceProducts,
  restProductsText,
  serviceProductChoices,
  servedProductsText,
  toggleServiceProduct,
} from './service-products';

// 本文件钉售价卡适用服务产品（ADR-0378 决定一、二）的显示与勾选：列表、详情、导入预览念同一句话，编辑器的多选按同一套规则增删。

// Covers: 售价卡指名了产品就列出来；没声明（键缺席或空数组）念成接其余产品；采购与内部价卡不出这一格。
test('适用产品一格：指名、其余、非售价卡三种', () => {
  equal(servedProductsText('SELL', ['SYN-SP-EXPRESS', 'SYN-SP-FBA']), 'SYN-SP-EXPRESS、SYN-SP-FBA');
  equal(servedProductsText('SELL', undefined), restProductsText);
  equal(servedProductsText('SELL', []), '其余产品（不指名）');
  equal(servedProductsText('BUY', ['SYN-SP-FBA']), null);
  equal(servedProductsText('BUY', undefined), null);
  equal(servedProductsText('INTERNAL', undefined), null);
  equal(servedProductsText('', undefined), null);
});

test('只有售价方向能声明产品', () => {
  deepEqual(['SELL', 'BUY', 'INTERNAL', ''].map(declaresServiceProducts), [true, false, false, false]);
});

// Covers: 勾选不重复记、取消即删、空标识不记；写进工作簿的那份去重排序。
test('勾选与写出：同一个产品写两遍按一个算', () => {
  deepEqual(toggleServiceProduct([], 'SYN-SP-FBA', true), ['SYN-SP-FBA']);
  deepEqual(toggleServiceProduct(['SYN-SP-FBA'], ' SYN-SP-FBA ', true), ['SYN-SP-FBA']);
  deepEqual(toggleServiceProduct(['SYN-SP-FBA', 'SYN-SP-EXPRESS'], 'SYN-SP-FBA', false), ['SYN-SP-EXPRESS']);
  deepEqual(toggleServiceProduct(['SYN-SP-FBA'], '  ', true), ['SYN-SP-FBA']);
  deepEqual(canonicalServiceProducts(['SYN-SP-FBA', ' SYN-SP-EXPRESS', 'SYN-SP-FBA', '']), ['SYN-SP-EXPRESS', 'SYN-SP-FBA']);
});

// Covers: 多选框按册的先后列出册上的产品；已选却不在册上的接在后面、照样勾着，不被读面刷新静默清掉。
test('多选框：册上的产品在前，已选却不在册上的接在后面', () => {
  const options = [
    { value: 'SYN-SP-EXPRESS', label: 'SYN-SP-EXPRESS · v1 · 在用 v1' },
    { value: 'SYN-SP-FBA', label: 'SYN-SP-FBA · v1' },
  ];
  deepEqual(serviceProductChoices(options, ['SYN-SP-FBA', 'SYN-SP-GONE']), [
    { value: 'SYN-SP-EXPRESS', label: 'SYN-SP-EXPRESS · v1 · 在用 v1', checked: false, listed: true },
    { value: 'SYN-SP-FBA', label: 'SYN-SP-FBA · v1', checked: true, listed: true },
    { value: 'SYN-SP-GONE', label: 'SYN-SP-GONE（不在服务产品册上）', checked: true, listed: false },
  ]);
  deepEqual(serviceProductChoices([], []), []);
});
