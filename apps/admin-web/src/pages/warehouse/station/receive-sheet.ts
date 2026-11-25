// 收货工位看一张入库预报：能不能在这个仓收、预报了什么、以前收了多少、这一次怎么装成收货确认的报文。
//
// 到 Go 端核过的规则（application.ConfirmReceiptHandler.Confirm，载荷 registrationjson.ReceiptConfirmationCommand）：
//   - 已预报（ADVISED）与收货中（RECEIVING）才能入账（preAdviceReceivable）。已取消与已完结不再入账：不带待认领引用时，
//     这次收货原样记成待认领到货，答复 DIVERTED_TO_UNCLAIMED（divertUnscreened），不核库位与批号。
//   - 预报的仓必须是这台设备的仓，否则内容不合。入库形态必须与预报一致：商品预报走 BY_GOODS，整箱预报走 BY_CARTON。
//   - 库存状态只认 GOOD、PENDING_INSPECTION、DEFECTIVE（domain.NewStockState）。旧作业台表单里的 QUARANTINE 提交会被整份退回，
//     这里不摆它。
//   - 认得出的货：计量单位必须是包装层级；商品跟踪批号或效期时必须带，不跟踪时不能带（lineDefects），否则内容不合、一行都不落。
//   - 认不出的码仍可入这一笔：记成未知货，并记一笔错货差异（ReceiptLineUnknownGoods）。预报里没有的、但认得出的商品入账后记错货差异。
//     超过预报数量的记多收差异。这些都是确认入账（CONFIRMED），不是拒绝。
//   - 整箱一行一个箱号，外箱破损是可选的 damaged；商品行不能带箱的字段，整箱行不能带商品的字段，带了是坏载荷。
//   - 数量是 JSON 数字。发生时刻要，设备序号不要。

import type { GuidedFormValues } from '../guided-forms';
import type { PreAdviceRecord } from '../api';
import { overdueLabel } from '../waiting';

export const receiptStockStates: readonly { value: string; label: string }[] = [
  { value: 'GOOD', label: '良品' },
  { value: 'PENDING_INSPECTION', label: '待检' },
  { value: 'DEFECTIVE', label: '次品' },
];

export interface AdviceGoods {
  goodsCode: string;
  unit: string;
  advised: number;
  received: number;
  batchNumber?: string;
  expiryDate?: string;
}

export interface AdviceCarton {
  cartonNumber: string;
  /** 箱里是什么，只给人看。 */
  summary: string;
}

export interface AdviceSheet {
  preAdviceId: string;
  documentNumber: string | null;
  ownerAccount: string;
  warehouseCode: string;
  status: string;
  /** 商品入库或整箱入库。别的形态不收。 */
  inboundMode: 'BY_GOODS' | 'BY_CARTON';
  expectedAt?: string;
  /** 已预报且预计时刻已过时的「已迟 …」；否则空串。 */
  late: string;
  goods: AdviceGoods[];
  cartons: AdviceCarton[];
}

export type AdviceFit =
  | { kind: 'receive' }
  /** 已取消或已完结：还能提交，但后端不入账，转成待认领。 */
  | { kind: 'divert' }
  | { kind: 'elsewhere'; warehouse: string }
  | { kind: 'bad-mode'; mode: string };

export function judgeAdvice(sheet: AdviceSheet, warehouse: string): AdviceFit {
  if (sheet.warehouseCode !== warehouse) return { kind: 'elsewhere', warehouse: sheet.warehouseCode };
  if (sheet.status === 'CANCELLED' || sheet.status === 'CLOSED') return { kind: 'divert' };
  return { kind: 'receive' };
}

function receivedOf(record: PreAdviceRecord, goodsCode: string): number {
  return (record.cumulativeReceived ?? [])
    .filter((item) => item.goodsCode === goodsCode)
    .reduce((sum, item) => sum + item.quantity, 0);
}

