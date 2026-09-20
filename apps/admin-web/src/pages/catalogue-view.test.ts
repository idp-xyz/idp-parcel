import { test } from 'node:test';
import { deepEqual } from 'node:assert/strict';
import type { ModuleInfo } from '../navigation';
import { catalogueViewState } from './catalogue-view';

// 本文件钉的是目录页共用判读 catalogueViewState 里与形态有关的那一格（票 admin-web-ux-alignment/06 第 1 条）：
// 答案未到即 loading，且骨架形状默认是表——目录页等的都是一张表，页面零改动就换成表格骨架。

const moduleInfo: ModuleInfo = { title: '预览', owner: '预览上下文', source: '预览来源' };

// 夹具里的读口写端点表上实有的一条：internal/architecture 的 TestEveryAdminWebPathIsOnTheParcelAPIEndpointTable
// 扫的是 src 下所有 .ts/.tsx（含测试），随手编一条路径它就红——与同目录其它 *.test.ts 的夹具同一写法。
// Covers: 答案还没到 → loading 且 shape 为 table；列数不给（模板不知道列定义，用骨架件默认）。
test('目录页答案未到时 loading 默认表格形状', () => {
  const state = catalogueViewState(null, 0, () => {}, {
    module: moduleInfo,
    endpoint: 'GET /shipment-request-views',
    emptyTitle: '暂无',
    emptyDescription: '无',
  });
  deepEqual(state, { kind: 'loading', shape: 'table' });
});
