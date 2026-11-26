import { useEffect, useRef, type SyntheticEvent } from 'react';
import type { PageSelectionState } from './list-selection';

// 多选列的复选框，列表模板与工作台模板共用一份：两处的勾要长得一样、读屏念得一样。

// 复选用原生 input 而不是 ui-primitives 的 Checkbox（Radix，渲成 button）：表格里一列几十个复选，原生控件自带 indeterminate 三态、
// 键盘与 AT 语义，且与仓内既有表单（party/* 的 checkbox）同一个控件；accent 色让它吃主题令牌。
export const checkboxClass = 'accent-idpxyz-accent';

// 表头复选。indeterminate 只是 DOM 属性、不是 HTML 特性，React 声明不了它，只能在提交后对着节点设；
// checked 只在「全」时为真——「部分」既不算勾也不算空，由 indeterminate 表达。
export function PageSelectAllCheckbox({
  state,
  disabled,
  onToggle,
}: {
  state: PageSelectionState;
  disabled: boolean;
  onToggle: () => void;
}) {
  const ref = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.indeterminate = state === 'some';
  }, [state]);
  return (
    <input
      ref={ref}
      type="checkbox"
      className={checkboxClass}
      aria-label="选择本页全部"
      checked={state === 'all'}
      disabled={disabled}
      onChange={onToggle}
    />
  );
}

// 复选格上截住指针事件：选择不等于预览（蓝图 10.5），格内的点击不冒泡成行的单击，双击不冒泡成开对象。
// Enter 不必截——行的 onKeyDown 只认 target 是行本身的按键，复选框上的 Enter 到不了那条分支。
export const stopRowEvent = (event: SyntheticEvent) => event.stopPropagation();
