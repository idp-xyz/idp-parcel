import { useEffect, useMemo, useRef, useState } from 'react';
import { Button, ConfirmDialog, Input } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote } from '../../components/registration';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, wallTimeOf } from '../moment';
import {
  legalEntityProfileOutcomeLabels,
  listRegistrationNumberTypes,
  type LegalEntityProfileContent as ProfileContent,
  type LegalEntityProfileRevisionRecord,
  type RegistrationNumberTypeListResponseBody,
} from './api';
import { CurrentLegalEntityProfile, useCurrentLegalEntityProfile } from './CurrentLegalEntityProfile';
import { FieldAnchor, LegalEntityProfileFields } from './LegalEntityProfileFields';
import { ChangePreview, scrollToField, useTypeNameOf } from './legal-entity-profile-form-parts';
import { compareLegalEntityProfiles, type ProfileComparison } from './legal-entity-profile-history';
import {
  legalEntityProfileDraftBasedOn,
  legalEntityProfileLocalProblems,
  legalEntityProfileMissingFields,
  legalEntityProfilePayloadOf,
  legalEntityProfileStatusOf,
  problemsWithMissing,
  profileComparisonContentOf,
  suggestedProfileRevision,
  type LegalEntityProfileDraft,
} from './legal-entity-profile';
import { useLegalEntityRead } from './legal-entity-reads';
import {
  draftWithNewRow,
  profileSectionBasisOf,
  profileSections,
  splitMissingBySection,
  type ProfileSection,
  type ProfileSectionEditing,
} from './legal-entity-profile-sections';
import { AdvancedGroup, FormGroup, RevisionField, WallTimeField, useRegistrationForm } from './party-registration-fields';
import { problemNote, profileRevisionLabel } from './presentation';
import { Field, useLoaded } from './PublicationFormFields';
import { advancedGroupOpen } from './registration-form';

const info = moduleInfoById['group-legal-entities'];
const kind = 'legal-entity-profile';
const note = 'text-[12px] leading-5 text-idpxyz-breadcrumb';

/**
 * 读出卡里的就地编辑（票 company-subject-section-edit/02，参考 SAP Fiori 对象页的显示态 / 编辑态）：点卡上「编辑」「添加…」，
 * 卡体从读出换成只装这一组格的表单，卡底一行「取消 / 保存」，其余卡照旧是读出。服务端只收整版资料，所以这里仍是
 * 「此刻的当前版 + 这一组的改动」拼成的整版新资料，机制与整张表单（LegalEntityProfileSection.tsx）同一套，只换容器。
 *
 * 先把当前版与版本清单现问回来（`fresh`）再挂表单：缓存那份可能早于别人刚落的一版，拿旧内容预填再保存会把那一版无声改回去。
 * 没有当前版无从「局部」改起，指路整张表单。
 */
export function ProfileSectionEditor({ editing, onClose }: { editing: ProfileSectionEditing; onClose: () => void }) {
  const { legalEntityId } = editing;
  const section = profileSections[editing.section];
  const current = useCurrentLegalEntityProfile(legalEntityId, undefined, { fresh: true });
  const revisions = useLegalEntityRead('profileRevisions', legalEntityId, undefined, { fresh: true });
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  const known =
    revisions.answer?.kind === 'outcome' && revisions.answer.body.legalEntityId === legalEntityId ? revisions.answer.body.revisions : null;
  const knownFailed = revisions.answer !== null && known === null;
  const answer = current.answer;
  const resolved = answer?.kind === 'outcome' && answer.body.legalEntityId === legalEntityId ? answer.body : null;
  if (answer === null) return <p className={note}>正在读取当前资料，表单会基于它预填…</p>;
  if (resolved === null || !resolved.effectiveRevision) {
    return (
      <div className="flex flex-col items-start gap-3">
        {resolved === null ? (
          <CurrentLegalEntityProfile query={current}>{() => null}</CurrentLegalEntityProfile>
        ) : (
          <p className={note}>
            当前没有生效的资料（{legalEntityProfileStatusOf(resolved).label}），无从只改「{section.title}」；请用「更新公司资料」登记整版。
          </p>
        )}
        <Button variant="outline" size="sm" className="text-[12px]" onClick={onClose}>
          关闭
        </Button>
      </div>
    );
  }
  return (
    <SectionForm
      legalEntityId={legalEntityId}
      section={section}
      addRow={editing.addRow}
      current={resolved.effectiveRevision}
      known={known}
      knownFailed={knownFailed}
      onRetryKnown={revisions.retry}
      typeCatalogue={typeCatalogue}
      onClose={onClose}
    />
  );
}

/**
 * 分区表单本体。来源单据按这次改动自动写（profileSectionBasisOf），人一改就按人的、可复位；生效时间预填打开时的此刻，与版本号
 * 一起收进高级组。本区的必填缺项点保存时点名并滚到格；别区的（例如改联系人时注册地址还空着）这张卡里没有那一格，列出来
 * 拦下保存、指路整张表单。落册后报一次 toast 并回到读出——读出由写口整域作废后自己重读；写口答了别的留在卡里原样示出。
 */
