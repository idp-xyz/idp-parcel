import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import {
  activeSectionAt,
  bottomSpacerHeight,
  sectionElementId,
  sectionExpanded,
  sectionScrollTop,
} from './object-page-sections';

// 本文件钉的是对象页「分区」版式的纯判读（票 company-subject-ux/13）：当前锚点、滚动偏移、末尾垫高与折叠态。
// 渲染件 DetailPageTemplate 引 ESM-only 的 @idpxyz 原语，run-tests 的 CommonJS 发射加载不了它，组件层在这里钉不住。

const sections = [
  { id: 'summary', top: 52 },
  { id: 'contacts', top: 900 },
  { id: 'related', top: 1500 },
] as const;

// Covers: 点锚点滚到的位置扣掉锚点栏的高度——分区顶边贴在吸顶的栏下沿，不被栏盖住；页首那一段退到 0。
test('滚动偏移：分区顶边减锚点栏高度，不出负数', () => {
  equal(sectionScrollTop(1500, 36), 1464);
  equal(sectionScrollTop(20, 36), 0);
});

// Covers: 滚动时高亮视口里最靠上的分区——顶边已过锚点栏下沿的最后一个；在页首高亮第一个；平滑滚动停在目标时
// 差零点几像素也算到了。
test('当前锚点：顶边过了栏下沿的最后一个分区；页首是第一个', () => {
  equal(activeSectionAt(sections, 0, 36), 'summary');
  equal(activeSectionAt(sections, 862, 36), 'summary');
  // 900 - 36 = 864 才真正贴上栏下沿；863 差一像素也算到了（容差）。
  equal(activeSectionAt(sections, 863, 36), 'contacts');
  equal(activeSectionAt(sections, 1463.4, 36), 'related');
  equal(activeSectionAt(sections, 5000, 36), 'related');
  equal(activeSectionAt([], 100, 36), null);
});

// Covers: 末尾垫高让最后一个分区也能滚到栏下沿；内容本来够高就不垫。
test('末尾垫高：最后一个分区到不了顶才垫，够高垫 0', () => {
  // 栏 36，视口 800，最后一个分区在 1500、内容总高 1716：要滚到 1464 得有 2264 高，差 548。
  equal(bottomSpacerHeight(1500, 36, 800, 1716), 548);
  equal(bottomSpacerHeight(100, 36, 800, 3000), 0);
});

// Covers: 折叠态归并——没点过听调用方的默认，点过听点过的；没声明可折叠的分区永远展开。
test('折叠态：默认按 collapsed，点过按点的，不可折叠的永远展开', () => {
  equal(sectionExpanded(true, undefined), false);
  equal(sectionExpanded(true, true), true);
  equal(sectionExpanded(false, undefined), true);
  equal(sectionExpanded(false, false), false);
  equal(sectionExpanded(undefined, false), true);
});

test('分区元素 id 按签 id 定', () => {
  equal(sectionElementId('profile-versions'), 'section-profile-versions');
});
