import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
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

// 票 catalogue-read-pagination/05 第 4 条：已迁到 ADR-0144 的读口把查询参数为何不成立写在 detail 里，页面原样示出。
// Covers: 带 detail → 问题码说明之后接一个空格再接那句散文，一字不改；不带 → 说明与先前一字不差。
test('调用方问题带理由散文时原样接在问题码说明之后，不带时说明不变', () => {
  const options = {
    module: moduleInfo,
    endpoint: 'GET /commercial-customer-accounts',
    emptyTitle: '暂无',
    emptyDescription: '无',
  };
  const detail = '游标与本次的排序或筛选不符，请从第一页重取';

  const without = catalogueViewState(
    { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST' },
    0,
    () => {},
    options,
  );
  const withDetail = catalogueViewState(
    { kind: 'callerProblem', status: 400, code: 'MALFORMED_REQUEST', detail },
    0,
    () => {},
    options,
  );

  ok(without.kind === 'error' && withDetail.kind === 'error');
  equal(without.description, '查询参数不在服务端接受的封闭集合内；请检查前端与服务端版本是否一致。');
  equal(withDetail.description, `${without.description} ${detail}`);
  equal(withDetail.title, without.title);
});

// 票 catalogue-read-pagination/05：检索下推之后，答回零行不再等于册为空。
// Covers: 带收窄条件的零行 → ready（由页面的 emptyRowsNote 说「这组条件下没有」）；不带条件的零行 → 空态，与先前同；有行时两者都是 ready。
test('带检索或筛选条件答出零行仍是 ready，不带条件的零行才是空态', () => {
  const options = {
    module: moduleInfo,
    endpoint: 'GET /commercial-customer-accounts',
    emptyTitle: '当前租户尚无货主客户账户登记',
    emptyDescription: '登记册为空',
  };
  const answer = { kind: 'outcome' as const, status: 200, body: {} };

  deepEqual(catalogueViewState(answer, 0, () => {}, { ...options, narrowed: true }), { kind: 'ready' });
  deepEqual(catalogueViewState(answer, 0, () => {}, options), {
    kind: 'empty',
    title: options.emptyTitle,
    description: options.emptyDescription,
  });
  deepEqual(catalogueViewState(answer, 3, () => {}, { ...options, narrowed: true }), { kind: 'ready' });
  deepEqual(catalogueViewState(answer, 3, () => {}, options), { kind: 'ready' });
});
