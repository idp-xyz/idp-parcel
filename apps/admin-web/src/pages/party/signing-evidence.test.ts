import { test } from 'node:test';
import { deepEqual, equal, ok } from 'node:assert/strict';
import { publicationOutcomeLabels } from './api';
import { publishOutcomeLabels } from './publication-draft-api';
import {
  signingEvidenceColumnText,
  signingEvidenceLocalProblem,
  signingEvidenceRegistrationPayload,
  signingEvidenceRequirementPayload,
  signingEvidenceRequirementText,
  type SigningEvidenceDraft,
} from './signing-evidence';

const draft: SigningEvidenceDraft = {
  evidenceId: ' evidence-1 ',
  signedOn: '2026-11-01',
  counterpartySignatory: ' 甲方 ',
  ourSignatory: '',
  fileName: 'signed.pdf',
  sha256: 'ab'.repeat(32),
  libraryIndex: ' EVD-1 ',
  corrects: '',
  recordedAt: '2026-11-02T08:00:00.000Z',
};

test('登记载荷不带租户和登记人，来源只有线下签署', () => {
  const payload = signingEvidenceRegistrationPayload('CUSTOMER_CONTRACT', 'contract-1', 'v1', draft);
  deepEqual(payload, {
    evidenceId: 'evidence-1',
    kind: 'CUSTOMER_CONTRACT',
    objectId: 'contract-1',
    version: 'v1',
    signedOn: '2026-11-01',
    fileName: 'signed.pdf',
    sha256: 'ab'.repeat(32),
    libraryIndex: 'EVD-1',
    source: 'OFFLINE_SIGNED',
    recordedAt: '2026-11-02T08:00:00.000Z',
    counterpartySignatory: '甲方',
  });
  ok(!('tenantId' in payload));
  ok(!('recordedBy' in payload));
});

test('要求载荷两个布尔都在，即使都是不要', () => {
  deepEqual(signingEvidenceRequirementPayload(false, false), {
    requiresCustomerContract: false,
    requiresSupplierAgreement: false,
  });
});

test('要求状态与目录列的中文', () => {
  equal(signingEvidenceRequirementText({ requirement: 'NOT_CONFIGURED' }), '签署证据要求：未配置');
  equal(
    signingEvidenceRequirementText({
      requirement: 'CONFIGURED',
      requiresCustomerContract: true,
      requiresSupplierAgreement: false,
    }),
    '签署证据要求：已配置。客户合同要有签署证据才能发布；供应商协议不作为发布前提。',
  );
  equal(signingEvidenceColumnText(true), '已登记');
  equal(signingEvidenceColumnText(false), '未登记');
  equal(signingEvidenceColumnText(undefined), '未登记');
  ok(publishOutcomeLabels.SIGNING_EVIDENCE_REQUIRED.includes('签署证据'));
  ok(publicationOutcomeLabels.SIGNING_EVIDENCE_REQUIRED.includes('签署证据'));
  ok(signingEvidenceLocalProblem({ ...draft, fileName: 'dir/signed.pdf' })?.includes('路径'));
  ok(signingEvidenceLocalProblem({ ...draft, corrects: 'evidence-1', evidenceId: 'evidence-1' })?.includes('自己'));
});
