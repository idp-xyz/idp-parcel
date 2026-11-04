import { LegalEntityProfileContent } from './LegalEntityProfileContent';
import { FieldAnchor, LegalEntityProfileFields } from './LegalEntityProfileFields';
import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Button,
  ConfirmDialog,
  Dialog,
  DialogBody,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
} from '@idpxyz/ui-primitives';
import { isOperatorRefusal } from '../../auth/access-refusal';
import { moduleInfoById } from '../../navigation';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote } from '../../components/registration';
import { OperatorRefusalNote, SectionError, TechnicalDetails, httpRows } from '../../components/states';
import { StatusPill } from '../../components/StatusPill';
import type { ApiResult } from '../catalogue-api';
import { currentDisplayTimeZone, formatInstant, wallTimeOf } from '../moment';
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
import { registrationNumberTypeNamesOf, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { Instant } from './detail-primitives';
import {
  AdvancedGroup,
  FormGroup,
  RevisionField,
  WallTimeField,
  useRegistrationForm,
} from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import {
  compareLegalEntityProfiles,
  type ProfileChangeKind,
  type ProfileComparison,
  type ProfileFieldChange,
} from './legal-entity-profile-history';
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
import { useLegalEntityRead } from './legal-entity-reads';
import {
  draftWithNewRow,
  profileSectionBasisOf,
  profileSections,
  splitMissingBySection,
  type ProfileEditScope,
  type ProfileSection,
} from './legal-entity-profile-sections';

// 公司资料的版本历史与更新表单沿同一条不可覆盖的资料链维护。当前时点摘要由 LegalEntityProfileSummary 负责；判读、折算与载荷
// 在 legal-entity-profile.ts（纯函数，node:test 钉着），这里只摆。
//
// 更新表单住在从右侧滑出的抽屉里（票 company-subject-ux/04 第 5 项），页头「更新公司资料」与本区底部的同名按钮打开的是同一个。
// 抽屉是模态的：开着的时候换签、换行、关详情栏都点不到，所以「未保存保护」只守一处——关抽屉（叉、Esc、点遮罩）与表单里
// 站内链接要离开时，有没提交的输入先确认。
//
// 调用方按公司主体给 key：换一个主体就整块重挂，表单草稿里钉的主体标识不会停在上一个身上。

const info = moduleInfoById['group-legal-entities'];
const kind = 'legal-entity-profile';
const note = 'mt-1 text-[12px] text-idpxyz-breadcrumb';
// 资料表单收在高级组里的那一格；组里有问题时组一律展开。
const profileAdvancedPaths = ['profiles[0].revision'];

export function LegalEntityProfileSection({
  legalEntityId,
  formOpenRequest = 0,
  collapseCurrentProfile = false,
  version,
}: {
  legalEntityId: string;
  /** 身份版本（legalEntityReadVersion）；资料版本签按它与签上计数同键读（legal-entity-reads.ts）。 */
  version?: string;
  /** 调用方每要一次「直接打开更新表单」就加一（页头的主动作）；0 即没要过。 */
  formOpenRequest?: number;
  /** 对象页已有当前资料摘要时折叠按日期查询，仍保留历史与未来日期的查看入口。 */
  collapseCurrentProfile?: boolean;
}) {
  // 登记落册一次加一：按日期查询那一区（CurrentProfile，仍是页内现读）按它重取，新登的那笔立刻可见。版本历史经取数层读，
  // 写口答到后整域作废（party-writes.ts），不靠它。
  const [landed, setLanded] = useState(0);
  const [lookupVisited, setLookupVisited] = useState(false);
  // 没有登记册配置写授予的操作者不摆更新表单与打开它的按钮（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [formOpen, setFormOpen] = useState(formOpenRequest > 0);
  useEffect(() => {
    if (formOpenRequest > 0) setFormOpen(true);
  }, [formOpenRequest]);
  // 注册号类型目录读一次，给按日期查询的读出与版本历史（对比表、快照）把监管登记号类型码译成目录译名；读不到按码显。
  // 抽屉自己另读一份——它是导出件，页头也能单独挂它，不能指望挂它的人都带目录来。
  const typeNameOf = useTypeNameOf(useLoaded(listRegistrationNumberTypes));
  return (
    <div className="flex flex-col gap-5">
      <section>
        <div>
          <h4 className="text-[13px] font-semibold text-idpxyz-text">资料版本历史</h4>
          <p className="mt-1 text-xs text-idpxyz-breadcrumb">每次更新都保留一版；当前生效的那一版显示在概要。</p>
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
          <summary className="cursor-pointer text-[13px] font-medium text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:outline-idpxyz-accent">
            查看某个日期的资料
          </summary>
          {lookupVisited ? <CurrentProfile legalEntityId={legalEntityId} landed={landed} typeNameOf={typeNameOf} /> : null}
        </details>
      ) : (
        <section className="border-t border-idpxyz-border pt-4">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h4 className="text-[13px] font-semibold text-idpxyz-text">当前有效资料</h4>
              <p className="mt-1 text-xs text-idpxyz-breadcrumb">
                开票时按当时生效的那一版资料；可以提前登记未来生效的资料。
              </p>
            </div>
          </div>
          <CurrentProfile legalEntityId={legalEntityId} landed={landed} typeNameOf={typeNameOf} />
        </section>
      )}
      {mayWrite ? (
        <section className="border-t border-idpxyz-border pt-5">
          <Button className="text-idpxyz-breadcrumb" variant="outline" onClick={() => setFormOpen(true)}>
            更新公司资料
          </Button>
          <LegalEntityProfileDrawer
            open={formOpen}
            onOpenChange={setFormOpen}
            legalEntityId={legalEntityId}
            onLanded={() => setLanded((count) => count + 1)}
          />
        </section>
      ) : null}
    </div>
  );
}

