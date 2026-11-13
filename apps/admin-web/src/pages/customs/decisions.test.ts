import { deepEqual, equal, match, ok } from 'node:assert/strict';
import { test } from 'node:test';
import { caseDecisionTargets, customsDecisionEndpoints, customsDecisionHints, customsDecisionTitles, restrictionDecisionTargets } from './decisions';
import { casesTabOf } from './cases-address';

test('清关的每个运营决定口都有路径、标题与填写提示', () => {
  for (const kind of Object.keys(customsDecisionEndpoints) as (keyof typeof customsDecisionEndpoints)[]) {
    match(customsDecisionEndpoints[kind], /^\/customs-[a-z-]+$/);
    ok(customsDecisionTitles[kind].length > 0, kind);
    ok(customsDecisionHints[kind].includes('请填'), kind);
  }
});

test('清关单页挂五个口、限制页挂两个口，选册键不重复', () => {
  deepEqual(
    caseDecisionTargets.map((target) => target.id),
    ['case-establishment', 'submission-authorization', 'declaration-submission', 'release-gate-verification', 'case-closure'],
  );
  deepEqual(
    restrictionDecisionTargets.map((target) => target.endpoint),
    ['POST /customs-regulatory-restrictions', 'POST /customs-regulatory-restriction-releases'],
  );
  const ids = [...caseDecisionTargets, ...restrictionDecisionTargets].map((target) => target.id);
  equal(new Set(ids).size, ids.length);
});

test('运营决定签在地址里认得出来', () => {
  equal(casesTabOf('#/customs-cases?tab=decisions'), 'decisions');
});
