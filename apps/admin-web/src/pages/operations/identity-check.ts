// 身份核对态在管理台上的写法（票 no-reception-source/05；ADR-0199 决定五）。
//
// 单独成模块：待识别实物区与节点收寄区两处都要写它，词只在这里定一份。最容易错的是未核对与零候选的已核对两条待识别——候选同为
// 空、种类同为「待识别」，页面不写核对态，看的人就把「没问到 parcel-shipment」读成「查无此号」。

import type { ReceptionRecord, UnidentifiedItemRecord } from './records-api';
import { labelOf, receptionKindLabels } from './presentation';

/**
 * 「身份核对」一格。未核对写明候选缺席不代表查无此号；没带核对态的待识别行（更早的服务端答复）不替它下结论；页面未收录的码
 * 原样示出，同 serviceMarkersCellOf 的口径。
 */
export function identityCheckCellOf(record: Pick<UnidentifiedItemRecord, 'identityCheck'>): string {
  switch (record.identityCheck) {
    case 'UNCHECKED':
      return '未核对：候选缺席不代表查无此号';
    case 'CHECKED':
      return '已核对';
    case undefined:
      return '未带核对态：查没查过不知道';
    default:
      return `核对态 ${record.identityCheck}（页面未收录）`;
  }
}

/**
 * 节点收寄区「判断」一格：待识别行未核对时带上一句，其余照种类词表。已核对的待识别只写「待识别」——本区不列候选，查无此号与
 * 身份冲突的分别在待识别实物区看。
 */
export function receptionKindCellOf(record: Pick<ReceptionRecord, 'kind' | 'identityCheck'>): string {
  const kind = labelOf(receptionKindLabels, record.kind);
  if (record.kind === 'PENDING_IDENTIFICATION' && record.identityCheck === 'UNCHECKED') return `${kind} · 身份未核对`;
  return kind;
}
