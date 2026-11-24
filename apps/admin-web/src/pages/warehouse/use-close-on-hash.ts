import { useEffect, useRef } from 'react';

/**
 * 开着弹窗时监听 hashchange，触发即调用 onClose。
 * Layout 多标签保活：换页不卸载上一页，Dialog 门户会盖到下一页上——换 hash 就关。
 */
export function subscribeCloseOnHashChange(onClose: () => void): () => void {
  const handler = () => onClose();
  window.addEventListener('hashchange', handler);
  return () => window.removeEventListener('hashchange', handler);
}

/** open 为真时订阅；onClose 经 ref 取最新，避免调用方内联函数反复拆装监听。 */
export function useCloseOnHashChange(open: boolean, onClose: () => void): void {
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;
  useEffect(() => {
    if (!open) return;
    return subscribeCloseOnHashChange(() => onCloseRef.current());
  }, [open]);
}
