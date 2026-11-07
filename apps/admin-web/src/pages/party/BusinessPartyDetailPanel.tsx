import { useState } from 'react';
import { FileText, Handshake, Info, Maximize2, X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger, ConfirmDialog } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { WorkspaceTabLabel } from '../../templates/workspace-tab-icons';
import { useTabViewState } from '../../templates/workspace-view-state';
import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyRecord, PartyRelationshipListResponseBody, BusinessPartyListResponseBody } from './api';
import { businessPartyPanelTabsOf, shownBusinessPartyPanelTab, type BusinessPartyPanelTab } from './business-party-list';
import { businessPartyObjectHash, profileEditHash } from './business-parties-address';
import {
  BusinessPartyContactsReadout,
  BusinessPartyProfileReadout,
  BusinessPartyProfileVersions,
  effectiveProfileOf,
  ourCompanyProfileNote,
  useBusinessPartyProfiles,
} from './BusinessPartyProfileSection';
import { businessPartyProfileVersionFact, otherLanguageNameOf } from './business-party-profile';
import type { AnswerQuery } from '../../query/answers';
import type { BusinessPartyProfileRevisionListResponseBody } from './api';
import { updateOrdinalLabel } from './presentation';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { BusinessPartyBanners, BusinessPartyMoreMenu, BusinessPartyTypeTags } from './BusinessPartyHeaderParts';
import { useMayWriteRegistry } from '../use-operator-grants';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { PartyOverviewSection, partyOverviewSectionTitles, usePartyOverviewStates, type PartyOverviewStates } from './PartyOverviewSection';
import { countText } from './party-overview';
import { liveRelationshipCountOf, panelHeaderActionsOf, panelHintOf } from './business-party-panel-first-screen';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import { businessPartyRevisionHistory } from './BusinessPartyDetailPage';
import { RevisionHistorySection } from './RevisionHistorySection';
import { DetailRow, Fact, FactStrip, Instant } from './detail-primitives';
import { BusinessPartyIdentificationFacts } from './BusinessPartyIdentificationRows';
import { CopyAside, RefText } from './copy-affordance';

const tabTriggerClass = 'px-0 type-body font-medium text-idpxyz-breadcrumb data-[state=active]:text-idpxyz-accent';

/** 资料三签的正文；修订链由栏顶读一次，抬头的英文名与「资料」格用的是同一份。 */
function PanelProfilePart({
  row,
  pinned,
  part,
  profiles,
  roles,
  onUpdate,
}: {
  row: BusinessPartyRecord;
  pinned: boolean | null;
  part: 'summary' | 'contacts' | 'versions';
  profiles: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>;
  roles: readonly string[];
  onUpdate?: () => void;
}) {
  if (pinned === true) return <p className="type-value leading-6 text-idpxyz-breadcrumb">{ourCompanyProfileNote}</p>;
  if (part === 'contacts') return <BusinessPartyContactsReadout partyId={row.partyId} query={profiles} onUpdate={onUpdate} />;
  if (part === 'versions') return <BusinessPartyProfileVersions partyId={row.partyId} query={profiles} onUpdate={onUpdate} />;
  return (
    <BusinessPartyProfileReadout
      partyId={row.partyId}
      query={profiles}
      registrationCountry={row.identityLayerRegistered ? row.registrationCountry : undefined}
      roles={roles}
      onUpdate={onUpdate}
    />
  );
}

