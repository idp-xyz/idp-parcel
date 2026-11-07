import { test } from 'node:test';
import { deepEqual, equal } from 'node:assert/strict';
import { catalogueConditionsNarrow, catalogueRequestPath } from '../catalogue-query';
import {
  allStatuses,
  awaitingJudgmentOnPage,
  claimsRegistryConditions,
  defaultCaseSort,
  defaultEpisodeSort,
  defaultRequestSort,
  defaultTrackingSort,
  dispositionRequestConditions,
  episodeOnPage,
  exceptionCaseConditions,
  requestOnPage,
  signalEpisodeConditions,
  supersededOnPage,
  trackingListConditions,
  untriagedOnPage,
  workbenchSortParam,
} from './list-conditions';
import type { SignalEpisodeRecord } from './case-api';

// 本文件钉物流动态与异常区各页屏上的检索、胶囊与表头怎么换成读口的条件（票 catalogue-read-pagination-rollout/14）：缺省序不进
// 查询串，第一页不带检索即裸路径；读口点名的维下推，读口不认的派生状态留成只筛本页、不进查询串。

test('表头排序态换成读口的 sort：缺省序不写，正序写维名，倒序加 -，没对应维的列当缺省', () => {
  const fields = { parcel: 'parcel', 'derived-at': 'derivedAt' };
  equal(workbenchSortParam(defaultTrackingSort, fields, defaultTrackingSort), undefined);
  equal(workbenchSortParam({ key: 'derived-at', dir: 1 }, fields, defaultTrackingSort), 'derivedAt');
  equal(workbenchSortParam({ key: 'parcel', dir: -1 }, fields, defaultTrackingSort), '-parcel');
  equal(workbenchSortParam({ key: 'parcel', dir: 1 }, fields, defaultTrackingSort), 'parcel');
  equal(workbenchSortParam({ key: 'number', dir: 1 }, fields, defaultTrackingSort), undefined);
});

test('物流动态：检索词与表头下推，不带检索的第一页是裸路径', () => {
  equal(catalogueRequestPath('/tracking-projections', trackingListConditions('', defaultTrackingSort), null), '/tracking-projections');
  equal(
    catalogueRequestPath('/tracking-projections', trackingListConditions('DPCL', { key: 'parcel', dir: 1 }), null),
    '/tracking-projections?sort=parcel&q=DPCL',
  );
});

test('异常案件：状态胶囊下推成 phase，「全部」不带这一维', () => {
  equal(catalogueRequestPath('/exception-case-records', exceptionCaseConditions('', allStatuses, defaultCaseSort), null), '/exception-case-records');
  const closed = exceptionCaseConditions('', 'CLOSED', { key: 'parcel', dir: -1 });
  equal(catalogueRequestPath('/exception-case-records', closed, null), '/exception-case-records?sort=-rootParcel&phase=CLOSED');
  equal(catalogueConditionsNarrow(closed), true);
  equal(catalogueConditionsNarrow(exceptionCaseConditions('', allStatuses, { key: 'established', dir: 1 })), false);
});

test('异常信号：分诊结论下推成 outcome；「待分诊」只筛本页，不进查询串、不算收窄', () => {
  equal(
    catalogueRequestPath('/exception-triage-records', signalEpisodeConditions('', 'MANUAL_REVIEW', defaultEpisodeSort), null),
    '/exception-triage-records?registry=signal-episode&outcome=MANUAL_REVIEW',
  );
  const untriaged = signalEpisodeConditions('', untriagedOnPage, { key: 'last-hit', dir: -1 });
  equal(catalogueRequestPath('/exception-triage-records', untriaged, null), '/exception-triage-records?registry=signal-episode&sort=-lastHitAt');
  equal(catalogueConditionsNarrow(untriaged), false);

  const triaged = { triage: { outcome: 'NO_CASE', rule: 'R', triagedAt: '2026-10-28T10:00:00Z' } } as SignalEpisodeRecord;
  const pending = {} as SignalEpisodeRecord;
  equal(episodeOnPage(pending, untriagedOnPage), true);
  equal(episodeOnPage(triaged, untriagedOnPage), false);
  // 下推了的胶囊读口已筛过，本页原样放行。
  equal(episodeOnPage(triaged, 'NO_CASE'), true);
  equal(episodeOnPage(pending, allStatuses), true);
});

test('处置请求：答复下推成 judgment；「等对方答复」「已替代」只筛本页', () => {
  equal(
    catalogueRequestPath('/exception-triage-records', dispositionRequestConditions('案件', 'REFUSED', defaultRequestSort), null),
    '/exception-triage-records?registry=disposition-request&judgment=REFUSED&q=%E6%A1%88%E4%BB%B6',
  );
  equal(
    catalogueRequestPath('/exception-triage-records', dispositionRequestConditions('', supersededOnPage, { key: 'sent', dir: 1 }), null),
    '/exception-triage-records?registry=disposition-request&sort=sentAt',
  );
  equal(requestOnPage({}, awaitingJudgmentOnPage), true);
  equal(requestOnPage({ judgment: 'ACCEPTED' }, awaitingJudgmentOnPage), false);
  // 被替代而还没答复的请求不算「等对方答复」：在办的已是新的一版。
  equal(requestOnPage({ supersededBy: 'REQ-2' }, awaitingJudgmentOnPage), false);
  equal(requestOnPage({ supersededBy: 'REQ-2', judgment: 'ACCEPTED' }, supersededOnPage), true);
  equal(requestOnPage({ judgment: 'ACCEPTED' }, supersededOnPage), false);
  equal(requestOnPage({ judgment: 'ACCEPTED' }, 'ACCEPTED'), true);
});

test('索赔追偿三册只下推检索词，registry 是选择器', () => {
  deepEqual(claimsRegistryConditions('claim-item', ' SYN '), { q: ' SYN ', selectors: { registry: 'claim-item' } });
  equal(
    catalogueRequestPath('/claims-recovery-records', claimsRegistryConditions('recovery-matter', ''), null),
    '/claims-recovery-records?registry=recovery-matter',
  );
});
