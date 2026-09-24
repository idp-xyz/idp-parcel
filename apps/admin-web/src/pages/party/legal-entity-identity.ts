// 法人身份层两格的呈现判读（票 legal-entity-profile/04「列表与详情上身份两格照答复原样示出」）。目录行、详情抽屉与
// 修订历史三处读同一份：各写一份会在改号的写法时分叉。纯函数，node:test 钉着；页面只负责摆。
//
// **照答复原样**：国家码与号不译不补，类型码也不查目录换成类型名——换名要再读一次目录，而目录会修订，拿今天的目录去
// 译历史修订上的码可能译错。身份层没登记的修订交回 null，由调用方写 identityLayerAbsentNote，不拿空串顶格。

import type { IdentityLayerRecord, LifetimeRegistrationNumberRecord } from './api';

export interface IdentityLayerCells {
  country: string;
  /** 一号一段「类型码 号」，多号之间用中文分号。 */
  numbers: string;
}

export function identityLayerCellsOf(record: IdentityLayerRecord): IdentityLayerCells | null {
  if (!record.identityLayerRegistered) return null;
  return {
    country: record.registrationCountry ?? '',
    numbers: lifetimeNumbersText(record.lifetimeRegistrationNumbers ?? []),
  };
}

export function lifetimeNumbersText(numbers: readonly LifetimeRegistrationNumberRecord[]): string {
  return numbers.map((entry) => `${entry.typeCode} ${entry.number}`).join('；');
}
