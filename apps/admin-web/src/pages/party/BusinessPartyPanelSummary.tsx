import { Fragment, type ReactNode } from 'react';
import {
  ChevronRight,
  CircleMinus,
  Clock,
  Contact,
  Handshake,
  Info,
  Landmark,
  Link2,
  Mail,
  MapPin,
  MessageCircle,
  Phone,
  Receipt,
  type LucideIcon,
} from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import type { AnswerQuery } from '../../query/answers';
import { CurrencyLabel } from '../refdata-labels';
import type {
  BusinessPartyProfileRevisionListResponseBody,
  BusinessPartyProfileRevisionRecord,
  BusinessPartyRecord,
  GroupLegalEntityRecord,
  LegalEntityBankAccountRecord,
} from './api';
import { formatCount } from './business-parties-workbench';
import type { ProfileEditSectionId } from './business-party-edit-origin';
import {
  contactInitialOf,
  contactPlatformsOf,
  leadingRows,
  panelRelatedObjectSections,
  summaryContactLimit,
  summaryRelationshipLimit,
  type PanelRelatedTarget,
} from './business-party-panel-content';
import type { PanelHint } from './business-party-panel-first-screen';
import {
  businessPartyProfileStateOf,
  customerLacksContacts,
  customerWithoutContactsLine,
  profileContactsEmptyNoteOf,
  profileSummaryEmptyNoteOf,
  profileVersionsEmptyNoteOf,
  type BusinessPartyProfileState,
} from './business-party-profile';
import { BusinessPartyDeactivationNote } from './BusinessPartyHeaderParts';
import { PanelCard, PanelCardEmpty, PanelNoteCard, RelationshipPeriod, panelLinkClass } from './BusinessPartyPanelParts';
import {
  BusinessPartyNamesAndAddresses,
  BusinessPartyProfileVersions,
  BusinessPartyTaxAndInvoicing,
  ourCompanyProfileNoteParts,
} from './BusinessPartyProfileSection';
import { CopyAside, RefText } from './copy-affordance';
import {
  contactCardOf,
  socialAccountSeparator,
  socialAccountsText,
  type ContactCard,
  type ContactCardPurpose,
} from './legal-entity-contact-cards';
import { ProfileReadoutField, ProfileReadoutGrid, ReadoutText } from './LegalEntityProfileReadout';
import { OverviewSectionStateNote, partyOverviewSectionTitles, type PartyOverviewStates } from './PartyOverviewSection';
import { countText, type OverviewSectionState, type PartyRelationshipView } from './party-overview';
import { PartyRoleTag } from './party-role-visuals';

// 往来单位工作台右栏的「概要」「联系人」「资料版本」三签（票 business-party-redesign/05）。判读在 business-party-profile.ts 与
// business-party-panel-content.ts，这里只摆。资料读出的字段格、联系人卡的判读与公司主体页、对象页共用，卡框是右栏自己的。

const rowButtonClass =
  'flex w-full items-center gap-2.5 px-3.5 py-[9px] text-left hover:bg-idpxyz-hover focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-idpxyz-accent';

