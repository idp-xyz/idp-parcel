// 发布表单里「这个对象已有哪些版本」的提示（票 party-commercial-uiux/15）。
//
// 改动靠发新版本，版本号是租户自己起的名，表单不替人起；但人起之前该看得见这个对象已有什么：已有 v1、v2、v3（v3 在用）。
// 提示三层：
//   - 已有版本：标识对上了某个对象，列出它的各版与在用的那版；标识对不上任何对象、也不是某个对象标识的前缀，说「这会是第一版」
//     ——给已有对象发新版本时多敲或少敲一个字符，会悄悄成了另一个对象；正在敲的标识（是已有标识的前缀）不提示，免得闪。
//   - 撞号：版本号与该对象某一版相同。同一个商业对象版本只能发布一次（PC CONTEXT），换号。
//   - 区间重叠：新版的生效区间与该对象一个在用或已计划生效的版本重叠。新版发布本身不收尾旧版，重叠不阻发布，但重叠的部分在
//     旧版收尾（截止在新版生效时点，或同起点退役）之前，解析答「适用冲突」（PC CONTEXT、ADR-0233）。
//
// 重叠只对读面带状态的册判：状态是判「旧版已退役 / 到期 / 被替代」的唯一依据，没有状态的册（信用政策、结算政策、价格政策、接单
// 规则包）读不出旧版是否已经收尾，报了就可能是错报，所以只列版本与查撞号。区间是哪一层的也要同层相比：带状态的册读面上的
// 区间是版本壳的，表单里取壳的那一对。
//
// 纯函数，node:test 直接跑。

import { formatRange } from '../catalogue-view';
import type {
  CommercialPolicyListResponseBody,
  CustomerContractListResponseBody,
  ServiceProductListResponseBody,
  SupplierAgreementListResponseBody,
} from './api';
import { commercialStatusLabels, labelOf } from './presentation';

export interface KnownVersion {
  objectId: string;
  version: string;
  /** 服务端的状态码（`EFFECTIVE` 等）；读面没有状态的册不给。 */
  status?: string;
  effectiveStartsAt: string;
  effectiveEndsAt?: string;
}

/** 这些状态的版本还在参与解析（或已计划生效），新版与它们区间重叠就会在重叠处冲突。 */
const stillResolving = new Set(['EFFECTIVE', 'PUBLISHED']);

const versionOrder = new Intl.Collator('en', { numeric: true, sensitivity: 'base' });

export interface VersionHint {
  /** 该对象的各版，新的在前。空即没有这个对象。 */
  versions: KnownVersion[];
  /** 输入的标识是某个已有标识的前缀（还在敲），不报「第一版」。 */
  typingKnownPrefix: boolean;
  /** 与输入的版本号相同的那一版。 */
  sameVersion: KnownVersion | null;
  /** 与新版区间重叠、且还在参与解析的版本。 */
  overlaps: KnownVersion[];
}

export interface HintInput {
  objectId: string;
  version: string;
  /** 新版生效起点（UTC 的 RFC 3339）；没填或换不成时刻即 null，不判重叠。 */
  startsAt: string | null;
  /** 新版生效止点；null 即持续有效（没填）。 */
  endsAt: string | null;
}

export function versionHintOf(known: readonly KnownVersion[], input: HintInput): VersionHint | null {
  const objectId = input.objectId.trim();
  if (objectId === '') return null;
  const versions = known
    .filter((row) => row.objectId === objectId)
    .sort((a, b) => b.effectiveStartsAt.localeCompare(a.effectiveStartsAt) || versionOrder.compare(b.version, a.version));
  const typingKnownPrefix = versions.length === 0 && known.some((row) => row.objectId.startsWith(objectId));
  const version = input.version.trim();
  const sameVersion = version === '' ? null : (versions.find((row) => row.version === version) ?? null);
  return { versions, typingKnownPrefix, sameVersion, overlaps: overlapsOf(versions, input) };
}

function overlapsOf(versions: readonly KnownVersion[], input: HintInput): KnownVersion[] {
  if (input.startsAt === null) return [];
  const newStart = Date.parse(input.startsAt);
  const newEnd = input.endsAt === null ? Number.POSITIVE_INFINITY : Date.parse(input.endsAt);
  if (Number.isNaN(newStart) || Number.isNaN(newEnd)) return [];
  const version = input.version.trim();
  return versions.filter((row) => {
    // 同号的那一版由撞号那条说，不再当「旧版」报重叠。
    if (row.version === version) return false;
    if (row.status === undefined || !stillResolving.has(row.status)) return false;
    const start = Date.parse(row.effectiveStartsAt);
    const end = row.effectiveEndsAt === undefined ? Number.POSITIVE_INFINITY : Date.parse(row.effectiveEndsAt);
    if (Number.isNaN(start) || Number.isNaN(end)) return false;
    return newStart < end && start < newEnd;
  });
}

