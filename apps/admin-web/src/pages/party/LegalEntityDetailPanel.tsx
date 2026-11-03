import { useState } from 'react';
import { FileText, X } from 'lucide-react';
import {
  Button,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import type { GroupLegalEntityRecord } from './api';
import { StatusPill } from '../../components/StatusPill';
import { TabCountBadge } from '../../components/TabCountBadge';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { identityLayerCellsOf, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import {
  LegalEntityBanners,
  LegalEntityLifecycle,
  LegalEntityMoreMenu,
  ProfileStatusValue,
} from './LegalEntityHeaderParts';
import { LegalEntityProfileSection } from './LegalEntityProfileSection';
import { LegalEntityProfileSummary } from './LegalEntityProfileSummary';
import { LegalEntityContactsSection } from './LegalEntityContactsSection';
import { useCurrentLegalEntityProfile } from './CurrentLegalEntityProfile';
import { legalEntityContactCountOf } from './legal-entity-profile';
import { legalEntityRevisionHistoryOf, legalEntityTechnicalRows } from './LegalEntityDetailPage';
import { RevisionHistorySection } from './RevisionHistorySection';
import { LegalEntityReferencesSection, useLegalEntityReferences } from './LegalEntityReferencesSection';
import { profileStatusCellOf, referencedCount, referencedCountNote, referencedCountText } from './legal-entity-references';
import {
  Fact,
  FactStrip,
  Instant,
  RefText,
  UnknownPartyName,
  useCopyToClipboard,
  DetailRow,
} from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';
import { CountryLabel } from '../refdata-labels';
import { identityLayerAbsentNote } from './presentation';
import { useTabViewState } from '../../templates/workspace-view-state';

type PanelTab = 'overview' | 'contacts' | 'profile' | 'history' | 'related';

const tabTriggerClass = 'px-0 text-[12px] font-medium text-idpxyz-breadcrumb data-[state=active]:text-idpxyz-accent';

/**
 * 集团与法人工作台的详情栏（票 admin-web-group-legal-entities/15）：形态照 idp-prism 采购订单的 PODetailPanel——
 * 头部两行（名称 + 状态徽章；一行小字 meta + 生命周期小步进）· 主动作 + 「更多」菜单 + 关闭 · 关键事实一行 · 页签。
 *
 * 一个事实在首屏只出现一处（票 group-legal-entities-uiux/06）：事实条三格是生效日期、资料状态、被引用（已停用时多一格
 * 停用日期，票 company-subject-ux/03），国家、注册号归引用签，参与方名称归标题；身份版本只在「登记记录」签看。
 * 概要签补充当前公司资料摘要，技术标识、依据与往来单位档案收进技术详情。
 *
 * 内容只取行上已有的与几个既有区块（公司资料、登记记录、关联各自取数），不为详情栏另开读口。横幅与「更多」菜单
 * 在 LegalEntityHeaderParts.tsx，对象页摆的是同一份。参照页的收货、证据、协作三签在法人册上没有对应的事实，不摆。
 */
export function LegalEntityDetailPanel({
  row,
  onClose,
  onOpenObjectPage,
  onChanged,
  typeNameOf,
}: {
  row: GroupLegalEntityRecord;
  onClose: () => void;
  onOpenObjectPage: () => void;
  /** 停用落册后回调；列表页借它重取法人册（GroupLegalEntitiesPage 的 renderDetail 传 retry）。 */
  onChanged?: () => void;
  /** 注册号类型目录的译名（列表页读过的那一份）；不给就按码显。 */
  typeNameOf?: RegistrationNumberTypeNameOf;
}) {
  const [tab, setTab] = useTabViewState<PanelTab>('legal-entity.panel-tab', 'overview');
  // 去过的签留着挂载（隐藏而不卸载）：Radix 默认卸掉非活动签，法人资料与修订历史两区每切回来一次就重取一次、闪一次加载态。
  // 只留「去过的」，不一开栏就把所有签挂上——方向键逐行翻时不为没看的签取数。
  const [visited, setVisited] = useState<ReadonlySet<PanelTab>>(() => new Set<PanelTab>(['overview', tab]));
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [formOpenRequest, setFormOpenRequest] = useState(0);
  const [profileRefresh, setProfileRefresh] = useState(0);
  const profileQuery = useCurrentLegalEntityProfile(row.legalEntityId, `${row.revision}:${row.status}:${profileRefresh}`);
  const contactCount = legalEntityContactCountOf(
    profileQuery.answer?.kind === 'outcome' && profileQuery.answer.body.legalEntityId === row.legalEntityId
      ? profileQuery.answer.body
      : undefined,
  );
  // 两册一栏只读一次：事实条「被引用」与「关联」签用同一份答案；方向键换行只换关联、不重读。
  const references = useLegalEntityReferences(row.legalEntityId);
  const profileCell = profileStatusCellOf(profileQuery.answer, row.legalEntityId);
  const referenced = referencedCount(references.states);
  const copy = useCopyToClipboard();
  // 注册号的类型码经目录译成名；目录没读到就按码显。
  const identity = identityLayerCellsOf(row, typeNameOf);
  const selectTab = (next: PanelTab) => {
    setTab(next);
    setVisited((previous) => (previous.has(next) ? previous : new Set(previous).add(next)));
  };
  const openProfileForm = () => {
    selectTab('profile');
    setFormOpenRequest((count) => count + 1);
  };

  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`公司主体 ${row.legalEntityId}`}>
      <div className="space-y-2 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <h2 className="truncate text-[16px] font-semibold text-idpxyz-textBright">
              {row.partyNameKnown ? row.partyName : <UnknownPartyName />}
            </h2>
            <StatusPill code={row.status} />
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {mayWrite ? (
              <Button size="sm" variant="default" className={`${primaryActionClass} text-[12px]`} onClick={openProfileForm}>
                <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                更新公司资料
              </Button>
            ) : null}
            <LegalEntityMoreMenu row={row} onOpenObjectPage={onOpenObjectPage} onChanged={onChanged} />
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

        {/* 头区第二行是一行小字的 meta（票 company-subject-ux/08）：编码、注册地、注册号纯文本一行，号仍可点按复制；
            注册地与注册号是业务事实，留在头部，登记依据与各标识收进概要签底部的「技术详情」。类型名跟在号前面而不进 title
            ——一行小字放得下，胶囊那套「截断只显号」的取舍不再需要。生命周期收成一行小步进贴在这一行末尾。 */}
        <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-1">
          <dl className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1">
            <Fact inline label="编码">
              <RefText value={row.legalEntityId} name="编码" onCopy={copy} />
            </Fact>
            <Fact inline label="注册地">
              {identity ? <CountryLabel code={identity.country} /> : <span className="text-idpxyz-breadcrumb">{identityLayerAbsentNote}</span>}
            </Fact>
            {identity?.numberCells.map((cell, index) => (
              <Fact inline label="注册号" key={`${cell.number}#${index}`}>
                <span className="text-idpxyz-breadcrumb">{cell.typeName}</span> <RefText value={cell.number} name={cell.typeName} onCopy={copy} />
              </Fact>
            ))}
          </dl>
          <LegalEntityLifecycle row={row} />
        </div>

        <LegalEntityBanners row={row} />
      </div>

      <FactStrip>
        <Fact label="生效日期">
          <Instant value={row.effectiveFrom} precision="day" />
        </Fact>
        <Fact label="资料状态">
          <ProfileStatusValue cell={profileCell} />
        </Fact>
        <Fact label="被引用">
          <span title={referencedCountNote}>{referencedCountText(referenced)}</span>
        </Fact>
        {row.deactivatedAt ? (
          <Fact label="停用日期">
            <Instant value={row.deactivatedAt} precision="day" />
          </Fact>
        ) : null}
      </FactStrip>

      <Tabs value={tab} onValueChange={(value) => selectTab(value as PanelTab)} className="flex min-h-0 flex-1 flex-col gap-0">
        <TabsList className="mx-5 mt-2 h-9 w-auto shrink-0 gap-5">
          <TabsTrigger value="overview" className={tabTriggerClass}>
            概要
          </TabsTrigger>
          <TabsTrigger value="contacts" className={tabTriggerClass}>
            联系人
            {contactCount !== undefined ? <TabCountBadge count={contactCount} /> : null}
          </TabsTrigger>
          <TabsTrigger value="profile" className={tabTriggerClass}>
            资料版本
          </TabsTrigger>
          <TabsTrigger value="history" className={tabTriggerClass}>
            登记记录
          </TabsTrigger>
          <TabsTrigger value="related" className={tabTriggerClass}>
            关联
          </TabsTrigger>
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
          <div className="w-full max-w-[1120px]">
            <TabsContent value="overview" className="mt-0">
              {row.deactivationBasis ? (
                <dl className="mb-4">
                  <DetailRow label="停用来源单据" mono>
                    {row.deactivationBasis}
                  </DetailRow>
                </dl>
              ) : null}
              <LegalEntityProfileSummary query={profileQuery} />
              <TechnicalDetails className="mt-4" rows={legalEntityTechnicalRows(row)} />
            </TabsContent>
            <TabsContent
              value="contacts"
              forceMount={visited.has('contacts') || undefined}
              className="mt-0 data-[state=inactive]:hidden"
            >
              <LegalEntityContactsSection query={profileQuery} />
            </TabsContent>
            <TabsContent
              value="profile"
              forceMount={visited.has('profile') || undefined}
              className="mt-0 data-[state=inactive]:hidden"
            >
              <LegalEntityProfileSection
                key={row.legalEntityId}
                legalEntityId={row.legalEntityId}
                formOpenRequest={formOpenRequest}
                collapseCurrentProfile
                onProfileUpdated={() => setProfileRefresh((value) => value + 1)}
              />
            </TabsContent>
            <TabsContent
              value="history"
              forceMount={visited.has('history') || undefined}
              className="mt-0 data-[state=inactive]:hidden"
            >
              <RevisionHistorySection register={legalEntityRevisionHistoryOf(typeNameOf)} subjectId={row.legalEntityId} revision={row.revision} />
            </TabsContent>
            <TabsContent value="related" className="mt-0">
              <LegalEntityReferencesSection legalEntityId={row.legalEntityId} references={references} />
            </TabsContent>
          </div>
        </div>
      </Tabs>
    </div>
  );
}
