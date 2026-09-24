import { Component, type ReactNode } from 'react';
import { SectionError } from '../components/states';

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

/** 页面块取回之前页面区里的那一句；外壳不动。 */
export function PageLoading() {
  return <p className="p-4 text-[12px] text-idpxyz-textMuted">正在加载页面…</p>;
}