/** 目录答案折成查法；没答（还在读、读不到）就是查不到任何码的空表，调用方按码显。 */
function useTypeNameOf(typeCatalogue: ApiResult<RegistrationNumberTypeListResponseBody> | null): RegistrationNumberTypeNameOf {
  return useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
}

/**
 * 更新公司资料的抽屉：受控的 open / onOpenChange，调用方（本区或页头）只管开关。用 Dialog 的 DialogContent 改成贴右侧的整高
 * 面板而不用组件库的 Drawer：那个 Drawer 定宽 24rem 且不收 className，装不下两列的资料表单；DialogContent 收 className，
 * cn 走 tailwind-merge，定位与宽度的冲突类按后者算。宽度与工作台详情栏同级。
 *
 * 关抽屉走 requestLeave：有没提交的输入先问一句，确认了才真关；表单里的站内链接要去别处同一道门。
 */
export function LegalEntityProfileDrawer({
  open,
  onOpenChange,
  legalEntityId,
  onLanded,
  scope = fullScope,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  legalEntityId: string;
  /** 登记册答已落册时调用，调用方借它重取读签。 */
  onLanded: () => void;
  /** 装整张表单，还是某一区（读出卡上的「编辑」「添加」，票 company-subject-section-edit/01）；缺省整张。 */
  scope?: ProfileEditScope;
}) {
  const [unsaved, setUnsaved] = useState(false);
  // 要离开时的去向：null 是关抽屉，字符串是表单里的站内链接要去的地址；有没提交的输入时先问一句再走。
  const [pendingLeave, setPendingLeave] = useState<{ href: string | null } | null>(null);
  const leave = (href: string | null) => {
    setPendingLeave(null);
    onOpenChange(false);
    if (href !== null) window.location.hash = href;
  };
  const requestLeave = (href: string | null) => (unsaved ? setPendingLeave({ href }) : leave(href));
  return (
    <>
      <Dialog open={open} onOpenChange={(next) => (next ? onOpenChange(true) : requestLeave(null))}>
        <DialogContent
          size="xl"
          className="left-auto right-0 top-0 flex h-full w-[min(60rem,94vw)] max-w-none translate-x-0 translate-y-0 flex-col rounded-none border-y-0 border-r-0"
          aria-label={scopeTitleOf(scope)}
        >
          {/* 关了就卸：下一次打开是一次全新的挂载，基线与目录重读，上一次的草稿不留。换了区同样重挂。 */}
          {open ? (
            <ProfileDrawerBody
              key={`${legalEntityId}|${scopeKeyOf(scope)}`}
              legalEntityId={legalEntityId}
              scope={scope}
              onLanded={onLanded}
              onUnsavedChange={setUnsaved}
              onRequestLeave={requestLeave}
            />
          ) : null}
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={pendingLeave !== null}
        tone="warning"
        title={pendingLeave?.href ? '离开这一页？' : '关闭表单？'}
        message="有内容还没有保存，离开后填写的内容不会保留。"
        confirmLabel={pendingLeave?.href ? '离开' : '放弃并关闭'}
        cancelLabel="继续填写"
        onConfirm={() => leave(pendingLeave?.href ?? null)}
        onCancel={() => setPendingLeave(null)}
      />
    </>
  );
}

