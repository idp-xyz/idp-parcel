import type { ReactNode } from 'react';
import { Contact, Landmark, Receipt, UserRound } from 'lucide-react';
import type { LegalEntityBankAccountRecord, LegalEntityProfileContent as ProfileContent, RegisteredAddressRecord } from './api';
import { CurrencyLabel } from '../refdata-labels';
import { LegalEntityContactCards } from './LegalEntityContactCards';
import {
  ProfileReadoutAddress,
  ProfileReadoutCard,
  ProfileReadoutField,
  ProfileReadoutGrid,
  ProfileReadoutPairs,
  absentValueMark,
} from './LegalEntityProfileReadout';
import { languageTagLabel, profileMaintenanceNotes, representativeLabel } from './legal-entity-profile';
import type { RegistrationNumberTypeNameOf } from './legal-entity-identity';

/**
 * 当前资料的读出，四张卡（票 company-subject-ux/09）：代表人与地址、税务与开票、收款账户、联系人。代表人与地址并成
 * 一张——代表人只有一格，单独成卡是一条标题条带一行字。详情栏概要签、对象页概要签与按日期查询的读出都是这一份；
 * 概要签不带联系人卡（联系人有自己的签），按日期查询带。卡之间只留间距，不再画横线分节。
 */
export function LegalEntityProfileContent({
  content,
  includeContacts = true,
  typeNameOf,
}: {
  content?: ProfileContent;
  includeContacts?: boolean;
  /** 监管登记号的类型码 → 目录译名（按注册地址国家查）；不给或查不到按码显。码留在 title 里给要对照的人。 */
  typeNameOf?: RegistrationNumberTypeNameOf;
}) {
  const notes = content ? profileMaintenanceNotes(content) : [];
  const bankAccounts = content?.bankAccounts ?? [];
  const contacts = content?.contacts ?? [];
  return (
    <div className="legal-entity-profile-readout mt-3 flex flex-col gap-3">
      <ProfileReadoutCard icon={UserRound} title="代表人与地址">
        <ProfileReadoutGrid>
          <ProfileReadoutField
            label={content?.representative ? representativeLabel(content.representative.kind) : '代表人 / 负责人'}
            value={content?.representative?.name}
            emphasized
          />
          <ProfileReadoutField label="注册地址" value={addressOf(content?.registeredAddress)} />
          <ProfileReadoutField label="办公地址" value={addressOf(content?.officeAddress)} />
          <ProfileReadoutField label="通信地址" value={addressOf(content?.correspondenceAddress)} />
        </ProfileReadoutGrid>
      </ProfileReadoutCard>
      <ProfileReadoutCard icon={Receipt} title="税务与开票">
        <ProfileReadoutGrid>
          {/* 主抬头仍是「可开票」的判据，语言只是它的标注，所以跟在抬头后面弱化显，不另起一格。 */}
          <ProfileReadoutField
            label="开票抬头"
            value={
              content?.invoicingRegistered && content.invoiceTitle ? (
                <>
                  {content.invoiceTitle}
                  {content.invoiceTitleLanguage ? (
                    <span className="ml-1.5 text-[12px] font-normal text-idpxyz-breadcrumb">{languageTagLabel(content.invoiceTitleLanguage)}</span>
                  ) : null}
                </>
              ) : undefined
            }
            emphasized
          />
          {/* 语言 → 抬头、类型名 → 号各是一张两列小表（ProfileReadoutPairs）；监管登记号占整行——几种号并排时半列放不下。 */}
          <ProfileReadoutField
            label="其他语言的抬头"
            value={
              content?.invoicingRegistered && content.invoiceTitles?.length ? (
                <ProfileReadoutPairs
                  rows={content.invoiceTitles.map((row, index) => ({
                    key: `${row.language}-${index}`,
                    head: languageTagLabel(row.language),
                    value: row.title,
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
                  rows={content.taxRegistrationNumbers.map((row, index) => ({
                    key: `${row.typeCode}-${row.number}-${index}`,
                    head: typeNameOf?.(content.registeredAddress.country, row.typeCode, 'PROFILE') ?? row.typeCode,
                    headTitle: row.typeCode,
                    value: row.number,
                  }))}
                />
              ) : undefined
            }
          />
        </ProfileReadoutGrid>
      </ProfileReadoutCard>
      <ProfileReadoutCard icon={Landmark} title="收款账户" extra={bankAccounts.length ? `${bankAccounts.length} 个` : undefined}>
        <LegalEntityBankAccountsTable accounts={bankAccounts} />
      </ProfileReadoutCard>
      {includeContacts ? (
        <ProfileReadoutCard icon={Contact} title="联系人" extra={contacts.length ? `${contacts.length} 位` : undefined}>
          <LegalEntityContactCards contacts={contacts} />
        </ProfileReadoutCard>
      ) : null}
      {notes.length ? (
        <div className="mt-2 border-t border-idpxyz-border pt-4 text-[12px] leading-5 text-idpxyz-breadcrumb">
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
 * 收款账户表，一行一个账户、币种打头（ADR-0359 决定三「按币种登记」，开单方按单据币种挑账户）。没有账户是结论句
 * 「未登记收款账户」（ADR 给旧修订的读法）；表内空格「—」。币种经基础数据译名，账号与 SWIFT 等宽、可选中复制。
 */
export function LegalEntityBankAccountsTable({ accounts }: { accounts: readonly LegalEntityBankAccountRecord[] }) {
  if (!accounts.length) return <p className="text-[13px] leading-5 text-idpxyz-breadcrumb">未登记收款账户</p>;
  return (
    <div className="overflow-x-auto">
      <table className="legal-entity-readout-table w-full min-w-[40rem] table-fixed border-collapse text-left text-[13px] leading-5">
        <caption className="sr-only">收款账户清单</caption>
        <colgroup>
          <col className="w-[18%]" />
          <col className="w-[22%]" />
          <col className="w-[22%]" />
          <col className="w-[24%]" />
          <col className="w-[14%]" />
        </colgroup>
        <thead className="border-b border-idpxyz-border text-[12px] text-idpxyz-breadcrumb">
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
            <tr key={`${account.currency}-${account.accountNumber}-${index}`} className="align-top hover:bg-idpxyz-hover">
              <th scope="row" className="py-2.5 pr-3 font-medium text-idpxyz-textBright">
                <span className="block"><CurrencyLabel code={account.currency} /></span>
                {account.label ? <span className="block text-[12px] font-normal text-idpxyz-breadcrumb">{account.label}</span> : null}
              </th>
              <td className="break-words px-3 py-2.5 text-idpxyz-text">{account.holderName}</td>
              <td className="px-3 py-2.5">
                <span className="block text-idpxyz-text">{account.bankName}</span>
                {account.bankAddress ? <span className="block text-[12px] text-idpxyz-breadcrumb">{account.bankAddress}</span> : null}
              </td>
              <td className="px-3 py-2.5 font-mono text-[12px]">
                <span className="block select-text text-idpxyz-text [overflow-wrap:anywhere]">{account.accountNumber}</span>
                {account.swiftCode ? <span className="block select-text text-idpxyz-breadcrumb">{account.swiftCode}</span> : null}
              </td>
              <td className="break-words py-2.5 pl-3 text-[12px]">
                {account.note ? <span className="text-idpxyz-text">{account.note}</span> : <span className="text-idpxyz-breadcrumb">{absentValueMark}</span>}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// 没有地址交 undefined 让格显「—」；有地址按 ProfileReadoutAddress 的版式（行在前、国家并进行末）排。
function addressOf(address?: RegisteredAddressRecord): ReactNode {
  return address ? <ProfileReadoutAddress address={address} /> : undefined;
}
