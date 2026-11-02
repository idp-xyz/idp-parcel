import type { ReactNode } from 'react';
import type { LegalEntityProfileContent as ProfileContent } from './api';
import { LegalEntityContactsTable } from './LegalEntityContactsTable';
import { ProfileReadoutField, ProfileReadoutGrid } from './LegalEntityProfileReadout';
import {
  addressText,
  profileMaintenanceNotes,
  representativeLabel,
} from './legal-entity-profile';

export function LegalEntityProfileContent({ content, includeContacts = true }: { content?: ProfileContent; includeContacts?: boolean }) {
  const notes = content ? profileMaintenanceNotes(content) : [];
  return (
    <div className="legal-entity-profile-readout mt-5 space-y-5">
      <ProfileGroup title="代表人信息">
        <ProfileReadoutGrid>
          <ProfileReadoutField
            label={content?.representative ? representativeLabel(content.representative.kind) : '代表人 / 负责人'}
            value={content?.representative?.name}
            emphasized
          />
        </ProfileReadoutGrid>
      </ProfileGroup>
      <ProfileGroup title="地址信息">
        <ProfileReadoutGrid>
          <ProfileReadoutField label="注册地址" value={addressText(content?.registeredAddress)} />
          <ProfileReadoutField label="办公地址" value={addressText(content?.officeAddress)} />
          <ProfileReadoutField label="通信地址" value={addressText(content?.correspondenceAddress)} />
        </ProfileReadoutGrid>
      </ProfileGroup>
      <ProfileGroup title="税务与开票">
        <ProfileReadoutGrid>
          <ProfileReadoutField label="开票抬头" value={content?.invoicingRegistered ? content.invoiceTitle : undefined} emphasized />
          <ProfileReadoutField
            label="税务登记号"
            value={
              content?.taxRegistrationNumbers.length
                ? content.taxRegistrationNumbers.map((row) => `${row.typeCode} ${row.number}`).join('；')
                : undefined
            }
            mono
          />
        </ProfileReadoutGrid>
      </ProfileGroup>
      {includeContacts ? (
        <ProfileGroup title="联系人" extra={content?.contacts.length ? `${content.contacts.length} 位` : undefined}>
          <LegalEntityContactsTable contacts={content?.contacts ?? []} />
        </ProfileGroup>
      ) : null}
      {notes.length ? (
        <div className="border-t border-idpxyz-border pt-4 text-[12px] leading-6 text-idpxyz-breadcrumb">
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

function ProfileGroup({ title, extra, children }: { title: string; extra?: string; children: ReactNode }) {
  return (
    <section className="border-t border-idpxyz-border pt-4">
      <div className="mb-1 flex items-baseline gap-2">
        <h4 className="text-[14px] font-semibold leading-6 text-idpxyz-textBright">{title}</h4>
        {extra ? <span className="text-[12px] text-idpxyz-breadcrumb">{extra}</span> : null}
      </div>
      {children}
    </section>
  );
}
