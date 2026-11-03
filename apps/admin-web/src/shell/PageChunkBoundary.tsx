import { Component, type ReactNode } from 'react';
import { SectionError } from '../components/states';
import { revealClass, useDelayedReveal } from '../components/use-delayed-reveal';

// 页面区的错误边界与加载态（票 admin-web-bundle-size/01）。页面按域懒加载之后，页面块可能取不回来——最常见的是发版之后
// 旧块已不在服务器上。只红这一张标签的页面区，外壳与其余标签照常。React.lazy 会记住失败的那次加载，原地重渲取不回来，
// 所以「重试」是整页刷新。

export class PageChunkBoundary extends Component<{ children: ReactNode }, { error: Error | null }> {
  state: { error: Error | null } = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  render() {
    if (this.state.error === null) return this.props.children;
    return (
      <div className="p-4">
        <SectionError
          title="页面没有加载出来"
          description={`${this.state.error.message}。若刚发过版，旧的页面块已不在服务器上，刷新即可。`}
          onRetry={() => window.location.reload()}
        />
      </div>
    );
  }
}

/**
 * 页面块取回之前页面区里的那一句；外壳不动。块取得快时这句不露（components/use-delayed-reveal.ts）。
 * 等得比别处久：这句是 Suspense 的后备，React 19 让后备至少停 300ms 才换成页面——按 `loadingRevealDelayMs` 露出
 * （它短于这一段）时，块早到了也要露一下这句再换掉，正是要免的那一闪。等过那三百毫秒还没换上页面，才是真在取块。
 */
export function PageLoading() {
  const revealed = useDelayedReveal(400);
  return <p className={`p-4 text-[12px] text-idpxyz-textMuted ${revealClass(revealed)}`}>正在加载页面…</p>;
}
