import { createContext, useContext } from 'react';

// 本页所在的工作区标签 id，外壳在每张标签的页面外包一层（Layout 的 renderTab）。不经 @idpxyz/ui-workspace 的
// useWorkspaceTabPanel：要在 node:test 能直接加载的纯 .ts 里用（pages/party/unsaved-guard.ts 由测试加载），而那个包只有
// ESM 出口，run-tests 的 CommonJS 发射加载不了它。不在工作区标签里时是 undefined。
export const CurrentTabContext = createContext<string | undefined>(undefined);

export function useCurrentTabId(): string | undefined {
  return useContext(CurrentTabContext);
}
