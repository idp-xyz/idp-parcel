import { exchangeMasterData, postMasterData, type ApiResult } from '../catalogue-api';
import { afterPartyWrite } from './party-writes';

// 签署证据挂在客户合同或供应商协议的某一版旁边。写面不送 tenantId、recordedBy：那两格由认证信封填。

export const signingEvidenceRegistrationPath = '/commercial-signing-evidence-registrations';
export const signingEvidenceRequirementPath = '/commercial-signing-evidence-requirement-registrations';

export interface SigningEvidenceRecord {
  evidenceId: string;
  signedOn: string;
  counterpartySignatory?: string;
  ourSignatory?: string;
  fileName: string;
  sha256: string;
  libraryIndex: string;
  source: string;
  corrects?: string;
  recordedBy: string;
  recordedAt: string;
}

export interface SigningEvidenceReadBody {
  outcome: 'SIGNING_EVIDENCES_READ';
  requirement: 'NOT_CONFIGURED' | 'CONFIGURED';
  requiresCustomerContract?: boolean;
  requiresSupplierAgreement?: boolean;
  evidences: SigningEvidenceRecord[];
}

export interface SigningEvidenceWriteBody {
  outcome: string;
}

export interface SigningEvidenceDraft {
  evidenceId: string;
  signedOn: string;
  counterpartySignatory: string;
  ourSignatory: string;
  fileName: string;
  sha256: string;
  libraryIndex: string;
  corrects: string;
  recordedAt: string;
}

export function signingEvidenceReadPath(kind: string, objectId: string, version: string): string {
  return `/commercial-signing-evidences/${encodeURIComponent(kind)}/${encodeURIComponent(objectId)}/${encodeURIComponent(version)}`;
}

export function readSigningEvidences(kind: string, objectId: string, version: string): Promise<ApiResult<SigningEvidenceReadBody>> {
  return exchangeMasterData<SigningEvidenceReadBody>(signingEvidenceReadPath(kind, objectId, version));
}

/** 目录列上的那一格。缺键与 false 都是还没有，不把缺键读成「已登记」。 */
export function signingEvidenceColumnText(registered: boolean | undefined): string {
  return registered ? '已登记' : '未登记';
}

export function signingEvidenceRequirementText(body: Pick<SigningEvidenceReadBody, 'requirement' | 'requiresCustomerContract' | 'requiresSupplierAgreement'>): string {
  if (body.requirement !== 'CONFIGURED') return '签署证据要求：未配置';
  const contract = body.requiresCustomerContract ? '要有签署证据才能发布' : '不作为发布前提';
  const agreement = body.requiresSupplierAgreement ? '要有签署证据才能发布' : '不作为发布前提';
  return `签署证据要求：已配置。客户合同${contract}；供应商协议${agreement}。`;
}

export function signingEvidenceRegistrationPayload(
  kind: 'CUSTOMER_CONTRACT' | 'SUPPLIER_AGREEMENT',
  objectId: string,
  version: string,
  draft: SigningEvidenceDraft,
): Record<string, string> {
  const payload: Record<string, string> = {
    evidenceId: draft.evidenceId.trim(),
    kind,
    objectId,
    version,
    signedOn: draft.signedOn.trim(),
    fileName: draft.fileName.trim(),
    sha256: draft.sha256.trim(),
    libraryIndex: draft.libraryIndex.trim(),
    source: 'OFFLINE_SIGNED',
    recordedAt: draft.recordedAt,
  };
  const counterparty = draft.counterpartySignatory.trim();
  const ours = draft.ourSignatory.trim();
  const corrects = draft.corrects.trim();
  if (counterparty !== '') payload.counterpartySignatory = counterparty;
  if (ours !== '') payload.ourSignatory = ours;
  if (corrects !== '') payload.corrects = corrects;
  return payload;
}

export function signingEvidenceRequirementPayload(requiresCustomerContract: boolean, requiresSupplierAgreement: boolean): {
  requiresCustomerContract: boolean;
  requiresSupplierAgreement: boolean;
} {
  return { requiresCustomerContract, requiresSupplierAgreement };
}

export function signingEvidenceLocalProblem(draft: SigningEvidenceDraft): string | null {
  if (draft.evidenceId.trim() === '') return '请填写证据标识。';
  if (!/^\d{4}-\d{2}-\d{2}$/.test(draft.signedOn.trim())) return '签署日期用 YYYY-MM-DD。';
  if (draft.fileName.trim() === '' || /[/\\]/.test(draft.fileName)) return '文件名不能带路径。';
  if (!/^[0-9a-f]{64}$/.test(draft.sha256.trim())) return 'SHA-256 要 64 位小写十六进制。';
  if (draft.libraryIndex.trim() === '') return '请填写受限证据库索引。';
  if (draft.corrects.trim() !== '' && draft.corrects.trim() === draft.evidenceId.trim()) return '不能更正自己。';
  return null;
}

export const signingEvidenceOutcomeLabels: Record<string, string> = {
  SAVED: '已登记。',
  ALREADY_REGISTERED: '这条证据已经登记过同样的内容，没有再写一条。',
  CONTENT_CONFLICT: '这个证据标识已经登记过不同的内容，没有覆盖。',
  TARGET_NOT_FOUND: '这一版还不在册上，也没有待批准的发布稿，所以不能登记签署证据。',
  ALREADY_CORRECTED: '被更正的那一条已经有过更正，不能再更正一次。',
  CROSS_VERSION: '不能更正另一版上的签署证据。',
  CORRECTS_NOT_FOUND: '指名要更正的那条签署证据不存在。',
};

export function registerSigningEvidence(
  kind: 'CUSTOMER_CONTRACT' | 'SUPPLIER_AGREEMENT',
  objectId: string,
  version: string,
  draft: SigningEvidenceDraft,
): Promise<ApiResult<SigningEvidenceWriteBody>> {
  return afterPartyWrite(
    postMasterData<SigningEvidenceWriteBody>(
      signingEvidenceRegistrationPath,
      signingEvidenceRegistrationPayload(kind, objectId, version, draft),
    ),
  );
}

export function registerSigningEvidenceRequirement(
  requiresCustomerContract: boolean,
  requiresSupplierAgreement: boolean,
): Promise<ApiResult<SigningEvidenceWriteBody>> {
  return afterPartyWrite(
    postMasterData<SigningEvidenceWriteBody>(
      signingEvidenceRequirementPath,
      signingEvidenceRequirementPayload(requiresCustomerContract, requiresSupplierAgreement),
    ),
  );
}
