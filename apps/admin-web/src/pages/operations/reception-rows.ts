// 节点收寄区的行转写（票 operator-channel/44）。
//
// 单独成模块而不写在页面里：这一区最容易错的是「服务标记」一格，而页面组件要跑起来得有 React 与
// DOM。判读是纯函数就能拿 node:test 钉住（同 handover-scope-summary 的先例）。

import { formatInstant } from '../catalogue-view';
import type { ReceptionRecord } from './records-api';
import { controlKindLabels, labelOf, receptionKindLabels } from './presentation';

export interface ReceptionReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/** 没取得服务结果时的通用后半句：空的标记什么也证明不了。 */
const ABSENCE_PROVES_NOTHING = '没有标记不代表没有取消或终局';

/**
 * 「服务标记」一格的文字。
 *
 * 三种「没有标记」各写各的字：已查无标记是业务答案；读不动是没取得 PS 的答复，标记缺席什么也说明
 * 不了（ADR-0186 决定一）；待识别身份未定，本就不核对服务结果。三者在顿号拼接下都是空白，而今天
 * 没有方向性门禁，看的人会把空白读成「没有取消、没有终局」。
 */
export function serviceMarkersCellOf(
  record: Pick<ReceptionRecord, 'kind' | 'serviceMarkers' | 'serviceResultCheck'>,
): string {
  // 按 kind 判而不按核对态缺席判：形成行缺席核对态是另一件事（见末两支），不能借「不适用」盖过去。
  if (record.kind === 'PENDING_IDENTIFICATION') return '不适用';
  // 读不动排在标记之前：读不动时标记必为空（ADR-0186 决定一，库面 CHECK 守），
  // 这里不替那种不可能的行另编一套说法。
  if (record.serviceResultCheck === 'UNREADABLE') return `服务结果读不动：${ABSENCE_PROVES_NOTHING}`;
  // 标记只出自读成了的那一次，在场即查到过，核对态缺不缺都照实列出。
  if (record.serviceMarkers.length > 0) return record.serviceMarkers.join('、');
  if (record.serviceResultCheck === 'CHECKED') return '无取消与终局';
  // 形成行没带核对态（更早的服务端答复，或库面约束没拦住的不带核对态的形成行），或带了页面未收录
  // 的值：查没查成都不知道。写「无」是替它下了没下过的结论，写「读不动」又是替它编了一次失败。
  if (record.serviceResultCheck === undefined) {
    return `未带服务结果核对态：${ABSENCE_PROVES_NOTHING}`;
  }
  return `服务结果核对态 ${record.serviceResultCheck}（页面未收录）：${ABSENCE_PROVES_NOTHING}`;
}

/** 一行收寄登记转写成节点收寄区的一行。 */
export function receptionRowOf(record: ReceptionRecord): ReceptionReviewRow {
  return {
    key: `reception:${record.sourceId}`,
    values: {
      unit: record.unit,
      kind: labelOf(receptionKindLabels, record.kind),
      node: record.node,
      deliveredBy: record.deliveredBy,
      receivedAt: formatInstant(record.receivedAt),
      // released 两键成对缺席即实物仍在节点控制中（传输层在场规则）；转出行
      // 照登记示引用与时刻，不代判「在库/出库」。
      control: record.controlReleasedBy
        ? `${labelOf(controlKindLabels, record.controlKind)} · 已转出 ${record.controlReleasedBy}${
            record.controlReleasedAt ? ` · ${formatInstant(record.controlReleasedAt)}` : ''
          }`
        : `${labelOf(controlKindLabels, record.controlKind)} · 控制中`,
      serviceMarkers: serviceMarkersCellOf(record),
    },
  };
}
