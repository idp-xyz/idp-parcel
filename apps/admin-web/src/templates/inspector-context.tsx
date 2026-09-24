import { createContext, useContext } from 'react';
import type { InspectorContent } from './inspector';

// 检查器的控制口（票 admin-web-workspace-form/02 第 3 条）：列表模板在行被单击时把 inspector(row) 交给它，谁在右栏渲染由壳层定。
//
// 默认值是一个什么都不做的实现而不是 null——没有 Provider 的地方（模板单测、没装右栏的宿主）单击行仍照旧走 onRowClick，
// 只是没有检查器可看；模板不必为「壳层装没装」写两条路。Layout 挂 Provider 时把真实现注进来（壳层段）。

export interface InspectorController {
  /** 显示一份内容；再调一次即换掉上一份。 */
  show(content: InspectorContent): void;
  /** 回到空态。切换活动标签时由壳层调——检查器显示的是当前列表选中的行，换页就不成立了。 */
  clear(): void;
  /**
   * 声明本页会往检查器里交内容，返回撤回函数；列表模板接了 inspector 时挂载期调、卸载时撤回。栏对所有页常驻，
   * 壳层凭它选空态句（inspector.ts 的 inspectorEmptyNote）。
   */
  offer(): () => void;
}

const noopController: InspectorController = {
  show() {},
  clear() {},
  offer() {
    return () => {};
  },
};

const InspectorContext = createContext<InspectorController>(noopController);

export const InspectorProvider = InspectorContext.Provider;

export function useInspector(): InspectorController {
  return useContext(InspectorContext);
}
