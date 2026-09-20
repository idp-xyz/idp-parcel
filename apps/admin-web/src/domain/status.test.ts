import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import {
  domainStatusLayers,
  statusLayerShapes,
  statusLayers,
  statusShapes,
  tagVariantByTone,
} from './status-layers';

// 本文件只钉 status-layers.ts 这份纯数据（票 admin-web-ux-alignment/05）。渲染件 LayeredStatusBadge / StatusBadgeFor 在 status.tsx，
// 它引 @idpxyz/ui-patterns——那些包的 exports 只有 import 入口，本仓测试链按 CommonJS 发射后 require 不到
// （ERR_PACKAGE_PATH_NOT_EXPORTED，实测于本票），组件层在这里钉不住；「词表每词恰一层、不多不少」由 status-layers.ts 的
// `satisfies Record<DomainStatus, StatusLayer>` 在编译期守，这里只钉运行期能看到的那半。

// Covers: 每个词归的层都是五层之一；五层每层都定了形且形各不同——层是设计系统层的分类，词进哪层由词表说，形由层说，
// 两张表都不许有第六个值悄悄混进来。
test('词归五层之一，五层各有一形且形各不同', () => {
  const layerSet = new Set<string>(statusLayers);
  for (const [word, layer] of Object.entries(domainStatusLayers)) {
    equal(layerSet.has(layer), true, `${word} 归了不存在的层 ${layer}`);
  }
  const shapes = statusLayers.map((layer) => statusLayerShapes[layer]);
  equal(new Set(shapes).size, statusLayers.length);
  for (const shape of shapes) equal(statusShapes.includes(shape), true);
});

// Covers: 今天词表里的词全部是生命周期层——四个空层留层不留词（票面裁决 2）；哪天有词进了别的层，这条会红，改它的人得同时
// 回答「那个词凭什么不是所在对象的状态」。
test('今天词表里的词全在 lifecycle 层，其余四层留空', () => {
  const layersInUse = new Set(Object.values(domainStatusLayers));
  deepEqual([...layersInUse], ['lifecycle']);
});

// Covers: 色调 → Tag 变体五档齐全，与 status.tsx 里色调 → StatusBadge 变体同一档次划分（neutral / info / warning / critical / positive）。
test('色调到 Tag 变体五档齐全', () => {
  deepEqual(Object.keys(tagVariantByTone).sort(), ['critical', 'info', 'neutral', 'positive', 'warning']);
  equal(new Set(Object.values(tagVariantByTone)).size, 5);
});
