// 往来单位工作台右栏各签的判读（票 business-party-redesign/05）：列几条、签上的数、联系人行里的头像字与平台。

import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import type { BusinessPartyProfileRevisionRecord } from './api';
import {
  contactInitialOf,
  contactPlatformsOf,
  leadingRows,
  panelProfileTabCountsOf,
  panelRelatedCountOf,
  panelRelatedObjectSections,
  relatedRelationshipLimit,
  summaryRelationshipLimit,
  type PanelRelatedStates,
} from './business-party-panel-content';

const record: BusinessPartyProfileRevisionRecord = {
  tenantId: 'SYN-TENANT-01',
  partyId: 'SYN-PARTY-SHIPPER-01',
  revision: 1,
  basis: 'SYN-BASIS',
  effectiveFrom: '2026-01-01T00:00:00Z',
  registeredAt: '2026-01-01T00:00:00Z',
  taxRegistrationNumbers: [],
  invoicingRegistered: false,
  contacts: [{ name: '林悦' }, { name: '周航' }],
};

// Covers: 只列前几条并交回没列出的条数；不够数就全列、剩 0；概要与「关联」签各自的上限是 5 与 50。
test('leadingRows 只列前几条', () => {
  const rows = Array.from({ length: 7 }, (_, index) => `SYN-REL-${index + 1}`);
  deepEqual(leadingRows(rows, summaryRelationshipLimit), { listed: rows.slice(0, 5), rest: 2 });
  deepEqual(leadingRows(rows.slice(0, 5), summaryRelationshipLimit), { listed: rows.slice(0, 5), rest: 0 });
  deepEqual(leadingRows([], relatedRelationshipLimit), { listed: [], rest: 0 });
  equal(leadingRows(Array.from({ length: 51 }, (_, index) => index), relatedRelationshipLimit).rest, 1);
});

// Covers: 两枚资料签上的数——读到这一家才报，零也报；在读、读不到、别家的答复都不报；联系人只数此刻有效那一版里的，
// 登过却没有生效版时联系人是 0、版本数照报。
test('panelProfileTabCountsOf 只报读到的数', () => {
  const party = 'SYN-PARTY-SHIPPER-01';
  const answer = (effectiveRevision: number | undefined, revisions: BusinessPartyProfileRevisionRecord[], partyId = party) =>
    ({ kind: 'outcome', status: 200, body: { outcome: 'X', partyId, effectiveRevision, revisions } }) as const;
  const unknown = { contacts: null, versions: null };
  deepEqual(panelProfileTabCountsOf(null, party), unknown);
  deepEqual(panelProfileTabCountsOf({ kind: 'transport', message: 'down' }, party), unknown);
  deepEqual(panelProfileTabCountsOf(answer(1, [record], 'SYN-PARTY-OTHER'), party), unknown);
  deepEqual(panelProfileTabCountsOf(answer(undefined, []), party), { contacts: 0, versions: 0 });
  deepEqual(panelProfileTabCountsOf(answer(undefined, [{ ...record, effectiveFrom: '2099-01-01T00:00:00Z' }]), party), { contacts: 0, versions: 1 });
  deepEqual(panelProfileTabCountsOf(answer(1, [record, { ...record, revision: 2, contacts: [] }]), party), { contacts: 2, versions: 2 });
});

// Covers: 「关联」签上的数是五段行数之和；任一段没拿到业务答案、或合作关系册不止一页，都不报；全是零报 0。
test('panelRelatedCountOf 五段都答出才报', () => {
  const answered = (count: number) => ({ kind: 'answered' as const, rows: Array.from({ length: count }, (_, index) => index) });
  const states: PanelRelatedStates = {
    relationships: answered(2),
    accounts: answered(1),
    agreements: answered(0),
    settlement: answered(3),
    entities: answered(0),
  };
  equal(panelRelatedCountOf(states, true), 6);
  equal(panelRelatedCountOf(states, false), null);
  equal(panelRelatedCountOf({ ...states, settlement: { kind: 'loading' } }, true), null);
  equal(panelRelatedCountOf({ ...states, agreements: { kind: 'transport', message: 'down' } }, true), null);
  equal(
    panelRelatedCountOf({ relationships: answered(0), accounts: answered(0), agreements: answered(0), settlement: answered(0), entities: answered(0) }, true),
    0,
  );
  deepEqual([...panelRelatedObjectSections], ['accounts', 'agreements', 'settlement', 'entities']);
});

// Covers: 头像字——汉字、假名、谚文取末字，其余取首字母大写；按码点取（扩展区汉字不劈成半个）；前后空白不算，空名不放字。
test('contactInitialOf 取头像里的那个字', () => {
  equal(contactInitialOf('林悦'), '悦');
  equal(contactInitialOf(' 周航 '), '航');
  equal(contactInitialOf('さくら'), 'ら');
  equal(contactInitialOf('김민준'), '준');
  equal(contactInitialOf('syn operator'), 'S');
  equal(contactInitialOf('Émile'), 'É');
  equal(contactInitialOf('王𠮷'), '𠮷');
  equal(contactInitialOf('  '), '');
});

// Covers: 名字尾部的括注不是名字——先剥掉再取，带括注的与不带的取到同一个字；可以连着几个、全角半角都算；整个名字都在括号
// 里时不剥；不是字母数字的开头跳过去。
test('contactInitialOf 不把尾部的括注当成名字', () => {
  equal(contactInitialOf('林悦（演示）'), '悦');
  equal(contactInitialOf('王伟 (财务)'), '伟');
  equal(contactInitialOf('周航（财务）（已离职）'), '航');
  equal(contactInitialOf('陈静【对账】'), '静');
  equal(contactInitialOf('Lynn (demo)'), 'L');
  equal(contactInitialOf('（演示）'), '演');
  equal(contactInitialOf('(demo) lynn'), 'D');
  equal(contactInitialOf('#7 ops'), '7');
});

// Covers: 概要里联系人行只报平台——按登记先后、同一平台只报一次、没写平台的不报；没有社交账号就是空。
test('contactPlatformsOf 只报有哪几个平台', () => {
  deepEqual(
    contactPlatformsOf({
      socialAccounts: [
        { platform: '微信', account: 'SYN-WX-1' },
        { platform: 'WhatsApp', account: 'SYN-WA-1' },
        { platform: '微信', account: 'SYN-WX-2' },
        { platform: '', account: 'SYN-X-1' },
      ],
    }),
    ['微信', 'WhatsApp'],
  );
  deepEqual(contactPlatformsOf({ socialAccounts: [] }), []);
});