/** 读口上的一张预报收成工位用的表。形态不是商品也不是整箱时答 bad-mode，不装表。 */
export function sheetOf(record: PreAdviceRecord, now: Date): AdviceSheet | { kind: 'bad-mode'; mode: string } {
  if (record.inboundMode !== 'BY_GOODS' && record.inboundMode !== 'BY_CARTON') {
    return { kind: 'bad-mode', mode: record.inboundMode };
  }
  const expectedAt = record.currentNotice?.expectedAt;
  return {
    preAdviceId: record.preAdviceId,
    documentNumber: record.documentNumber && record.documentNumber.trim() !== '' ? record.documentNumber : null,
    ownerAccount: record.ownerAccount,
    warehouseCode: record.warehouseCode,
    status: record.status,
    inboundMode: record.inboundMode,
    expectedAt,
    late: record.status === 'ADVISED' && overdueLabel(expectedAt, now) !== '' ? `已迟 ${overdueLabel(expectedAt, now)}` : '',
    goods: (record.goodsLines ?? []).map((line) => ({
      goodsCode: line.goodsCode,
      unit: line.unit,
      advised: line.quantity,
      received: receivedOf(record, line.goodsCode),
      batchNumber: line.batchNumber,
      expiryDate: line.expiryDate,
    })),
    cartons: (record.cartons ?? []).map((carton) => ({
      cartonNumber: carton.cartonNumber,
      summary: carton.lines.map((line) => `${line.goodsCode} × ${line.quantity}`).join('，'),
    })),
  };
}

/** 扫到的码是不是这张预报：预报标识或单据号，逐字相等。 */
export function adviceMatches(sheet: AdviceSheet, code: string): boolean {
  return sheet.preAdviceId === code || sheet.documentNumber === code;
}

export interface GoodsDraft {
  observed: string;
  /** 认成的商品；认不出是 null，照收后记成收货差异。 */
  goodsCode: string | null;
  unit: string;
  quantity: number;
  batchNumber?: string;
  expiryDate?: string;
}

export interface CartonDraft {
  cartonNumber: string;
  damaged: boolean;
}

export function addGoods(lines: readonly GoodsDraft[], next: GoodsDraft): GoodsDraft[] {
  const index = lines.findIndex(
    (line) =>
      line.observed === next.observed &&
      line.unit === next.unit &&
      line.batchNumber === next.batchNumber &&
      line.expiryDate === next.expiryDate,
  );
  if (index < 0) return [...lines, next];
  return lines.map((line, at) => (at === index ? { ...line, quantity: line.quantity + next.quantity } : line));
}

export function goodsThisTime(lines: readonly GoodsDraft[], goodsCode: string): number {
  return lines.filter((line) => line.goodsCode === goodsCode).reduce((sum, line) => sum + line.quantity, 0);
}

/** 装成旧作业台「商品收货确认」那一格的值。数量用字符串，装报文时变成 JSON 数字。空的批号与效期不放进去。 */
export function goodsFactValues(sheet: AdviceSheet, location: string, stockState: string, lines: readonly GoodsDraft[]): GuidedFormValues {
  return {
    preAdviceId: sheet.preAdviceId,
    lines: lines.map((line) => {
      const row: Record<string, string> = {
        observed: line.observed,
        unit: line.unit,
        stockState,
        quantity: String(line.quantity),
        locationCode: location,
      };
      if (line.batchNumber) row.batchNumber = line.batchNumber;
      if (line.expiryDate) row.expiryDate = line.expiryDate;
      return row;
    }),
  };
}

export function cartonFactValues(sheet: AdviceSheet, location: string, lines: readonly CartonDraft[]): GuidedFormValues {
  return {
    preAdviceId: sheet.preAdviceId,
    lines: lines.map((line) => {
      const row: Record<string, string | boolean> = { cartonNumber: line.cartonNumber, locationCode: location };
      if (line.damaged) row.damaged = true;
      return row;
    }),
  };
}

/** 认不出的到货另记一笔待认领。观察写扫到的码，预报号与货主只当线索。 */
export function unclaimedFactValues(sheet: AdviceSheet, code: string, location: string | null): GuidedFormValues {
  const values: GuidedFormValues = {
    kind: 'INBOUND',
    observation: `收货时扫到认不出的码 ${code}`,
    preAdviceClue: sheet.preAdviceId,
    ownerClue: sheet.ownerAccount,
  };
  if (location) values.storageLocation = location;
  return values;
}
