import type { ReactNode } from 'react';
import { documentTypeWords } from '../../vocabulary/document-numbering';
import { parcelNoOf, shipmentNoOf } from '../shipment-request/ref-no';
import type { ShipmentNumberCell } from '../shipment-request/shipment-numbers';
import type { CustomerNameIndex } from '../shipment-request/shipment-request-list';

// 路线规划四本册共用的几格：运单号 + 包裹号的行首格、客户名、检索命中的格、详情栏标题与副标题、技术详情里委托那两行，以及
// 页头。单放一处，免得两个决定面的视图回头去引页面本身（那会成环）。
//
// 读模型里只有委托标识：委托与它的单据号归 parcel-shipment，路由判断不复制（CONTEXT-MAP 的所有权）。运单号由各册按页上的
// 委托标识经委托查阅取回（shipment-request/use-shipment-numbers），这里只按取回的那一格显，规则在 shipment-numbers.ts。

type ParcelRef = { shipmentRequestId: string; declaredParcelId: string; customerAccountId: string };

/** 委托标识 → 屏上那一格（useShipmentNumbers 交回的读法）。 */
export type ShipmentNumberOf = (shipmentRequestId: string) => ShipmentNumberCell;

const numberName = documentTypeWords.SHIPMENT.numberName;

/** 行首那一列的列头：以运单号为主（ADR-0355 决定十一），包裹号挂在下面。 */
export const parcelColumnHeader = numberName;

/**
 * 运单号在上、包裹号在下的两行格。工作台里行首是纯文字不是复制签：签会截住点击（复制不冒泡），点运单号就选不中这一行；
 * 整串与复制在右栏技术详情里。
 */
export function parcelCell(numberOf: ShipmentNumberOf) {
  return (row: { shipmentRequestId: string; declaredParcelId: string }) => {
    const shipment = numberOf(row.shipmentRequestId);
    return (
      <div className="flex min-w-0 flex-col items-start gap-0.5">
        <span
          className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]"
          title={shipment.documentNumber ?? row.shipmentRequestId}
        >
          {shipment.text}
        </span>
        <span className="type-caption text-idpxyz-breadcrumb">
          包裹 <span className="font-mono">{parcelNoOf(row.declaredParcelId)}</span>
        </span>
      </div>
    );
  };
}

export function customerText(account: string, names: CustomerNameIndex): ReactNode {
  const name = names.get(account);
  return name ? <span title={account}>{name}</span> : <span className="font-mono">{account}</span>;
}

/** 运单号、委托标识与包裹号（整串与短写法）、客户名称与编码都能搜。 */
export function parcelSearchCells(row: ParcelRef, names: CustomerNameIndex, numberOf: ShipmentNumberOf): string[] {
  return [
    numberOf(row.shipmentRequestId).documentNumber ?? '',
    row.shipmentRequestId,
    shipmentNoOf(row.shipmentRequestId),
    row.declaredParcelId,
    parcelNoOf(row.declaredParcelId),
    row.customerAccountId,
    names.get(row.customerAccountId) ?? '',
  ];
}

/** 详情栏标题：运单号；没有运单号时去前缀的委托标识。 */
export function parcelTitle(row: { shipmentRequestId: string }, numberOf: ShipmentNumberOf): string {
  return numberOf(row.shipmentRequestId).text;
}

/** 详情栏副标题：包裹号与客户。 */
export function parcelSubtitle(row: ParcelRef, names: CustomerNameIndex): ReactNode {
  return (
    <>
      包裹 <span className="font-mono">{parcelNoOf(row.declaredParcelId)}</span> · {customerText(row.customerAccountId, names)}
    </>
  );
}

/**
 * 技术详情里委托那两行：运单号一行只在没有号时出现、说为什么没有（或读不到）；有号时号已在标题上，不再列一遍。委托标识
 * 整串原样。
 */
export function shipmentTechnicalRows(
  row: { shipmentRequestId: string },
  numberOf: ShipmentNumberOf,
): { label: string; value: string | undefined }[] {
  const note = numberOf(row.shipmentRequestId).note;
  return [
    { label: numberName, value: note === '' ? undefined : note },
    { label: '委托标识', value: row.shipmentRequestId },
  ];
}

/** 四本册共用的页头：标题、一句话与切册签。 */
export interface RoutePlansHeader {
  title: string;
  subtitle: string;
  sections: ReactNode;
}
