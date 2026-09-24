import { useCallback, useEffect, useState } from 'react';
import { ADDRESS_KEYWORD_PARAM, hashQueryValue, hashWithQueryValue } from './address-query';

/**
 * 列表页的检索词放在地址里（`?q=`，票 admin-web-workspace-form/06）：读自 hash、写回 hash。写用 history.replaceState——每敲一个字
 * 不压一条后退记录，也不触发 hashchange 让壳层空转；壳层离开这张标签时按 oldURL 记下带 `?q=` 的完整地址，回程落回它，页面重新
 * 挂载时从这里读回。地址被别处改了（浏览器前进后退）也跟着读：hash 是位置的唯一权威，页面不另存一份。
 */
export function useAddressKeyword(): [string, (keyword: string) => void] {
  const [keyword, setKeywordState] = useState(() => hashQueryValue(window.location.hash, ADDRESS_KEYWORD_PARAM));

  useEffect(() => {
    const onHashChange = () => setKeywordState(hashQueryValue(window.location.hash, ADDRESS_KEYWORD_PARAM));
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  const setKeyword = useCallback((next: string) => {
    setKeywordState(next);
    const hash = window.location.hash;
    // 只改 `#/` 起头的地址：给 replaceState 一个不带 `#` 的相对串，它改的就是 search 而不是 hash 了。
    if (!hash.startsWith('#/')) return;
    window.history.replaceState(window.history.state, '', hashWithQueryValue(hash, ADDRESS_KEYWORD_PARAM, next));
  }, []);

  return [keyword, setKeyword];
}