const fullScope: ProfileEditScope = { kind: 'full' };

function scopeTitleOf(scope: ProfileEditScope): string {
  return scope.kind === 'full' ? '更新公司资料' : `更新${profileSections[scope.section].title}`;
}

function scopeKeyOf(scope: ProfileEditScope): string {
  return scope.kind === 'full' ? 'full' : `${scope.section}|${scope.addRow ? 'add' : 'edit'}`;
}

/**
 * 抽屉体：先把当前生效的那一版与已登记的版本清单读回来，再挂表单——表单打开即预填当前版（票面第 1 项），没有就等它。
 * 两份都打开时必现问（`fresh`），不拿缓存那份预填：缓存那份可能早于别人刚落的一版，拿旧内容保存就把那一版无声改回去。
 * 当前资料读不到时不把人堵死：整张表单留一个「改为从空白开始」；分区抽屉没有这条路——没有当前版无从「局部」改起，指路整张表单。
 */
function ProfileDrawerBody({
  legalEntityId,
  scope,
  onLanded,
  onUnsavedChange,
  onRequestLeave,
}: {
  legalEntityId: string;
  scope: ProfileEditScope;
  onLanded: () => void;
  onUnsavedChange: (unsaved: boolean) => void;
  onRequestLeave: (href: string | null) => void;
}) {
  const current = useCurrentLegalEntityProfile(legalEntityId, undefined, { fresh: true });
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  // 版本清单给版本号建议与「带入版本」；落册后由写口整域作废（party-writes.ts）再问，不另拧钥匙。
  const revisions = useLegalEntityRead('profileRevisions', legalEntityId, undefined, { fresh: true });
  const known =
    revisions.answer?.kind === 'outcome' && revisions.answer.body.legalEntityId === legalEntityId ? revisions.answer.body.revisions : null;
  const knownFailed = revisions.answer !== null && known === null;
  const [blankFallback, setBlankFallback] = useState(false);

  const answer = current.answer;
  const resolved = answer?.kind === 'outcome' && answer.body.legalEntityId === legalEntityId ? answer.body : null;
  const header = (
    <DialogHeader className="shrink-0">
      <div className="min-w-0">
        <DialogTitle>{scopeTitleOf(scope)}</DialogTitle>
        <DialogDescription className="mt-0.5 font-mono">编码 {legalEntityId}</DialogDescription>
      </div>
      <DialogClose />
    </DialogHeader>
  );
  const sectionWithoutCurrent = scope.kind === 'section' && resolved !== null && !resolved.effectiveRevision;
  if (sectionWithoutCurrent) {
    return (
      <>
        {header}
        <DialogBody className="min-h-0 flex-1 overflow-y-auto">
          <p className="text-[12px] text-idpxyz-breadcrumb">
            当前没有生效的资料（{legalEntityProfileStatusOf(resolved).label}），无从只改「{profileSections[scope.section].title}」；请用「更新公司资料」登记整版。
          </p>
        </DialogBody>
        <DialogFooter className="shrink-0">
          <Button variant="outline" onClick={() => onRequestLeave(null)}>
            关闭
          </Button>
        </DialogFooter>
      </>
    );
  }
  if (resolved === null && !blankFallback) {
    return (
      <>
        {header}
        <DialogBody className="min-h-0 flex-1 overflow-y-auto">
          {answer === null ? (
            <p className="text-[12px] text-idpxyz-breadcrumb">正在读取当前资料，表单会基于它预填…</p>
          ) : (
            <div className="flex flex-col items-start gap-3">
              <CurrentLegalEntityProfile query={current}>{() => null}</CurrentLegalEntityProfile>
              {scope.kind === 'full' ? (
                <Button variant="outline" onClick={() => setBlankFallback(true)}>
                  改为从空白开始
                </Button>
              ) : null}
            </div>
          )}
        </DialogBody>
        <DialogFooter className="shrink-0">
          <Button variant="outline" onClick={() => onRequestLeave(null)}>
            取消
          </Button>
        </DialogFooter>
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
    <>
      {header}
      <ProfileRegistrationForm
        legalEntityId={legalEntityId}
        scope={scope}
        current={currentContent}
        noCurrentNote={noCurrentNote}
        known={known}
        knownFailed={knownFailed}
        onRetryKnown={revisions.retry}
        typeCatalogue={typeCatalogue}
        onLanded={onLanded}
        onUnsavedChange={onUnsavedChange}
        onRequestLeave={onRequestLeave}
      />
    </>
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

/** 把第一个缺项滚进视野并聚焦它的控件：锚是 FieldAnchor 按路径挂的 data-field-path。 */
function scrollToField(root: HTMLElement | null, path: string): void {
  const anchor = root?.querySelector<HTMLElement>(`[data-field-path="${path}"]`);
  if (!anchor) return;
  anchor.scrollIntoView({ block: 'center' });
  anchor.querySelector<HTMLElement>('input, textarea, select')?.focus({ preventScroll: true });
}

/**
 * 登记新版本的逐字段表单。本地提示输入结构、联系人分工（legalEntityProfileLocalProblems）与必填未填
 * （legalEntityProfileMissingFields）；地址国家与身份对不对得上、登记号合不合目录、版本连不连续都由服务端答，答复原样示出。
 *
 * 保存前先看差异（票面第 2 项）：有当前生效版时点「保存」先把草稿折成资料内容与它比，列出新增 / 修改 / 移除，预览里再点
 * 「确认保存」才发；没有当前生效版（第一版）直接发。版本号有建议值，收进高级组。
 *
 * 分区（`scope` 为某一区，票 company-subject-section-edit/01）：只渲染那一区的组，其余内容照当前版原样带着；不显「基于哪一版 /
 * 带入版本」——分区只认当前版。来源单据按这次改动自动写（profileSectionBasisOf），人一改就按人的；生效时间预填打开抽屉的此刻，
 * 与版本号一起收进高级组。别区的必填缺项这张抽屉里没有那一格，列出来拦下保存、指路整张表单。
 */
function ProfileRegistrationForm({
  legalEntityId,
  scope,
  current,
  noCurrentNote,
  known,
  knownFailed,
  onRetryKnown,
  typeCatalogue,
  onLanded,
  onUnsavedChange,
  onRequestLeave,
}: {
  legalEntityId: string;
  scope: ProfileEditScope;
  /** 当前生效的那一版；没有为 null。预填默认基于它，差异预览与它比。 */
  current: ProfileContent | null;
  /** 没有当前生效版时说一句为什么（带资料状态的词）；有就 null。 */
  noCurrentNote: string | null;
  known: LegalEntityProfileRevisionRecord[] | null;
  knownFailed: boolean;
  onRetryKnown: () => void;
  typeCatalogue: ApiResult<RegistrationNumberTypeListResponseBody> | null;
  onLanded: () => void;
  onUnsavedChange: (unsaved: boolean) => void;
  onRequestLeave: (href: string | null) => void;
}) {
  const section: ProfileSection | null = scope.kind === 'section' ? profileSections[scope.section] : null;
  const timeZone = currentDisplayTimeZone();
  const [base, setBase] = useState<DraftBase>(current === null ? { kind: 'blank' } : { kind: 'current' });
  const baseContent =
    base.kind === 'current' ? current : base.kind === 'blank' ? null : (known?.find((row) => row.revision === base.revision) ?? null);
  // 未保存的基线就是打开（或改基）时预填进去的那份：只有预填、人什么都没动，不算没提交。分区时生效时间预填此刻、从「添加」
  // 打开先加一空行，都算进基线——`scope` 由页面存在状态里、引用稳定，这里不会每次渲染重算出一个新的「此刻」。
  const opening = useMemo(() => {
    const based = legalEntityProfileDraftBasedOn(legalEntityId, baseContent);
    if (section === null || scope.kind !== 'section') return { draft: based, focusPath: null as string | null };
    const timed = { ...based, effectiveFrom: wallTimeOf(new Date(), timeZone) };
    return scope.addRow ? draftWithNewRow(section, timed) : { draft: timed, focusPath: null };
  }, [legalEntityId, baseContent, section, scope, timeZone]);
  const baseDraft = opening.draft;
  const [landedCount, setLandedCount] = useState(0);
  const form = useRegistrationForm<LegalEntityProfileDraft>({
    kind,
    empty: () => baseDraft,
    suggestion: () => suggestedProfileRevision(known, legalEntityId),
    localProblems: legalEntityProfileLocalProblems,
    payloadOf: legalEntityProfilePayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded: () => {
      setLandedCount((value) => value + 1);
      onLanded();
    },
  });
  const { draft, patch, locked } = form;
  useEffect(() => {
    onUnsavedChange(form.unsaved);
  }, [onUnsavedChange, form.unsaved]);
  useEffect(() => () => onUnsavedChange(false), [onUnsavedChange]);
  const bodyRef = useRef<HTMLDivElement>(null);
  // 从「添加」打开：新加的那一空行滚进视野、聚焦它的第一个格。
  useEffect(() => {
    if (opening.focusPath !== null) scrollToField(bodyRef.current, opening.focusPath);
  }, [opening.focusPath]);
  const typeNameOf = useTypeNameOf(typeCatalogue);

  // 分区的来源单据：没手改时按这次改动自动写；手改后按人的，可复位。送出与缺项判读都看「将要送的」那份。
  const [basisEdited, setBasisEdited] = useState(false);
  const autoBasis =
    section !== null && current !== null
      ? profileSectionBasisOf(section, compareLegalEntityProfiles(current, profileComparisonContentOf(form.effectiveDraft, timeZone), typeNameOf))
      : null;
  const useAutoBasis = autoBasis !== null && !basisEdited;
  const sendingDraft = useAutoBasis ? { ...form.effectiveDraft, basis: autoBasis } : form.effectiveDraft;

  const switchBase = (next: DraftBase, content: ProfileContent | null) => {
    setBase(next);
    patch(profileDraftContentOf(content ?? blankContent));
  };

  const [sourceRevision, setSourceRevision] = useState('');
  const source = known?.find((row) => String(row.revision) === sourceRevision) ?? null;

  // 必填未填只在点过保存之后才显在格下与按钮旁；之后随输入实时更新，填上一格少一条。分区时别区的缺项另列（outsideMissing）。
  const [attempted, setAttempted] = useState(false);
  const allMissing = legalEntityProfileMissingFields(sendingDraft);
  const { inside: missing, outside: outsideMissing } =
    section !== null ? splitMissingBySection(section, allMissing) : { inside: allMissing, outside: [] };
  const problems = attempted ? problemsWithMissing(form.problems, missing) : form.problems;
  const [preview, setPreview] = useState<ProfileComparison | null>(null);
  const save = () => {
    if (missing.length > 0) {
      setAttempted(true);
      scrollToField(bodyRef.current, missing[0].path);
      return;
    }
    if (current === null) {
      send();
      return;
    }
    setPreview(compareLegalEntityProfiles(current, profileComparisonContentOf(sendingDraft, timeZone), typeNameOf));
  };
  // 送的是带自动依据的那份；落册簿记（landedKey）仍按草稿本身记，之后人什么都没动就不算没提交。
  const send = () =>
    form.sendVia((sending, submitDraft) =>
      submitDraft(legalEntityProfilePayloadOf(useAutoBasis ? { ...sending, basis: autoBasis } : sending, timeZone), sending),
    );

  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedPaths = section !== null ? [...profileAdvancedPaths, 'profiles[0].effectiveFrom'] : profileAdvancedPaths;
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, false);
  const revisionSummary = `版本号 ${form.revisionField.value.trim() || '未填'}${form.revisionField.edited ? '（手填）' : '（建议值）'}`;
  const advancedSummary =
    section !== null ? `生效时间 ${draft.effectiveFrom.trim().replace('T', ' ') || '未填'} · ${revisionSummary}` : revisionSummary;
  const landedNow = landedCount > 0 && !form.unsaved;
  const knownOptions = [...(known ?? [])].sort((a, b) => b.revision - a.revision);

  return (
    <>
      <DialogBody className="min-h-0 flex-1 overflow-y-auto">
        <div ref={bodyRef}>
          {preview !== null && current !== null ? (
            <ChangePreview
              comparison={preview}
              current={current}
              nextRevision={form.effectiveDraft.revision}
              effectiveFrom={draft.effectiveFrom}
              timeZone={timeZone}
              basis={section !== null ? sendingDraft.basis : undefined}
            />
          ) : (
            <div className="flex flex-col gap-5">
              {section !== null && current !== null ? (
                <p className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 text-[12px] leading-5 text-idpxyz-text">
                  基于当前生效的 <span className="font-medium">{profileRevisionLabel(current.revision)}</span> 只改「{section.title}」，其余内容原样带着；
                  保存后登记为新的一版。
                </p>
              ) : (
              <div className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 text-[12px] leading-5 text-idpxyz-text">
                <BaseLine base={base} current={current} baseContent={baseContent} known={known} noCurrentNote={noCurrentNote} locked={locked} onSwitch={switchBase} />
                {knownOptions.length > 0 || knownFailed ? (
                  <div className="mt-1.5 flex flex-wrap items-center gap-2 border-t border-idpxyz-border pt-1.5 text-idpxyz-breadcrumb">
                    <label htmlFor={`profile-source-${legalEntityId}`}>从已登记版本带入</label>
                    <select
                      id={`profile-source-${legalEntityId}`}
                      disabled={locked || known === null}
                      value={sourceRevision}
                      onChange={(event) => setSourceRevision(event.target.value)}
                      className="h-7 min-w-0 max-w-full rounded-md border border-idpxyz-border bg-idpxyz-card px-2 text-[12px] text-idpxyz-text"
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
                      className="text-[12px]"
                      disabled={locked || source === null}
                      onClick={() => {
                        if (source) switchBase({ kind: 'revision', revision: source.revision }, source);
                      }}
                    >
                      带入此版本
                    </Button>
                    {knownFailed ? (
                      <Button variant="outline" size="sm" className="text-[12px]" disabled={locked} onClick={onRetryKnown}>
                        重试读取版本
                      </Button>
                    ) : null}
                  </div>
                ) : null}
              </div>
              )}

              {section === null ? (
                <ul className="list-disc pl-4 text-xs leading-5 text-idpxyz-breadcrumb">
                  <li>每次更新都留一版，新的一版从它的生效时间起取代上一版；可以提前登记未来生效的资料。</li>
                  <li>注册地址的国家 / 地区须与身份上的注册国家 / 地区一致。</li>
                  <li>没填开票抬头的那一版生效后，开票会因「资料不全」被拒。</li>
                </ul>
              ) : null}

              <LegalEntityProfileFields
                draft={draft}
                patch={patch}
                problems={problems}
                locked={locked}
                typeCatalogue={typeCatalogue}
                onLeave={onRequestLeave}
                groups={section?.groups}
              />

              {section !== null ? (
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
              ) : (
                <FormGroup title="这次更新" description="依据哪份单据、从什么时候起生效；生效时间可以在未来。">
                  <FieldAnchor path="profiles[0].basis">
                    <Field label="来源单据 *" path="profiles[0].basis" problems={problems}>
                      <Input
                        value={draft.basis}
                        readOnly={locked}
                        className="font-mono text-[13px]"
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
              )}

              <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
                {section !== null ? (
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
                ) : null}
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
      </DialogBody>
      <DialogFooter className="shrink-0 justify-between gap-4">
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
          <RegistrationAnswerNote
            state={form.state}
            owner={info.owner}
            outcomeLabels={legalEntityProfileOutcomeLabels}
            problemNote={problemNote}
          />
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
                  send();
                }}
              >
                确认保存
              </Button>
            </>
          ) : landedNow ? (
            <Button variant="default" className={primaryActionClass} onClick={() => onRequestLeave(null)}>
              关闭
            </Button>
          ) : (
            <>
              <Button variant="outline" disabled={locked} onClick={() => onRequestLeave(null)}>
                取消
              </Button>
              <Button
                variant="default"
                className={primaryActionClass}
                onClick={save}
                disabled={locked || known === null || !form.canSend || outsideMissing.length > 0}
              >
                {locked ? '保存中…' : '保存'}
              </Button>
            </>
          )}
        </div>
      </DialogFooter>
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

const changeKindLabels: Record<ProfileChangeKind, string> = { added: '新增', modified: '修改', removed: '移除', unchanged: '未变化' };
const changeKindClasses: Record<ProfileChangeKind, string> = {
  added: 'text-emerald-700 dark:text-emerald-300',
  modified: 'text-idpxyz-accent',
  removed: 'text-idpxyz-danger',
  unchanged: 'text-idpxyz-breadcrumb',
};

/**
 * 保存前的「这次改了什么」（票面第 2 项）：与当前生效版比，只列有变化的项；空白项移除旧值就是一行「移除」，不靠说明文字。
 * 比较的是将要送上去的内容（profileComparisonContentOf），来源单据与生效时间不在比较里、另起一句说。
 */
function ChangePreview({
  comparison,
  current,
  nextRevision,
  effectiveFrom,
  timeZone,
  basis,
}: {
  comparison: ProfileComparison;
  current: ProfileContent;
  nextRevision: string;
  /** datetime-local 原值，按操作者时区读；送出时才换 RFC 3339。 */
  effectiveFrom: string;
  timeZone: string;
  /** 分区保存时把将要送的来源单据也复述一遍——它多半是自动写的，人在确认前该看到。 */
  basis?: string;
}) {
  const changed = comparison.fields.filter((field) => field.kind !== 'unchanged');
  return (
    <div className="flex flex-col gap-3" aria-label="保存前差异预览">
      <div>
        <h3 className="text-[14px] font-semibold text-idpxyz-textBright">这次改了什么</h3>
        <p className="mt-1 text-[12px] leading-5 text-idpxyz-breadcrumb">
          与当前生效的{profileRevisionLabel(current.revision)}相比。确认后登记为第 {nextRevision.trim() || '?'} 版，{effectiveFrom.replace('T', ' ')}（{timeZone}）起生效；
          版本连不连续、国家对不对得上由服务端判。
          {basis !== undefined ? <>来源单据：<span className="font-mono text-idpxyz-text">{basis}</span>。</> : null}
        </p>
        {comparison.contactMatchingLimited ? (
          <p role="note" className="mt-1 text-[12px] leading-5 text-idpxyz-breadcrumb">
            资料里没有联系人的稳定标识：无法唯一对应的联系人按移除和新增列出，不推测为同一人。
          </p>
        ) : null}
      </div>
      {changed.length === 0 ? (
        <p role="status" className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2 text-[12px] text-idpxyz-text">
          资料内容与{profileRevisionLabel(current.revision)}完全相同。确认后仍会登记为新的一版，只是来源单据与生效时间换成这一次的。
        </p>
      ) : (
        <div className="overflow-x-auto rounded-md border border-idpxyz-border">
          <table className="w-full min-w-[32rem] text-left text-[12px]">
            <thead className="bg-idpxyz-sidebar text-idpxyz-breadcrumb">
              <tr>
                <th className="px-3 py-2 font-medium">项目</th>
                <th className="px-3 py-2 font-medium">{profileRevisionLabel(current.revision)}</th>
                <th className="px-3 py-2 font-medium">这一次</th>
              </tr>
            </thead>
            <tbody>
              {changed.map((field) => (
                <ChangeRow key={field.id} field={field} />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function ChangeRow({ field }: { field: ProfileFieldChange }) {
  const label = field.contactKey ? `${field.subject}${field.detail ? `（${field.detail}）` : ''} · ${field.field}` : field.field;
  return (
    <tr className="border-t border-idpxyz-border align-top">
      <td className="min-w-28 px-3 py-2">
        <span className={`mr-1.5 font-medium ${changeKindClasses[field.kind]}`}>{changeKindLabels[field.kind]}</span>
        <span className="text-idpxyz-text">{label}</span>
      </td>
      <td className={`max-w-64 whitespace-pre-wrap px-3 py-2 [overflow-wrap:anywhere] ${field.before ? 'line-through decoration-idpxyz-border text-idpxyz-breadcrumb' : 'text-idpxyz-breadcrumb'}`}>
        {field.before || '—'}
      </td>
      <td className={`max-w-64 whitespace-pre-wrap px-3 py-2 [overflow-wrap:anywhere] ${field.after ? 'font-medium text-idpxyz-text' : 'text-idpxyz-breadcrumb'}`}>
        {field.after || '—'}
      </td>
    </tr>
  );
}

/**
 * 查某个日期的资料（留空即此刻）。答资料不全照写原因——那是明确的非成功，开立方据此拒开，这里不拿默认值补。
 * 日期可改到未来，看登记了未来生效的那一版到时会不会接上。
 */
function CurrentProfile({
  legalEntityId,
  landed,
  typeNameOf,
}: {
  legalEntityId: string;
  landed: number;
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
  }, [legalEntityId, at, timeZone, landed, reloadKey]);

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
    <div className="text-[12px]">
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
            <span className="font-mono text-[13px]">{effective.basis}</span>
          </dd>
        </dl>
      ) : null}
      {effective ? <LegalEntityProfileContent content={effective} typeNameOf={typeNameOf} /> : null}
    </div>
  );
}
