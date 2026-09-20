import { useEffect, useState, type ReactNode } from 'react';
import {
  beginLogin,
  completeLogin,
  ensureSession,
  isCallback,
  millisUntilRenewal,
  type OidcSession,
} from './oidc';

// 登录门包在应用最外层（main.tsx），不进 Layout/页面注册表：那两处是页面形态的
// 地盘，门只回答「进不进得来」，形态一概不管。未登录只渲染登录页，控制台组件树
// 根本不挂载——不是挂上再遮住。
type GateState =
  | { phase: 'checking' }
  | { phase: 'login'; error?: string }
  | { phase: 'ready'; session: OidcSession };

export function AuthGate({ children }: { children: ReactNode }) {
  const [state, setState] = useState<GateState>({ phase: 'checking' });

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        if (isCallback()) {
          const { session, returnTo } = await completeLogin();
          // 回调地址不该留在历史栈里：刷新会拿着已用过的 code 再交换一次并失败。
          window.history.replaceState(null, '', returnTo);
          if (!cancelled) setState({ phase: 'ready', session });
          return;
        }
        const session = await ensureSession();
        if (cancelled) return;
        setState(session ? { phase: 'ready', session } : { phase: 'login' });
      } catch (error) {
        if (!cancelled) {
          window.history.replaceState(null, '', '/');
          setState({ phase: 'login', error: error instanceof Error ? error.message : String(error) });
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // 会话看守：令牌到期前主动续一次，续不动就如实退回登录页。缺这一格的话，标签
  // 页开着过夜后徽章仍挂着操作员名字，而手上的令牌早已作废——门在说假话。
  const ready = state.phase === 'ready' ? state.session : null;
  useEffect(() => {
    if (!ready) return;
    let cancelled = false;
    let timer = 0;

    const schedule = (session: OidcSession) => {
      window.clearTimeout(timer);
      // 下限 1 秒兜住「续期后到期时刻没往前挪」的退化情形，免得定时器空转。
      timer = window.setTimeout(() => void check(), Math.max(1000, millisUntilRenewal(session)));
    };

    const check = async () => {
      const session = await ensureSession();
      if (cancelled) return;
      if (!session) {
        setState({ phase: 'login', error: '登录已过期且无法续期，请重新登录' });
        return;
      }
      // 换到新令牌才动 state；本轮效果随之重建并重排定时器，这里不必再排。
      if (session.expiresAt !== ready.expiresAt) {
        setState({ phase: 'ready', session });
        return;
      }
      schedule(session);
    };

    // setTimeout 在后台标签页会被节流、机器休眠期间干脆不走，所以除定时之外还挂
    // visibilitychange：回到前台立刻补一次检查。
    const onVisible = () => {
      if (document.visibilityState === 'visible') void check();
    };

    schedule(ready);
    document.addEventListener('visibilitychange', onVisible);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisible);
    };
  }, [ready]);

  if (state.phase === 'checking') {
    return (
      <div className="flex min-h-screen items-center justify-center bg-neutral-950 text-neutral-400">
        正在检查登录状态…
      </div>
    );
  }

  if (state.phase === 'login') {
    return <LoginScreen error={state.error} />;
  }

  // 放行后只渲染子树：会话主体名与「退出」在壳层顶栏的用户菜单里（shell/TopBar），门这里不再挂悬浮徽章——
  // 两个退出入口是重复，而 Layout 在放行后的每条渲染路径上都在，顶栏那一个不会缺席。
  return <>{children}</>;
}

function LoginScreen({ error }: { error?: string }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-neutral-950">
      <div className="w-full max-w-sm rounded-xl border border-neutral-800 bg-neutral-900 p-8 text-center shadow-xl">
        <h1 className="text-lg font-semibold text-neutral-100">包裹网络管理台</h1>
        <p className="mt-2 text-sm text-neutral-400">请先通过统一身份服务登录</p>
        {error ? (
          <p className="mt-4 rounded-md border border-red-900 bg-red-950 p-2 text-xs text-red-300">{error}</p>
        ) : null}
        <button
          type="button"
          className="mt-6 w-full rounded-lg bg-indigo-600 px-4 py-2 text-sm font-medium text-white hover:bg-indigo-500"
          onClick={() => void beginLogin(window.location.pathname + window.location.search)}
        >
          使用 gk.idp.xyz 登录
        </button>
      </div>
    </div>
  );
}