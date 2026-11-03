import { useEffect, useRef } from 'react';
import { useWorkspaceTabPanel } from '@idpxyz/ui-workspace';
import { hashBelongsToTab } from '../shell/workspace-state';

/**
 * 页面听地址变化的唯一入口：只在新地址落在本页这张标签名下时才调 `onOwnAddressChange`。
 *
 * 标签保活之后，切走的页仍挂着、仍听得到每一次 hashchange。照旧直接挂 `hashchange` 的话，藏着的页会按别的标签的地址
 * 重算自己的视图——列表标签听到自己那张详情标签的地址会翻成详情，往来单位页读到地址里的停用预填还会把它从别人的地址上剥掉
 * （BusinessPartiesPage 的 readAddress）。激活本标签的那次地址变化本身就落在本标签名下，所以回到这张标签时照样会补读一次，
 * 不必另挂「被激活」的钩子。
 *
 * 不在工作区标签里（单测、没有 EditorGroup 的宿主）时每次都调，与从前直接挂监听同义。`onOwnAddressChange` 每次渲染
 * 可以是新函数，这里按最新那一份调。
 */
export function useOwnAddressChange(onOwnAddressChange: () => void): void {
  const tabId = useWorkspaceTabPanel()?.tabId;
  const latest = useRef(onOwnAddressChange);
  latest.current = onOwnAddressChange;

  useEffect(() => {
    const listener = () => {
      if (tabId !== undefined && !hashBelongsToTab(window.location.hash, tabId)) return;
      latest.current();
    };
    window.addEventListener('hashchange', listener);
    return () => window.removeEventListener('hashchange', listener);
  }, [tabId]);
}
