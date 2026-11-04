import { Contact, Landmark, Pencil, Plus, Receipt, UserRound } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import type { LegalEntityBankAccountRecord, LegalEntityProfileContent as ProfileContent } from './api';
import { CurrencyLabel } from '../refdata-labels';
import { LegalEntityContactCards } from './LegalEntityContactCards';
import { CopyAside, RefText } from './detail-primitives';
import {
  ProfileReadoutAddresses,
  ProfileReadoutCard,
  ProfileReadoutField,
  ProfileReadoutGrid,
  ProfileReadoutPairs,
  ReadoutText,
  absentValueMark,
} from './LegalEntityProfileReadout';
import { invoiceTitleRowsOf, languageTagLabel, profileMaintenanceNotes, representativeLabel } from './legal-entity-profile';
import type { RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { profileSections, type ProfileEditRequest, type ProfileSectionId } from './legal-entity-profile-sections';

/**
 * 当前资料的读出，四张卡（票 company-subject-ux/09）：代表人与地址、税务与开票、收款账户、联系人。代表人与地址并成
 * 一张——代表人只有一格，单独成卡是一条标题条带一行字。详情栏概要签、对象页概要签与按日期查询的读出都是这一份；
 * 概要签不带联系人卡（联系人有自己的签），按日期查询带。卡之间只留间距，不再画横线分节。
 */
export function LegalEntityProfileContent({
  content,
  includeContacts = true,
  typeNameOf,
  onEdit,
}: {
  content?: ProfileContent;
  includeContacts?: boolean;
  /** 监管登记号的类型码 → 目录译名（按注册地址国家查）；不给或查不到按码显。码留在 title 里给要对照的人。 */
  typeNameOf?: RegistrationNumberTypeNameOf;
  /**
   * 每张卡标题条上的「编辑」与逐行区的「添加…」交给页面开分区抽屉（票 company-subject-section-edit/01）。页面有写授予才给；
   * 按日期查询那种历史视图不给。没读到当前版时即使给了也不摆——没有当前版无从「局部」改起，走「更新公司资料」整张表单。
   */
  onEdit?: (request: ProfileEditRequest) => void;
}) {
  const notes = content ? profileMaintenanceNotes(content) : [];
  const actionsOf = (section: ProfileSectionId) => (onEdit && content ? <SectionActions section={section} onEdit={onEdit} /> : undefined);
  const bankAccounts = content?.bankAccounts ?? [];
  const contacts = content?.contacts ?? [];
  const invoiceTitleRows = content ? invoiceTitleRowsOf(content) : [];
  return (
    <div className="legal-entity-profile-readout mt-3 flex flex-col gap-3">
      <ProfileReadoutCard icon={UserRound} title="代表人与地址" actions={actionsOf('representativeAndAddresses')}>
        <ProfileReadoutGrid>
          <ProfileReadoutField
            label={content?.representative ? representativeLabel(content.representative.kind) : '代表人 / 负责人'}
            value={content?.representative?.name}
            emphasized
            wide
          />
        </ProfileReadoutGrid>
        {/* 注册地址是资料的必填格，没有也画一栏「—」；办公 / 通信两个可缺的地址没有就不画栏。 */}
        <div className="mt-3">
          <ProfileReadoutAddresses
            items={[
              { key: 'registered', label: '注册地址', address: content?.registeredAddress },
              ...(content?.officeAddress ? [{ key: 'office', label: '办公地址', address: content.officeAddress }] : []),
              ...(content?.correspondenceAddress ? [{ key: 'correspondence', label: '通信地址', address: content.correspondenceAddress }] : []),
            ]}
          />
        </div>
      </ProfileReadoutCard>
      <ProfileReadoutCard icon={Receipt} title="税务与开票" actions={actionsOf('invoicing')}>
        <ProfileReadoutGrid>
          {/* 语言 → 抬头、类型名 → 号各是一张两列小表（ProfileReadoutPairs），两张都占整行——抬头与几种号并排时半列放不下。
              主抬头排第一、语言格后带「主抬头」：它仍是「可开票」的判据，但与其他语言的抬头是同一类内容，不再分两格两种排法。
              没标语言的主抬头语言格显「—」：空格一律「—」（spec「统一用语」）。
              抬头是文本类、号是号码类（票 company-subject-ux/14）：抬头末尾悬停出复制图标，号整个是按钮点击即复制。 */}
          <ProfileReadoutField
            label="开票抬头"
            wide
            value={
              invoiceTitleRows.length ? (
                <ProfileReadoutPairs
                  headNowrap
                  rows={invoiceTitleRows.map((row, index) => ({
                    key: `${row.language}-${index}`,
                    head: (
                      <>
                        {row.language ? languageTagLabel(row.language) : absentValueMark}
                        {row.primary ? <PrimaryTitleMark /> : null}
                      </>
                    ),
                    value: (
                      <CopyAside value={row.title} name="开票抬头">
                        {row.primary ? <span className="font-medium text-idpxyz-textBright"><ReadoutText text={row.title} /></span> : <ReadoutText text={row.title} />}
                      </CopyAside>
                    ),
                  }))}
                />
              ) : undefined
            }
          />
          <ProfileReadoutField
            label="监管登记号"
            title="税务 / 海关的监管登记号"
            wide
            value={
              content?.taxRegistrationNumbers.length ? (
                <ProfileReadoutPairs
                  mono
                  rows={content.taxRegistrationNumbers.map((row, index) => {
                    const typeName = typeNameOf?.(content.registeredAddress.country, row.typeCode, 'PROFILE') ?? row.typeCode;
                    return {
                      key: `${row.typeCode}-${row.number}-${index}`,
                      head: typeName,
                      headTitle: row.typeCode,
                      value: <RefText value={row.number} name={typeName} />,
                    };
                  })}
                />
              ) : undefined
            }
          />
        </ProfileReadoutGrid>
      </ProfileReadoutCard>
      <ProfileReadoutCard icon={Landmark} title="收款账户" extra={bankAccounts.length ? `${bankAccounts.length} 个` : undefined} actions={actionsOf('bankAccounts')}>
        <LegalEntityBankAccountsTable accounts={bankAccounts} />
      </ProfileReadoutCard>
      {includeContacts ? (
        <ProfileReadoutCard icon={Contact} title="联系人" extra={contacts.length ? `${contacts.length} 位` : undefined} actions={actionsOf('contacts')}>
          <LegalEntityContactCards contacts={contacts} />
        </ProfileReadoutCard>
      ) : null}
      {notes.length ? (
        <div className="mt-2 border-t border-idpxyz-border pt-4 text-[13px] leading-6 text-idpxyz-breadcrumb">
          <p className="font-medium text-idpxyz-text">资料维护提示</p>
          <ul className="mt-1 list-disc pl-4">
            {notes.map((note) => (
              <li key={note}>{note}</li>
            ))}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

/**
 * 读出卡标题条上的动作：逐行的区（收款账户、联系人）先摆「添加…」再摆「编辑」，其余只有「编辑」。按钮弱显——它们是
 * 标题条上的次要动作，主动作「更新公司资料」在页头。
 */
export function SectionActions({ section, onEdit }: { section: ProfileSectionId; onEdit: (request: ProfileEditRequest) => void }) {
  const definition = profileSections[section];
  const buttonClass = 'h-6 px-1.5 text-[12px] text-idpxyz-breadcrumb hover:text-idpxyz-text';
  return (
    <>
      {definition.list ? (
        <Button variant="ghost" size="sm" className={buttonClass} onClick={() => onEdit({ section, addRow: true })}>
          <Plus className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
          {definition.list.addLabel}
        </Button>
      ) : null}
      <Button variant="ghost" size="sm" className={buttonClass} aria-label={`编辑${definition.title}`} onClick={() => onEdit({ section })}>
        <Pencil className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
        编辑
      </Button>
    </>
  );
}

/**
 * 「主抬头」小标，跟在主抬头那行的语言格后。画法照联系人卡的「主要」胶囊（LegalEntityContactCards）：同一块面上两种
 * 「这是主的」记号该长一个样；不用强调色——强调色在这里读作链接。
 */
function PrimaryTitleMark() {
  return (
    <span className="ml-1.5 inline-flex items-center rounded-full border border-idpxyz-border bg-idpxyz-editor px-1.5 align-[1px] text-[11px] font-medium leading-4 text-idpxyz-text">
      主抬头
    </span>
  );
}

/**
 * 收款账户表，一行一个账户、币种打头（ADR-0359 决定三「按币种登记」，开单方按单据币种挑账户）。没有账户是结论句
 * 「未登记收款账户」（ADR 给旧修订的读法）；表内空格「—」。币种经基础数据译名；账号与 SWIFT 是号码类、点击即复制，
 * 户名与开户行是文本类、末尾悬停出复制图标（票 company-subject-ux/14）。
 * 表与读出同一把尺（票 company-subject-ux/13 第 3 条）：正文 14、表头与次要行 13、等宽 13，行高一律 6。
 */
export function LegalEntityBankAccountsTable({ accounts }: { accounts: readonly LegalEntityBankAccountRecord[] }) {
  if (!accounts.length) return <p className="text-[14px] leading-6 text-idpxyz-breadcrumb">未登记收款账户</p>;
  return (
    <div className="overflow-x-auto">
      <table className="legal-entity-readout-table w-full min-w-[40rem] table-fixed border-collapse text-left text-[14px] leading-6">
        <caption className="sr-only">收款账户清单</caption>
        {/* 列宽按内容的常见长度分（票 company-subject-ux/12 第 4 条）：户名与开户行（带地址）最长，币种与备注最短；
            账号一列要装下 IBAN 的长度，等宽字再按字符折。币种列取 15 而不是票面的 14：详情栏拖到 2000 宽时表约 840px，
            14% 装不下「人民币（CNY）」一行，会在「（」前折开；那 1% 从最宽的开户行列出。 */}
        <colgroup>
          <col className="w-[15%]" />
          <col className="w-[24%]" />
          <col className="w-[25%]" />
          <col className="w-[22%]" />
          <col className="w-[14%]" />
        </colgroup>
        <thead className="border-b border-idpxyz-border text-[13px] text-idpxyz-breadcrumb">
          <tr>
            <th scope="col" className="py-2 pr-3 font-medium">币种 / 用途</th>
            <th scope="col" className="px-3 py-2 font-medium">户名</th>
            <th scope="col" className="px-3 py-2 font-medium">开户行</th>
            <th scope="col" className="px-3 py-2 font-medium">账号 / IBAN · SWIFT / BIC</th>
            <th scope="col" className="py-2 pl-3 font-medium">备注</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-idpxyz-border">
          {accounts.map((account, index) => (
            <tr key={`${account.currency}-${account.accountNumber}-${index}`} className="align-top">
              <th scope="row" className="py-2.5 pr-3 font-medium text-idpxyz-textBright">
                <span className="block"><CurrencyLabel code={account.currency} /></span>
                {account.label ? <span className="block text-[13px] font-normal text-idpxyz-breadcrumb">{account.label}</span> : null}
              </th>
              <td className="px-3 py-2.5 text-idpxyz-text">
                <CopyAside value={account.holderName} name="户名"><ReadoutText text={account.holderName} /></CopyAside>
              </td>
              <td className="px-3 py-2.5">
                <span className="block text-idpxyz-text">
                  <CopyAside value={account.bankName} name="开户行"><ReadoutText text={account.bankName} /></CopyAside>
                </span>
                {account.bankAddress ? <span className="block text-[13px] text-idpxyz-breadcrumb"><ReadoutText text={account.bankAddress} /></span> : null}
              </td>
              <td className="px-3 py-2.5 font-mono text-[13px]">
                <span className="block text-idpxyz-text"><RefText value={account.accountNumber} name="账号" /></span>
                {account.swiftCode ? (
                  <span className="block text-idpxyz-breadcrumb"><RefText value={account.swiftCode} name="SWIFT / BIC" /></span>
                ) : null}
              </td>
              <td className="py-2.5 pl-3">
                {account.note ? <span className="text-idpxyz-text"><ReadoutText text={account.note} /></span> : <span className="text-idpxyz-breadcrumb">{absentValueMark}</span>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