function SectionForm({
  legalEntityId,
  section,
  addRow,
  current,
  known,
  knownFailed,
  onRetryKnown,
  typeCatalogue,
  onClose,
}: {
  legalEntityId: string;
  section: ProfileSection;
  addRow: boolean;
  current: ProfileContent;
  known: LegalEntityProfileRevisionRecord[] | null;
  knownFailed: boolean;
  onRetryKnown: () => void;
  typeCatalogue: ApiResult<RegistrationNumberTypeListResponseBody> | null;
  onClose: () => void;
}) {
  const timeZone = currentDisplayTimeZone();
  // 未保存的基线就是打开时预填进去的那份：只有预填、人什么都没动，不算没提交。生效时间的「此刻」与从「添加」加的空行都算进基线。
  const opening = useMemo(() => {
    const timed = { ...legalEntityProfileDraftBasedOn(legalEntityId, current), effectiveFrom: wallTimeOf(new Date(), timeZone) };
    return addRow ? draftWithNewRow(section, timed) : { draft: timed, focusPath: null as string | null };
  }, [legalEntityId, current, section, addRow, timeZone]);
  const { addToast } = useToast();
  const form = useRegistrationForm<LegalEntityProfileDraft>({
    kind,
    empty: () => opening.draft,
    suggestion: () => suggestedProfileRevision(known, legalEntityId),
    localProblems: legalEntityProfileLocalProblems,
    payloadOf: legalEntityProfilePayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded: (sent) => {
      addToast({ type: 'success', title: `${section.title}已更新${sent ? `，登记为${profileRevisionLabel(Number(sent.revision))}` : ''}` });
      onClose();
    },
  });
  const { draft, patch, locked } = form;
  const bodyRef = useRef<HTMLDivElement>(null);
  // 从「添加」打开：新加的那一空行滚进视野、聚焦它的第一个格。
  useEffect(() => {
    if (opening.focusPath !== null) scrollToField(bodyRef.current, opening.focusPath);
  }, [opening.focusPath]);
  const typeNameOf = useTypeNameOf(typeCatalogue);

  const [basisEdited, setBasisEdited] = useState(false);
  const autoBasis = profileSectionBasisOf(
    section,
    compareLegalEntityProfiles(current, profileComparisonContentOf(form.effectiveDraft, timeZone), typeNameOf),
  );
  const useAutoBasis = !basisEdited;
  // 将要送的那份：缺项判读、预览、提交都看它；落册簿记（landedKey）仍按草稿本身记。
  const sendingDraft = useAutoBasis ? { ...form.effectiveDraft, basis: autoBasis } : form.effectiveDraft;

  // 必填未填只在点过保存之后才显在格下与按钮旁；之后随输入实时更新，填上一格少一条。
  const [attempted, setAttempted] = useState(false);
  const { inside: missing, outside: outsideMissing } = splitMissingBySection(section, legalEntityProfileMissingFields(sendingDraft));
  const problems = attempted ? problemsWithMissing(form.problems, missing) : form.problems;
  const [preview, setPreview] = useState<ProfileComparison | null>(null);
  const save = () => {
    if (missing.length > 0) {
      setAttempted(true);
      scrollToField(bodyRef.current, missing[0].path);
      return;
    }
    setPreview(compareLegalEntityProfiles(current, profileComparisonContentOf(sendingDraft, timeZone), typeNameOf));
  };
  const send = () =>
    form.sendVia((sending, submitDraft) =>
      submitDraft(legalEntityProfilePayloadOf(useAutoBasis ? { ...sending, basis: autoBasis } : sending, timeZone), sending),
    );

  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, ['profiles[0].revision', 'profiles[0].effectiveFrom'], false);
  const advancedSummary = `生效时间 ${draft.effectiveFrom.trim().replace('T', ' ') || '未填'} · 版本号 ${
    form.revisionField.value.trim() || '未填'
  }${form.revisionField.edited ? '（手填）' : '（建议值）'}`;

  // 不是模态：取消、表单里的站内链接要离开时，有没提交的输入先问一句。null 是取消，字符串是要去的地址。
  const [pendingLeave, setPendingLeave] = useState<{ href: string | null } | null>(null);
  const leave = (href: string | null) => {
    setPendingLeave(null);
    onClose();
    if (href !== null) window.location.hash = href;
  };
  const requestLeave = (href: string | null) => (form.unsaved ? setPendingLeave({ href }) : leave(href));

  return (
    <div ref={bodyRef} className="flex flex-col gap-4" role="form" aria-label={`更新${section.title}`}>
      {preview !== null ? (
        <ChangePreview
          comparison={preview}
          current={current}
          nextRevision={form.effectiveDraft.revision}
          effectiveFrom={draft.effectiveFrom}
          timeZone={timeZone}
          basis={sendingDraft.basis}
        />
      ) : (
        <>
          <p className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 text-[12px] leading-5 text-idpxyz-text">
            基于当前生效的 <span className="font-medium">{profileRevisionLabel(current.revision)}</span> 只改「{section.title}」，其余内容原样带着；
            保存后登记为新的一版。
          </p>
          <LegalEntityProfileFields
            draft={draft}
            patch={patch}
            problems={problems}
            locked={locked}
            typeCatalogue={typeCatalogue}
            onLeave={requestLeave}
            groups={section.groups}
          />
          <FormGroup title="这次更新" description="来源单据按这次改动自动写好，也可以改成单据号；生效时间默认现在，要改在高级里。">
            <FieldAnchor path="profiles[0].basis">
              <Field label="来源单据 *" path="profiles[0].basis" problems={problems}>
                <div className="flex items-center gap-2">
                  <Input
                    value={useAutoBasis ? autoBasis : draft.basis}
                    readOnly={locked}
                    className="font-mono text-[13px]"
                    onChange={(event) => {
                      setBasisEdited(true);
                      patch({ basis: event.target.value });
                    }}
                  />
                  {basisEdited ? (
                    <Button
                      variant="outline"
                      size="sm"
                      className="shrink-0 text-[12px] text-idpxyz-breadcrumb"
                      disabled={locked}
                      onClick={() => {
                        setBasisEdited(false);
                        patch({ basis: '' });
                      }}
                    >
                      用自动写的
                    </Button>
                  ) : null}
                </div>
              </Field>
            </FieldAnchor>
          </FormGroup>
          <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
            <FieldAnchor path="profiles[0].effectiveFrom">
              <WallTimeField
                label="生效时间 *"
                path="profiles[0].effectiveFrom"
                problems={problems}
                value={draft.effectiveFrom}
                locked={locked}
                timeZone={timeZone}
                note="默认是打开这张表单的时刻；可以改到未来。"
                onChange={(effectiveFrom) => patch({ effectiveFrom })}
              />
            </FieldAnchor>
            <RevisionField
              label="版本号"
              path="profiles[0].revision"
              problems={problems}
              {...form.revisionField}
              note="系统按已登记的记录自动算好，一般不用改。"
            />
            {knownFailed ? (
              <Button variant="outline" size="sm" className="text-[12px]" disabled={locked} onClick={onRetryKnown}>
                重试读取版本清单
              </Button>
            ) : null}
          </AdvancedGroup>
        </>
      )}
      {/* 卡底动作条（Fiori 的页脚动作条收进卡里）：左边说话，右边动作、主动作靠右。 */}
      <div className="flex flex-wrap items-start justify-between gap-3 border-t border-idpxyz-border pt-3">
        <div className="min-w-0 flex-1 text-[12px]">
          {attempted && missing.length > 0 ? (
            <p role="alert" className="text-idpxyz-danger">
              还有 {missing.length} 项必填未填：{missing.map((field) => field.label).join('、')}
            </p>
          ) : null}
          {outsideMissing.length > 0 ? (
            <p role="alert" className="text-idpxyz-danger">
              其他分区还有必填未填：{outsideMissing.map((field) => field.label).join('、')}。请先用「更新公司资料」整张表单补齐。
            </p>
          ) : null}
          <RegistrationAnswerNote state={form.state} owner={info.owner} outcomeLabels={legalEntityProfileOutcomeLabels} problemNote={problemNote} />
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {preview !== null ? (
            <>
              <Button variant="outline" size="sm" className="text-[12px]" disabled={locked} onClick={() => setPreview(null)}>
                返回修改
              </Button>
              <Button
                variant="default"
                size="sm"
                className={`${primaryActionClass} text-[12px]`}
                disabled={locked}
                onClick={() => {
                  setPreview(null);
                  send();
                }}
              >
                {locked ? '保存中…' : '确认保存'}
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" size="sm" className="text-[12px]" disabled={locked} onClick={() => requestLeave(null)}>
                取消
              </Button>
              <Button
                variant="default"
                size="sm"
                className={`${primaryActionClass} text-[12px]`}
                onClick={save}
                disabled={locked || known === null || !form.canSend || outsideMissing.length > 0}
              >
                {locked ? '保存中…' : '保存'}
              </Button>
            </>
          )}
        </div>
      </div>
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={pendingLeave !== null}
        tone="warning"
        title={pendingLeave?.href ? '离开这一页？' : '放弃修改？'}
        message="有内容还没有保存，离开后填写的内容不会保留。"
        confirmLabel={pendingLeave?.href ? '离开' : '放弃'}
        cancelLabel="继续编辑"
        onConfirm={() => leave(pendingLeave?.href ?? null)}
        onCancel={() => setPendingLeave(null)}
      />
    </div>
  );
}
