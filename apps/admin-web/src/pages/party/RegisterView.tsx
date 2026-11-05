import type { ReactNode } from 'react';
import { ArrowLeft } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';

/**
 * 登记视图的壳（票 commercial-registers-edit-containers/01，照往来单位页的登记视图）：页头「返回列表」+ 标题 + 副题，表单住在
 * 下面整页滚动。视图由地址决定，进出与离开守卫由页面做（`onBack` 是页面给的、已带守卫的回调）；`aside` 摆在页头右端
 * （换册的切换条之类）。
 */
export function RegisterView({
  title,
  subtitle,
  onBack,
  backLabel = '返回列表',
  aside,
  children,
}: {
  title: string;
  subtitle?: string;
  onBack: () => void;
  backLabel?: string;
  aside?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 flex-wrap items-center gap-4 border-b border-idpxyz-border px-6 py-3">
        <Button variant="ghost" size="sm" className="type-body text-idpxyz-breadcrumb" onClick={onBack}>
          <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
          {backLabel}
        </Button>
        <div className="min-w-0">
          <h1 className="type-title font-semibold leading-tight text-idpxyz-textBright">{title}</h1>
          {subtitle ? <p className="truncate type-body text-idpxyz-breadcrumb">{subtitle}</p> : null}
        </div>
        {aside ? <div className="ml-auto shrink-0">{aside}</div> : null}
      </div>
      <div className="flex flex-1 flex-col overflow-auto">{children}</div>
    </div>
  );
}
