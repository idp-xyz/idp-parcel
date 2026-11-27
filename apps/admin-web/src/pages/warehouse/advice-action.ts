// 已经点中的入库预报上的动作：抬头给人看单据号，报文仍带预报标识。
// 认领待认领到货时人还在指定预报，填的是单据号，提交前换成预报标识；对不上单据号就照原样送，那是预报标识。

import type { GuidedFormValues } from './guided-forms';
import { preAdviceProgress } from './pre-advice-progress';

// 与 wmsStatusMap 里入库预报那四格同一套词。不从 wms-ui 引：那个文件带着界面包，测试加载不到。
const preAdviceStatusLabel: Record<string, string> = {
  ADVISED: '已预报',
  RECEIVING: '收货中',
  CLOSED: '已完结',
  CANCELLED: '已取消',
};

export interface AdviceHeading {
  title: string;
  detail: string;
}

interface AdviceIdentity {
  preAdviceId: string;
  documentNumber?: string;
  status: string;
  inboundMode: string;
  goodsLines?: ReadonlyArray<{ goodsCode: string; unit: string; quantity: number }>;
  cartons?: ReadonlyArray<{ cartonNumber: string; lines: ReadonlyArray<{ goodsCode: string; quantity: number }> }>;
  cumulativeReceived?: ReadonlyArray<{ goodsCode?: string; cartonNumber?: string; quantity: number }>;
}

/** 有单据号就用单据号。还没编出来时改标内部标识，不把标识叫做单号。 */
export function adviceHeading(advice: AdviceIdentity, withProgress: boolean): AdviceHeading {
  const number = advice.documentNumber?.trim() ?? '';
  const title = number !== '' ? number : `内部标识 ${advice.preAdviceId}`;
  const status = preAdviceStatusLabel[advice.status] ?? advice.status;
  if (!withProgress) return { title, detail: status };
  const progress = preAdviceProgress(advice);
  const unit = progress.byCarton ? '箱' : '';
  return { title, detail: `${status} · 已收 ${progress.received}${unit} / 预报 ${progress.advised}${unit}` };
}

/** 打开完结、更新告知或撤回时带上的预填。heading 不是报文字段。 */
export function adviceActionPrefill(advice: AdviceIdentity, withProgress: boolean): GuidedFormValues {
  return { preAdviceId: advice.preAdviceId, heading: adviceHeading(advice, withProgress) };
}

export function headingOf(prefill: GuidedFormValues | undefined): AdviceHeading | null {
  const raw = prefill?.heading;
  if (!raw || typeof raw !== 'object') return null;
  const title = (raw as { title?: unknown }).title;
  const detail = (raw as { detail?: unknown }).detail;
  if (typeof title !== 'string' || title.trim() === '') return null;
  return { title, detail: typeof detail === 'string' ? detail : '' };
}

/**
 * 认领报文里的预报这一格。按单据号问到了就换成预报标识；问到的答复说没有这张单，就留下人填的原文。
 * 读口没答上来时不送，免得把单据号当成标识交出去。空着的格是退件，不动。
 */
export function claimAdvicePayload(
  payload: Record<string, unknown>,
  lookup: { ok: true; found: boolean; preAdviceId?: string } | { ok: false },
): { ok: true; payload: Record<string, unknown> } | { ok: false; message: string } {
  const typed = payload.preAdviceId;
  if (typeof typed !== 'string' || typed.trim() === '') return { ok: true, payload };
  if (!lookup.ok) return { ok: false, message: '没有问到这张入库单，请重试。' };
  const id = lookup.preAdviceId?.trim() ?? '';
  if (lookup.found && id !== '') return { ok: true, payload: { ...payload, preAdviceId: id } };
  return { ok: true, payload };
}
