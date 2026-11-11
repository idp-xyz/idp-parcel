import { useEffect, useRef, type RefObject } from 'react';
import { useWorkspaceTabPanel } from '@idpxyz/ui-workspace';
import { stepSelection, workbenchKeyAction, type WorkbenchFocus } from '../../templates/workbench';

// 往来单位页两种列表共用的键盘与「选中行跟着走」（票 business-party-redesign/03）：PartyWorkbench.tsx 的表与 PartyRoster.tsx 的
// 名录栏长相不同，键是同一套——`/` 进检索、Esc 收右栏、↑/↓ 换行——判读仍是模板那组纯函数（templates/workbench.ts）。
// 行由 `data-row-key` 认，不问它是表格行还是按钮。

const OVERLAY_SELECTOR = '[role="menu"],[role="dialog"],[role="listbox"],[role="combobox"]';
// 方向键另有主的地方，判读与页签条同一档（templates/workbench.ts 的 workbenchKeyAction：只让出方向键与 `/`，Esc 照旧归列表
// 收右栏）：
// - 页签条：方向键切签。
// - 弹层的触发钮（`aria-haspopup`：菜单、选单、带搜索的选单）：方向键按惯例是展开它。带搜索的那种自己不接方向键，漏给列表
//   的话，人在表单里想展开候选，列表却换了选中行、右栏也跟着开。
// - 声明了方向键不归列表的一块（`data-own-arrow-keys`）：表上方的登记表单——焦点在它的按钮上按方向键，不该动下面的表。
const ARROW_OWNER_SELECTOR = '[role="tablist"],[aria-haspopup],[data-own-arrow-keys]';
const ROW_SELECTOR = '[data-row-key]';

function focusOf(target: EventTarget | null): WorkbenchFocus {
  if (!(target instanceof HTMLElement)) return 'free';
  const tag = target.tagName;
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT' || target.isContentEditable) return 'typing';
  if (target.closest(OVERLAY_SELECTOR)) return 'overlay';
  if (target.closest(ARROW_OWNER_SELECTOR)) return 'tablist';
  return 'free';
}

export interface PartyListKeysOptions {
  /** 装着各行的滚动容器。 */
  listRef: RefObject<HTMLElement | null>;
  searchRef: RefObject<HTMLInputElement | null>;
  /** 摆出来的各行的键，照摆的次序。 */
  keys: readonly string[];
  selectedKey: string | null;
  /** 右栏此刻开没开着：开着 Esc 才算关它。 */
  detailOpen: boolean;
  ready: boolean;
  onSelect: (key: string | null) => void;
  /**
   * 选中行第一次滚进可视区时摆到列表中间：名录栏挂上来时选中的行可能在很下面，摆到中间好看得见它的上下文。之后换行一律就近。
   * 不给即第一次也就近。
   */
  centerFirstReveal?: boolean;
  /** 行晚到的列表（挂上来时还在加载）：行到了再把选中行滚进可视区一次。 */
  revealWhenReady?: boolean;
}

export function usePartyListKeys({
  listRef,
  searchRef,
  keys,
  selectedKey,
  detailOpen,
  ready,
  onSelect,
  centerFirstReveal = false,
  revealWhenReady = false,
}: PartyListKeysOptions): void {
  const tabPanel = useWorkspaceTabPanel();
  const activeTab = tabPanel?.isActiveTab ?? true;
  const keysRef = useRef(keys);
  keysRef.current = keys;
  useEffect(() => {
    if (!activeTab) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.defaultPrevented) return;
      const action = workbenchKeyAction(event.key, focusOf(event.target), detailOpen);
      if (action === null) return;
      event.preventDefault();
      if (action === 'focus-search') searchRef.current?.focus();
      else if (action === 'close-detail') onSelect(null);
      else if (ready) {
        // 焦点在列表某一行上时从那一行起步，不从原先选中的那行起步。
        const focusedRowKey =
          event.target instanceof HTMLElement && event.target.matches(ROW_SELECTOR) && listRef.current?.contains(event.target)
            ? event.target.dataset.rowKey ?? null
            : null;
        const next = stepSelection(keysRef.current, focusedRowKey ?? selectedKey, action);
        if (next !== null) onSelect(next);
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [activeTab, selectedKey, detailOpen, onSelect, ready, listRef, searchRef]);

  // 选中行换了就滚进可视区；焦点原在列表的某一行上时跟到新选中的行，在别处时不被拉回列表。
  const revealed = useRef(false);
  const rowsIn = revealWhenReady && ready;
  useEffect(() => {
    if (selectedKey === null) return;
    const list = listRef.current;
    const row = list?.querySelector<HTMLElement>(`[data-row-key="${CSS.escape(selectedKey)}"]`);
    if (!list || !row) return;
    if (centerFirstReveal && !revealed.current) {
      // 只动列表自己的滚动位置：scrollIntoView 摆中间会连外层容器一起滚。
      list.scrollTop += row.getBoundingClientRect().top - list.getBoundingClientRect().top - (list.clientHeight - row.offsetHeight) / 2;
    } else {
      row.scrollIntoView({ block: 'nearest' });
    }
    revealed.current = true;
    const focused = document.activeElement;
    if (focused instanceof HTMLElement && focused !== row && focused.matches(ROW_SELECTOR) && list?.contains(focused)) {
      row.focus({ preventScroll: true });
    }
  }, [selectedKey, rowsIn, listRef, centerFirstReveal]);
}
