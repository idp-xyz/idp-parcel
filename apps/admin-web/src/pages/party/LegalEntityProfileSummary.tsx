import { LegalEntityProfileContent } from './LegalEntityProfileContent';
import { CurrentLegalEntityProfile, type CurrentLegalEntityProfileQuery } from './CurrentLegalEntityProfile';
import { StatusPill } from '../../components/StatusPill';
import { formatInstant } from '../moment';
import type { LegalEntityProfileResolutionResponseBody } from './api';
import { profileRevisionLabel } from './presentation';
import { legalEntityProfileStatusOf } from './legal-entity-profile';
import { TechnicalDetails } from '../../components/TechnicalDetails';

/** 法人首屏要能判断“现在能不能拿来开票”：这里只读当前时点，不把完整资料表单塞进概要。 */
export function LegalEntityProfileSummary({ query }: { query: CurrentLegalEntityProfileQuery }) {
  return (
    <CurrentLegalEntityProfile query={query}>
      {(body) => <ProfileSummary body={body} />}
    </CurrentLegalEntityProfile>
  );
}

function ProfileSummary({ body }: { body: LegalEntityProfileResolutionResponseBody }) {
  const status = legalEntityProfileStatusOf(body);
  const effective = body.effectiveRevision;
  return (
    <section className="legal-entity-profile-readout" aria-label="当前有效法人资料">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
        <StatusPill label={`资料状态：${status.label}`} tone={status.tone} />
        {effective ? (
          <span className="text-[12px] leading-6 tabular-nums text-idpxyz-breadcrumb">
            {profileRevisionLabel(effective.revision)} · 生效自 {formatInstant(effective.effectiveFrom)}
          </span>
        ) : null}
      </div>
      {status.tone !== 'good' ? (
        <p className="mt-1 text-[12px] leading-6 text-idpxyz-breadcrumb">{status.description}</p>
      ) : null}
      <LegalEntityProfileContent content={effective} includeContacts={false} />
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
