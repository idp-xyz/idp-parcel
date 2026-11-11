import { useCallback, useState } from 'react';
import { ClipboardList, FileText, Handshake, Maximize2, X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger, ConfirmDialog } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { WorkspaceTabLabel } from '../../templates/workspace-tab-icons';
import { useTabViewState } from '../../templates/workspace-view-state';
import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyRecord, PartyRelationshipListResponseBody, BusinessPartyListResponseBody } from './api';
import { businessPartyPanelTabsOf, shownBusinessPartyPanelTab, type BusinessPartyPanelTab } from './business-party-list';
import { businessPartyObjectHash, identityEditHash, profileEditHash } from './business-parties-address';
import { markEditFromRoster, type ProfileEditSectionId } from './business-party-edit-origin';
import { effectiveProfileOf, useBusinessPartyProfiles } from './BusinessPartyProfileSection';
import { businessPartyProfileVersionFact, otherLanguageNameOf } from './business-party-profile';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { BusinessPartyMoreMenu, BusinessPartyTypeTags } from './BusinessPartyHeaderParts';
import { useMayWriteRegistry } from '../use-operator-grants';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { partyOverviewStatesOf, usePartyOverviewLists } from './PartyOverviewSection';
import { liveRelationshipCountOf, panelHeaderActionsOf, panelHintOf } from './business-party-panel-first-screen';
import { panelProfileTabCountsOf, panelRelatedCountOf, type PanelRelatedTarget } from './business-party-panel-content';
import { businessPartyRevisionHistory } from './BusinessPartyDetailPage';
import { RevisionHistorySection } from './RevisionHistorySection';
import { DetailRow, Fact, Instant } from './detail-primitives';
import { formatCount, partyTileKindOf, type PartyTileKind } from './business-parties-workbench';
import { PartyTile } from './party-role-visuals';
import { BusinessPartyIdentificationFacts } from './BusinessPartyIdentificationRows';
import { CopyAside, RefText } from './copy-affordance';
import { PanelBand } from './BusinessPartyPanelParts';
import { BusinessPartyPanelRelated } from './BusinessPartyPanelRelated';
import { BusinessPartyPanelContacts, BusinessPartyPanelSummary, BusinessPartyPanelVersions } from './BusinessPartyPanelSummary';

// 签条：字与图标同色，选中的换成亮字；下划线压在签条的底线上、两端不收圆。不加粗——加粗在中文上落不到字形，只把数字与拉丁字母加粗。
const tabTriggerClass =
  'px-2.5 type-value font-normal text-idpxyz-breadcrumb data-[state=active]:text-idpxyz-textBright data-[state=active]:after:-bottom-px data-[state=active]:after:rounded-none';

// 各签自己滚：卡片式的签垫一层底色、留小留白，通栏的签贴边。两种都是签内容的容器，卡片两列与关系表的列数按它的宽度定
// （business-party-panel.css）；data-panel-scroller 给「落到某一段」找滚动区用。
const tabBodyClass = 'business-party-panel-body min-h-0 flex-1 overflow-y-auto data-[state=inactive]:hidden';
const cardTabClass = `${tabBodyClass} bg-idpxyz-editor p-4`;
const flushTabClass = `${tabBodyClass} bg-idpxyz-bg`;

/** 签上的数：读到了才摆，0 也摆——「一条都没有」与「还没读到」要分得开。底色随签的选中态换。 */
function TabCount({ value }: { value: number | null }) {
  if (value === null) return null;
  return (
    <span className="rounded-full bg-idpxyz-hover px-1.5 type-caption leading-[18px] tabular-nums text-idpxyz-breadcrumb group-data-[state=active]:bg-idpxyz-activeItem group-data-[state=active]:text-idpxyz-textBright">
      {formatCount(value)}
    </span>
  );
}

