import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { normalizeScan, scanFromKey, shouldReclaimFocus, shouldRedirectKey } from './scan-input';

test('规范化：去首尾空白', () => {
  equal(normalizeScan('  6900001000028  '), '6900001000028');
  equal(normalizeScan('\t SYN-LOC-OUT-01 \n'), 'SYN-LOC-OUT-01');
  equal(normalizeScan('\u3000SYN-GOODS-01\u00a0'), 'SYN-GOODS-01');
});

test('规范化：去控制字符，枪夹在码前后的 STX、CR、LF、TAB 都不留', () => {
  equal(normalizeScan('\u0002A-01-03\r\n'), 'A-01-03');
  equal(normalizeScan('A-01-03\r'), 'A-01-03');
  equal(normalizeScan('\u0000\u0001ABC\u007f'), 'ABC');
  equal(normalizeScan('AB\u001dCD'), 'ABCD');
});

test('规范化：码逐字保存，中间的空白与大小写不动', () => {
  equal(normalizeScan('SYN GOODS 01'), 'SYN GOODS 01');
  equal(normalizeScan('Syn-Goods-01'), 'Syn-Goods-01');
  equal(normalizeScan('a-01-03'), 'a-01-03');
});

test('规范化：只剩空白与控制字符的是空串', () => {
  equal(normalizeScan(''), '');
  equal(normalizeScan('   '), '');
  equal(normalizeScan('\r\n'), '');
  equal(normalizeScan('\u0002 \u0003'), '');
});

test('回车提交一次扫码，交回规范化后的码', () => {
  equal(scanFromKey({ key: 'Enter' }, '6900001000028'), '6900001000028');
  equal(scanFromKey({ key: 'Enter' }, ' \u0002A-01-03\r '), 'A-01-03');
});

test('空的不算一次扫码：单独的回车、只有空白的回车', () => {
  equal(scanFromKey({ key: 'Enter' }, ''), null);
  equal(scanFromKey({ key: 'Enter' }, '   '), null);
  equal(scanFromKey({ key: 'Enter' }, '\r\n'), null);
});

test('不是回车的键不算，输入法组字时的回车也不算', () => {
  equal(scanFromKey({ key: 'a' }, 'ABC'), null);
  equal(scanFromKey({ key: 'Tab' }, 'ABC'), null);
  equal(scanFromKey({ key: 'Escape' }, 'ABC'), null);
  equal(scanFromKey({ key: 'Enter', isComposing: true }, 'ABC'), null);
  equal(scanFromKey({ key: 'Enter', isComposing: false }, 'ABC'), 'ABC');
});

test('失焦后抢回扫描框：焦点落在页面上、落在按钮上（点按钮之后）都抢回', () => {
  equal(shouldReclaimFocus({ suspended: false, active: null, byTab: false }), true);
  equal(shouldReclaimFocus({ suspended: false, active: { editable: false }, byTab: false }), true);
});

test('弹出的数量输入、确认框开着时不抢', () => {
  equal(shouldReclaimFocus({ suspended: true, active: null, byTab: false }), false);
  equal(shouldReclaimFocus({ suspended: true, active: { editable: false }, byTab: false }), false);
  equal(shouldReclaimFocus({ suspended: true, active: { editable: true }, byTab: false }), false);
});

test('焦点在别的输入或下拉里时不抢：人正在用它', () => {
  equal(shouldReclaimFocus({ suspended: false, active: { editable: true }, byTab: false }), false);
});

test('用 Tab 走到按钮上时不抢，键盘用户要能走到按钮', () => {
  equal(shouldReclaimFocus({ suspended: false, active: { editable: false }, byTab: true }), false);
});

test('焦点落在按钮上时来了可打印字符，送回扫描框', () => {
  equal(shouldRedirectKey({ key: '6' }, { editable: false }, false), true);
  equal(shouldRedirectKey({ key: 'S' }, null, false), true);
  equal(shouldRedirectKey({ key: '-' }, { editable: false }, false), true);
});

test('回车与空格不送：焦点在按钮上时它们是按下去', () => {
  equal(shouldRedirectKey({ key: 'Enter' }, { editable: false }, false), false);
  equal(shouldRedirectKey({ key: ' ' }, { editable: false }, false), false);
});

test('Tab、方向键、Esc、功能键、组合键都不送', () => {
  for (const key of ['Tab', 'ArrowDown', 'Escape', 'F5', 'Shift', 'Backspace']) {
    equal(shouldRedirectKey({ key }, null, false), false, key);
  }
  equal(shouldRedirectKey({ key: 'k', ctrlKey: true }, null, false), false);
  equal(shouldRedirectKey({ key: 'w', metaKey: true }, null, false), false);
  equal(shouldRedirectKey({ key: 'f', altKey: true }, null, false), false);
});

test('弹层开着，或焦点已经在别的输入里时，字符不送', () => {
  equal(shouldRedirectKey({ key: '6' }, null, true), false);
  equal(shouldRedirectKey({ key: '6' }, { editable: true }, false), false);
});
