import { createContext, useContext, useEffect, useState, type ComponentPropsWithoutRef, type ReactNode } from 'react';
import { Tabs, TabsContent } from '@idpxyz/ui-primitives';

// 切走不丢草稿的页签（票 party-commercial-uiux/09）。
//
// 组件库的 Tabs 是 Radix：不加 forceMount 时非当前签整棵卸载，切到别的签查个标识再切回来，填了一半的发布表单与已走到的
// 步骤全空。这里改成「访问过才挂上，之后切走也不卸载、只藏起来」：一上来全挂会让每个签的表单都去读自己的词表与册，首屏
// 请求翻几倍；只挂访问过的，代价只落在用过的签上。各签照旧用 `data-[state=inactive]:hidden` 藏起来。

/** 访问过的那几个值：当前值随时算在内，切走的留着。 */
export function useVisited<T extends string>(current: T): (value: T) => boolean {
  const [visited, setVisited] = useState<ReadonlySet<T>>(() => new Set([current]));
  useEffect(() => {
    setVisited((previous) => (previous.has(current) ? previous : new Set(previous).add(current)));
  }, [current]);
  return (value) => value === current || visited.has(value);
}

const SeenContext = createContext<(value: string) => boolean>(() => true);

type TabsProps = ComponentPropsWithoutRef<typeof Tabs>;

/** 用法同 Tabs（受控、非受控都行）；里面的签换成 KeepAliveTabsContent。 */
export function KeepAliveTabs({ value, defaultValue, onValueChange, children, ...rest }: TabsProps) {
  const [inner, setInner] = useState(defaultValue ?? '');
  const current = value ?? inner;
  const seen = useVisited(current);
  return (
    <Tabs
      {...rest}
      value={current}
      onValueChange={(next) => {
        setInner(next);
        onValueChange?.(next);
      }}
    >
      <SeenContext.Provider value={seen}>{children}</SeenContext.Provider>
    </Tabs>
  );
}

type TabsContentProps = ComponentPropsWithoutRef<typeof TabsContent>;

/** 没访问过的签不挂内容；访问过的一直挂着，非当前时由 `data-[state=inactive]:hidden` 藏起。 */
export function KeepAliveTabsContent({ value, children, ...rest }: TabsContentProps & { children?: ReactNode }) {
  const seen = useContext(SeenContext);
  return (
    <TabsContent {...rest} value={value} forceMount>
      {seen(value) ? children : null}
    </TabsContent>
  );
}
