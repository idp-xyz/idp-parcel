import type { LegalEntityContactRecord } from './api';
import { contactPurposeLabels } from './legal-entity-profile';

export function LegalEntityContactsTable({ contacts }: { contacts: readonly LegalEntityContactRecord[] }) {
  if (!contacts.length) return <p className="text-[13px] text-idpxyz-breadcrumb">未配置联系人</p>;
  return (
    <div className="overflow-x-auto">
      <table className="w-full min-w-[40rem] table-fixed border-collapse text-left text-[13px] leading-5">
        <caption className="sr-only">联系人清单</caption>
        <colgroup>
          <col className="w-[18%]" />
          <col className="w-[16%]" />
          <col className="w-[25%]" />
          <col className="w-[25%]" />
          <col className="w-[16%]" />
        </colgroup>
        <thead className="border-b border-idpxyz-border text-[12px] text-idpxyz-breadcrumb">
          <tr>
            <th scope="col" className="py-2 pr-3 font-medium">联系人</th>
            <th scope="col" className="px-3 py-2 font-medium">部门 / 职务</th>
            <th scope="col" className="px-3 py-2 font-medium">联系方式</th>
            <th scope="col" className="px-3 py-2 font-medium">社交账号</th>
            <th scope="col" className="py-2 pl-3 font-medium">联系用途</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-idpxyz-border">
          {contacts.map((contact, index) => (
            <tr key={index} className="align-middle hover:bg-idpxyz-hover">
              <th scope="row" className="break-words py-2.5 pr-3 font-medium text-idpxyz-textBright">
                {contact.name}
              </th>
              <td className="px-3 py-2.5">
                <ContactValue value={contact.department} empty="部门未配置" />
                <ContactValue value={contact.position} empty="职务未配置" secondary />
              </td>
              <td className="px-3 py-2.5">
                <ContactValue value={contact.phone} empty="电话未配置" />
                <ContactValue value={contact.email} empty="邮箱未配置" secondary />
              </td>
              <td className="px-3 py-2.5 text-[12px]">
                {contact.socialAccounts?.length ? (
                  <dl className="space-y-1.5">
                    {contact.socialAccounts.map((social, accountIndex) => (
                      <div key={accountIndex} className="[overflow-wrap:anywhere]">
                        <dt className="text-idpxyz-breadcrumb">{social.platform}</dt>
                        <dd className="select-text text-idpxyz-text">{social.account}</dd>
                      </div>
                    ))}
                  </dl>
                ) : <span className="text-idpxyz-breadcrumb">未配置社交账号</span>}
              </td>
              <td className="py-2.5 pl-3 text-[12px]">
                {contact.purposes?.length ? (
                  contact.purposes.map((purpose) => (
                    <div key={purpose} className="break-words text-idpxyz-text">
                      {contactPurposeLabels[purpose] ?? purpose}
                      {contact.primaryFor?.includes(purpose) ? (
                        <span className="ml-1 whitespace-nowrap text-idpxyz-accent">· 主要</span>
                      ) : null}
                    </div>
                  ))
                ) : (
                  <span className="text-idpxyz-breadcrumb">用途未配置</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function ContactValue({ value, empty, secondary = false }: { value?: string; empty: string; secondary?: boolean }) {
  return (
    <p className={`[overflow-wrap:anywhere] ${secondary ? 'text-[12px]' : 'tabular-nums'} ${value && !secondary ? 'text-idpxyz-text' : 'text-idpxyz-breadcrumb'}`}>
      {value || empty}
    </p>
  );
}
