import {
  createContext,
  useContext,
  useLayoutEffect,
  useState,
  type Dispatch,
  type MutableRefObject,
  type ReactNode,
  type SetStateAction,
} from 'react';
import { useWorkspaceTabPanel } from '@idpxyz/ui-workspace';

type ViewStateStore = Map<string, Map<string, MutableRefObject<unknown>>>;
const ViewStateContext = createContext<ViewStateStore | null>(null);

/** 只在当前工作区内保留呈现状态；关掉标签即释放，不把业务答案或草稿写入浏览器存储。 */
export function WorkspaceViewStateProvider({ tabIds, children }: { tabIds: readonly string[]; children: ReactNode }) {
  const [store] = useState<ViewStateStore>(() => new Map());
  useLayoutEffect(() => {
    const open = new Set(tabIds);
    for (const tabId of store.keys()) {
      if (!open.has(tabId)) store.delete(tabId);
    }
  }, [store, tabIds]);
  return <ViewStateContext.Provider value={store}>{children}</ViewStateContext.Provider>;
}

/** 滚动位置、分栏比例等无需驱动渲染的快照，按工作区标签隔离。 */
export function useTabViewRef<T>(key: string | null, initial: T | (() => T)): MutableRefObject<T> {
  const store = useContext(ViewStateContext);
  const tabId = useWorkspaceTabPanel()?.tabId;
  const [snapshot] = useState<MutableRefObject<T>>(() => {
    const saved = tabId && key ? store?.get(tabId)?.get(key) : undefined;
    return (saved as MutableRefObject<T> | undefined) ?? {
      current: typeof initial === 'function' ? (initial as () => T)() : initial,
    };
  });
  useLayoutEffect(() => {
    if (!store || !tabId || !key) return;
    let entries = store.get(tabId);
    if (!entries) {
      entries = new Map();
      store.set(tabId, entries);
    }
    entries.set(key, snapshot);
  }, [store, tabId, key, snapshot]);
  return snapshot;
}

/** 页面卸载前已经提交的界面状态，回到同一标签时作为初值恢复。 */
export function useTabViewState<T>(key: string, initial: T | (() => T)): [T, Dispatch<SetStateAction<T>>] {
  const snapshot = useTabViewRef(key, initial);
  const [value, setValue] = useState<T>(() => snapshot.current);
  useLayoutEffect(() => {
    snapshot.current = value;
  }, [snapshot, value]);
  return [value, setValue];
}