/** 概要上方随状态的一条提示：图标带语气色，正文是正文色；右端可以有一个去处。 */
function Banner({ icon: Icon, frame, iconTone, action, children }: { icon: LucideIcon; frame: string; iconTone: string; action?: ReactNode; children: ReactNode }) {
  return (
    <div className={`flex items-start gap-2.5 rounded-lg border px-3 py-2.5 type-value leading-5 ${frame}`}>
      <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${iconTone}`} aria-hidden="true" />
      <div className="min-w-0 flex-1">{children}</div>
      {action ? <div className="-my-1 shrink-0">{action}</div> : null}
    </div>
  );
}

/**
 * 随状态的提示条，摆在两列之上。出不出、说什么照 business-party-panel-first-screen.ts 的 panelHintOf 与停用横幅原来的规则；
 * 语气跟着状态徽章走：待生效是等着的事（警示、钟），已停用是结束了的事（中性、横），还没有合作关系是给下一步（提示）。
 */
function StatusBanners({
  row,
  hint,
  mayWrite,
  relationshipFormOpen,
  onRegisterRelationship,
}: {
  row: BusinessPartyRecord;
  hint: PanelHint;
  mayWrite: boolean;
  relationshipFormOpen: boolean;
  onRegisterRelationship: () => void;
}) {
  if (!row.deactivatedAt && hint === null) return null;
  return (
    <div className="mb-3 flex flex-col gap-2">
      {row.deactivatedAt ? (
        <Banner icon={CircleMinus} frame="border-idpxyz-border bg-idpxyz-bg text-idpxyz-text" iconTone="text-idpxyz-breadcrumb">
          <BusinessPartyDeactivationNote deactivatedAt={row.deactivatedAt} deactivationBasis={row.deactivationBasis} />
        </Banner>
      ) : null}
      {hint?.kind === 'pending' ? (
        <Banner icon={Clock} frame={statusToneBanner.warning} iconTone={statusToneText.warning}>
          还没到生效时间：在那之前不按它办新业务；登记合作关系时，开始时间不能早于它的生效时间。
        </Banner>
      ) : null}
      {hint?.kind === 'no-relationship' ? (
        <Banner
          icon={Info}
          frame={statusToneBanner.info}
          iconTone={statusToneText.info}
          action={
            mayWrite && !relationshipFormOpen ? (
              <Button size="sm" variant="outline" className="h-7 type-body" onClick={onRegisterRelationship}>
                <Handshake className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                登记合作关系
              </Button>
            ) : undefined
          }
        >
          还没有合作关系。
        </Banner>
      ) : null}
    </div>
  );
}

/** 联系人头像：圆底里一个字，只是视觉记号。 */
function ContactAvatar({ name, large = false }: { name: string; large?: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={`grid shrink-0 place-items-center rounded-full bg-idpxyz-hover text-idpxyz-breadcrumb ${large ? 'h-10 w-10 type-section' : 'h-8 w-8 type-value'}`}
    >
      {contactInitialOf(name)}
    </span>
  );
}

/** 用途标签；是这个用途的主要联系人就标出来，用提示语气的淡底与一般用途分开。 */
function PurposeChip({ purpose }: { purpose: ContactCardPurpose }) {
  return (
    <span
      title={purpose.primary ? `${purpose.label}的主要联系人` : undefined}
      className={`inline-flex items-center whitespace-nowrap rounded border px-1.5 py-0.5 type-body leading-4 ${
        purpose.primary ? statusToneBanner.info : 'border-idpxyz-border bg-idpxyz-editor text-idpxyz-text'
      }`}
    >
      {purpose.label}
      {purpose.primary ? ' · 主要' : ''}
    </span>
  );
}

/** 概要里联系人的一行：人、用途、电话与有哪几个平台；邮箱与账号在「联系人」签。 */
function SummaryContactRow({ card }: { card: ContactCard }) {
  const platforms = contactPlatformsOf(card);
  return (
    <li className="flex items-center gap-2.5 border-b border-idpxyz-border px-3.5 py-[9px] last:border-b-0">
      <ContactAvatar name={card.name} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-1.5 gap-y-1">
          <span className="break-words type-value leading-5 text-idpxyz-text">{card.name}</span>
          {card.purposes.map((purpose) => (
            <PurposeChip key={purpose.code} purpose={purpose} />
          ))}
        </div>
        {card.affiliation ? <p className="truncate type-body leading-4 text-idpxyz-breadcrumb">{card.affiliation}</p> : null}
      </div>
      {card.phone || platforms.length > 0 ? (
        <div className="shrink-0 text-right">
          {card.phone ? (
            <p className="leading-5 text-idpxyz-text">
              <RefText value={card.phone} name="电话" className="type-body" />
            </p>
          ) : null}
          {/* 右边让出的一段是电话末尾留给复制图标的位置：两行的字才在同一条竖线上收住。 */}
          {platforms.length > 0 ? (
            <p className="pr-[18px] type-body leading-4 text-idpxyz-breadcrumb">{platforms.join(socialAccountSeparator)}</p>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}

function ContactsCard({
  state,
  roles,
  mayUpdateProfile,
  takesRevisions,
  onOpenContacts,
  onUpdateProfile,
}: {
  state: Extract<BusinessPartyProfileState, { kind: 'present' }>;
  roles: readonly string[];
  /** 「更新资料」的入口摆不摆：有写授予、这家单位还收新的一版资料。 */
  mayUpdateProfile: boolean;
  /** 这家单位还收不收新的一版资料（identity-write-entries.ts 的 identityTakesRevisions）：不收时空着的资料换一句说明。 */
  takesRevisions: boolean;
  onOpenContacts: () => void;
  onUpdateProfile: (section?: ProfileEditSectionId) => void;
}) {
  const cards = state.record.contacts.map(contactCardOf);
  // 登过停用的不提「建议登记一位」：登记册不再收它新的资料，登不了的事不建议。
  const hinted = takesRevisions && customerLacksContacts(roles, state.record);
  return (
    <PanelCard
      flush
      icon={Contact}
      title="联系人"
      action={
        cards.length > 0 ? (
          <button type="button" className={panelLinkClass} onClick={onOpenContacts}>
            全部 {formatCount(cards.length)} 位
          </button>
        ) : undefined
      }
    >
      {cards.length === 0 ? (
        <PanelCardEmpty title={profileContactsEmptyNoteOf(state, takesRevisions).title} lines={hinted ? [customerWithoutContactsLine] : []}>
          {hinted && mayUpdateProfile ? (
            <Button size="sm" variant="outline" className="type-body" onClick={() => onUpdateProfile('contacts')}>
              更新资料
            </Button>
          ) : null}
        </PanelCardEmpty>
      ) : (
        <ul aria-label="联系人清单">
          {leadingRows(cards, summaryContactLimit).listed.map((card, index) => (
            <SummaryContactRow key={index} card={card} />
          ))}
        </ul>
      )}
    </PanelCard>
  );
}

/** 概要里合作关系的一行：整行可点，去「关联」签里的这一段。 */
function SummaryRelationshipRow({ view, onOpen }: { view: PartyRelationshipView; onOpen: () => void }) {
  const other = view.otherPartyNameKnown && view.otherPartyName ? view.otherPartyName : null;
  const name = other ? <span className="text-idpxyz-textBright">{other}</span> : <span className="font-mono type-body text-idpxyz-text">{view.otherPartyId}</span>;
  return (
    <li className="border-b border-idpxyz-border last:border-b-0">
      <button type="button" className={rowButtonClass} onClick={onOpen}>
        <span className="min-w-0 flex-1">
          {/* 一句话顺着排：对方名称长了就在句子里折行，角色标签跟在句尾，不各占一行。 */}
          <span className="block break-words type-value leading-6 text-idpxyz-breadcrumb">
            {view.side === 'HOLDER' ? (
              <>
                对 {name} 担任
              </>
            ) : (
              <>{name} 对它担任</>
            )}{' '}
            <span className="inline-flex align-middle">
              <PartyRoleTag role={view.role} />
            </span>
          </span>
          <span className="block truncate type-body leading-4 tabular-nums text-idpxyz-breadcrumb">
            <RelationshipPeriod view={view} /> · <span className="font-mono">{view.relationshipId}</span>
          </span>
        </span>
        <StatusPill code={view.status} />
        <ChevronRight className="h-4 w-4 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
      </button>
    </li>
  );
}

function RelationshipsCard({
  state,
  complete,
  retry,
  onOpenRelated,
  onShowRelationships,
}: {
  state: OverviewSectionState<PartyRelationshipView>;
  /** 合作关系册不带条件那一问已是整册；不是的话到手的只是一页，不说「全部」、也不说「还没有」。 */
  complete: boolean;
  retry: () => void;
  onOpenRelated: (target: PanelRelatedTarget) => void;
  onShowRelationships: () => void;
}) {
  if (state.kind !== 'answered') {
    return (
      <PanelCard icon={Handshake} title={partyOverviewSectionTitles.relationships}>
        <OverviewSectionStateNote state={state} what="合作关系" retry={retry} />
      </PanelCard>
    );
  }
  const total = state.rows.length;
  const action = !complete ? (
    <button type="button" className={panelLinkClass} onClick={onShowRelationships}>
      在「合作关系」里看全部
    </button>
  ) : total > 0 ? (
    <button type="button" className={panelLinkClass} onClick={() => onOpenRelated({ section: 'relationships' })}>
      全部 {formatCount(total)} 段
    </button>
  ) : undefined;
  return (
    <PanelCard flush icon={Handshake} title={partyOverviewSectionTitles.relationships} action={action}>
      {total > 0 ? (
        <ul>
          {leadingRows(state.rows, summaryRelationshipLimit).listed.map((view) => (
            <SummaryRelationshipRow
              key={view.relationshipId}
              view={view}
              onOpen={() => onOpenRelated({ section: 'relationships', relationshipId: view.relationshipId })}
            />
          ))}
        </ul>
      ) : complete ? (
        <PanelCardEmpty title="还没有合作关系" />
      ) : null}
      {complete ? null : (
        <p className={`px-3.5 py-2.5 type-body leading-5 text-idpxyz-breadcrumb ${total > 0 ? 'border-t border-idpxyz-border' : ''}`}>
          这家单位名下的合作关系不止一页，这里只列已取到的那一部分。
        </p>
      )}
    </PanelCard>
  );
}

/** 另外四册各有几条与它有关：整行可点，去「关联」签里的那一段。哪一册没答出显「—」，不拿 0 冒充「没有」（countText）。 */
function RelatedObjectsCard({ overview, onOpenRelated }: { overview: PartyOverviewStates; onOpenRelated: (target: PanelRelatedTarget) => void }) {
  return (
    <PanelCard flush icon={Link2} title="关联对象">
      <ul>
        {panelRelatedObjectSections.map((section) => {
          const state = overview[section];
          const some = state.kind === 'answered' && state.rows.length > 0;
          return (
            <li key={section} className="border-b border-idpxyz-border last:border-b-0">
              <button type="button" className={`${rowButtonClass} type-value leading-5`} onClick={() => onOpenRelated({ section })}>
                <span className="min-w-0 flex-1 text-idpxyz-text">{partyOverviewSectionTitles[section]}</span>
                <span className={`tabular-nums ${some ? 'text-idpxyz-textBright' : 'text-idpxyz-breadcrumb'}`}>{countText(state)}</span>
                <ChevronRight className="h-4 w-4 shrink-0 text-idpxyz-breadcrumb" aria-hidden="true" />
              </button>
            </li>
          );
        })}
      </ul>
    </PanelCard>
  );
}

/**
 * 收款账户：一个账户一块，块里是标签 / 值的格。右栏的卡只有半栏宽，公司主体页那张五列表在这里要横向滚动才看得全；
 * 多数单位只有一两个账户，摆成格一眼看完。没有的可缺项整行不摆。
 */
function BankAccounts({ accounts }: { accounts: readonly LegalEntityBankAccountRecord[] }) {
  if (accounts.length === 0) return <p className="type-value leading-6 text-idpxyz-breadcrumb">未登记收款账户</p>;
  return (
    <ul className="flex flex-col divide-y divide-idpxyz-border">
      {accounts.map((account, index) => (
        <li key={`${account.currency}-${account.accountNumber}-${index}`} className="py-3 first:pt-0 last:pb-0">
          <p className="mb-1.5 flex flex-wrap items-baseline gap-x-2 type-value leading-5 text-idpxyz-textBright">
            <CurrencyLabel code={account.currency} />
            {account.label ? <span className="type-body text-idpxyz-breadcrumb">{account.label}</span> : null}
          </p>
          <ProfileReadoutGrid>
            <ProfileReadoutField
              label="户名"
              value={
                <CopyAside value={account.holderName} name="户名">
                  <ReadoutText text={account.holderName} />
                </CopyAside>
              }
            />
            <ProfileReadoutField
              label="开户行"
              value={
                <>
                  <CopyAside value={account.bankName} name="开户行">
                    <ReadoutText text={account.bankName} />
                  </CopyAside>
                  {account.bankAddress ? (
                    <span className="block type-body text-idpxyz-breadcrumb">
                      <ReadoutText text={account.bankAddress} />
                    </span>
                  ) : null}
                </>
              }
            />
            <ProfileReadoutField label="账号 / IBAN" mono value={<RefText value={account.accountNumber} name="账号" />} />
            {account.swiftCode ? <ProfileReadoutField label="SWIFT / BIC" mono value={<RefText value={account.swiftCode} name="SWIFT / BIC" />} /> : null}
            {account.note ? <ProfileReadoutField label="备注" value={<ReadoutText text={account.note} />} /> : null}
          </ProfileReadoutGrid>
        </li>
      ))}
    </ul>
  );
}

/** 左列：这家单位自己的资料。我方公司、还没登记资料各是一张说明卡；有生效的一版就是三张卡。 */
function ProfileColumn({
  row,
  roles,
  state,
  entities,
  mayUpdateProfile,
  takesRevisions,
  onUpdateProfile,
}: {
  row: BusinessPartyRecord;
  roles: readonly string[];
  /** null 即法人册上有公司主体钉着它：不读也不摆往来单位资料。 */
  state: BusinessPartyProfileState | null;
  entities: OverviewSectionState<GroupLegalEntityRecord>;
  /** 「更新资料」的入口摆不摆：有写授予、这家单位还收新的一版资料。 */
  mayUpdateProfile: boolean;
  /** 这家单位还收不收新的一版资料（identity-write-entries.ts 的 identityTakesRevisions）：不收时空着的资料换一句说明。 */
  takesRevisions: boolean;
  onUpdateProfile: (section?: ProfileEditSectionId) => void;
}) {
  if (state === null) {
    return (
      <PanelNoteCard title={ourCompanyProfileNoteParts.title} lines={[ourCompanyProfileNoteParts.line]}>
        {(entities.kind === 'answered' ? entities.rows : []).map((entity) => (
          <a
            key={entity.legalEntityId}
            href={`#/group-legal-entities/${encodeURIComponent(entity.legalEntityId)}`}
            className="inline-flex h-7 items-center gap-1.5 rounded-md border border-idpxyz-border bg-idpxyz-bg px-2.5 type-body text-idpxyz-text hover:bg-idpxyz-hover focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent"
          >
            打开公司主体 <span className="font-mono">{entity.legalEntityId}</span>
          </a>
        ))}
      </PanelNoteCard>
    );
  }
  if (state.kind === 'note') return <PanelNoteCard lines={[state.text]} />;
  if (state.kind === 'empty') {
    return (
      <PanelNoteCard {...profileSummaryEmptyNoteOf(state.scheduled, roles, takesRevisions)}>
        {mayUpdateProfile ? (
          <Button size="sm" variant="outline" className="type-body" onClick={() => onUpdateProfile()}>
            更新资料
          </Button>
        ) : null}
      </PanelNoteCard>
    );
  }
  const record: BusinessPartyProfileRevisionRecord = state.record;
  const bankAccounts = record.bankAccounts ?? [];
  // 三张卡的「编辑」进的是同一个编辑态（对象页的「更新资料」），各落到表单里自己那一段。
  const editOf = (section: ProfileEditSectionId) =>
    mayUpdateProfile ? (
      <button type="button" className={panelLinkClass} onClick={() => onUpdateProfile(section)}>
        编辑
      </button>
    ) : undefined;
  return (
    <>
      <PanelCard readout icon={MapPin} title="名称与地址" action={editOf('names-addresses')}>
        <BusinessPartyNamesAndAddresses record={record} />
      </PanelCard>
      <PanelCard readout icon={Receipt} title="税务与开票" action={editOf('invoicing')}>
        <BusinessPartyTaxAndInvoicing record={record} registrationCountry={row.identityLayerRegistered ? row.registrationCountry : undefined} />
      </PanelCard>
      <PanelCard readout icon={Landmark} title="收款账户" extra={bankAccounts.length ? `${bankAccounts.length} 个` : undefined} action={editOf('bank-accounts')}>
        <BankAccounts accounts={bankAccounts} />
      </PanelCard>
    </>
  );
}

