import type { ReactNode } from 'react';
import { parcelNoOf, shipmentNoOf } from '../shipment-request/ref-no';
import type { CustomerNameIndex } from '../shipment-request/shipment-request-list';

// 路线规划四本册共用的几格：运单号 + 包裹号的行首格、客户名、检索命中的格、详情栏副标题，以及页头。单放一处，免得两个
// 决定面的视图回头去引页面本身（那会成环）。

type ParcelRef = { shipmentRequestId: string; declaredParcelId: string; customerAccountId: string };

/**
 * 运单号在上、包裹号在下的两行格。工作台里行首是纯文字不是复制签：签会截住点击（复制不冒泡），点运单号就选不中这一行；
 * 整串与复制在右栏技术详情里。
 */
export function parcelCell(row: { shipmentRequestId: string; declaredParcelId: string }) {
  return (
    <div className="flex min-w-0 flex-col items-start gap-0.5">
      <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.shipmentRequestId}>
        {shipmentNoOf(row.shipmentRequestId)}
      </span>
      <span className="text-[11px] text-idpxyz-breadcrumb">
        包裹 <span className="font-mono">{parcelNoOf(row.declaredParcelId)}</span>
      </span>
    </div>
  );
}

export function customerText(account: string, names: CustomerNameIndex): ReactNode {
  const name = names.get(account);
  return name ? <span title={account}>{name}</span> : <span className="font-mono">{account}</span>;
}

/** 运单号、包裹号（整串与短写法）、客户名称与编码都能搜。 */
export function parcelSearchCells(row: ParcelRef, names: CustomerNameIndex): string[] {
  return [
    row.shipmentRequestId,
    shipmentNoOf(row.shipmentRequestId),
    row.declaredParcelId,
    parcelNoOf(row.declaredParcelId),
    row.customerAccountId,
    names.get(row.customerAccountId) ?? '',
  ];
}

/** 详情栏副标题：包裹号与客户。标题用运单号短写法。 */
export function parcelSubtitle(row: ParcelRef, names: CustomerNameIndex): ReactNode {
  return (
    <>
      包裹 <span className="font-mono">{parcelNoOf(row.declaredParcelId)}</span> · {customerText(row.customerAccountId, names)}
    </>
  );
}

/** 四本册共用的页头：标题、一句话与切册签。 */
export interface RoutePlansHeader {
  title: string;
  subtitle: string;
  sections: ReactNode;
}

