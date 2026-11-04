import { useEffect, useRef } from 'react';

/**
 * 进视图（登记视图、对象页编辑态）压一条历史、出视图优先后退的收口。
 *
 * 压过之后优先用后退，是为了保存或取消之后浏览器后退不会又回到视图地址、重新进入视图。但后退只认「上一条历史」，不认是谁
 * 压的：压完之后地址若被别处动过——切到别的工作区标签再回来、从视图里的链接开了别的标签——后退就落到那条别的地址上。所以这里
 * 听着地址变化：只要变到了不是自己压的那一条，`back` 就改答「没压过」，调用方照常替换。替换会在历史里多留一条目录，但不会
 * 把人送去别的页。视图内换册用的 history.replaceState 不经 hashchange，不会误清。
 */
export function usePushedAddress(): {
  /** 压一条历史进视图。 */
  push: (next: string) => void;
  /** 压过且此后地址没被别处动过：后退并答真；否则答假，由调用方替换地址。 */
  back: () => boolean;
  /** 调用方不走后退、自己替换出了视图时清掉。 */
  forget: () => void;
} {
  const pushedTo = useRef<string | null>(null);
  useEffect(() => {
    const listener = () => {
      if (pushedTo.current !== null && window.location.hash !== pushedTo.current) pushedTo.current = null;
    };
    window.addEventListener('hashchange', listener);
    return () => window.removeEventListener('hashchange', listener);
  }, []);
  return {
    push: (next) => {
      pushedTo.current = next;
      window.location.hash = next;
    },
    back: () => {
      if (pushedTo.current === null) return false;
      pushedTo.current = null;
      window.history.back();
      return true;
    },
    forget: () => {
      pushedTo.current = null;
    },
  };
}
