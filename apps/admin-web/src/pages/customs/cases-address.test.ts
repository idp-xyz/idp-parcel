import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { casesHash, casesTabOf, unadmittedObjectsHash, unadmittedOnlyOf } from './cases-address';

test('地址里没写签时落在清关单签、不只看卡关', () => {
  equal(casesTabOf('#/customs-cases'), 'cases');
  equal(unadmittedOnlyOf('#/customs-cases'), false);
});

test('首页卡关那一格的地址落在放行记录签、只看卡关', () => {
  equal(unadmittedObjectsHash, '#/customs-cases?tab=admission&only=unadmitted');
  equal(casesTabOf(unadmittedObjectsHash), 'admission');
  equal(unadmittedOnlyOf(unadmittedObjectsHash), true);
});

test('只看卡关只跟着放行记录签走；换签清掉，别的键原样', () => {
  equal(casesHash('#/customs-cases?q=PCL&tab=admission&only=unadmitted', { tab: 'results', unadmittedOnly: true }), '#/customs-cases?q=PCL&tab=results');
  equal(casesHash(unadmittedObjectsHash, { tab: 'admission', unadmittedOnly: false }), '#/customs-cases?tab=admission');
  equal(casesHash(unadmittedObjectsHash, { tab: 'cases', unadmittedOnly: false }), '#/customs-cases');
});

test('认不得的签与筛选退回缺省，不猜', () => {
  equal(casesTabOf('#/customs-cases?tab=held'), 'cases');
  equal(unadmittedOnlyOf('#/customs-cases?tab=admission&only=blocked'), false);
});
