import { LegalEntityProfileContent } from './LegalEntityProfileContent';
import { FieldAnchor, LegalEntityProfileFields } from './LegalEntityProfileFields';
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Button, Input } from '@idpxyz/ui-primitives';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote } from '../../components/registration';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { StatusPill } from '../../components/StatusPill';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, formatInstant } from '../moment';
import {
  legalEntityProfileOutcomeLabels,
  listRegistrationNumberTypes,
  resolveLegalEntityProfile,
  type LegalEntityProfileContent as ProfileContent,
  type LegalEntityProfileResolutionResponseBody,
  type LegalEntityProfileRevisionRecord,
  type RegistrationNumberTypeListResponseBody,
} from './api';
import { profileRevisionLabel } from './presentation';
import { problemNote } from './presentation';
import { Field, useLoaded } from './PublicationFormFields';
import type { RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { Instant } from './detail-primitives';
import {
  AdvancedGroup,
  FormGroup,
  RevisionField,
  WallTimeField,
  useRegistrationForm,
} from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import { compareLegalEntityProfiles, type ProfileComparison } from './legal-entity-profile-history';
import {
  legalEntityProfileDraftBasedOn,
  legalEntityProfileLocalProblems,
  legalEntityProfileMissingFields,
  legalEntityProfilePayloadOf,
  legalEntityProfileStatusOf,
  problemsWithMissing,
  profileComparisonContentOf,
  profileDraftContentOf,
  resolutionInstantOf,
  suggestedProfileRevision,
  type LegalEntityProfileDraft,
} from './legal-entity-profile';
import { useMayWriteRegistry } from '../use-operator-grants';
import { useTabViewState } from '../../templates/workspace-view-state';
import { CurrentLegalEntityProfile, useCurrentLegalEntityProfile } from './CurrentLegalEntityProfile';
import { LegalEntityProfileHistory } from './LegalEntityProfileHistory';
import { ChangePreview, scrollToField, useTypeNameOf } from './legal-entity-profile-form-parts';
import { useLegalEntityRead } from './legal-entity-reads';

// 公司资料的版本历史与更新表单沿同一条不可覆盖的资料链维护。当前时点摘要由 LegalEntityProfileSummary 负责；判读、折算与载荷
// 在 legal-entity-profile.ts（纯函数，node:test 钉着），这里只摆。
//
// 整张更新表单是对象页的编辑态（票 company-subject-section-edit/03，参考 Fiori 对象页的显示态 / 编辑态）：住在概要分区里，
// 「保存 / 取消」钉在页脚；票 company-subject-ux/04 定的右侧抽屉已撤。页头与本区底部的「更新公司资料」都把人带进那个编辑态
// （地址上的 `?edit=profile`）。编辑态不是模态，「未保存保护」由对象页守：返回列表与表单里的站内链接要离开时先确认。
//
// 调用方按公司主体给 key：换一个主体就整块重挂，表单草稿里钉的主体标识不会停在上一个身上。

const info = moduleInfoById['group-legal-entities'];
const kind = 'legal-entity-profile';
// 编辑态下联系人那一组落到联系人分区（票 company-subject-section-edit/04），其余四组留在概要分区。
const groupsOutsideContacts = ['representative', 'addresses', 'invoicing', 'bankAccounts'] as const;
const contactsOnly = ['contacts'] as const;
const note = 'mt-1 type-body text-idpxyz-breadcrumb';
// 资料表单收在高级组里的那一格；组里有问题时组一律展开。
const profileAdvancedPaths = ['profiles[0].revision'];

export function LegalEntityProfileSection({
  legalEntityId,
  collapseCurrentProfile = false,
  version,
  onOpenForm,
}: {
  legalEntityId: string;
  /** 身份版本（legalEntityReadVersion）；资料版本签按它与签上计数同键读（legal-entity-reads.ts）。 */
  version?: string;
  /** 对象页已有当前资料摘要时折叠按日期查询，仍保留历史与未来日期的查看入口。 */
  collapseCurrentProfile?: boolean;
  /** 本区底部「更新公司资料」：把人带进对象页的编辑态；没有写授予时按钮不摆。 */
  onOpenForm?: () => void;
}) {
  const [lookupVisited, setLookupVisited] = useState(false);
  // 没有登记册配置写授予的操作者不摆更新表单与打开它的按钮（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  // 注册号类型目录读一次，给按日期查询的读出与版本历史（对比表、快照）把监管登记号类型码译成目录译名；读不到按码显。
  // 编辑态自己另读一份——它是导出件，不能指望挂它的人都带目录来。
  const typeNameOf = useTypeNameOf(useLoaded(listRegistrationNumberTypes));
  return (
    <div className="flex flex-col gap-5">
      <section>
        <div>
          <h4 className="type-value font-semibold text-idpxyz-text">资料版本历史</h4>
          <p className="mt-1 type-body leading-4 text-idpxyz-breadcrumb">每次更新都保留一版；当前生效的那一版显示在概要。</p>
        </div>
        <LegalEntityProfileHistory key={legalEntityId} legalEntityId={legalEntityId} typeNameOf={typeNameOf} version={version} />
      </section>
      {collapseCurrentProfile ? (
        <details
          className="border-t border-idpxyz-border pt-4"
          onToggle={(event) => {
            if (event.currentTarget.open) setLookupVisited(true);
          }}
        >
          <summary className="cursor-pointer type-value font-medium text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent">
            查看某个日期的资料
          </summary>
          {lookupVisited ? <CurrentProfile legalEntityId={legalEntityId} typeNameOf={typeNameOf} /> : null}
        </details>
      ) : (
        <section className="border-t border-idpxyz-border pt-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h4 className="type-value font-semibold text-idpxyz-text">当前有效资料</h4>
              <p className="mt-1 type-body leading-4 text-idpxyz-breadcrumb">
                开票时按当时生效的那一版资料；可以提前登记未来生效的资料。
              </p>
            </div>
          </div>
          <CurrentProfile legalEntityId={legalEntityId} typeNameOf={typeNameOf} />
        </section>
      )}
      {mayWrite && onOpenForm ? (
        <section className="border-t border-idpxyz-border pt-5">
          <Button className="text-idpxyz-breadcrumb" variant="outline" onClick={onOpenForm}>
            更新公司资料
          </Button>
        </section>
      ) : null}
    </div>
  );
}

/**
 * 对象页的编辑态本体（票 company-subject-section-edit/03）：先把当前生效的那一版与已登记的版本清单读回来，再挂表单——表单打开
 * 即预填当前版（票 company-subject-ux/04「默认基于当前版本」），没有就等它。两份都打开时必现问（`fresh`），不拿缓存那份预填：缓存那份可能
 * 早于别人刚落的一版，拿旧内容保存就把那一版无声改回去。当前资料读不到时不把人堵死：留一个「改为从空白开始」。
 *
 * 「保存 / 取消」与答复不在正文里：经 `footerSlot` 钉进对象页的页脚动作条（DetailPageTemplate 的 footer），槽位还没挂上时
 * 暂摆在正文末尾。分区的改动不走这里：读出卡上就地编（LegalEntityProfileSectionEditor.tsx）。
 */
export function ProfileEditMode({
  legalEntityId,
  footerSlot,
  contactsSlot = null,
  onLanded,
  onUnsavedChange,
  onRequestLeave,
}: {
  legalEntityId: string;
  /** 页脚动作条的槽位元素；null 即还没挂上。 */
  footerSlot: HTMLElement | null;
  /** 联系人分区的槽位元素（票 company-subject-section-edit/04）：表单的联系人那一组 portal 进去；null 即落回概要分区里。 */
  contactsSlot?: HTMLElement | null;
  /** 登记册答已落册时调用，带送出的草稿（版本号在里面）。 */
  onLanded: (sent: LegalEntityProfileDraft | null) => void;
  onUnsavedChange: (unsaved: boolean) => void;
  /** 要离开编辑态：null 是取消，字符串是表单里的站内链接要去的地址；有没提交的输入由对象页先问一句。 */
  onRequestLeave: (href: string | null) => void;
}) {
  const current = useCurrentLegalEntityProfile(legalEntityId, undefined, { fresh: true });
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  // 版本清单给版本号建议与「带入版本」；打开时现问（fresh），落册后由写口整域作废（party-writes.ts）再问，不另拧钥匙。
  const revisions = useLegalEntityRead('profileRevisions', legalEntityId, undefined, { fresh: true });
  const known =
    revisions.answer?.kind === 'outcome' && revisions.answer.body.legalEntityId === legalEntityId ? revisions.answer.body.revisions : null;
  const knownFailed = revisions.answer !== null && known === null;
  const [blankFallback, setBlankFallback] = useState(false);
  const renderFooter = (node: ReactNode) => (footerSlot ? createPortal(node, footerSlot) : node);
  const renderContacts = contactsSlot ? (node: ReactNode) => createPortal(node, contactsSlot) : undefined;

  const answer = current.answer;
  const resolved = answer?.kind === 'outcome' && answer.body.legalEntityId === legalEntityId ? answer.body : null;
  if (resolved === null && !blankFallback) {
    return (
      <>
        {answer === null ? (
          <p className="type-body text-idpxyz-breadcrumb">正在读取当前资料，表单会基于它预填…</p>
        ) : (
          <div className="flex flex-col items-start gap-3">
            <CurrentLegalEntityProfile query={current}>{() => null}</CurrentLegalEntityProfile>
            <Button variant="outline" onClick={() => setBlankFallback(true)}>
              改为从空白开始
            </Button>
          </div>
        )}
        {renderFooter(
          <div className="flex justify-end">
            <Button variant="outline" onClick={() => onRequestLeave(null)}>
              取消
            </Button>
          </div>,
        )}
      </>
    );
  }
  const currentContent = resolved?.effectiveRevision ?? null;
  const noCurrentNote =
    resolved === null
      ? '当前资料读不到'
      : currentContent === null
        ? `当前没有生效的资料（${legalEntityProfileStatusOf(resolved).label}）`
        : null;
  return (
    <ProfileRegistrationForm
      legalEntityId={legalEntityId}
      current={currentContent}
      noCurrentNote={noCurrentNote}
      known={known}
      knownFailed={knownFailed}
      onRetryKnown={revisions.retry}
      typeCatalogue={typeCatalogue}
      renderFooter={renderFooter}
      renderContacts={renderContacts}
      onLanded={onLanded}
      onUnsavedChange={onUnsavedChange}
      onRequestLeave={onRequestLeave}
    />
  );
}

/** 表单内容格基于哪一版：当前生效的、空白、或操作者另选的一版。只决定预填，差异预览一律与当前生效版比。 */
type DraftBase = { kind: 'current' } | { kind: 'blank' } | { kind: 'revision'; revision: number };

/** 「改为从空白开始」要把内容格清空而不碰来源单据、生效时间与版本号：取一份空内容折成的那几格。 */
const blankContent: ProfileContent = {
  revision: 0,
  basis: '',
  effectiveFrom: '',
  registeredAddress: { country: '', lines: [] },
  taxRegistrationNumbers: [],
  invoicingRegistered: false,
  contacts: [],
};

/**
 * 登记新版本的逐字段表单。本地提示输入结构、联系人分工（legalEntityProfileLocalProblems）与必填未填
 * （legalEntityProfileMissingFields）；地址国家与身份对不对得上、登记号合不合目录、版本连不连续都由服务端答，答复原样示出。
 *
 * 保存前先看差异（票面第 2 项）：有当前生效版时点「保存」先把草稿折成资料内容与它比，列出新增 / 修改 / 移除，预览里再点
 * 「确认保存」才发；没有当前生效版（第一版）直接发。版本号有建议值，收进高级组。
 */
function ProfileRegistrationForm({
  legalEntityId,
  current,
  noCurrentNote,
  known,
  knownFailed,
  onRetryKnown,
  typeCatalogue,
  renderFooter,
  renderContacts,
  onLanded,
  onUnsavedChange,
  onRequestLeave,
}: {
  legalEntityId: string;
  /** 当前生效的那一版；没有为 null。预填默认基于它，差异预览与它比。 */
  current: ProfileContent | null;
  /** 没有当前生效版时说一句为什么（带资料状态的词）；有就 null。 */
  noCurrentNote: string | null;
  known: LegalEntityProfileRevisionRecord[] | null;
  knownFailed: boolean;
  onRetryKnown: () => void;
  typeCatalogue: ApiResult<RegistrationNumberTypeListResponseBody> | null;
  /** 动作条摆到哪里去（页脚槽位）；ProfileEditMode 给。 */
  renderFooter: (node: ReactNode) => ReactNode;
  /** 联系人那一组摆到哪里去（联系人分区的槽位，票 company-subject-section-edit/04）；不给就留在本表单里。 */
  renderContacts?: (node: ReactNode) => ReactNode;
  onLanded: (sent: LegalEntityProfileDraft | null) => void;
  onUnsavedChange: (unsaved: boolean) => void;
  onRequestLeave: (href: string | null) => void;
}) {
  const [base, setBase] = useState<DraftBase>(current === null ? { kind: 'blank' } : { kind: 'current' });
  const baseContent =
    base.kind === 'current' ? current : base.kind === 'blank' ? null : (known?.find((row) => row.revision === base.revision) ?? null);
  // 未保存的基线就是打开（或改基）时预填进去的那份：只有预填、人什么都没动，不算没提交。
  const baseDraft = useMemo(() => legalEntityProfileDraftBasedOn(legalEntityId, baseContent), [legalEntityId, baseContent]);
  const form = useRegistrationForm<LegalEntityProfileDraft>({
    kind,
    empty: () => baseDraft,
    suggestion: () => suggestedProfileRevision(known, legalEntityId),
    localProblems: legalEntityProfileLocalProblems,
    payloadOf: legalEntityProfilePayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded,
  });
  const { draft, patch, locked, timeZone } = form;
  useEffect(() => {
    onUnsavedChange(form.unsaved);
  }, [onUnsavedChange, form.unsaved]);
  useEffect(() => () => onUnsavedChange(false), [onUnsavedChange]);

  const switchBase = (next: DraftBase, content: ProfileContent | null) => {
    setBase(next);
    patch(profileDraftContentOf(content ?? blankContent));
  };

  const [sourceRevision, setSourceRevision] = useState('');
  const source = known?.find((row) => String(row.revision) === sourceRevision) ?? null;

  // 必填未填只在点过保存之后才显在格下与按钮旁；之后随输入实时更新，填上一格少一条。
  const [attempted, setAttempted] = useState(false);
  const missing = legalEntityProfileMissingFields(form.effectiveDraft);
  const problems = attempted ? problemsWithMissing(form.problems, missing) : form.problems;
  const bodyRef = useRef<HTMLDivElement>(null);
  const [preview, setPreview] = useState<ProfileComparison | null>(null);
  // 预览比表单短得多：表单住在整页滚动的分区里，内容一换短，滚动位置会落到后面的分区上，人看不见预览——把它滚进视野。
  useEffect(() => {
    if (preview !== null) bodyRef.current?.scrollIntoView({ block: 'start' });
  }, [preview]);
  const typeNameOf = useTypeNameOf(typeCatalogue);
  // 内容没变就不登新版（票 06）：有当前版时把将要送的内容与它比，一处都没变就禁用保存；只改来源单据或生效时间不算。第一版照旧。
  const changed =
    current === null ||
    compareLegalEntityProfiles(current, profileComparisonContentOf(form.effectiveDraft, timeZone), typeNameOf).fields.some(
      (field) => field.kind !== 'unchanged',
    );
  const save = () => {
    if (missing.length > 0) {
      setAttempted(true);
      scrollToField(bodyRef.current, missing[0].path);
      return;
    }
    if (current === null) {
      form.send();
      return;
    }
    setPreview(compareLegalEntityProfiles(current, profileComparisonContentOf(form.effectiveDraft, timeZone), typeNameOf));
  };

  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, profileAdvancedPaths, false);
  const advancedSummary = `版本号 ${form.revisionField.value.trim() || '未填'}${
    form.revisionField.edited ? '（手填）' : '（建议值）'
  }`;
  const knownOptions = [...(known ?? [])].sort((a, b) => b.revision - a.revision);

  return (
    <>
      {/* scroll-mt：对象页的锚点栏吸在滚动容器顶上，滚进视野时按它留出高度，预览的标题不被盖住。 */}
      <div ref={bodyRef} className="min-w-0 scroll-mt-14">
          {preview !== null && current !== null ? (
            <>
            {renderContacts ? renderContacts(<p className="type-value leading-6 text-idpxyz-breadcrumb">联系人的改动在上方「概要」分区的预览里。</p>) : null}
            <ChangePreview
              comparison={preview}
              current={current}
              nextRevision={form.effectiveDraft.revision}
              effectiveFrom={draft.effectiveFrom}
              timeZone={timeZone}
            />
            </>
          ) : (
            <div className="flex flex-col gap-5">
              <div className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 type-body leading-5 text-idpxyz-text">
                <BaseLine base={base} current={current} baseContent={baseContent} known={known} noCurrentNote={noCurrentNote} locked={locked} onSwitch={switchBase} />
                {knownOptions.length > 0 || knownFailed ? (
                  <div className="mt-1.5 flex flex-wrap items-center gap-2 border-t border-idpxyz-border pt-1.5 text-idpxyz-breadcrumb">
                    <label htmlFor={`profile-source-${legalEntityId}`}>从已登记版本带入</label>
                    <select
                      id={`profile-source-${legalEntityId}`}
                      disabled={locked || known === null}
                      value={sourceRevision}
                      onChange={(event) => setSourceRevision(event.target.value)}
                      className="h-7 min-w-0 max-w-full rounded-md border border-idpxyz-border bg-idpxyz-card px-2 type-body text-idpxyz-text"
                    >
                      <option value="">{knownFailed ? '版本读取失败' : '选择资料版本'}</option>
                      {knownOptions.map((row) => (
                        <option key={row.revision} value={row.revision}>
                          {profileRevisionLabel(row.revision)} · {formatInstant(row.effectiveFrom, { precision: 'day' })} 起生效
                        </option>
                      ))}
                    </select>
                    <Button
                      variant="outline"
                      size="sm"
                      className="type-body"
                      disabled={locked || source === null}
                      onClick={() => {
                        if (source) switchBase({ kind: 'revision', revision: source.revision }, source);
                      }}
                    >
                      带入此版本
                    </Button>
                    {knownFailed ? (
                      <Button variant="outline" size="sm" className="type-body" disabled={locked} onClick={onRetryKnown}>
                        重试读取版本
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </div>

              <ul className="list-disc pl-4 type-body leading-5 text-idpxyz-breadcrumb">
                <li>每次更新都留一版，新的一版从它的生效时间起取代上一版；可以提前登记未来生效的资料。</li>
                <li>注册地址的国家 / 地区须与身份上的注册国家 / 地区一致。</li>
                <li>没填开票抬头的那一版生效后，开票会因「资料不全」被拒。</li>
              </ul>

              <LegalEntityProfileFields
                draft={draft}
                patch={patch}
                problems={problems}
                locked={locked}
                typeCatalogue={typeCatalogue}
                onLeave={onRequestLeave}
                groups={renderContacts ? groupsOutsideContacts : undefined}
              />
              {renderContacts
                ? renderContacts(
                    <LegalEntityProfileFields
                      draft={draft}
                      patch={patch}
                      problems={problems}
                      locked={locked}
                      typeCatalogue={typeCatalogue}
                      onLeave={onRequestLeave}
                      groups={contactsOnly}
                    />,
                  )
                : null}

              <FormGroup title="这次更新" description="依据哪份单据、从什么时候起生效；生效时间可以在未来。">
                <FieldAnchor path="profiles[0].basis">
                  <Field label="来源单据 *" path="profiles[0].basis" problems={problems}>
                    <Input
                      value={draft.basis}
                      readOnly={locked}
                      className="font-mono type-value"
                      placeholder="营业执照、税务登记或审批单号"
                      onChange={(event) => patch({ basis: event.target.value })}
                    />
                  </Field>
                </FieldAnchor>
                <FieldAnchor path="profiles[0].effectiveFrom">
                  <WallTimeField
                    label="生效时间 *"
                    path="profiles[0].effectiveFrom"
                    problems={problems}
                    value={draft.effectiveFrom}
                    locked={locked}
                    timeZone={timeZone}
                    onChange={(effectiveFrom) => patch({ effectiveFrom })}
                  />
                </FieldAnchor>
              </FormGroup>

              <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
                <RevisionField
                  label="版本号"
                  path="profiles[0].revision"
                  problems={problems}
                  {...form.revisionField}
                  note="系统按已登记的记录自动算好，一般不用改。"
                />
              </AdvancedGroup>
            </div>
          )}
      </div>
      {renderFooter(
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0 flex-1 type-body">
          {attempted && missing.length > 0 ? (
            <p role="alert" className="text-idpxyz-danger">
              还有 {missing.length} 项必填未填：{missing.map((field) => field.label).join('、')}
            </p>
          ) : null}
          <RegistrationAnswerNote
            state={form.state}
            owner={info.owner}
            outcomeLabels={legalEntityProfileOutcomeLabels}
            problemNote={problemNote}
          />
          {!changed && preview === null && form.state.kind === 'idle' ? <p className="text-idpxyz-breadcrumb">资料没有改动，不用保存。</p> : null}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {preview !== null ? (
            <>
              <Button variant="outline" disabled={locked} onClick={() => setPreview(null)}>
                返回修改
              </Button>
              <Button
                variant="default"
                className={primaryActionClass}
                disabled={locked}
                onClick={() => {
                  setPreview(null);
                  form.send();
                }}
              >
                确认保存
              </Button>
            </>
          ) : (
            <>
              <Button variant="outline" disabled={locked} onClick={() => onRequestLeave(null)}>
                取消
              </Button>
              <Button
                variant="default"
                className={primaryActionClass}
                onClick={save}
                disabled={locked || known === null || !form.canSend || !changed}
              >
                {locked ? '保存中…' : '保存'}
              </Button>
            </>
          )}
        </div>
      </div>,
      )}
    </>
  );
}

/** 表单顶上那一行：内容格基于哪一版、怎么换（票面第 1 项）。 */
function BaseLine({
  base,
  current,
  baseContent,
  known,
  noCurrentNote,
  locked,
  onSwitch,
}: {
  base: DraftBase;
  current: ProfileContent | null;
  baseContent: ProfileContent | null;
  known: LegalEntityProfileRevisionRecord[] | null;
  noCurrentNote: string | null;
  locked: boolean;
  onSwitch: (next: DraftBase, content: ProfileContent | null) => void;
}) {
  const link = 'text-idpxyz-accent underline-offset-2 hover:underline disabled:opacity-60';
  const versionOf = (content: ProfileContent) => (
    <>
      <span className="font-medium">{profileRevisionLabel(content.revision)}</span>（
      <Instant value={content.effectiveFrom} precision="day" /> 起生效）
    </>
  );
  const backToCurrent =
    current !== null && base.kind !== 'current' ? (
      <button type="button" className={link} disabled={locked} onClick={() => onSwitch({ kind: 'current' }, current)}>
        改回基于{profileRevisionLabel(current.revision)}
      </button>
    ) : null;
  const toBlank =
    base.kind !== 'blank' ? (
      <button type="button" className={link} disabled={locked} onClick={() => onSwitch({ kind: 'blank' }, null)}>
        改为从空白开始
      </button>
    ) : null;
  if (base.kind === 'current' && current !== null) {
    return (
      <p className="flex flex-wrap items-center gap-x-2">
        <span>基于 {versionOf(current)}修改</span>
        <span className="text-idpxyz-breadcrumb">·</span>
        {toBlank}
      </p>
    );
  }
  if (base.kind === 'revision' && baseContent !== null) {
    return (
      <p className="flex flex-wrap items-center gap-x-2">
        <span>已带入 {versionOf(baseContent)}的内容</span>
        <span className="text-idpxyz-breadcrumb">·</span>
        {backToCurrent}
        {backToCurrent ? <span className="text-idpxyz-breadcrumb">·</span> : null}
        {toBlank}
      </p>
    );
  }
  // 空白：是第一版，还是当前没有生效版、或人主动改成空白的，各说各的。
  const why =
    current !== null
      ? '从空白开始'
      : (known?.length ?? 0) === 0
        ? '这是第一版资料，从空白开始'
        : `${noCurrentNote ?? '当前没有生效的资料'}，从空白开始；也可以从下面带入已登记的版本`;
  return (
    <p className="flex flex-wrap items-center gap-x-2">
      <span>{why}</span>
      {backToCurrent ? (
        <>
          <span className="text-idpxyz-breadcrumb">·</span>
          {backToCurrent}
        </>
      ) : null}
    </p>
  );
}

/**
 * 查某个日期的资料（留空即此刻）。答资料不全照写原因——那是明确的非成功，开立方据此拒开，这里不拿默认值补。
 * 日期可改到未来，看登记了未来生效的那一版到时会不会接上。
 */
function CurrentProfile({
  legalEntityId,
  typeNameOf,
}: {
  legalEntityId: string;
  typeNameOf: RegistrationNumberTypeNameOf;
}) {
  const timeZone = currentDisplayTimeZone();
  const [at, setAt] = useTabViewState('legal-entity.profile-at', '');
  const [answer, setAnswer] = useState<ApiResult<LegalEntityProfileResolutionResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const instant = resolutionInstantOf(at, timeZone, new Date());
  const askable = instant !== null;

  useEffect(() => {
    // 时刻在 effect 里现取：「此刻」每次渲染都不同，放进依赖会一直重取。
    const moment = resolutionInstantOf(at, timeZone, new Date());
    if (moment === null) return;
    let cancelled = false;
    setAnswer(null);
    void resolveLegalEntityProfile(legalEntityId, moment).then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [legalEntityId, at, timeZone, reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  return (
    <div className="mt-1 flex flex-col gap-2">
      <div className="mt-3 max-w-80 rounded-md border border-idpxyz-border bg-idpxyz-sidebar p-3">
        <WallTimeField
          label="查询日期（留空即此刻）"
          path="at"
          problems={askable ? {} : { at: ['查询日期要是可解析的日期时间'] }}
          value={at}
          locked={false}
          timeZone={timeZone}
          onChange={setAt}
        />
      </div>
      {askable ? <ResolutionAnswer answer={answer} legalEntityId={legalEntityId} retry={retry} typeNameOf={typeNameOf} /> : null}
    </div>
  );
}

function ResolutionAnswer({
  answer,
  legalEntityId,
  retry,
  typeNameOf,
}: {
  answer: ApiResult<LegalEntityProfileResolutionResponseBody> | null;
  legalEntityId: string;
  retry: () => void;
  typeNameOf: RegistrationNumberTypeNameOf;
}) {
  if (answer === null) return <p className={note}>正在读取该日期的公司资料…</p>;
  if (answer.kind === 'unconfigured') {
    return (
      <p className={note}>暂时读不到公司资料：接入渠道还没有配置。这不代表资料不全；配置好之后重新打开本页即可。</p>
    );
  }
  if (isOperatorRefusal(answer)) {
    return <OperatorRefusalNote kind={answer.kind} what="公司资料按日期查询" className={note} />;
  }
  if (answer.kind === 'callerProblem') {
    return (
      <div className={note}>
        <p>调用方式问题：{problemNote(answer.code)}</p>
        <TechnicalDetails rows={httpRows(answer.status, answer.code)} />
      </div>
    );
  }
  if (answer.kind === 'noAnswer') {
    return (
      <SectionError
        title="没有得到答案"
        description={problemNote(answer.code)}
        technical={httpRows(answer.status, answer.code)}
        onRetry={retry}
      />
    );
  }
  if (answer.kind === 'transport') {
    return <SectionError title="无法连接主数据读取服务" description={answer.message} onRetry={retry} />;
  }
  const body = answer.body;
  if (body.legalEntityId !== legalEntityId) {
    return (
      <p className={note}>
        答案回显的公司主体（{body.legalEntityId}）与所问（{legalEntityId}）不符，已丢弃。
      </p>
    );
  }
  const status = legalEntityProfileStatusOf(body);
  const effective = body.effectiveRevision;
  return (
    <div className="type-body">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <StatusPill label={`资料状态：${status.label}`} tone={status.tone} />
        <span className="text-idpxyz-breadcrumb">
          查询日期 <Instant value={body.at} precision="day" />
        </span>
      </div>
      <p className="mt-1 text-idpxyz-breadcrumb">{status.description}</p>
      {effective !== undefined ? (
        <dl className="mt-2 grid grid-cols-[6rem_1fr] gap-x-3 gap-y-1">
          <dt className="text-idpxyz-breadcrumb">生效版本</dt>
          <dd>
            {profileRevisionLabel(effective.revision)} · <Instant value={effective.effectiveFrom} precision="day" /> 起生效 · 来源单据{' '}
            <span className="font-mono type-value">{effective.basis}</span>
          </dd>
        </dl>
      ) : null}
      {effective ? <LegalEntityProfileContent content={effective} typeNameOf={typeNameOf} /> : null}
    </div>
  );
}
