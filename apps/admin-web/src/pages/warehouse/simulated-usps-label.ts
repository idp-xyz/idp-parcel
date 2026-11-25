// 本产品网络已打包的包裹打一张模拟的 USPS 面单。读口没有面单文件，这张单只用出库单上已经有的仓、收件人、地址和包裹重量。
//
// 页上写明不能当邮资，也不充当出库确认所缺的面单本体：在线交出仍然按原来的放行条件答。自备面单和自提不走这里。
// 模拟号以 SIM 起头，不是一串纯数字的承运商邮资号。下面的条是示意，不是邮资条码。

import { escapePrint } from './print-sheet';

export interface SimulatedLabelParcel {
  sequence: number;
  weight?: string;
  weightUnit?: string;
}

export interface SimulatedUspsLabel {
  documentNumber: string;
  warehouseCode: string;
  recipient: string;
  address: string;
  parcels: readonly SimulatedLabelParcel[];
}

/** 模拟跟踪号。同一张单、同一个包裹序号，每次打出来一样。 */
export function simulatedTracking(reference: string, sequence: number): string {
  const body = reference.toUpperCase().replace(/[^A-Z0-9]/g, '').slice(0, 16);
  const seq = String(Math.max(0, Math.trunc(sequence))).padStart(2, '0');
  return `SIM${seq}${body}`;
}

export function simulatedUspsLabelHtml(label: SimulatedUspsLabel): string {
  const pages = label.parcels.map((parcel) => pageHtml(label, parcel)).join('');
  const body = pages === '' ? '<p>没有可打的包裹。</p>' : pages;
  return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>模拟面单 ${escapePrint(label.documentNumber)}</title><style>
@page{size:4in 6in;margin:0.16in}
body{margin:0;font-family:"Source Han Sans SC","Noto Sans SC","Microsoft YaHei",sans-serif;color:#111}
.sheet{width:3.68in;height:5.68in;border:2px solid #111;padding:10px 12px;box-sizing:border-box;break-after:page;display:flex;flex-direction:column}
.sheet:last-child{break-after:auto}
.banner{background:#111;color:#fff;font-size:13px;font-weight:700;letter-spacing:0.04em;padding:4px 8px;margin:-10px -12px 8px}
.brand{display:flex;justify-content:space-between;align-items:baseline;border-bottom:2px solid #111;padding-bottom:6px}
.brand b{font-size:22px;letter-spacing:0.08em}
.brand span{font-size:12px;font-weight:700}
.block{margin-top:10px}
.k{font-size:10px;letter-spacing:0.12em;color:#333}
.v{font-size:15px;font-weight:700;margin:2px 0 0;white-space:pre-wrap}
.addr{font-size:13px;font-weight:400}
.track{margin-top:auto}
.num{font-family:ui-monospace,monospace;font-size:14px;letter-spacing:0.08em;text-align:center;margin:4px 0 0}
.meta{display:flex;justify-content:space-between;font-size:11px;margin-top:8px}
</style></head><body>${body}</body></html>`;
}

function pageHtml(label: SimulatedUspsLabel, parcel: SimulatedLabelParcel): string {
  const tracking = simulatedTracking(label.documentNumber, parcel.sequence);
  const recipient = label.recipient.trim() === '' ? '收件人没记' : label.recipient;
  const address = label.address.trim() === '' ? '地址没记' : label.address;
  const warehouse = label.warehouseCode.trim() === '' ? '仓没记' : label.warehouseCode;
  const weight = parcel.weight && parcel.weight.trim() !== '' ? `${parcel.weight} ${parcel.weightUnit ?? ''}`.trim() : '重量没记';
  return `<article class="sheet">
<div class="banner">模拟面单 · 不能当邮资 · 不做出库确认</div>
<div class="brand"><b>USPS</b><span>GROUND ADVANTAGE<br>SIMULATED</span></div>
<div class="block"><div class="k">FROM</div><p class="v">发货仓 ${escapePrint(warehouse)}</p></div>
<div class="block"><div class="k">TO</div><p class="v">${escapePrint(recipient)}</p><p class="v addr">${escapePrint(address)}</p></div>
<div class="track">${hintBarcode(tracking)}<p class="num">${escapePrint(tracking)}</p></div>
<div class="meta"><span>出库单 ${escapePrint(label.documentNumber)}</span><span>包裹 ${parcel.sequence}</span><span>${escapePrint(weight)}</span></div>
</article>`;
}

/** 示意条。宽窄由字符码决定，只为看起来像面单上的条码；不是承运商邮资条码。 */
function hintBarcode(tracking: string): string {
  const widths: number[] = [2, 1, 1, 2];
  for (const ch of tracking) {
    widths.push((ch.charCodeAt(0) % 3) + 1, 1);
  }
  widths.push(2, 1, 2);
  let x = 0;
  const rects: string[] = [];
  let ink = true;
  for (const width of widths) {
    const px = width * 2;
    if (ink) rects.push(`<rect x="${x}" y="0" width="${px}" height="48"/>`);
    x += px;
    ink = !ink;
  }
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${x} 48" width="100%" height="56" role="img" aria-label="模拟条码">${rects.join('')}</svg>`;
}
