import type { ReactNode } from 'react';
import { ExternalLink, X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { objectNouns } from '../../vocabulary/objects';
import { formatInstant } from '../moment';
import type { ShipmentRequestSummary } from './api';
import { customerDisplayOf, type CustomerNameIndex } from './shipment-request-list';

// 运单工作台的详情栏（设计源「五、改版示意 · 运单」）：头部运单号 + 状态胶囊 + 一行副题（客户 · 客户单号），主动作「打开详情」，
// 三格事实，最底下一格「技术详情」收各种 ID——委托标识、客户账户编码、提交主体、来源请求键、提交版本。首屏只放人要看的，
// 系统字段折叠（设计原则三）。内容全部取自行里已有的读模型字段，不为详情栏另开读口。

export function ShipmentRequestDetailPanel({
  row,
  names,
  onClose,
  onOpenDetail,
}: {
  row: ShipmentRequestSummary;
  names: CustomerNameIndex;
  onClose: () => void;
  onOpenDetail: () => void;
}) {
  const customer = customerDisplayOf(row, names);
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`${objectNouns.shipment} ${row.shipmentRequestId}`}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate font-mono text-[16px] font-semibold text-idpxyz-textBright" title={row.shipmentRequestId}>
                {row.shipmentRequestId}
              </h2>
              <StatusPill code={row.state} />
            </div>
            <p className="mt-0.5 text-[12px] text-idpxyz-breadcrumb">
              <span className={customer.isCode ? 'font-mono' : 'text-idpxyz-text'}>{customer.text}</span>
              {' · '}
              {objectNouns.customerReference} <span className="font-mono">{row.sourceRequestKey}</span>
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button size="sm" variant="default" className={`${primaryActionClass} text-[12px]`} onClick={onOpenDetail}>
              <ExternalLink className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
              打开详情
            </Button>
            <Button
              className="text-idpxyz-breadcrumb"
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
      </div>

      <div className="grid grid-cols-3 gap-2.5 border-b border-idpxyz-border px-5 py-3">
        <Fact label={objectNouns.customer}>{customer.text}</Fact>
        <Fact label={objectNouns.parcelCount}>{row.declaredParcelCount}</Fact>
        <Fact label={objectNouns.submittedAt}>
          <time dateTime={row.submittedAt} title={row.submittedAt}>
            {formatInstant(row.submittedAt)}
          </time>
        </Fact>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <p className="text-[12px] leading-5 text-idpxyz-breadcrumb">
          包裹明细、受理进度与决定在「打开详情」里；这里只放一眼要看的。
        </p>
        <TechnicalDetails
          className="mt-4"
          rows={[
            { label: '委托标识', value: row.shipmentRequestId },
            { label: '客户账户编码', value: row.customerAccountId },
            { label: '提交主体', value: row.source },
            { label: '来源请求键', value: row.sourceRequestKey },
            { label: '提交版本', value: row.submissionVersionId },
          ]}
        />
      </div>
    </div>
  );
}

/** 事实条里的一格：标签在上、值在下。 */
function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
      <p className="text-[12px] text-idpxyz-breadcrumb">{label}</p>
      <p className="mt-0.5 break-words text-[12px] font-semibold leading-snug text-idpxyz-textBright">{children}</p>
    </div>
  );
}
