import { useEffect, useRef, type ReactNode } from 'react';
import { ArrowLeft, Maximize2, RefreshCw, X } from 'lucide-react';
import { PageHeader, PageHeaderActions, PageHeaderContent, PageHeaderTitle } from '@idpxyz/ui-patterns';
import { Button } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { CodeRef } from '../../components/CodeRef';
import { StateSlot } from '../../templates/state-slot';
import { openDocument } from './use-document';

/**
 * 仓配单据页的外框：一张单据占一张标签，有自己的地址。
 *
 * 头区照对象页的写法——种类、单号（点按复制）、状态在左，此刻能做的动作在右；头区下面是进度条；正文是纵向排开的几块，
 * 宽屏上右边多一栏放关联单据。动作钉在头区不随正文滚：人滚到拣货任务那一块时，「放波次」还在手边。
 *
 * 不用 DetailPageTemplate：它的签是固定词表（概要、联系人、时间线……），区块只收带标题的卡片；仓配单据要进度条，
 * 正文是几张各带计数的子表。头区用的是同一套 PageHeader 件，看上去是同一种页。
 */
export function WmsDocumentPage({
  kind,
  measure = '张',
  title,
  subtitle,
  badge,
  actions,
  stepper,
  loading,
  error,
  onBack,
  onReload,
  /** 同一张单被点了刷新、这一问还在途。换到另一张不走这里。 */
  reloading = false,
  busy = false,
  aside,
  panel,
  children,
}: {
  /** 单据种类的叫法：出库单、波次、入库预报。 */
  kind: string;
  /** 「没有取到这张出库单」里的量词，缺省是张；待认领到货是件，商品档案是份。 */
  measure?: string;
  /** 单号。还没读到时用地址里的那一个。 */
  title: string;
  subtitle?: string;
  badge?: ReactNode;
  actions?: ReactNode;
  stepper?: ReactNode;
  loading?: boolean;
  /** 读不到或没有这张单据时的那一句；给了就不画正文。 */
  error?: string | null;
  onBack: () => void;
  onReload?: () => void;
  /** 同一张单被点了刷新、这一问还在途。换到另一张不走这里。 */
  reloading?: boolean;
  /**
   * 详情栏换到下一张、新的还没读到：头和正文留着上一张，只是不能点。加载只让右上角那个刷新标转起来，不另写一行——多出来的字会把栏撑一下再收回去。
   * 新的到了由调用方换上。关闭、整页打开在换的过程中也能用。
   */
  busy?: boolean;
  /** 右栏：关联单据一类看不见也不缺的辅助内容，只在宽屏显示。 */
  aside?: ReactNode;
  /**
   * 摆在列表的详情栏里时给：动作与单号排在同一行、靠右，栏窄到一行放不下时动作才换到下一行；「返回列表」换成
   * 「整页打开」与「关闭」，钉在右上角；关联单据那一栏排到正文下面——栏可能只有半屏宽，右栏会把正文挤窄。
   */
  panel?: { onClose: () => void; onOpenPage: () => void };
  children: ReactNode;
}) {
  const ready = !loading && !error;
  const { addToast } = useToast();
  // 只报点了刷新标的那一次，而且要先看到它在转再看到停：换行时标也转，那种不报。
  const refreshRequested = useRef(false);
  const refreshSeen = useRef(false);
  useEffect(() => {
    if (!refreshRequested.current) return;
    // 换到另一张时这一问作废，不把那次刷新的结果报出来。
    if (busy) {
      refreshRequested.current = false;
      refreshSeen.current = false;
      return;
    }
    if (reloading) {
      refreshSeen.current = true;
      return;
    }
    if (!refreshSeen.current) return;
    refreshRequested.current = false;
    refreshSeen.current = false;
    if (error) addToast({ type: 'error', title: '刷新未形成答案', message: error });
    else addToast({ type: 'success', title: '已刷新' });
  }, [reloading, busy, error, addToast]);
  const requestReload = () => {
    if (!onReload || reloading || loading || busy) return;
    refreshRequested.current = true;
    refreshSeen.current = false;
    onReload();
  };
  const reading = busy || reloading;
  if (panel) {
    return (
      <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor" aria-busy={reading || undefined}>
        {/* 头区与下面的进度条说的是同一件事——这张单是什么、走到哪了：有进度条时两块之间不画线，线只画在进度条下面一道。 */}
        <div className={`shrink-0 px-4 ${ready && stepper ? 'pb-1 pt-3' : 'border-b border-idpxyz-border py-3'}`}>
          {/* 刷新、整页打开、关闭钉在右上角，不跟动作钮一起换行：栏再窄，关闭也在老地方。
              动作钮是一组，放得下整组留在这一行，放不下整组换到下一行并靠右。小字在下一行，用满整行。 */}
          <div>
            <div className="flex items-start gap-2">
              {/* 动作钮是一组：这一行放得下就整组留在单号右边，放不下整组换到下一行，并且靠右。
                  组自己不换行，也不被拉满整行——拉满之后靠右只作用在盒子上，钮还是从左边排。 */}
              <div className={`flex min-w-0 flex-1 flex-wrap items-center justify-end gap-2 ${busy ? 'pointer-events-none' : ''}`}>
                <div className="flex min-w-0 grow items-center gap-2">
                  <span className="type-section font-semibold text-idpxyz-textBright">{kind}</span>
                  {title ? <CodeRef value={title} title={`${kind}号`} /> : null}
                  {ready ? badge : null}
                </div>
                {ready && actions ? <div className="flex w-max shrink-0 flex-nowrap items-center gap-2">{actions}</div> : null}
              </div>
              <div className="flex shrink-0 items-center">
              {onReload ? (
                <Button type="button" variant="ghost" size="icon" disabled={loading || reading} title={reading ? '正在读取' : '刷新'} aria-label={reading ? '正在读取' : '刷新'} onClick={requestReload}>
                  <RefreshCw className={`h-3.5 w-3.5 ${reading ? 'animate-spin' : ''}`} aria-hidden="true" />
                </Button>
              ) : null}
              <Button type="button" variant="ghost" size="icon" title="整页打开" aria-label="整页打开" onClick={panel.onOpenPage}>
                <Maximize2 className="h-3.5 w-3.5" aria-hidden="true" />
              </Button>
              <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={panel.onClose}>
                <X className="h-4 w-4" aria-hidden="true" />
              </Button>
              </div>
            </div>
            {ready && subtitle ? <p className={`mt-1 type-caption leading-4 text-idpxyz-textMuted ${busy ? 'pointer-events-none' : ''}`}>{subtitle}</p> : null}
          </div>
        </div>
        {ready && stepper ? <div className="shrink-0 overflow-x-auto border-b border-idpxyz-border px-4 py-2.5">{stepper}</div> : null}
        {loading ? (
          <div className="flex flex-1 items-center justify-center overflow-auto">
            <StateSlot state={{ kind: 'loading', shape: 'detail' }} />
          </div>
        ) : error ? (
          <div className="flex flex-1 items-center justify-center overflow-auto">
            <StateSlot state={{ kind: 'error', title: `没有取到这${measure}${kind}`, description: error, onRetry: onReload }} />
          </div>
        ) : (
          <div className={`min-h-0 flex-1 overflow-auto ${busy ? 'pointer-events-none' : ''}`}>
            <div className="flex flex-col gap-5 p-4">
              {children}
              {aside}
            </div>
          </div>
        )}
      </div>
    );
  }
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <PageHeader className="items-start">
        <PageHeaderContent className="flex-col items-start gap-1">
          <div className="flex min-w-0 max-w-full flex-wrap items-center gap-3">
            <PageHeaderTitle>{kind}</PageHeaderTitle>
            {title ? <CodeRef value={title} title={`${kind}号`} /> : null}
            {ready ? badge : null}
          </div>
          {ready && subtitle ? <p className="type-caption leading-4 text-idpxyz-textMuted">{subtitle}</p> : null}
        </PageHeaderContent>
        <PageHeaderActions>
          {ready ? actions : null}
          {onReload ? (
            <Button type="button" variant="outline" disabled={loading || reloading} onClick={requestReload}>
              <RefreshCw className={`mr-1 h-3.5 w-3.5 ${reloading ? 'animate-spin' : ''}`} />
              刷新
            </Button>
          ) : null}
          <Button type="button" variant="outline" onClick={onBack}>
            <ArrowLeft className="mr-1 h-3.5 w-3.5" />
            返回列表
          </Button>
        </PageHeaderActions>
      </PageHeader>

      {ready && stepper ? <div className="shrink-0 border-b border-idpxyz-border px-4 py-2.5">{stepper}</div> : null}

      {loading ? (
        <div className="flex flex-1 items-center justify-center overflow-auto">
          <StateSlot state={{ kind: 'loading', shape: 'detail' }} />
        </div>
      ) : error ? (
        <div className="flex flex-1 items-center justify-center overflow-auto">
          <StateSlot state={{ kind: 'error', title: `没有取到这${measure}${kind}`, description: error, onRetry: onReload }} />
        </div>
      ) : (
        <div className="flex-1 overflow-auto">
          <div className="mx-auto flex max-w-[1320px] gap-4 p-4">
            <div className="flex min-w-0 flex-1 flex-col gap-5">{children}</div>
            {aside ? <aside className="hidden w-[280px] shrink-0 flex-col gap-4 xl:flex">{aside}</aside> : null}
          </div>
        </div>
      )}
    </div>
  );
}

/** 右栏里的一块：一个小标题加内容。 */
export function WmsAsideBlock({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded-lg border border-idpxyz-border bg-idpxyz-sidebar p-3">
      <h3 className="type-caption font-medium text-idpxyz-textMuted">{title}</h3>
      <div className="mt-2 flex flex-col gap-1.5 type-body text-idpxyz-text">{children}</div>
    </section>
  );
}

/** 指到另一张单据：点了在它自己的标签里打开，这一张留着。 */
export function DocumentLink({
  moduleId,
  parts,
  children,
}: {
  moduleId: string;
  parts: readonly string[];
  /** 显示的字，缺省是最后一段标识。 */
  children?: ReactNode;
}) {
  return (
    <button
      type="button"
      className="max-w-full truncate text-left font-mono text-[color:var(--ds-text-link-hover)] hover:underline"
      onClick={(event) => {
        // 它常落在表格行里：不让行把这一下当成选中或打开。
        event.stopPropagation();
        openDocument(moduleId, parts);
      }}
    >
      {children ?? parts[parts.length - 1]}
    </button>
  );
}
