import { QueryClient } from '@tanstack/react-query';
import { answerStaleTimeOf, type AnswerLike } from './freshness';

// 管理台唯一的取数客户端（ADR-0360 决定一、二），main.tsx 把它挂在应用根上。
//
// - 查询函数照旧答 ApiResult、不抛：非 outcome 是要显给人看的答案，不是异常。所以自动重试关——重试一个「未配置」只是
//   多问一遍同样的答案；再问由人（刷新键、续办）或下一次用到触发。
// - 新鲜期按答案算（answerStaleTimeOf）：只有 outcome 留满新鲜期，其余即刻过期。
// - 不随窗口聚焦重读：人切回浏览器不代表要刷新；标签保活之后藏着的页也挂着，聚焦一次会让每张开过的页一齐重读。
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: (query) => answerStaleTimeOf(query.state.data as AnswerLike | undefined),
      retry: false,
      refetchOnWindowFocus: false,
    },
  },
});
