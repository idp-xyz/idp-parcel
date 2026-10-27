// 登记答复行里折进「技术详情」的那几行（票 party-commercial-uiux/18）。抽成纯函数是因为 RegistrationPanel 引 @idpxyz/*，
// node:test 里钉不到；这里只引类型，钉得到。

import type { TechnicalRow } from '../states/technical-details';

/** 答复码、拒绝理由码、在等的原因码；没有的那一格留空，折叠件不显空行。 */
export function outcomeTechnicalRows(outcome: string, refusalReason?: string, undecidedReason?: string): TechnicalRow[] {
  return [
    { label: '答复码', value: outcome },
    { label: '拒绝理由码', value: refusalReason ?? '' },
    { label: '在等的原因码', value: undecidedReason ?? '' },
  ];
}
