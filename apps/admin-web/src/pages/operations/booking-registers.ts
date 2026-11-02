// 订舱与承运答复挡回两本册子的判读与行转写（票 demo-chain-gaps/178）。
//
// 单独成模块、不依赖 React 与网络：状态词各格、取消时刻单列、挡回两种依据与筛选参数拼装
// 这些风险点都在纯函数里，拿 node:test 就能钉住（同 handover-scope-summary 的先例）。
// 状态词的读法只在读口（ports.BookingStanding*），这里只做示文，不由答复值与接受量再算一遍。

import { weightUnitLabels } from '../../vocabulary/price-card';
import { formatInstant } from '../catalogue-view';
import type {
  BookingAnswerBounceRecord,
  BookingAnswerRecord,
  BookingRequestRecord,
  TransportFulfillmentRegistry,
} from './records-api';
import { shortenEmbeddedPrincipalRefs } from '../../components/principal-ref';
import { referencedNumberOf } from './transport-document-numbers';

export interface BookingRegisterRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/** 订舱状态词（读口 BookingStanding* 七格）。取消不在其中。词不在集合内原样示文，不猜。 */
export const bookingStandingLabels: Record<string, string> = {
  PENDING_CONFIRMATION: '待确认',
  WAITLISTED: '候补',
  CONFIRMED: '已确认',
  PARTIALLY_CONFIRMED: '部分确认',
  REFUSED: '已拒绝',
  EXPIRED: '已失效',
  WITHDRAWN: '已撤回',
};

/** 承运答复值五格（TF CONTEXT「订舱」词条）。 */
export const carrierAnswerLabels: Record<string, string> = {
  ACCEPTED: '接受',
  REFUSED: '拒绝',
  EXPIRED: '失效',
  WITHDRAWN: '撤回',
  WAITLISTED: '候补',
};

/** 挡回依据两格（TF CONTEXT「承运答复挡回」词条；ports.BookingAnswerBounceBasis）。 */
export const bounceBasisLabels: Record<string, string> = {
  BOOKING_NOT_REGISTERED: '订舱不在册',
  SOURCE_NOT_SERVICE_PROVIDER: '不是服务提供方',
};

function labelOr(labels: Record<string, string>, word: string): string {
  return labels[word] ?? word;
}

/** 一份答复示成一句：答复值 · 接受量 · 答复标识 · 答复时刻（接受量、依据缺席即不示）。 */
export function answerLineOf(answer: BookingAnswerRecord): string {
  const parts = [labelOr(carrierAnswerLabels, answer.outcome)];
  if (answer.quantity !== undefined) parts.push(`接受量 ${answer.quantity}`);
  if (answer.basis) parts.push(`依据 ${answer.basis}`);
  parts.push(`标识 ${answer.answer}`, formatInstant(answer.decidedAt));
  return parts.join(' · ');
}

/**
 * 舱位与订舱的计量单位：重量单位取词典的那一册；按件计的 PCS 词典里没有重量义，这里只说「件」。查不到的单位原样显示——
 * 单位是承运商与租户登记的取值，页面不猜。
 */
export function unitWord(unit: string): string {
  if (unit === 'PCS') return '件';
  return weightUnitLabels[unit] ?? unit;
}

/**
 * 订舱册一行。主列「订舱号」是单据号，没有号的照旧显示订舱标识（ADR-0355 决定十一）；订舱标识总进技术详情（承运方答复认的是它），
 * 无号原因也进技术详情。所属委托在技术详情里：「委托号」有号显号、无号按原因注明，委托标识另列。
 */
export function bookingRowOf(record: BookingRequestRecord): BookingRegisterRow {
  const current = record.currentAnswer;
  return {
    key: `booking:${record.booking}`,
    values: {
      booking: record.documentNumber ?? record.booking,
      bookingId: record.booking,
      numberNote: record.documentNumber ? '' : referencedNumberOf(undefined, record.documentNumberAbsence),
      commission: referencedNumberOf(record.commissionDocumentNumber, record.commissionDocumentNumberAbsence),
      commissionId: record.commission,
      schedule: record.schedule,
      requested: `${record.quantity} ${unitWord(record.unit)}`,
      requestedAt: formatInstant(record.requestedAt),
      standing: labelOr(bookingStandingLabels, record.standing),
      currentAnswer: current ? labelOr(carrierAnswerLabels, current.outcome) : '',
      acceptedQuantity: current && current.quantity !== undefined ? String(current.quantity) : '',
      currentAnsweredAt: current ? formatInstant(current.decidedAt) : '',
      // 取消时刻单列，不并进状态词；缺席即没有取消。
      cancelledAt: record.cancelledAt ? formatInstant(record.cancelledAt) : '',
      submittedBy: shortenEmbeddedPrincipalRefs(record.submittedBy ?? ''),
      cancelledBy: shortenEmbeddedPrincipalRefs(record.cancelledBy ?? ''),
      answerCount: String(record.answers.length),
      answers: record.answers.map(answerLineOf).join('\n'),
    },
  };
}

export function bounceRowOf(record: BookingAnswerBounceRecord): BookingRegisterRow {
  return {
    key: `bounce:${record.source}:${record.booking}:${record.answer}`,
    values: {
      source: record.source,
      booking: record.booking,
      answer: record.answer,
      basis: labelOr(bounceBasisLabels, record.basis),
      firstBouncedAt: formatInstant(record.firstBouncedAt),
      lastBouncedAt: formatInstant(record.lastBouncedAt),
      count: record.count,
    },
  };
}

/** 挡回册的两个筛选参数；缺席即不筛。 */
export interface BounceFilter {
  booking?: string;
  source?: string;
}

/**
 * 册名加（挡回册才有的）筛选参数拼成查询路径。空白筛选值不发——服务端把给了却空白的参数判坏请求，
 * 而页面上输入框空着说的是「没筛」；两端空白去掉。筛选只对挡回册生效，其余册子不带。
 */
export function transportRecordsPathOf(
  registry: TransportFulfillmentRegistry,
  filter?: BounceFilter,
): string {
  let path = `/transport-fulfillment-records?registry=${encodeURIComponent(registry)}`;
  if (registry === 'booking-answer-bounce' && filter) {
    const booking = filter.booking?.trim();
    const source = filter.source?.trim();
    if (booking) path += `&booking=${encodeURIComponent(booking)}`;
    if (source) path += `&source=${encodeURIComponent(source)}`;
  }
  return path;
}