/**
 * 「概要」签：随状态的提示条在上，下面两列各排各的——左列是这家单位自己的资料，右列是它与别人的关系（联系人、合作关系、
 * 另外四册的条数）。右栏窄到放不下两列时并成一列（business-party-panel.css）。
 */
export function BusinessPartyPanelSummary({
  row,
  roles,
  pinned,
  profiles,
  overview,
  retryRelationships,
  relationshipsComplete,
  hint,
  mayWrite,
  takesRevisions,
  relationshipFormOpen,
  onUpdateProfile,
  onRegisterRelationship,
  onOpenContacts,
  onOpenRelated,
  onShowRelationships,
}: {
  row: BusinessPartyRecord;
  roles: readonly string[];
  pinned: boolean | null;
  profiles: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>;
  overview: PartyOverviewStates;
  retryRelationships: () => void;
  relationshipsComplete: boolean;
  hint: PanelHint;
  mayWrite: boolean;
  /**
   * 这家单位还收不收新的一版资料（identity-write-entries.ts 的 identityTakesRevisions）。不收时「更新资料」「编辑」都不摆、空着的
   * 资料换一句说明；提示条上的「登记合作关系」不归它管，照旧只看写授予。
   */
  takesRevisions: boolean;
  /** 「关联」签里登记合作关系的表单开着：提示条上的入口先收起。 */
  relationshipFormOpen: boolean;
  onUpdateProfile: (section?: ProfileEditSectionId) => void;
  onRegisterRelationship: () => void;
  onOpenContacts: () => void;
  onOpenRelated: (target: PanelRelatedTarget) => void;
  /** 去「合作关系」视图看这家单位名下的全部关系。 */
  onShowRelationships: () => void;
}) {
  const profile = pinned === true ? null : businessPartyProfileStateOf(profiles.answer, row.partyId);
  const mayUpdateProfile = mayWrite && takesRevisions;
  return (
    <>
      <StatusBanners
        row={row}
        hint={hint}
        mayWrite={mayWrite}
        relationshipFormOpen={relationshipFormOpen}
        onRegisterRelationship={onRegisterRelationship}
      />
      <div className="business-party-panel-columns">
        <div className="business-party-panel-column">
          <ProfileColumn
            row={row}
            roles={roles}
            state={profile}
            entities={overview.entities}
            mayUpdateProfile={mayUpdateProfile}
            takesRevisions={takesRevisions}
            onUpdateProfile={onUpdateProfile}
          />
        </div>
        <div className="business-party-panel-column">
          {profile?.kind === 'present' ? (
            <ContactsCard
              state={profile}
              roles={roles}
              mayUpdateProfile={mayUpdateProfile}
              takesRevisions={takesRevisions}
              onOpenContacts={onOpenContacts}
              onUpdateProfile={onUpdateProfile}
            />
          ) : null}
          <RelationshipsCard
            state={overview.relationships}
            complete={relationshipsComplete}
            retry={retryRelationships}
            onOpenRelated={onOpenRelated}
            onShowRelationships={onShowRelationships}
          />
          <RelatedObjectsCard overview={overview} onOpenRelated={onOpenRelated} />
        </div>
      </div>
    </>
  );
}

