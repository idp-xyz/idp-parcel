import { test } from 'node:test';
import { equal } from 'node:assert/strict';
import { hasUnsavedDraft, publicationGroup, registrationGroup, setDraftUnsaved } from './unsaved-guard';

test('簿按实例记、按组查：一个实例报已提交，不抹掉同组另一个实例的没提交', () => {
  const group = publicationGroup('CUSTOMER_CONTRACT');
  equal(hasUnsavedDraft(group), false);
  setDraftUnsaved('a', group, true);
  setDraftUnsaved('b', group, true);
  equal(hasUnsavedDraft(group), true);
  setDraftUnsaved('a', group, false);
  equal(hasUnsavedDraft(group), true);
  setDraftUnsaved('b', group, false);
  equal(hasUnsavedDraft(group), false);
});

test('组之间互不相干；卸载后重复移出无害', () => {
  setDraftUnsaved('x', publicationGroup('SERVICE_PRODUCT'), true);
  equal(hasUnsavedDraft(publicationGroup('SUPPLIER_AGREEMENT')), false);
  equal(hasUnsavedDraft(registrationGroup('business-party')), false);
  setDraftUnsaved('x', publicationGroup('SERVICE_PRODUCT'), false);
  setDraftUnsaved('x', publicationGroup('SERVICE_PRODUCT'), false);
  equal(hasUnsavedDraft(publicationGroup('SERVICE_PRODUCT')), false);
});

test('价格政策在政策页叫 PRICE_POLICY、在发布轴叫 PRICE_RULE，同属一组', () => {
  equal(publicationGroup('PRICE_POLICY'), publicationGroup('PRICE_RULE'));
});
