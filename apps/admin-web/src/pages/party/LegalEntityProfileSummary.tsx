import { LegalEntityProfileContent } from './LegalEntityProfileContent';
import { CurrentLegalEntityProfile, type CurrentLegalEntityProfileQuery } from './CurrentLegalEntityProfile';
import { useMemo } from 'react';
import { formatInstant } from '../moment';
import { listRegistrationNumberTypes, type LegalEntityProfileResolutionResponseBody } from './api';
import { profileRevisionLabel } from './presentation';
import { legalEntityProfileStatusOf } from './legal-entity-profile';
import { registrationNumberTypeNamesOf, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { Instant, RefText } from './detail-primitives';
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
      {/* 读出的小标题只说「读的是哪一版」（票 company-subject-ux/08 第 3 条）：资料状态那枚药丸撤掉，它已在头区事实行里，
          同一块面上出现两次是重复。没有生效版本时没有版可点名，才把结论句「资料状态：…」写在这里。 */}
      <p className="text-[13px] leading-6 text-idpxyz-breadcrumb">
        {effective ? (
          <>
            当前资料 <span className="tabular-nums text-idpxyz-text">{profileRevisionLabel(effective.revision)}</span> ·{' '}
            <span className="text-idpxyz-text"><Instant value={effective.effectiveFrom} precision="day" /></span> 起生效 · 来源单据{' '}
            {/* 来源单据是号码类（票 company-subject-ux/14）：点击即复制；色随外层，这句是弱显的小字，号要正文色，所以套一层。 */}
            <span className="text-[13px] text-idpxyz-text"><RefText value={effective.basis} name="来源单据" /></span>
          </>
        ) : (
          <>资料状态：{status.label}</>
        )}
      </p>
      {status.tone !== 'good' ? (
        <p className="mt-1 text-[13px] leading-6 text-idpxyz-breadcrumb">{status.description}</p>
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
