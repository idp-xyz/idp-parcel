// 发货交接看一张已打包的出库单：哪些包裹还没交出、这一笔该走哪个写口、报文怎么装。
//
// 到 Go 端核过的规则（application.WaveHandler.Confirm）：
//   - 只收已打包（PACKED）的单。别的状态、这个序号没有包裹、这个包裹已经确认过，都答 STATUS_MISMATCH。
//   - 自提（PICKUP）必须走自提交付，别的发运方式走出库确认。用错口答内容不合。仓也对不上也是内容不合。
//   - 在线交出要核放行条件。缺的不止一格答 WITHHELD，这一笔没有记下、货不动。只缺面单本体（本产品网络的面单没有存放）
//     或只缺自提凭证时答未配置。离线已交出不拦这些缺口，照记（RECORDED）并标出缺了什么，同时消耗待出库的货。
//   - 一个包裹一笔。同一个事实号同样内容重放答已有结果。

import type { GuidedFormValues } from '../guided-forms';
import type { OutboundRequestDetailBody } from '../api';

const dispatchWords: Record<string, string> = {
  PRODUCT_NETWORK: '本产品网络',
  OWNER_LABEL: '货主面单',
  PICKUP: '到仓自提',
};

export interface DispatchParcel {
  sequence: number;
  summary: string;
  done: boolean;
}

export interface DispatchSheet {
  requestId: string;
  number: string;
  warehouse: string;
  status: string;
  dispatch: string;
  /** 给人看的发运方式。词表里没有的仍用原词，不编一个。 */
  dispatchLabel: string;
  pickup: boolean;
  parcels: DispatchParcel[];
}

export type DispatchFit =
  | { kind: 'ready' }
  | { kind: 'elsewhere'; warehouse: string }
  | { kind: 'not-packed'; text: string }
  | { kind: 'no-parcels' };

export function sheetOf(detail: OutboundRequestDetailBody): DispatchSheet | null {
  const request = detail.request;
  if (!request) return null;
  const confirmed = new Set((detail.confirmations ?? []).map((item) => item.parcelSequence));
  return {
    requestId: request.requestId,
    number: request.documentNumber && request.documentNumber.trim() !== '' ? request.documentNumber : request.requestId,
    warehouse: request.warehouseCode,
    status: request.status,
    dispatch: request.dispatch,
    dispatchLabel: dispatchWords[request.dispatch] ?? request.dispatch,
    pickup: request.dispatch === 'PICKUP',
    parcels: (detail.parcels ?? []).map((parcel) => ({
      sequence: parcel.parcelSequence,
      summary: parcel.lines.map((line) => `${line.goodsCode} × ${line.quantity}`).join('，'),
      done: confirmed.has(parcel.parcelSequence) || parcel.confirmationState === 'CONFIRMED' || parcel.confirmationState === 'RECORDED',
    })),
  };
}

export function judgeDispatch(sheet: DispatchSheet, warehouse: string): DispatchFit {
  if (sheet.warehouse !== warehouse) return { kind: 'elsewhere', warehouse: sheet.warehouse };
  if (sheet.status !== 'PACKED') {
    return { kind: 'not-packed', text: `${sheet.number} 还不能交出，现在不是已打包。` };
  }
  if (sheet.parcels.length === 0) return { kind: 'no-parcels' };
  return { kind: 'ready' };
}

export function nextParcel(sheet: DispatchSheet): DispatchParcel | null {
  return sheet.parcels.find((parcel) => !parcel.done) ?? null;
}

export function markParcelDone(sheet: DispatchSheet, sequence: number): DispatchSheet {
  return { ...sheet, parcels: sheet.parcels.map((parcel) => (parcel.sequence === sequence ? { ...parcel, done: true } : parcel)) };
}

/** 出库确认或自提交付的表单值。离线才带 offline；空的提货方与凭证不放进去。 */
export function handoffValues(sheet: DispatchSheet, sequence: number, offline: boolean, party = '', credential = ''): GuidedFormValues {
  const values: GuidedFormValues = { requestId: sheet.requestId, parcelSequence: String(sequence) };
  if (offline) values.offline = true;
  if (sheet.pickup && party.trim() !== '') values.pickupParty = party.trim();
  if (sheet.pickup && credential.trim() !== '') values.pickupCredential = credential.trim();
  return values;
}

export function handoffSpec(sheet: DispatchSheet): 'outbound-confirmation' | 'pickup-handover' {
  return sheet.pickup ? 'pickup-handover' : 'outbound-confirmation';
}
