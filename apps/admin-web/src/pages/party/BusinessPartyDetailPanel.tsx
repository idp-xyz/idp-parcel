import { useState } from 'react';
import { FileText, X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger, ConfirmDialog } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { WorkspaceTabLabel } from '../../templates/workspace-tab-icons';
import { useTabViewState } from '../../templates/workspace-view-state';
import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyRecord, PartyRelationshipListResponseBody, BusinessPartyListResponseBody } from './api';
import {
  businessPartyPanelTabsOf,
  ourCompanyLabel,
  shownBusinessPartyPanelTab,
  type BusinessPartyPanelTab,
} from './business-party-list';
import { businessPartyObjectHash, profileEditHash } from './business-parties-address';
import {
  BusinessPartyContactsReadout,
  BusinessPartyProfileReadout,
  BusinessPartyProfileVersions,
  ourCompanyProfileNote,
  useBusinessPartyProfiles,
} from './BusinessPartyProfileSection';
import { labelOf, partyRoleLabels, partyTypeFallbackLabel, updateOrdinalLabel } from './presentation';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { BusinessPartyBanners, BusinessPartyMoreMenu } from './BusinessPartyHeaderParts';
import { useMayWriteRegistry } from '../use-operator-grants';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { PartyOverviewSection } from './PartyOverviewSection';
import { businessPartyRevisionHistory } from './BusinessPartyDetailPage';
import { RevisionHistorySection } from './RevisionHistorySection';
import { DetailRow, Fact, FactStrip, Instant } from './detail-primitives';
import { BusinessPartyIdentificationFacts } from './BusinessPartyIdentificationRows';
import { RefText } from './copy-affordance';

const tabTriggerClass = 'px-0 type-body font-medium text-idpxyz-breadcrumb data-[state=active]:text-idpxyz-accent';

/**
 * 资料三签各挂一份修订链查询：键相同，一栏只发一笔；签没去过就不挂、不读。我方公司不读，法人册答出之前也先不读——同对象页，
 * 免得先发一笔再认出是我方公司。
 */
function PanelProfilePart({
  row,
  pinned,
  part,
}: {
  row: BusinessPartyRecord;
  pinned: boolean | null;
  part: 'summary' | 'contacts' | 'versions';
}) {
  const profiles = useBusinessPartyProfiles(row.partyId, false, pinned === false);
  if (pinned === true) return <p className="type-value leading-6 text-idpxyz-breadcrumb">{ourCompanyProfileNote}</p>;
  if (part === 'contacts') return <BusinessPartyContactsReadout partyId={row.partyId} query={profiles} />;
  if (part === 'versions') return <BusinessPartyProfileVersions partyId={row.partyId} query={profiles} />;
  return (
    <BusinessPartyProfileReadout
      partyId={row.partyId}
      query={profiles}
      registrationCountry={row.identityLayerRegistered ? row.registrationCountry : undefined}
    />
  );
}

/**
 * 业务参与方工作台身份册的详情栏（票 business-parties-uiux/01）：形态照 LegalEntityDetailPanel——头部（标识 + 状态徽章 +
 * 名称一行 + 引用签）· 主动作「更新资料」+「更多」菜单 + 关闭 · 停用横幅 · 事实条 · 签。
 *
 * 一个事实在首屏只出现一处（票 group-legal-entities-uiux/06）：更新次数、生效时点、登记时间与停用时点归事实条，名称是标题，
 * 编码是副题；标识、修订号、登记依据、租户这类排查用的字收进「登记记录」签底部的「技术详情」，不再占头部。
 *
 * 签照公司主体栏（票 business-party-profile/04）：「概要」（默认）读出往来单位资料，「联系人」「资料版本」读同一份资料修订链，
 * 「登记记录」（原称身份与修订、变更记录）装剩下的格与身份修订历史，「关联」（原称全景）回答这个参与方对我们是什么。我方公司
 * 不摆资料两签，判据在 business-party-list.ts 的 businessPartyPanelTabsOf。
 * 去过的签留着挂载（隐藏而不卸载）：全景一开就读四册、修订历史读一条链，每切回来一次就重取一次会闪一次加载态；只留
 * 「去过的」，方向键逐行翻时不为没看的签取数。签不按参与方重置：↑/↓ 逐行翻时停在哪一签就一直看哪一签，与法人栏同一做法。
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
            {/* 类型与列表类型格同一说法：我方公司排在角色前面；既不是我方公司、又没有角色时说「暂无合作关系」，不留空。 */}
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              {ours ? (
                <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 type-body font-medium text-idpxyz-breadcrumb">{ourCompanyLabel}</span>
              ) : null}
              {roles.length === 0 && !ours ? (
                <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 type-body font-medium text-idpxyz-breadcrumb">
                  {partyTypeFallbackLabel}
                </span>
              ) : (
                roles.map((role) => (
                  <span key={role} className="rounded bg-idpxyz-hover px-1.5 py-0.5 type-body font-medium text-idpxyz-breadcrumb">
                    {labelOf(partyRoleLabels, role)}
                  </span>
                ))
              )}
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {mayWrite && pinned !== true ? (
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
      </div>

      <FactStrip>
        <Fact label="变更">{updateOrdinalLabel(row.revision)}</Fact>
        <Fact label="生效时间">
          <Instant value={row.effectiveFrom} />
        </Fact>
        <Fact label="登记时间">
          <Instant value={row.registeredAt} />
        </Fact>
        <Fact label="停用时点">{row.deactivatedAt ? <Instant value={row.deactivatedAt} /> : '未停用'}</Fact>
      </FactStrip>

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
            <PanelProfilePart row={row} pinned={pinned} part="summary" />
          </TabsContent>
          {tabs.includes('contacts') ? (
            <TabsContent value="contacts" forceMount={visited.has('contacts') || undefined} className="mt-0 data-[state=inactive]:hidden">
              <PanelProfilePart row={row} pinned={pinned} part="contacts" />
            </TabsContent>
          ) : null}
          {tabs.includes('profile') ? (
            <TabsContent value="profile" forceMount={visited.has('profile') || undefined} className="mt-0 data-[state=inactive]:hidden">
              <PanelProfilePart row={row} pinned={pinned} part="versions" />
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
