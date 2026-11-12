// 「新增往来单位」表单旁「名录里相近的单位」一栏的纯逻辑（票 business-party-redesign/11）：什么时候问、问什么、几份答复怎么并。
// 全部是纯函数，node:test 钉着；取数与摆法在 BusinessPartySimilarParties.tsx。
//
// 这一栏只是提醒：主数据最怕同一家登两次，人在填名称与编码时顺手看一眼名录里有没有已经登过的。它不拦保存，也不替登记册判
// 「是不是同一家」——那要人点开看。问的是往来单位的目录读口，检索词是字面包含、不分大小写（读口的 q），所以列出来的是名称、
// 编码、注册号或外部编码里含有这几个字的单位，不是模糊相似；全称比已登的名称多出几个字时就查不到，栏里的话不把「没查到」
// 说成「没有」。

import type { BusinessPartyRecord } from './api';

/** 输到几个字才问：一个字几乎哪家都含，列出来的不是提醒而是半本名录。按字符数、不按 UTF-16 码元数。 */
export const similarKeywordMinChars = 2;

// 读口对检索词的长度上限（后端 cataloguepage 的 MaxKeywordRunes）。超过它读口按请求有误拒，这里就不问了——名称长到这个地步
// 也不可能整串包含在另一家的名称里。
const similarKeywordMaxChars = 100;

/** 一栏里最多列几家。再多就不是「看一眼」了，余下的请人到名录里搜。 */
export const similarPartiesShown = 6;

/**
 * 这一刻要问的检索词：名称与编码各自去掉首尾空白，够长的才问，名称在前；两格填的一样（不分大小写）只问一次。
 * 交回空数组即不问。
 */
export function similarPartyKeywords(name: string, partyId: string): string[] {
  const keywords: string[] = [];
  for (const typed of [name, partyId]) {
    const keyword = typed.trim();
    const chars = [...keyword].length;
    if (chars < similarKeywordMinChars || chars > similarKeywordMaxChars) continue;
    if (keywords.some((asked) => asked.toLowerCase() === keyword.toLowerCase())) continue;
    keywords.push(keyword);
  }
  return keywords;
}

/** 一个检索词的答复里这一栏用得上的那一半；读口没答出业务答案（未配置、无权、出错）传 null。 */
export interface SimilarPartyAnswer {
  parties: readonly BusinessPartyRecord[];
  /** 这一页之后还有没有。 */
  hasNext: boolean;
}

export interface SimilarParties {
  /** 列出来的几家：按检索词的次序（名称查到的在前），同一家只列一次，至多 similarPartiesShown 家。 */
  shown: BusinessPartyRecord[];
  /** 查到的不止列出来的这些。 */
  more: boolean;
  /** 有检索词没读到答复：列出来的可能不全，一家都没列时也不等于没有。 */
  unread: boolean;
}

/** 把各检索词的答复并成一栏。 */
export function similarPartiesOf(answers: readonly (SimilarPartyAnswer | null)[], limit: number = similarPartiesShown): SimilarParties {
  const found: BusinessPartyRecord[] = [];
  const seen = new Set<string>();
  let more = false;
  let unread = false;
  for (const answer of answers) {
    if (answer === null) {
      unread = true;
      continue;
    }
    if (answer.hasNext) more = true;
    for (const party of answer.parties) {
      if (seen.has(party.partyId)) continue;
      seen.add(party.partyId);
      found.push(party);
    }
  }
  return { shown: found.slice(0, limit), more: more || found.length > limit, unread };
}