/** 卡内的一种联系方式：标签一栏带小图标，值在右。 */
function ContactChannel({ icon: Icon, label, title, children }: { icon: LucideIcon; label: string; title?: string; children: ReactNode }) {
  return (
    <>
      <dt className="flex items-center gap-1.5 self-start type-body leading-5 text-idpxyz-breadcrumb">
        <Icon className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        {label}
      </dt>
      <dd className="min-w-0 type-value leading-5 text-idpxyz-text" title={title}>
        {children}
      </dd>
    </>
  );
}

/**
 * 「联系人」签里的一张卡：人名、部门与职务、用途，下面是电话、邮箱、社交账号，有哪样摆哪样（判读在
 * legal-entity-contact-cards.ts）。三样的值整个是按钮，点一下复制。社交账号一个一行、平台名自成一栏：几个账号串成一行时
 * 平台名与账号混在一起，折行处还会挂一个分隔点。
 */
function ContactCardView({ card }: { card: ContactCard }) {
  const hasChannels = card.phone !== undefined || card.email !== undefined || card.socialAccounts.length > 0;
  return (
    <li className="min-w-0 rounded-lg border border-idpxyz-border bg-idpxyz-bg p-3.5">
      <div className="flex items-center gap-2.5">
        <ContactAvatar name={card.name} large />
        <div className="min-w-0 flex-1">
          <p className="break-words type-section leading-5 text-idpxyz-textBright">
            <CopyAside value={card.name} name="联系人姓名">
              {card.name}
            </CopyAside>
          </p>
          {card.affiliation ? <p className="break-words type-body leading-4 text-idpxyz-breadcrumb">{card.affiliation}</p> : null}
        </div>
      </div>
      {card.purposes.length > 0 ? (
        <ul className="mt-2.5 flex flex-wrap gap-1.5" aria-label="联系用途">
          {card.purposes.map((purpose) => (
            <li key={purpose.code} className="flex">
              <PurposeChip purpose={purpose} />
            </li>
          ))}
        </ul>
      ) : null}
      {hasChannels ? (
        <dl className="mt-3 grid grid-cols-[92px_minmax(0,1fr)] gap-x-3 gap-y-2 border-t border-idpxyz-border pt-3">
          {card.phone ? (
            <ContactChannel icon={Phone} label="电话">
              <RefText value={card.phone} name="电话" className="type-body" />
            </ContactChannel>
          ) : null}
          {card.email ? (
            <ContactChannel icon={Mail} label="邮箱">
              {/* 邮箱按字符折、不按连字符折：按词折会在域名的连字符后断开，读的人要把两段拼回去才认得出是哪个域。 */}
              <RefText value={card.email} name="邮箱" className="break-all type-body" />
            </ContactChannel>
          ) : null}
          {card.socialAccounts.length > 0 ? (
            <ContactChannel icon={MessageCircle} label="社交账号" title={socialAccountsText(card.socialAccounts)}>
              <span className="grid grid-cols-[max-content_minmax(0,1fr)] items-baseline gap-x-2 gap-y-0.5">
                {card.socialAccounts.map((account, index) => (
                  <Fragment key={`${account.platform}-${account.account}-${index}`}>
                    <span className="type-body text-idpxyz-breadcrumb">{account.platform}</span>
                    <RefText
                      value={account.account}
                      name={account.platform ? `${account.platform}账号` : '社交账号'}
                      className="min-w-0 break-all type-body"
                    />
                  </Fragment>
                ))}
              </span>
            </ContactChannel>
          ) : null}
        </dl>
      ) : null}
    </li>
  );
}

