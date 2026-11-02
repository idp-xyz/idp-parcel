import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import type { RowStatus } from './exception-list';

// 异常处理与异常案件两页工作台的右栏详情：头部是运单号与状态胶囊，下面几格业务事实，正文一段人话，底部「技术详情」
// 收各种编号与原码。形状照物流动态页的 TrackingDetailPanel，两页共用这一个壳，各页只交内容。

export interface DetailFact {
  label: string;
  value: ReactNode;
}

export function ExceptionDetailPanel({
  ariaLabel,
  title,
  status,
  subtitle,
  facts,
  children,
  technical,
  onClose,
}: {
  ariaLabel: string;
  title: string;
  status: RowStatus;
  subtitle?: ReactNode;
  facts: DetailFact[];
  children?: ReactNode;
  technical: TechnicalRow[];
  onClose: () => void;
}) {
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={ariaLabel}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate font-mono text-[16px] font-semibold text-idpxyz-textBright" title={title}>
                {title}
              </h2>
              <RowStatusPill status={status} />
            </div>
            {subtitle ? <p className="mt-0.5 text-[12px] text-idpxyz-breadcrumb">{subtitle}</p> : null}
          </div>
          <Button
            className="shrink-0 text-idpxyz-breadcrumb"
            variant="ghost"
            size="icon"
            onClick={onClose}
            aria-label="关闭详情"
            title="关闭（Esc）"
          >
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <dl className="grid grid-cols-2 gap-2.5">
          {facts.map((fact) => (
            <div key={fact.label} className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
              <dt className="text-[12px] text-idpxyz-breadcrumb">{fact.label}</dt>
              <dd className="mt-0.5 break-words text-[12px] font-semibold leading-snug text-idpxyz-textBright">{fact.value}</dd>
            </div>
          ))}
        </dl>
        {children ? <div className="mt-4 space-y-2 text-[12px] leading-5 text-idpxyz-breadcrumb">{children}</div> : null}
        <TechnicalDetails className="mt-4" rows={technical} />
      </div>
    </div>
  );
}

/** 行状态胶囊：有码按词典，没有码按派生词与色调。 */
export function RowStatusPill({ status }: { status: RowStatus }) {
  return status.code !== undefined ? (
    <StatusPill code={status.code} scope={status.scope} />
  ) : (
    <StatusPill label={status.label} tone={status.tone} />
  );
}
