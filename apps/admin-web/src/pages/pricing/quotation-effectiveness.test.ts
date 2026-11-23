import { test } from 'node:test';
import { equal, match } from 'node:assert/strict';
import {
  quotationCanPublish,
  quotationEffectivenessLines,
  quotationNextStepText,
  quotationPublicationRequirementOutcomeLabels,
  quotationPublicationRequirementText,
} from './quotation-effectiveness';

test('发布前提三态与签署证据要求一样，没登记答未配置', () => {
  equal(quotationPublicationRequirementText(undefined), '发布以接受为前提：未配置');
  equal(quotationPublicationRequirementText('UNCONFIGURED'), '发布以接受为前提：未配置');
  equal(quotationPublicationRequirementText('REQUIRED'), '发布以接受为前提：要求');
  equal(quotationPublicationRequirementText('NOT_REQUIRED'), '发布以接受为前提：不要求');
});

test('生效三格与下一步各说自己的话', () => {
  const lines = quotationEffectivenessLines({
    priceOnRegister: true,
    selectsNow: false,
    scopeNamed: true,
    validAcceptance: true,
    recordedPublicationBasis: false,
  });
  equal(lines.length, 5);
  match(lines[1], /不在适用期内/);
  equal(quotationNextStepText('NOT_SELECTABLE').includes('不在适用期内'), true);
  equal(quotationNextStepText('ALREADY_EFFECTIVE').includes('已在册'), true);
  equal(quotationNextStepText('PUBLISH_CONTRACT').includes('发布指名这个范围的新一版合同'), true);
  equal(quotationNextStepText(undefined).includes('发布指名这个范围的新一版合同'), true);
});

test('发布按钮只在合同已指名、已批准草稿且未撤下、还没在册时出现', () => {
  const ready = { lane: 'APPROVED_DRAFT', scopeNamed: true, priceOnRegister: false, referencedDraftWithdrawn: false };
  equal(quotationCanPublish(ready), true);
  equal(quotationCanPublish({ ...ready, scopeNamed: false }), false);
  equal(quotationCanPublish({ ...ready, priceOnRegister: true }), false);
  equal(quotationCanPublish({ ...ready, referencedDraftWithdrawn: true }), false);
  equal(quotationCanPublish({ ...ready, lane: 'REGISTERED_CARD' }), false);
});

test('规则登记的答复用中文，重放与冲突不覆盖', () => {
  equal(quotationPublicationRequirementOutcomeLabels.RECORDED, '已登记。');
  match(quotationPublicationRequirementOutcomeLabels.ALREADY_RECORDED, /没有再写/);
  match(quotationPublicationRequirementOutcomeLabels.CONTENT_CONFLICT, /没有覆盖/);
});