/** 「联系人」签：一人一张卡，右栏宽就并排、窄就一列；卡的最小宽度够放下一行常见长度的邮箱或账号。 */
export function BusinessPartyPanelContacts({
  partyId,
  profiles,
  mayUpdateProfile,
  takesRevisions,
  onUpdateProfile,
}: {
  partyId: string;
  profiles: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>;
  /** 「更新资料」的入口摆不摆：有写授予、这家单位还收新的一版资料。 */
  mayUpdateProfile: boolean;
  /** 这家单位还收不收新的一版资料（identity-write-entries.ts 的 identityTakesRevisions）：不收时空着的资料换一句说明。 */
  takesRevisions: boolean;
  onUpdateProfile: (section?: ProfileEditSectionId) => void;
}) {
  const state = businessPartyProfileStateOf(profiles.answer, partyId);
  if (state.kind === 'note') return <PanelNoteCard lines={[state.text]} />;
  const contacts = state.kind === 'present' ? state.record.contacts : [];
  if (contacts.length === 0) {
    return (
      <PanelNoteCard {...profileContactsEmptyNoteOf(state, takesRevisions)}>
        {mayUpdateProfile ? (
          <Button size="sm" variant="outline" className="type-body" onClick={() => onUpdateProfile('contacts')}>
            更新资料
          </Button>
        ) : null}
      </PanelNoteCard>
    );
  }
  return (
    <>
      <div className="mb-3 flex min-h-7 flex-wrap items-center justify-between gap-x-3 gap-y-2">
        <p className="type-body leading-5 text-idpxyz-breadcrumb">{formatCount(contacts.length)} 位联系人 · 电话、邮箱与账号点一下就复制</p>
        {mayUpdateProfile ? (
          <Button size="sm" variant="outline" className="h-7 shrink-0 type-body" onClick={() => onUpdateProfile('contacts')}>
            编辑联系人
          </Button>
        ) : null}
      </div>
      <ul className="grid grid-cols-[repeat(auto-fill,minmax(340px,1fr))] items-start gap-3" aria-label="联系人清单">
        {contacts.map((contact, index) => (
          <ContactCardView key={index} card={contactCardOf(contact)} />
        ))}
      </ul>
    </>
  );
}