/**
 * 业务参与方工作台身份册的详情栏（票 business-parties-uiux/01）：形态照 LegalEntityDetailPanel——头部（名称 + 状态徽章 +
 * 其他语言名称一行 + 角色标签）· 主动作 +「更多」菜单 + 打开对象页 + 关闭 · meta 行 · 停用横幅 · 随状态的提示 · 一行事实 ·
 * 关联计数一行 · 签。主动作随状态（票 business-party-ux/08）：没有在用关系的单位是「登记合作关系」，其余是「更新资料」；
 * 判读在 business-party-panel-first-screen.ts。
 *
 * 一个事实在首屏只出现一处（票 group-legal-entities-uiux/06）：生效、停用、登记版次与资料版本归那一行事实，只显到日，照公司
 * 主体栏（票 business-party-ux/01）；登记时刻在「登记记录」签；名称是标题，编码与识别信息在 meta 行；标识、修订号、登记依据、
 * 租户这类排查用的字收进「登记记录」签底部的「技术详情」。
 *
 * 签照公司主体栏（票 business-party-profile/04）：「概要」（默认）读出往来单位资料，「联系人」「资料版本」读同一份资料修订链，
 * 「登记记录」（原称身份与修订、变更记录）装剩下的格与身份修订历史，「关联」（原称全景）回答这个参与方对我们是什么。我方公司
 * 不读也不摆资料，判据在 business-party-list.ts 的 businessPartyPanelTabsOf。资料修订链在栏顶读：抬头的英文名与「资料」格要它，
 * 与公司主体栏读当前资料同一做法。
 * 去过的签留着挂载（隐藏而不卸载）：每切回来一次就重取一次会闪一次加载态；只留「去过的」，方向键逐行翻时不为没看的签取数。
 * 全景那四册是例外：头区的计数行与提示要它们，栏一开就读（usePartyOverviewStates），「关联」签去不去过都是同一份缓存。
 * 签不按参与方重置：↑/↓ 逐行翻时停在哪一签就一直看哪一签，与法人栏同一做法。
 */