/**
 * 业务参与方工作台身份册的详情栏（票 business-parties-uiux/01；头区的排法自票 business-party-redesign/03，各签的内容自同册
 * 票 05）：头区第一排是图标块（与列表行同一套，按类型取）· 名称 + 状态徽章 + 其他语言名称一行 + 类型标签 · 动作；下面一行
 * 事实，再下面就是签。动作的位置固定——更新资料、登记合作关系、打开对象页、「更多」、关闭——人记的是位置；谁是主动作随提示
 * （票 business-party-ux/08）：提示是「还没有合作关系」时是「登记合作关系」，其余是「更新资料」，判读在
 * business-party-panel-first-screen.ts。栏窄到名称与动作排不成一行时动作整组折到名称下面、仍靠右，名称最多两行。
 *
 * 一个事实在首屏只出现一处（票 group-legal-entities-uiux/06）：编码、识别信息、生效日与资料版本归那一行事实，只显到日；
 * 停用的时点与随状态的提示在「概要」签顶上，不在事实行再摆一格；名称是标题；登记时刻、登记版次与修订历史在「登记记录」签，
 * 标识、修订号、登记依据、租户这类排查用的字收进那一签底部的「技术详情」。各册有几条不在头区另摆一行：签上有数，「概要」的
 * 卡里有分册的数。
 *
 * 签照公司主体栏（票 business-party-profile/04）：「概要」（默认）是这家单位的资料与它跟别人的关系，「联系人」「资料版本」读
 * 同一份资料修订链，「登记记录」装身份修订历史，「关联」是合作关系与另外四册里与它有关的行。我方公司不读也不摆资料，判据在
 * business-party-list.ts 的 businessPartyPanelTabsOf。资料修订链在栏顶读：抬头的英文名、「资料」格与签上的数要它。
 * 容器跟着内容的形态走：按话题分组的字段（概要、联系人、资料版本）用卡片，垫一层底色；表格与时间线（关联、登记记录）通栏，
 * 段头是带底色的窄带、文字的左缘与头区对齐。各签各自滚动：头区与签条一直在，滚的只是签的内容。
 * 去过的签留着挂载（隐藏而不卸载）：每切回来一次就重取一次会闪一次加载态；只留「去过的」，方向键逐行翻时不为没看的签取数。
 * 全景各册（PartyOverviewSection.tsx 的 usePartyOverviewLists）是例外：提示、签上的数与概要的卡要它们，栏一开就读，「关联」
 * 签去不去过都是同一份缓存。
 * 签不按参与方重置：↑/↓ 逐行翻时停在哪一签就一直看哪一签，与法人栏同一做法。
 */