export interface HintLine {
  tone: 'info' | 'warn';
  text: string;
}

function versionLabel(row: KnownVersion): string {
  if (row.status === undefined) return row.version;
  return `${row.version}（${row.status === 'EFFECTIVE' ? '在用' : labelOf(commercialStatusLabels, row.status)}）`;
}

/** 提示的几句话；没有可说的就是空数组。 */
export function versionHintLines(hint: VersionHint, objectId: string): HintLine[] {
  const id = objectId.trim();
  const lines: HintLine[] = [];
  if (hint.versions.length > 0) {
    lines.push({ tone: 'info', text: `「${id}」已有版本：${hint.versions.map(versionLabel).join('、')}。` });
  } else if (!hint.typingKnownPrefix) {
    lines.push({
      tone: 'info',
      text: `还没有叫「${id}」的对象，这会是它的第一版。如果想给已有对象发新版本，请核对标识。`,
    });
  }
  if (hint.sameVersion) {
    lines.push({ tone: 'warn', text: `版本号 ${hint.sameVersion.version} 已经发布过；同一个版本号只能发布一次，请换一个。` });
  }
  if (hint.overlaps.length > 0) {
    const items = hint.overlaps
      .map((row) => `${row.version}（${formatRange(row.effectiveStartsAt, row.effectiveEndsAt)}）`)
      .join('；');
    lines.push({
      tone: 'warn',
      text:
        `新版本的生效区间与 ${items} 重叠。两版都能发布，但重叠的部分在旧版收尾（把它截止在新版生效时点，或同起点退役）之前，` +
        '解析会答「适用冲突」。',
    });
  }
  return lines;
}

// ——读口响应 → 已知版本。带状态的册给状态，没有状态的册（信用政策、结算政策、价格政策、接单规则包）不给，见文件头。

export function contractKnownVersions(body: CustomerContractListResponseBody): KnownVersion[] {
  return body.contracts.map((row) => ({
    objectId: row.objectId,
    version: row.version,
    status: row.status,
    effectiveStartsAt: row.effectiveStartsAt,
    effectiveEndsAt: row.effectiveEndsAt,
  }));
}

export function agreementKnownVersions(body: SupplierAgreementListResponseBody): KnownVersion[] {
  return body.agreements.map((row) => ({
    objectId: row.objectId,
    version: row.version,
    status: row.status,
    effectiveStartsAt: row.effectiveStartsAt,
    effectiveEndsAt: row.effectiveEndsAt,
  }));
}

export function productKnownVersions(body: ServiceProductListResponseBody): KnownVersion[] {
  return body.products.map((row) => ({
    objectId: row.objectId,
    version: row.version,
    status: row.status,
    effectiveStartsAt: row.effectiveStartsAt,
    effectiveEndsAt: row.effectiveEndsAt,
  }));
}

/** 政策册按册判别；随所属版本发布的两本册（接受前财务控制、接受判断时点策略）没有自己的版本，给空。 */
export function policyKnownVersions(body: CommercialPolicyListResponseBody): KnownVersion[] {
  switch (body.kind) {
    case 'ACCEPTANCE_RULE_PACKAGE':
    case 'PRICE_POLICY':
    case 'SETTLEMENT_POLICY':
    case 'CREDIT_POLICY':
      return body.policies.map((row) => ({
        objectId: row.objectId,
        version: row.version,
        effectiveStartsAt: row.effectiveStartsAt,
        effectiveEndsAt: row.effectiveEndsAt,
      }));
    case 'AUTHORIZATION_RULE':
    case 'PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY':
    case 'CUSTOMER_SERVICE_RULE':
      return body.policies.map((row) => ({
        objectId: row.objectId,
        version: row.version,
        status: row.status,
        effectiveStartsAt: row.effectiveStartsAt,
        effectiveEndsAt: row.effectiveEndsAt,
      }));
    case 'PRE_ACCEPTANCE_CONTROL':
    case 'AS_OF_POLICY':
      return [];
  }
}