/** 「资料版本」签：内容是对象页那一份清单（BusinessPartyProfileVersions），这里只换外框——在读、读不到、一版都没有各是一张说明卡。 */
export function BusinessPartyPanelVersions({
  partyId,
  profiles,
  mayUpdateProfile,
  takesRevisions,
  onUpdateProfile,
}: {
  partyId: string;
  profiles: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>;
  /** 「更新资料」的入口摆不摆：有写授予、这家单位还收新的一版资料。 */
  mayUpdateProfile: boolean;
  /** 这家单位还收不收新的一版资料（identity-write-entries.ts 的 identityTakesRevisions）：不收时空着的资料换一句说明。 */
  takesRevisions: boolean;
  onUpdateProfile: (section?: ProfileEditSectionId) => void;
}) {
  const state = businessPartyProfileStateOf(profiles.answer, partyId);
  if (state.kind === 'note') return <PanelNoteCard lines={[state.text]} />;
  if (state.body.revisions.length === 0) {
    return (
      <PanelNoteCard {...profileVersionsEmptyNoteOf(takesRevisions)}>
        {mayUpdateProfile ? (
          <Button size="sm" variant="outline" className="type-body" onClick={() => onUpdateProfile()}>
            更新资料
          </Button>
        ) : null}
      </PanelNoteCard>
    );
  }
  return (
    <div className="rounded-md bg-idpxyz-bg">
      <BusinessPartyProfileVersions partyId={partyId} query={profiles} takesRevisions={takesRevisions} />
    </div>
  );
}
