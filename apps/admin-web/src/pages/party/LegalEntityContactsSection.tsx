import { CurrentLegalEntityProfile, type CurrentLegalEntityProfileQuery } from './CurrentLegalEntityProfile';
import { LegalEntityContactsTable } from './LegalEntityContactsTable';
import { formatInstant } from '../moment';
import { legalEntityContactCountOf, legalEntityProfileStatusOf } from './legal-entity-profile';
import { profileRevisionLabel } from './presentation';

export function LegalEntityContactsSection({ query }: { query: CurrentLegalEntityProfileQuery }) {
  return (
    <CurrentLegalEntityProfile query={query}>
      {(body) => {
        const profile = body.effectiveRevision;
        const status = legalEntityProfileStatusOf(body);
        if (!profile || legalEntityContactCountOf(body) === undefined) {
          return <p className="text-[13px] leading-6 text-idpxyz-breadcrumb">{status.description}</p>;
        }
        return (
          <section>
            <div className="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <h3 className="text-[15px] font-semibold leading-6 text-idpxyz-textBright">
                联系人 <span className="ml-1 text-[12px] font-normal text-idpxyz-breadcrumb">{profile.contacts.length} 位</span>
              </h3>
              <span className="text-[12px] leading-6 tabular-nums text-idpxyz-breadcrumb">
                {profileRevisionLabel(profile.revision)} · 生效自 {formatInstant(profile.effectiveFrom)}
              </span>
            </div>
            <LegalEntityContactsTable contacts={profile.contacts} />
            <p className="mt-4 text-[12px] leading-5 text-idpxyz-breadcrumb">联系人资料可通过右上方「更新法人资料」维护。</p>
          </section>
        );
      }}
    </CurrentLegalEntityProfile>
  );
}
