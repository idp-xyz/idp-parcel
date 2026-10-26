// 写入页「还没提交」的登记簿与浏览器离开确认（票 party-commercial-uiux/17）。
//
// 草稿跨页签保留（票 09）之后，一份已填未发布的草稿可以躺在看不见的页签里：刷新或关页会丢，目录行上的「发新版本」也会
// 把它悄悄换掉。登记簿让表单把「我这里有没提交的输入」报出来，页面在要替换草稿之前问一句；表单挂着时浏览器离开
// 前也弹自己的确认。
//
// 簿按实例记、按组查：同一页上同组可以有几个实例（法人资料节每个法人一份），一个实例报「已提交」不能把另一个的
// 「没提交」抹掉。登记簿是纯函数加一个模块级的表，node:test 直接跑；挂浏览器事件的那半在 useUnsavedGuard。

import { useEffect, useId } from 'react';

const registry = new Map<string, string>();

/** 报一个实例的状态：`unsaved` 为真记入、为假移出。 */
export function setDraftUnsaved(instance: string, group: string, unsaved: boolean): void {
  if (unsaved) registry.set(instance, group);
  else registry.delete(instance);
}

/** 这一组里有没有实例报了没提交。 */
export function hasUnsavedDraft(group: string): boolean {
  for (const registered of registry.values()) if (registered === group) return true;
  return false;
}

/** 发布表单按对象类别分组；价格政策在政策页里叫 PRICE_POLICY，发布轴上叫 PRICE_RULE，两处各用一个名会查不到。 */
export function publicationGroup(kind: string): string {
  return `publication:${kind === 'PRICE_POLICY' ? 'PRICE_RULE' : kind}`;
}

export function registrationGroup(kind: string): string {
  return `registration:${kind}`;
}

/**
 * 表单挂着且有没提交的输入时：登记进簿，并让浏览器在刷新、关页前弹确认。卸载时移出，不留死记录。
 * 浏览器只给固定的确认文案，不能自定义；要的只是「别悄悄丢」。
 */
export function useUnsavedGuard(group: string, unsaved: boolean): void {
  const instance = useId();
  useEffect(() => {
    setDraftUnsaved(instance, group, unsaved);
    return () => setDraftUnsaved(instance, group, false);
  }, [instance, group, unsaved]);
  useEffect(() => {
    if (!unsaved) return;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [unsaved]);
}
