import type { LegalEntityContactRecord } from './api';
import { contactPurposeLabels } from './legal-entity-profile';
import { absentValueMark } from './LegalEntityProfileReadout';

export function LegalEntityContactsTable({ contacts }: { contacts: readonly LegalEntityContactRecord[] }) {
  // 整张表没有人是结论句，照旧用文字；表内的空格一律「—」（spec 公司主体 UX「统一用语」）。
  if (!contacts.length) return <p className="text-[13px] text-idpxyz-breadcrumb">还没有登记联系人</p>;
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
                <StackedValues primary={contact.department} secondary={contact.position} />
              </td>
              <td className="px-3 py-2.5">
                <StackedValues primary={contact.phone} secondary={contact.email} primaryNumeric />
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
                ) : <span className="text-idpxyz-breadcrumb">{absentValueMark}</span>}
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
                  <span className="text-idpxyz-breadcrumb">{absentValueMark}</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/**
 * 上下两行的格（部门 / 职务、电话 / 邮箱）：有哪行显哪行，一行都没有才显一个「—」——两行各显一个「—」会让人以为是两个
 * 都要填的格，而这两格本来就可缺。
 */
function StackedValues({ primary, secondary, primaryNumeric = false }: { primary?: string; secondary?: string; primaryNumeric?: boolean }) {
  if (!primary && !secondary) return <p className="text-idpxyz-breadcrumb">{absentValueMark}</p>;
  return (
    <>
      {primary ? <p className={`[overflow-wrap:anywhere] text-idpxyz-text ${primaryNumeric ? 'tabular-nums' : ''}`}>{primary}</p> : null}
      {secondary ? <p className="[overflow-wrap:anywhere] text-[12px] text-idpxyz-breadcrumb">{secondary}</p> : null}
    </>
  );
}
