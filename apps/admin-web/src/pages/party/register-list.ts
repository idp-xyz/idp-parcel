// 参与方模块册页持有列表状态的钩子（票 admin-web-group-legal-entities/10 第 5 条立于业务参与方页，票 13 第 6 条抬成
// 各册页共用）。页面持有读签的列表答案再传下去：登记签从这份答案取修订号建议，登记册答 REGISTERED / DEACTIVATED 时触发
// 对应读签重取，两签共享同一份答案而不各取一次。
//
// 经取数层读（ADR-0360，query/answers.ts）：同一个读函数在哪页用都是同一份缓存，新鲜期内再挂载直接给、不再问；重取在途时
// 照旧显上一份业务答案，不清——清了，登记签那一瞬看到的是「列表没取到」，未手改的修订建议闪回 1、开着的抽屉因行找不到而
// 关掉再打开。判读在 query/freshness.ts 的 answerToShow，node:test 钉着。参与方的写口答到后整域作废（api.ts）。
//
// 这里取的是目录读口不带条件的第一页（ADR-0144）：列表本身按游标翻页（use-catalogue-cursor），这份只给表单候选、名称转写、
// 行详情的版本池这些不翻页的用处。

import type { ApiResult } from '../catalogue-api';
import { loaderKey, useAnswer } from '../../query/answers';

/**
 * 一册的列表状态：`answer` 最近一份答案、`retry` 重取、`pending` 有一问在途。`load` 要是稳定引用（模块级函数）：
 * 缓存按读函数本身认，写成内联箭头每次渲染都是一份新缓存。`enabled` 为假时不问（已缓存的照给）。
 */
export function useRegisterList<Body>(load: () => Promise<ApiResult<Body>>, options?: { enabled?: boolean }) {
  return useAnswer(loaderKey('party', load), load, options);
}
