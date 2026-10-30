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
import { labelOf, legalEntityKindLabels, updateOrdinalLabel } from './presentation';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { identityLayerCellsOf } from './legal-entity-identity';
import { LegalEntityBanners, LegalEntityMoreMenu } from './LegalEntityHeaderParts';
import { LegalEntityProfileSection } from './LegalEntityProfileSection';
import { LegalEntityProfileSummary } from './LegalEntityProfileSummary';
import { legalEntityRevisionHistory } from './LegalEntityDetailPage';
import { RevisionHistorySection } from './RevisionHistorySection';
import {
  Fact,
  FactStrip,
  Instant,
  RefChip,
  UnknownPartyName,
  useCopyToClipboard,
  DetailRow,
} from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';

type PanelTab = 'overview' | 'profile' | 'history';

/**
 * 集团与法人工作台的详情栏（票 admin-web-group-legal-entities/15）：形态照 idp-prism 采购订单的 PODetailPanel——
 * 头部（标识 + 种类 / 状态徽章 + 一行副题 + 引用签）· 主动作 + 「更多」菜单 + 关闭 · 关键事实条 · 页签。
 *
 * 一个事实在首屏只出现一处（票 group-legal-entities-uiux/06）：修订号与登记时间归事实条，国家、注册号与依据归引用签，
 * 参与方名称归副题；概要签补充当前法人资料摘要，技术标识与依据仍收进技术详情。
 *
 * 内容只取行上已有的与两个既有区块（法人资料、修订历史各自取数），不为详情栏另开读口。横幅与「更多」菜单
 * 在 LegalEntityHeaderParts.tsx，对象页摆的是同一份。参照页的收货、证据、协作三签在法人册上没有对应的事实，不摆。
 */
export function LegalEntityDetailPanel({
  row,
  onClose,
  onOpenObjectPage,
}: {
  row: GroupLegalEntityRecord;
  onClose: () => void;
  onOpenObjectPage: () => void;
}) {
  const [tab, setTab] = useState<PanelTab>('overview');
  // 去过的签留着挂载（隐藏而不卸载）：Radix 默认卸掉非活动签，法人资料与修订历史两区每切回来一次就重取一次、闪一次加载态。
  // 只留「去过的」，不一开栏就三签全挂——方向键逐行翻时不为没看的签取数。
  const [visited, setVisited] = useState<ReadonlySet<PanelTab>>(() => new Set<PanelTab>(['overview']));
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [formOpenRequest, setFormOpenRequest] = useState(0);
  const [profileRefresh, setProfileRefresh] = useState(0);
  const copy = useCopyToClipboard();
  const identity = identityLayerCellsOf(row);
  const kind = labelOf(legalEntityKindLabels, row.kind);
  const selectTab = (next: PanelTab) => {
    setTab(next);
    setVisited((previous) => (previous.has(next) ? previous : new Set(previous).add(next)));
  };
  const openProfileForm = () => {
    selectTab('profile');
    setFormOpenRequest((count) => count + 1);
  };

  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`责任法人 ${row.legalEntityId}`}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-[16px] font-semibold text-idpxyz-textBright">
                {row.partyNameKnown ? row.partyName : <UnknownPartyName />}
              </h2>
              <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">{kind}</span>
              <StatusPill code={row.status} />
            </div>
            <p className="mt-0.5 font-mono text-[12px] text-idpxyz-breadcrumb">编码 {row.legalEntityId}</p>
            {/* 注册地与注册号是业务事实，留在头部；登记依据与各标识收进概要签底部的「技术详情」。 */}
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              <RefChip label="注册地" value={identity?.country} title="注册国家 / 地区" onCopy={copy} />
              <RefChip label="注册号" value={identity?.numbers} title="注册号" onCopy={copy} />
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            {mayWrite ? (
              <Button size="sm" variant="default" className={`${primaryActionClass} text-[12px]`} onClick={openProfileForm}>
                <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                更新开票资料
              </Button>
            ) : null}
            <LegalEntityMoreMenu row={row} onOpenObjectPage={onOpenObjectPage} />
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

        <LegalEntityBanners row={row} />
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

      <Tabs value={tab} onValueChange={(value) => selectTab(value as PanelTab)} className="flex min-h-0 flex-1 flex-col gap-0">
        <TabsList className="mx-5 mt-2 h-9 w-auto shrink-0 gap-5">
          <TabsTrigger
            value="overview"
            className="px-0 text-[12px] font-medium text-idpxyz-breadcrumb data-[state=active]:text-idpxyz-accent"
          >
            概要
          </TabsTrigger>
          <TabsTrigger
            value="profile"
            className="px-0 text-[12px] font-medium text-idpxyz-breadcrumb data-[state=active]:text-idpxyz-accent"
          >
            开票资料
          </TabsTrigger>
          <TabsTrigger
            value="history"
            className="px-0 text-[12px] font-medium text-idpxyz-breadcrumb data-[state=active]:text-idpxyz-accent"
          >
            身份修订
          </TabsTrigger>
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-3">
          <div className="w-full max-w-[1120px]">
            <TabsContent value="overview" className="mt-0">
              {row.deactivationBasis ? (
                <dl className="mb-4">
                  <DetailRow label="停用依据" mono>
                    {row.deactivationBasis}
                  </DetailRow>
                </dl>
              ) : null}
              <LegalEntityProfileSummary
                key={`${row.legalEntityId}:${row.revision}:${row.status}`}
                legalEntityId={row.legalEntityId}
                refreshKey={profileRefresh}
              />
              <TechnicalDetails
                className="mt-4"
                rows={[
                  { label: '法人标识', value: row.legalEntityId },
                  { label: '参与方标识', value: row.partyId },
                  { label: '身份修订号', value: String(row.revision) },
                  { label: '登记依据', value: row.basis },
                  { label: '身份更正依据', value: row.identityCorrectionBasis ?? '' },
                  { label: '停用依据', value: row.deactivationBasis ?? '' },
                  { label: '租户', value: row.tenantId },
                ]}
              />
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
                onProfileUpdated={() => setProfileRefresh((value) => value + 1)}
              />
            </TabsContent>
            <TabsContent
              value="history"
              forceMount={visited.has('history') || undefined}
              className="mt-0 data-[state=inactive]:hidden"
            >
              <RevisionHistorySection register={legalEntityRevisionHistory} subjectId={row.legalEntityId} revision={row.revision} />
            </TabsContent>
          </div>
        </div>
      </Tabs>
    </div>
  );
}
