import { equal } from 'node:assert/strict';
import { test } from 'node:test';
import { shortPrincipalRef, shortenEmbeddedPrincipalRefs } from './principal-ref';

test('发行方#主体取主体部分', () => {
  equal(shortPrincipalRef('http://127.0.0.1:5558/dex#alice'), 'alice');
});

test('主体过长截到 16 字符加省略', () => {
  equal(shortPrincipalRef('http://x/dex#Ch1234567890abcdefghijkl'), 'Ch1234567890abcd…');
});

test('恰 16 字符不截', () => {
  equal(shortPrincipalRef('i#1234567890123456'), '1234567890123456');
});

test('无 # 原样', () => {
  equal(shortPrincipalRef('ERP-GATEWAY'), 'ERP-GATEWAY');
});

test('# 后为空退回原串', () => {
  equal(shortPrincipalRef('http://x/dex#'), 'http://x/dex#');
});

test('多个 # 以第一个为界', () => {
  equal(shortPrincipalRef('a#b#c'), 'b#c');
});

test('嵌入形态只换引用片段，保留标签', () => {
  equal(
    shortenEmbeddedPrincipalRefs('CUSTOMS_SUBMISSION_AUTHORIZATION@http://127.0.0.1:5556/dex#ChZTeW4tb3BlcmF0b3ItY29uZmlnLTAx'),
    'CUSTOMS_SUBMISSION_AUTHORIZATION@ChZTeW4tb3BlcmF0…',
  );
});

test('多个引用各自缩短', () => {
  equal(shortenEmbeddedPrincipalRefs('甲 http://a/dex#u1、乙 https://b/dex#u2 完'), '甲 u1、乙 u2 完');
});

test('无引用原样', () => {
  equal(shortenEmbeddedPrincipalRefs('CUSTOMS@SYN-ERP 范围 A#1'), 'CUSTOMS@SYN-ERP 范围 A#1');
});