export function BusinessPartyDetailPanel({
  row,
  roles,
  ours,
  typeUnread = false,
  pinned,
  relationships,
  retryRelationships,
  relationshipsComplete,
  tileKindOf,
  onShowRelationships,
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
   * 推类型用的册没读全（business-party-list.ts 的 partyTypeSourcesComplete 为假）。这时 `roles` 与 `ours` 只是读到的那一部分：
   * 头部推不出类型就显「—」，也不说「还没有合作关系」、不把「登记合作关系」提成主动作——它的关系可能只是没读到。
   */
  typeUnread?: boolean;
  /**
   * 法人册上有没有公司主体钉着它，已停用的也算（business-party-list.ts 的 partiesPinnedByLegalEntities），与对象页收资料入口
   * 同一判据；null 是法人册还没答出。头部的「我方公司」只数在用的（ours），两者用途不同。
   */
  pinned: boolean | null;
  /** 关系册答案随页面持有、交给概要与「关联」签，与关系册工作台同一份，不另取。 */
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
  /**
   * 关系册不带条件那一问已是整册（complete-register.ts）。不是的话 `relationships` 只是一页：这家单位有几段关系数不出来，
   * 「关联」签上不显数，概要与「关联」签里写明只按已取到的那一页列。
   */
  relationshipsComplete: boolean;
  /** 关系图与关系表里对方的图标块按它自己的类型取，与名录的行同一份；页面手上有全册的角色索引。 */
  tileKindOf?: (partyId: string) => PartyTileKind;
  /** 去「合作关系」视图看这家单位名下的全部关系。 */
  onShowRelationships: () => void;
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
  // 「关联」签「合作关系」段就地登记（票 business-party-edit-mode/03）。栏不按行重挂：换了行就不算这一份。
  const [registering, setRegistering] = useState<string | null>(null);
  const [relationshipUnsaved, setRelationshipUnsaved] = useState(false);
  const [confirmingCancel, setConfirmingCancel] = useState(false);
  // 从概要跳进「关联」签时要落到的那一段；「关联」签落过之后交还。
  const [relatedTarget, setRelatedTarget] = useState<PanelRelatedTarget | null>(null);
  const clearRelatedTarget = useCallback(() => setRelatedTarget(null), []);
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
  const openRelated = (target: PanelRelatedTarget) => {
    selectTab('overview');
    setRelatedTarget(target);
  };
  // 整份「更新资料」是对象页的编辑态，照公司主体栏：带编辑键去对象页，栏里不开表单。去之前记一笔「从名录来、落到哪一段」
  // （business-party-edit-origin.ts）：地址键不带来处，对象页退出编辑态时凭这一笔回到名录这张标签。
  const openProfileForm = (section: ProfileEditSectionId | null = null) => {
    markEditFromRoster(row.partyId, section);
    window.location.hash = profileEditHash(businessPartyObjectHash(row.partyId), true);
  };
  // 「更正登记信息」是同一个容器里的另一张表单（「更多」菜单里的那一项），去处与来处的记法同上。
  const openIdentityForm = () => {
    markEditFromRoster(row.partyId);
    window.location.hash = identityEditHash(businessPartyObjectHash(row.partyId), true);
  };
  // 提示、头区动作（票 business-party-ux/08，判读在 business-party-panel-first-screen.ts）、签上的数、概要的卡与「关联」签读同一份缓存。
  const lists = usePartyOverviewLists();
  const overview = partyOverviewStatesOf(row.partyId, relationships, lists);
  // 册没读全时两格都按「不知道」交给 panelHintOf：它对 null 的做法就是不说「还没有」。
  const liveRelationships =
    !typeUnread && overview.relationships.kind === 'answered' ? liveRelationshipCountOf(overview.relationships.rows) : null;
  const hint = panelHintOf(row, ours, liveRelationships, !typeUnread && overview.accounts.kind === 'answered' ? roles.length : null);
  const actions = panelHeaderActionsOf(mayWrite, pinned, hint, row.status);
  // 「登记合作关系」就是「关联」签里那张内联表单：切到签、把表单打开、落到那一段，不另造一处写口。
  const openRelationshipForm = () => {
    openRelated({ section: 'relationships' });
    setRegistering(row.partyId);
  };
  const profileCounts = panelProfileTabCountsOf(profiles.answer, row.partyId);
  const relatedCount = panelRelatedCountOf(overview, relationshipsComplete);

  return (
    <>
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`业务参与方 ${row.partyId}`}>
      <div className="space-y-3 px-6 pt-4">
        {/* 名称那一块要 280 才排得开；这一排放不下它与动作时动作整组折到下一行、靠右（ml-auto），不去挤名称。 */}
        <div className="flex flex-wrap items-start gap-x-3.5 gap-y-2.5">
          <PartyTile large kind={partyTileKindOf(ours, roles)} />
          <div className="min-w-0 flex-[1_1_280px]">
            <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
              <h2 className="line-clamp-2 min-w-0 break-words type-display font-semibold leading-7 text-idpxyz-textBright" title={row.partyName}>
                {row.partyName}
              </h2>
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
            {/* 类型与列表类型格同一份标签（BusinessPartyTypeTags）：我方公司排在角色前面；两样都没有时说「暂无合作关系」，册没读全时显「—」，不留空。 */}
            <div className="mt-1.5">
              <BusinessPartyTypeTags ours={ours} roles={roles} unread={typeUnread} />
            </div>
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-1.5">
            {/* 两枚写动作摆不摆、谁是主动作都照 panelHeaderActionsOf；次序不随主次换。 */}
            {actions.updateProfile ? (
              <Button
                size="sm"
                variant={actions.updateProfile === 'primary' ? 'default' : 'outline'}
                className={actions.updateProfile === 'primary' ? `${primaryActionClass} type-body` : 'type-body text-idpxyz-breadcrumb'}
                onClick={() => openProfileForm()}
              >
                <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                更新资料
              </Button>
            ) : null}
            {actions.registerRelationship ? (
              <Button
                size="sm"
                variant={actions.registerRelationship === 'primary' ? 'default' : 'outline'}
                className={actions.registerRelationship === 'primary' ? `${primaryActionClass} type-body` : 'type-body text-idpxyz-breadcrumb'}
                onClick={openRelationshipForm}
              >
                <Handshake className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                登记合作关系
              </Button>
            ) : null}
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
            <BusinessPartyMoreMenu
              row={row}
              parties={parties}
              liveRelationships={liveRelationships}
              onOpenObjectPage={onOpenObjectPage}
              onCorrectIdentity={openIdentityForm}
              onChanged={onChanged}
            />
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

        {/* 一行事实：编码与识别信息（ADR-0367）、生效日、资料版本，号与码点击即复制，放不下才折行。头区一直可见，所以识别信息
            只在这里出现一次，「登记记录」签不列它。生效日照状态说：还没到的是「将于…生效」，到了的是「自…生效」。 */}
        <dl className="flex min-w-0 flex-wrap items-center gap-x-5 gap-y-1">
          <Fact inline label="编码">
            <RefText value={row.partyId} name="编码" />
          </Fact>
          <BusinessPartyIdentificationFacts record={row} />
          <Fact inline label={row.status === 'REGISTERED' ? '将于' : '自'}>
            <Instant value={row.effectiveFrom} precision="day" /> 生效
          </Fact>
          {pinned === true ? null : (
            <Fact inline label="资料">
              {businessPartyProfileVersionFact(profiles.answer, row.partyId)}
            </Fact>
          )}
        </dl>
      </div>

      <Tabs value={shown} onValueChange={(value) => selectTab(value as BusinessPartyPanelTab)} className="flex min-h-0 flex-1 flex-col gap-0">
        {/* 签名与图标取对象工作区签的固定表（票 company-subject-ux/10），与对象页同名（票 business-parties-object-page/01）：
            「关联」是这家单位和我们有哪些往来，「登记记录」是身份修订链——同一种内容同一个词、同一个图标。
            签条通到栏的两边，第一张签的字与头区的左缘对齐。 */}
        <TabsList className="mt-3 h-[42px] shrink-0 gap-0.5 px-3.5">
          <TabsTrigger value="summary" className={tabTriggerClass}>
            <WorkspaceTabLabel id="summary" />
          </TabsTrigger>
          {tabs.includes('contacts') ? (
            <TabsTrigger value="contacts" className={tabTriggerClass}>
              <WorkspaceTabLabel id="contacts" />
              <TabCount value={profileCounts.contacts} />
            </TabsTrigger>
          ) : null}
          {tabs.includes('profile') ? (
            <TabsTrigger value="profile" className={tabTriggerClass}>
              <WorkspaceTabLabel id="profile-versions" />
              <TabCount value={profileCounts.versions} />
            </TabsTrigger>
          ) : null}
          <TabsTrigger value="identity" className={tabTriggerClass}>
            <WorkspaceTabLabel id="identity-revisions" />
          </TabsTrigger>
          <TabsTrigger value="overview" className={tabTriggerClass}>
            <WorkspaceTabLabel id="related" />
            <TabCount value={relatedCount} />
          </TabsTrigger>
        </TabsList>
        <TabsContent value="summary" forceMount={visited.has('summary') || undefined} data-panel-scroller className={cardTabClass}>
          <BusinessPartyPanelSummary
            row={row}
            roles={roles}
            pinned={pinned}
            profiles={profiles}
            overview={overview}
            retryRelationships={retryRelationships}
            relationshipsComplete={relationshipsComplete}
            hint={hint}
            mayWrite={mayWrite}
            relationshipFormOpen={registering === row.partyId}
            onUpdateProfile={openProfileForm}
            onRegisterRelationship={openRelationshipForm}
            onOpenContacts={() => selectTab('contacts')}
            onOpenRelated={openRelated}
            onShowRelationships={onShowRelationships}
          />
        </TabsContent>
        {tabs.includes('contacts') ? (
          <TabsContent value="contacts" forceMount={visited.has('contacts') || undefined} data-panel-scroller className={cardTabClass}>
            <BusinessPartyPanelContacts partyId={row.partyId} profiles={profiles} mayWrite={mayWrite} onUpdateProfile={openProfileForm} />
          </TabsContent>
        ) : null}
        {tabs.includes('profile') ? (
          <TabsContent value="profile" forceMount={visited.has('profile') || undefined} data-panel-scroller className={cardTabClass}>
            <BusinessPartyPanelVersions partyId={row.partyId} profiles={profiles} mayWrite={mayWrite} onUpdateProfile={openProfileForm} />
          </TabsContent>
        ) : null}
        <TabsContent value="overview" forceMount={visited.has('overview') || undefined} data-panel-scroller className={flushTabClass}>
          {/* 按单位重挂：栏换行时不重挂，这一签里开着的表单、对话框、点开的行与「结束」不能串到下一家单位。 */}
          <BusinessPartyPanelRelated
            key={row.partyId}
            row={row}
            ours={ours}
            centerKind={partyTileKindOf(ours, roles)}
            tileKindOf={tileKindOf}
            overview={overview}
            lists={lists}
            relationships={relationships}
            retryRelationships={retryRelationships}
            relationshipsComplete={relationshipsComplete}
            relationshipRegistration={relationshipRegistration}
            onRelationshipEnded={mayWrite ? retryRelationships : undefined}
            accountWrites={mayWrite ? { parties } : undefined}
            onShowRelationships={onShowRelationships}
            target={relatedTarget}
            onTargetHandled={clearRelatedTarget}
          />
        </TabsContent>
        <TabsContent value="identity" forceMount={visited.has('identity') || undefined} data-panel-scroller className={flushTabClass}>
          <PanelBand icon={ClipboardList} title="历次更新" />
          <div className="px-6 py-4">
            {row.deactivationBasis ? (
              <dl className="mb-4">
                <DetailRow label="停用的来源单据" mono>
                  {row.deactivationBasis}
                </DetailRow>
              </dl>
            ) : null}
            <RevisionHistorySection register={businessPartyRevisionHistory} subjectId={row.partyId} revision={row.revision} />
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
          </div>
        </TabsContent>
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
