// 往来单位工作台右栏各签「列几条、签上的数是多少、联系人行里的头像字与平台怎么取」的判读（票 business-party-redesign/05）。
// 纯函数，node:test 钉着；BusinessPartyDetailPanel.tsx、BusinessPartyPanelSummary.tsx 与 BusinessPartyPanelRelated.tsx 只照着摆。

import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyProfileRevisionListResponseBody } from './api';
import { effectiveBusinessPartyProfile } from './business-party-profile';
import type { ContactCard } from './legal-entity-contact-cards';
import type { OverviewSectionState } from './party-overview';

/** 概要的「合作关系」卡只列最近这么几段、「联系人」卡只列前这么几位：概要是一眼看完的一屏，整份在各自的签里。 */
export const summaryRelationshipLimit = 5;
export const summaryContactLimit = 5;

/**
 * 「关联」签的合作关系表只列最近这么多段：我方公司名下可以有成百上千段，一张不翻页的表列不完；再多的去「合作关系」视图按这家
 * 单位筛看。
 */
export const relatedRelationshipLimit = 50;

/** 列前 `limit` 条，交回没列出的条数。先后由调用方定（合作关系是生效起点新→旧，relationshipsOfParty）。 */
export function leadingRows<Row>(rows: readonly Row[], limit: number): { listed: Row[]; rest: number } {
  return { listed: rows.slice(0, limit), rest: Math.max(rows.length - limit, 0) };
}

export interface PanelProfileTabCounts {
  contacts: number | null;
  versions: number | null;
}

/**
 * 「联系人」「资料版本」两枚签上的数。读到这一家的答复才报，零也报——「一位都没有」是答案；还在读、读不到、答的是别家（换行时
 * 上一家的答复还没换掉）都不报，不拿 0 冒充「没有」。联系人数的是此刻有效那一版里的，没有生效版就是 0。
 */
export function panelProfileTabCountsOf(
  answer: ApiResult<BusinessPartyProfileRevisionListResponseBody> | null,
  partyId: string,
): PanelProfileTabCounts {
  if (answer === null || answer.kind !== 'outcome' || answer.body.partyId !== partyId) return { contacts: null, versions: null };
  return {
    contacts: (effectiveBusinessPartyProfile(answer.body)?.contacts ?? []).length,
    versions: answer.body.revisions.length,
  };
}

/** 概要「关联对象」卡与「关联」签里另外四册的先后：先是它作为客户的账户，再是它作为供应商的协议、作为客户的结算政策，最后是钉着它的公司主体。 */
export const panelRelatedObjectSections = ['accounts', 'agreements', 'settlement', 'entities'] as const;
export type PanelRelatedObjectSection = (typeof panelRelatedObjectSections)[number];

export type PanelRelatedStates = Record<'relationships' | PanelRelatedObjectSection, OverviewSectionState<unknown>>;

/** 从概要跳进「关联」签时落在哪：某一段的段头，或合作关系表里的某一段（落到那一行并把它展开）。 */
export interface PanelRelatedTarget {
  section: 'relationships' | PanelRelatedObjectSection;
  relationshipId?: string;
}

/**
 * 「关联」签上的数：合作关系的段数加另外四册里与这家有关的行数。五段都拿到业务答案才报——少一段就不是全数，报出来会被当成
 * 「就这些」；合作关系册不止一页时到手的只是其中一页，同样不报。
 */
export function panelRelatedCountOf(states: PanelRelatedStates, relationshipsComplete: boolean): number | null {
  if (!relationshipsComplete) return null;
  let total = 0;
  for (const state of Object.values(states)) {
    if (state.kind !== 'answered') return null;
    total += state.rows.length;
  }
  return total;
}

const lastCharacterNames = /\p{Script=Han}|\p{Script=Hiragana}|\p{Script=Katakana}|\p{Script=Hangul}/u;

/**
 * 联系人头像里的那一个字。汉字、假名、谚文写的名字取末字——同姓的几位末字多半不同，取姓就都一样了；其余文字取首字母大写。
 * 按码点取，不把一个字劈成半个；名字空着就不放字。
 */
export function contactInitialOf(name: string): string {
  const characters = [...name.trim()];
  if (characters.length === 0) return '';
  const last = characters[characters.length - 1];
  return lastCharacterNames.test(last) ? last : characters[0].toLocaleUpperCase();
}

/** 概要里联系人那一行只报有哪几个平台（账号在「联系人」签）：按登记的先后，同一个平台只报一次，没写平台的不报。 */
export function contactPlatformsOf(card: Pick<ContactCard, 'socialAccounts'>): string[] {
  return [...new Set(card.socialAccounts.map((account) => account.platform).filter((platform) => platform !== ''))];
}
