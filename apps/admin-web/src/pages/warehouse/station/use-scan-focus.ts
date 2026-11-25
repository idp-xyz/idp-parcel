import { useEffect, useRef, type RefObject } from 'react';
import { shouldReclaimFocus, shouldRedirectKey } from './scan-input';

// 让扫描框一直有焦点（判断在 scan-input.ts，这里只接 DOM）：
//   - 页面挂上、弹层关上（数量输入、确认框、换波次的下拉），把焦点放进扫描框；
//   - 焦点被别的控件拿走（点了按钮、点了空白处）后，下一拍抢回来；
//   - 焦点落在按钮上时来了可打印字符，在按键按下那一刻把焦点送回扫描框，这个字符与后面的字符都落进去。
// 这几个弹层开着时（suspended）一概不抢。

function activeElementFacts(): { editable: boolean } | null {
  const element = document.activeElement;
  if (element === null || element === document.body) return null;
  const editable =
    element instanceof HTMLInputElement ||
    element instanceof HTMLTextAreaElement ||
    element instanceof HTMLSelectElement ||
    (element instanceof HTMLElement && element.isContentEditable);
  return { editable };
}

// 离开扫描框是不是 Tab 走的：Tab 的按键按下在先，失焦随后一两毫秒内到。
const tabWindowMs = 300;

export function useScanFocus(input: RefObject<HTMLInputElement | null>, suspended: boolean): void {
  const suspendedRef = useRef(suspended);
  suspendedRef.current = suspended;

  useEffect(() => {
    if (!suspended) input.current?.focus();
  }, [input, suspended]);

  // 每次渲染之后查一遍：拿着焦点的按钮点完就禁用了（比如「换放货位」），浏览器不一定为此发失焦事件，焦点就悬在页面上；
  // 状态一变本组件就重渲，在这里把悬着的焦点收回来。别的控件拿着焦点时不动，那是人有意去的。
  useEffect(() => {
    if (suspendedRef.current) return;
    const field = input.current;
    const active = document.activeElement;
    if (field === null || active === field) return;
    if (active === null || active === document.body || (active instanceof HTMLButtonElement && active.disabled)) field.focus();
  });

  useEffect(() => {
    let lastTabAt = Number.NEGATIVE_INFINITY;

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Tab') lastTabAt = Date.now();
      const field = input.current;
      if (field === null || document.activeElement === field) return;
      if (shouldRedirectKey(event, activeElementFacts(), suspendedRef.current)) field.focus();
    };

    const onFocusOut = () => {
      window.setTimeout(() => {
        const field = input.current;
        if (field === null || document.activeElement === field) return;
        const byTab = Date.now() - lastTabAt < tabWindowMs;
        if (shouldReclaimFocus({ suspended: suspendedRef.current, active: activeElementFacts(), byTab })) field.focus();
      }, 0);
    };

    // 捕获阶段：先于页面上任何控件自己的按键处理，字符才来得及送回扫描框
    document.addEventListener('keydown', onKeyDown, true);
    document.addEventListener('focusout', onFocusOut);
    return () => {
      document.removeEventListener('keydown', onKeyDown, true);
      document.removeEventListener('focusout', onFocusOut);
    };
  }, [input]);
}
