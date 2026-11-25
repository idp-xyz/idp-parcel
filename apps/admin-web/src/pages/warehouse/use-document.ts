import { useMemo } from 'react';
import { objectIdOfTab } from '../../shell/workspace-state';
import { useCurrentTabId } from '../../templates/current-tab';
import { useTabReturn } from '../../templates/tab-return-context';
import { documentHash, documentKeyParts } from './document-address';

/**
 * 本页所在的标签开的是哪张单据：各段标识，模块自己的列表页是 null。
 *
 * 读标签 id 而不听地址变化：一张标签从开到关只对应一张单据，换一张单据就是换一张标签。标签保活之后藏着的页还挂着，
 * 听地址的话得逐页筛掉别人的地址；标签 id 不会变，没有这件事。
 */
export function useDocumentParts(): string[] | null {
  const tabId = useCurrentTabId();
  return useMemo(() => {
    const key = tabId === undefined ? null : objectIdOfTab(tabId);
    return key === null ? null : documentKeyParts(key);
  }, [tabId]);
}

/** 打开一张单据：写地址，由壳层开成（或激活）它自己的标签。 */
export function openDocument(moduleId: string, parts: readonly string[]): void {
  window.location.hash = documentHash(moduleId, parts);
}

/** 单据页的「返回列表」：回到模块列表那张标签上次停的地址，筛选与检索词都还在。 */
export function useReturnToList(moduleId: string): () => void {
  const tabReturn = useTabReturn();
  return () => tabReturn.returnTo(moduleId);
}