export function BusinessPartyDetailPanel({
  row,
  roles,
  ours,
  pinned,
  relationships,
  retryRelationships,
  onClose,
  onOpenObjectPage,
  parties,
  onChanged,
}: {
  row: BusinessPartyRecord;
  /** 这个参与方持有的角色（business-party-list.ts 的 indexPartyRoles：在用关系，加上未停用客户账户的所属客户），头部按它称呼；空即「暂无合作关系」。 */
  roles: readonly string[];
  /** 是不是法人册上钉着的我方公司；与列表类型格同一判据（business-party-list.ts 的 businessPartyTypeLabel）。 */
  ours: boolean;
  /**
   * 法人册上有没有公司主体钉着它，已停用的也算（business-party-list.ts 的 partiesPinnedByLegalEntities），与对象页收资料入口
   * 同一判据；null 是法人册还没答出。头部的「我方公司」只数在用的（ours），两者用途不同。
   */
  pinned: boolean | null;
  /** 关系册答案随页面持有、交给全景签，与关系册工作台同一份，不另取。 */
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
  onClose: () => void;
  onOpenObjectPage: () => void;
  /** 页面持有的参与方列表答案，给「更多 › 停用」的对话框与「登记合作关系」的双方候选。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 停用落册后回调；列表页借它重取参与方册。 */
  onChanged: () => void;
}) {
  const [tab, setTab] = useTabViewState<BusinessPartyPanelTab>('business-party.panel-tab', 'summary');
  const [visited, setVisited] = useState<ReadonlySet<BusinessPartyPanelTab>>(() => new Set<BusinessPartyPanelTab>([tab]));
  const tabs = businessPartyPanelTabsOf(pinned);
  const shown = shownBusinessPartyPanelTab(tab, tabs);
  // 我方公司不读，法人册答出之前也先不读——同对象页，免得先发一笔再认出是我方公司。
  const profiles = useBusinessPartyProfiles(row.partyId, false, pinned === false);
  const otherName = otherLanguageNameOf(effectiveProfileOf(profiles, row.partyId).record);
  // 没有登记册配置写授予的操作者不摆登记入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const { addToast } = useToast();
  // 全景签「合作关系」块就地登记（票 business-party-edit-mode/03）。栏不按行重挂：换了行就不算这一份。
  const [registering, setRegistering] = useState<string | null>(null);
  const [relationshipUnsaved, setRelationshipUnsaved] = useState(false);
  const [confirmingCancel, setConfirmingCancel] = useState(false);
  const knownRelationships = relationships?.kind === 'outcome' ? relationships.body.relationships : null;
  const relationshipRegistration = mayWrite
    ? {
        active: registering === row.partyId,
        onOpen: () => setRegistering(row.partyId),
        form: (
          <PartyRelationshipRegistrationForm
            key={row.partyId}
            knownRelationships={knownRelationships}
            parties={parties}
            preset={{ holder: row.partyId }}
            onRegistered={() => {
              addToast({ type: 'success', title: '合作关系已登记' });
              setRelationshipUnsaved(false);
              setRegistering(null);
              retryRelationships();
            }}
            onUnsavedChange={setRelationshipUnsaved}
            inline={{ onCancel: () => (relationshipUnsaved ? setConfirmingCancel(true) : setRegistering(null)) }}
          />
        ),
      }
    : undefined;
  const selectTab = (next: BusinessPartyPanelTab) => {
    setTab(next);
    setVisited((previous) => (previous.has(next) ? previous : new Set(previous).add(next)));
  };
  // 整份「更新资料」是对象页的编辑态，照公司主体栏：带编辑键去对象页，栏里不开表单。
  const openProfileForm = () => {
    window.location.hash = profileEditHash(businessPartyObjectHash(row.partyId), true);
  };
  // 首屏那一句提示与头区动作（票 business-party-ux/08）：判读在 business-party-panel-first-screen.ts。各段计数与「关联」签读同一份缓存。
  const overview = usePartyOverviewStates(row.partyId, relationships);
  const liveRelationships = overview.relationships.kind === 'answered' ? liveRelationshipCountOf(overview.relationships.rows) : null;
  const hint = panelHintOf(row, ours, liveRelationships, roles.length);
  const actions = panelHeaderActionsOf(mayWrite, pinned, hint);
  // 「登记合作关系」就是「关联」签里那张内联表单：切到签、把表单打开，不另造一处写口。
  const openRelationshipForm = () => {
    selectTab('overview');
    setRegistering(row.partyId);
  };
  const countsLineLabel = (Object.keys(partyOverviewSectionTitles) as (keyof PartyOverviewStates)[])
    .map((key) => `${partyOverviewSectionTitles[key]} ${countText(overview[key])}`)
    .join('，');

  return (
    <>
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`业务参与方 ${row.partyId}`}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate type-title font-semibold text-idpxyz-textBright">{row.partyName}</h2>
              <StatusPill code={row.status} />
            </div>
            {/* 对外单据与海外往来认的写法（多半是英文名）紧跟在名称下面；文本类，悬停复制（票 company-subject-ux/14）。 */}
            {otherName ? (
              <p className="mt-0.5 truncate type-body text-idpxyz-breadcrumb">
                <CopyAside value={otherName} name="其他语言名称">
                  {otherName}
                </CopyAside>
              </p>
            ) : null}
            {/* 类型与列表类型格同一份标签（BusinessPartyTypeTags）：我方公司排在角色前面；两样都没有时说「暂无合作关系」，不留空。 */}
            <div className="mt-1">
              <BusinessPartyTypeTags ours={ours} roles={roles} />
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {/* 动作随状态（panelHeaderActionsOf）：没有在用关系的单位下一步是登关系，「更新资料」退成描边；其余照旧。 */}
            {actions.registerRelationship ? (
              <Button size="sm" variant="default" className={`${primaryActionClass} type-body`} onClick={openRelationshipForm}>
                <Handshake className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                登记合作关系
              </Button>
            ) : null}
            {actions.updateProfile === 'outline' ? (
              <Button size="sm" variant="outline" className="type-body text-idpxyz-breadcrumb" onClick={openProfileForm}>
                更新资料
              </Button>
            ) : null}
            {actions.updateProfile === 'primary' ? (
              <Button size="sm" variant="default" className={`${primaryActionClass} type-body`} onClick={openProfileForm}>
                <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                更新资料
              </Button>
            ) : null}
            <BusinessPartyMoreMenu row={row} parties={parties} onOpenObjectPage={onOpenObjectPage} onChanged={onChanged} />
            <Button
              className="text-idpxyz-breadcrumb"
              variant="ghost"
              size="icon"
              onClick={onOpenObjectPage}
              aria-label="在新标签打开对象页"
              title="在新标签打开对象页"
            >
              <Maximize2 className="h-4 w-4" aria-hidden="true" />
            </Button>
            <Button
              className="text-idpxyz-breadcrumb"
              variant="ghost"
              size="icon"
              onClick={onClose}
              aria-label="关闭详情"
              title="关闭（Esc）"
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>

        {/* 头区 meta 行照公司主体右栏（票 company-subject-ux/08）：编码与识别信息（ADR-0367）一行小字，号与码点击即复制。
            头区一直可见，所以识别信息只在这里出现一次，「登记记录」签不再列它。 */}
        <dl className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1">
          <Fact inline label="编码">
            <RefText value={row.partyId} name="编码" />
          </Fact>
          <BusinessPartyIdentificationFacts record={row} />
        </dl>

        <BusinessPartyBanners row={row} />

        {/* 随状态的提示，`info` 语气、照票 02 空状态那种「给下一步」的写法；已停用由上面的横幅说，这里不再说一遍。 */}
        {hint ? (
          <div className={`flex items-start gap-2 rounded border px-2.5 py-1.5 type-body ${statusToneBanner.info}`}>
            <Info className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${statusToneText.info}`} aria-hidden="true" />
            {hint.kind === 'pending' ? (
              <span>还没到生效时间：在那之前不按它办新业务；登记合作关系时，开始时间不能早于它的生效时间。</span>
            ) : (
              <span className="flex flex-wrap items-center gap-x-2">
                <span>还没有合作关系。</span>
                {mayWrite ? (
                  <button type="button" className="text-idpxyz-accent hover:underline" onClick={openRelationshipForm}>
                    登记合作关系
                  </button>
                ) : null}
              </span>
            )}
          </div>
        ) : null}
      </div>

      <FactStrip inline>
        <Fact inline label="生效">
          <Instant value={row.effectiveFrom} precision="day" />
        </Fact>
        {row.deactivatedAt ? (
          <Fact inline label="停用">
            <Instant value={row.deactivatedAt} precision="day" />
          </Fact>
        ) : null}
        <Fact inline label="登记">
          {updateOrdinalLabel(row.revision)}
        </Fact>
        {pinned === true ? null : (
          <Fact inline label="资料">
            {businessPartyProfileVersionFact(profiles.answer, row.partyId)}
          </Fact>
        )}
      </FactStrip>

      {/* 关联计数一行（票 business-party-ux/08）：让人在首屏知道「关联」签里有什么，点了切过去；哪一册没答出那一格显「—」
          （countText），不拿 0 冒充「没有」。签里不再摆那行计数（summaryCounts），一个事实首屏只出现一处。 */}
      <button
        type="button"
        onClick={() => selectTab('overview')}
        aria-label={`查看关联：${countsLineLabel}`}
        className="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 border-b border-idpxyz-border px-5 py-1.5 text-left type-body text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-text focus-visible:outline focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-idpxyz-accent"
      >
        {(Object.keys(partyOverviewSectionTitles) as (keyof PartyOverviewStates)[]).map((key, index) => (
          <span key={key} className="flex items-center gap-x-1.5">
            {index > 0 ? <span aria-hidden="true">·</span> : null}
            <span>
              {partyOverviewSectionTitles[key]} <span className="font-mono tabular-nums text-idpxyz-text">{countText(overview[key])}</span>
            </span>
          </span>
        ))}
      </button>

      <Tabs value={shown} onValueChange={(value) => selectTab(value as BusinessPartyPanelTab)} className="flex min-h-0 flex-1 flex-col gap-0">
        {/* 签名与图标取对象工作区签的固定表（票 company-subject-ux/10），与对象页同名（票 business-parties-object-page/01）：
            「关联」是这家单位和我们有哪些往来，「登记记录」是身份修订链——同一种内容同一个词、同一个图标。 */}
        <TabsList className="mx-5 mt-2 h-9 w-auto shrink-0 gap-5">
          <TabsTrigger value="summary" className={tabTriggerClass}>
            <WorkspaceTabLabel id="summary" />
          </TabsTrigger>
          {tabs.includes('contacts') ? (
            <TabsTrigger value="contacts" className={tabTriggerClass}>
              <WorkspaceTabLabel id="contacts" />
            </TabsTrigger>
          ) : null}
          {tabs.includes('profile') ? (
            <TabsTrigger value="profile" className={tabTriggerClass}>
              <WorkspaceTabLabel id="profile-versions" />
            </TabsTrigger>
          ) : null}
          <TabsTrigger value="identity" className={tabTriggerClass}>
            <WorkspaceTabLabel id="identity-revisions" />
          </TabsTrigger>
          <TabsTrigger value="overview" className={tabTriggerClass}>
            <WorkspaceTabLabel id="related" />
          </TabsTrigger>
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          <TabsContent value="summary" forceMount={visited.has('summary') || undefined} className="mt-0 data-[state=inactive]:hidden">
            <PanelProfilePart row={row} pinned={pinned} profiles={profiles} roles={roles} onUpdate={mayWrite ? openProfileForm : undefined} part="summary" />
          </TabsContent>
          {tabs.includes('contacts') ? (
            <TabsContent value="contacts" forceMount={visited.has('contacts') || undefined} className="mt-0 data-[state=inactive]:hidden">
              <PanelProfilePart row={row} pinned={pinned} profiles={profiles} roles={roles} onUpdate={mayWrite ? openProfileForm : undefined} part="contacts" />
            </TabsContent>
          ) : null}
          {tabs.includes('profile') ? (
            <TabsContent value="profile" forceMount={visited.has('profile') || undefined} className="mt-0 data-[state=inactive]:hidden">
              <PanelProfilePart row={row} pinned={pinned} profiles={profiles} roles={roles} onUpdate={mayWrite ? openProfileForm : undefined} part="versions" />
            </TabsContent>
          ) : null}
          <TabsContent
            value="overview"
            forceMount={visited.has('overview') || undefined}
            className="mt-0 data-[state=inactive]:hidden"
          >
            <PartyOverviewSection
              partyId={row.partyId}
              relationships={relationships}
              retryRelationships={retryRelationships}
              relationshipRegistration={relationshipRegistration}
              onRelationshipEnded={mayWrite ? retryRelationships : undefined}
              accountWrites={mayWrite ? { parties } : undefined}
              summaryCounts={false}
            />
          </TabsContent>
          <TabsContent
            value="identity"
            forceMount={visited.has('identity') || undefined}
            className="mt-0 data-[state=inactive]:hidden"
          >
            {row.deactivationBasis ? (
              <dl>
                <DetailRow label="停用的来源单据" mono>
                  {row.deactivationBasis}
                </DetailRow>
              </dl>
            ) : null}
            <section className={row.deactivationBasis ? 'mt-4' : ''}>
              <h3 className="type-body font-medium text-idpxyz-text">历次更新</h3>
              <RevisionHistorySection register={businessPartyRevisionHistory} subjectId={row.partyId} revision={row.revision} />
            </section>
            <TechnicalDetails
              className="mt-4"
              rows={[
                { label: '参与方标识', value: row.partyId },
                { label: '当前修订号', value: String(row.revision) },
                { label: '登记依据', value: row.basis },
                { label: '停用依据', value: row.deactivationBasis ?? '' },
                { label: '租户', value: row.tenantId },
              ]}
            />
          </TabsContent>
        </div>
      </Tabs>
    </div>
    <ConfirmDialog
      renderFooter={confirmDialogFooter}
      open={confirmingCancel}
      tone="warning"
      title="放弃修改？"
      message="有内容还没有保存，离开后填写的内容不会保留。"
      confirmLabel="放弃"
      cancelLabel="继续编辑"
      onConfirm={() => {
        setConfirmingCancel(false);
        setRegistering(null);
      }}
      onCancel={() => setConfirmingCancel(false)}
    />
    </>
  );
}
