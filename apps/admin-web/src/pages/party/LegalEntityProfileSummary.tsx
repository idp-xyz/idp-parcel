import { LegalEntityProfileContent } from './LegalEntityProfileContent';
import { CurrentLegalEntityProfile, type CurrentLegalEntityProfileQuery } from './CurrentLegalEntityProfile';
import { StatusPill } from '../../components/StatusPill';
import { useMemo } from 'react';
import { formatInstant } from '../moment';
import { listRegistrationNumberTypes, type LegalEntityProfileResolutionResponseBody } from './api';
import { profileRevisionLabel } from './presentation';
import { legalEntityProfileStatusOf } from './legal-entity-profile';
import { registrationNumberTypeNamesOf, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { Instant } from './detail-primitives';
import { useLoaded } from './PublicationFormFields';
import { TechnicalDetails } from '../../components/TechnicalDetails';

/** 公司主体首屏要能判断「现在能不能拿来开票」：这里只读当前时点，不把完整资料表单塞进概要。 */
export function LegalEntityProfileSummary({ query }: { query: CurrentLegalEntityProfileQuery }) {
  // 注册号类型目录读一次，把税务登记号的类型码译成目录译名；读不到就按码显，译名是锦上添花，号本身在答复里。
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  const typeNameOf = useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
  return (
    <CurrentLegalEntityProfile query={query}>
      {(body) => <ProfileSummary body={body} typeNameOf={typeNameOf} />}
    </CurrentLegalEntityProfile>
  );
}

function ProfileSummary({
  body,
  typeNameOf,
}: {
  body: LegalEntityProfileResolutionResponseBody;
  typeNameOf: RegistrationNumberTypeNameOf;
}) {
  const status = legalEntityProfileStatusOf(body);
  const effective = body.effectiveRevision;
  return (
    <section className="legal-entity-profile-readout" aria-label="当前有效公司资料">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <StatusPill label={`资料状态：${status.label}`} tone={status.tone} />
        {effective ? (
          <span className="text-[12px] leading-6 tabular-nums text-idpxyz-breadcrumb">
            {profileRevisionLabel(effective.revision)} · <Instant value={effective.effectiveFrom} precision="day" /> 起生效
          </span>
        ) : null}
      </div>
      {status.tone !== 'good' ? (
        <p className="mt-1 text-[12px] leading-6 text-idpxyz-breadcrumb">{status.description}</p>
      ) : null}
      <LegalEntityProfileContent content={effective} includeContacts={false} typeNameOf={typeNameOf} />
      <TechnicalDetails
        title="资料来源"
        className="mt-4"
        rows={[
          { label: '查询时点', value: formatInstant(body.at) },
          { label: '资料依据', value: effective?.basis ?? '' },
        ]}
      />
    </section>
  );
}
