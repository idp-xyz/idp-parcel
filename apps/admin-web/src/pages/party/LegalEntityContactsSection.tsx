import { CurrentLegalEntityProfile, type CurrentLegalEntityProfileQuery } from './CurrentLegalEntityProfile';
import { LegalEntityContactCards } from './LegalEntityContactCards';
import { SectionActions } from './LegalEntityProfileContent';
import { Instant } from './detail-primitives';
import { legalEntityContactCountOf, legalEntityProfileStatusOf } from './legal-entity-profile';
import type { ProfileEditRequest } from './legal-entity-profile-sections';
import { profileRevisionLabel } from './presentation';

export function LegalEntityContactsSection({
  query,
  onEdit,
}: {
  query: CurrentLegalEntityProfileQuery;
  /** 「添加联系人」「编辑」开分区抽屉（票 company-subject-section-edit/01）；页面有写授予才给，不给就只指路整张表单。 */
  onEdit?: (request: ProfileEditRequest) => void;
}) {
  return (
    <CurrentLegalEntityProfile query={query}>
      {(body) => {
        const profile = body.effectiveRevision;
        const status = legalEntityProfileStatusOf(body);
        if (!profile || legalEntityContactCountOf(body) === undefined) {
          return <p className="text-[14px] leading-6 text-idpxyz-breadcrumb">{status.description}</p>;
        }
        // 小字与读出的标签同一档 13（票 company-subject-ux/13 第 3 条）。
        return (
          <section>
            <div className="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
              <h4 className="text-[14px] font-semibold leading-6 text-idpxyz-textBright">
                联系人 <span className="ml-1 text-[13px] font-normal text-idpxyz-breadcrumb">{profile.contacts.length} 位</span>
              </h4>
              {/* 生效是「哪一天」，只显日期、按显示时区；完整时刻在悬停里（spec 公司主体 UX「统一用语」时间那行）。 */}
              <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[13px] leading-6 tabular-nums text-idpxyz-breadcrumb">
                <span>
                  {profileRevisionLabel(profile.revision)} · 生效自 <Instant value={profile.effectiveFrom} precision="day" />
                </span>
                {onEdit ? <span className="flex items-center gap-1"><SectionActions section="contacts" onEdit={onEdit} /></span> : null}
              </span>
            </div>
            <LegalEntityContactCards contacts={profile.contacts} />
            {onEdit ? null : <p className="mt-4 text-[13px] leading-6 text-idpxyz-breadcrumb">要改联系人，用右上方的「更新公司资料」。</p>}
          </section>
        );
      }}
    </CurrentLegalEntityProfile>
  );
}
