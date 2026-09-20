import { useEffect, useState } from 'react';
import { displayName, ensureSession } from '../auth/oidc';

// 顶栏的用户菜单要显会话主体名（票 admin-web-ux-alignment/01 第 2 条），而登录门（auth/AuthGate）把会话握在自己的 state 里、
// 不往子树传——门只答「进不进得来」，形态一概不管，它的文件头就是这么裁的。这里不去改门，走 oidc.ts 公开的 ensureSession
// 读同一份 sessionStorage 会话：门放行之后它一定在，只是这个口是异步的，首帧显不出名字、下一拍补上。
// 临近到期时 ensureSession 会顺手续期一次，与门自己的续期由 oidc.ts 的在途单例合成同一次，不会多换一回令牌。

/** 当前会话主体的显示名；读到之前是 undefined，调用方据此决定显不显那一段。 */
export function useSessionPrincipal(): string | undefined {
  const [principal, setPrincipal] = useState<string | undefined>(undefined);

  useEffect(() => {
    let cancelled = false;
    void ensureSession().then((session) => {
      if (!cancelled && session) setPrincipal(displayName(session));
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return principal;
}
