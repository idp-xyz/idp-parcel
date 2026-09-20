import { useCallback, useEffect, useMemo } from 'react';
import { CommandPalette } from '@idpxyz/ui-workspace';
import { useDensity, useTheme } from '@idpxyz/ui-theme-runtime';
import { pageTitleById } from '../navigation';
import { buildCommandActions, isOpenCommandPaletteShortcut, type RecentEntry } from './command-actions';

// 命令面板宿主（票 admin-web-workspace-form/03 第 2 条）：把 @idpxyz/ui-workspace 的 CommandPalette 挂到壳层根下、
// 装上 Ctrl/⌘+K 监听、每次打开时重算动作集。动作集本身归 command-actions.ts，这里只挂。
//
// 面板里没有任何一条动作发请求（理由与边界见 command-actions.ts 文件头）。
//
// open 态不放在这里而由 Layout 持有：面板有两个入口——本件监听的快捷键与 TopBar 搜索位的按钮——TopBar 不在本件之下，
// 两个入口要指向同一份态，态只能在它们共同的父级。本件收 open / onOpenChange，像一个受控件。
//
// Escape 不在这里处理：vendor CommandPalette 0.1.25 打开时自己在 window 上监听 keydown，Escape → onClose、
// 方向键与 Enter 也是它的；这里再关一次是重复，两处各关一次还会让 onOpenChange(false) 调两遍。
//
// 最近对象经 readRecent 收进来，而不是本件自己去读 localStorage：存储归第一轮 02 的 pages/my-work/recent-objects.ts，
// 本件不 import 它——Layout 把那边的读函数与地址写法（recentObjectHash）一并注入。每次打开时重读，最近对象会变。

export interface CommandPaletteHostProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 读一遍最近对象，最近的在前；面板每次打开时调一次。没有存储时给 () => []。 */
  readRecent: () => RecentEntry[];
  /** 最近对象的地址写法，与各模块页写 hash 的形一致；归第一轮 02，这里只透传给动作集。 */
  recentObjectHash: (entry: RecentEntry) => string;
}

export function CommandPaletteHost({ open, onOpenChange, readRecent, recentObjectHash }: CommandPaletteHostProps) {
  const { theme, toggleTheme } = useTheme();
  const { density, toggleDensity } = useDensity();

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!isOpenCommandPaletteShortcut(e)) return;
      // Ctrl+K 在 Chromium 默认聚焦地址栏，不拦下来面板开了焦点却跑了。
      e.preventDefault();
      onOpenChange(true);
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onOpenChange]);

  const close = useCallback(() => onOpenChange(false), [onOpenChange]);

  // 关着时给空集：vendor 关着不渲染，算了也没人看；开着时按当下的主题 / 密度与最近对象重算，
  // 切换后标签「切换到 <目标态>」才跟得上。
  const actions = useMemo(
    () =>
      open
        ? buildCommandActions({
            pageTitleById,
            recentObjects: readRecent(),
            recentObjectHash,
            shellToggles: { theme, toggleTheme, density, toggleDensity },
          })
        : [],
    [open, readRecent, recentObjectHash, theme, toggleTheme, density, toggleDensity],
  );

  return <CommandPalette open={open} onClose={close} actions={actions} />;
}
