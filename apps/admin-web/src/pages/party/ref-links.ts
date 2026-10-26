// 参与方与商业区里「一格引用别的对象」的去处与名称（票 party-commercial-uiux/07）。
//
// 引用画成链接：点了去那个对象所在的页，并把它的标识当检索词带上（`?q=`），目标页从地址读检索词、筛到那一条。
// 参与方与责任法人有名称，从两本册各读一次建成目录，按标识查；合同、产品、规则包、策略今天没有名称，只显标识。
// 查不到名称时不猜、不报错，只显标识——名称是帮人认的，不是引用本身。
// 纯函数、只引零依赖的 address-query，node:test 直接跑。

import { ADDRESS_KEYWORD_PARAM, hashWithQueryValue } from '../../templates/address-query';
import type { BusinessPartyListResponseBody, CommercialPolicyKind, GroupLegalEntityListResponseBody } from './api';
import { policiesHash } from './commercial-policies-address';

export type RefKind =
  | 'party'
  | 'legal-entity'
  | 'contract'
  | 'service-product'
  | 'rule-package'
  | 'financial-control-policy';

function policyRegisterHash(register: CommercialPolicyKind): string {
  return policiesHash('#/commercial-policies', { tab: 'registers', register });
}

const pageOf: Record<RefKind, () => string> = {
  party: () => '#/business-parties',
  'legal-entity': () => '#/group-legal-entities',
  contract: () => '#/party-contracts',
  'service-product': () => '#/service-products',
  'rule-package': () => policyRegisterHash('ACCEPTANCE_RULE_PACKAGE'),
  'financial-control-policy': () => policyRegisterHash('PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY'),
};

/** 引用的去处：对象所在的页，带上它的标识作检索词。 */
export function refHash(kind: RefKind, id: string): string {
  return hashWithQueryValue(pageOf[kind](), ADDRESS_KEYWORD_PARAM, id);
}

/** 行模型里一格引用的原值：格里显示的字可能拼了版本号（`合同@v1`），链接要的是对象标识本身。 */
export interface RowRef {
  kind: RefKind;
  id: string;
}

/** 列标识 → 那一格的引用；没有引用的格不在表里。 */
export type RowRefs = Readonly<Record<string, RowRef>>;

export interface NameDirectory {
  parties: ReadonlyMap<string, string>;
  legalEntities: ReadonlyMap<string, string>;
}

export const emptyNameDirectory: NameDirectory = { parties: new Map(), legalEntities: new Map() };

/**
 * 两本册的答案建成名称目录。法人的名称是它所钉参与方的名称（法人册行上转写好的那一份）；转写不到（悬空）的不进目录，
 * 那一格照样只显标识。哪一本没取到就当它是空的。
 */
export function nameDirectoryOf(
  parties: BusinessPartyListResponseBody | null,
  entities: GroupLegalEntityListResponseBody | null,
): NameDirectory {
  return {
    parties: new Map((parties?.parties ?? []).map((party) => [party.partyId, party.partyName])),
    legalEntities: new Map(
      (entities?.entities ?? []).flatMap((entity) =>
        entity.partyNameKnown && entity.partyName ? [[entity.legalEntityId, entity.partyName] as const] : [],
      ),
    ),
  };
}

/** 这个引用的名称；只有参与方与责任法人有，查不到即 null。 */
export function refName(directory: NameDirectory, kind: RefKind, id: string): string | null {
  if (kind === 'party') return directory.parties.get(id) ?? null;
  if (kind === 'legal-entity') return directory.legalEntities.get(id) ?? null;
  return null;
}
