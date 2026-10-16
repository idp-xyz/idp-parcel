import { useState, type ReactNode } from 'react';
import { FileText, History, IdCard, X } from 'lucide-react';
import {
  Button,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import type { GroupLegalEntityRecord } from './api';
import { identityRevisionLabel, identityStatusLabels, labelOf, legalEntityKindLabels } from './presentation';
import { identityLayerCellsOf } from './legal-entity-identity';
import { LegalEntityBanners, LegalEntityLifecycle, LegalEntityMoreMenu } from './LegalEntityHeaderParts';
import { LegalEntityProfileSection } from './LegalEntityProfileSection';
import { legalEntityRevisionHistory } from './LegalEntityDetailPage';
import { RevisionHistorySection } from './RevisionHistorySection';
import { DetailRow, Instant, UnknownPartyName, statusBadge, useCopyToClipboard } from './detail-primitives';

type PanelTab = 'overview' | 'profile' | 'history';

/** 引用签：点一下复制，与参照页同一手势。 */
function RefChip({
  label,
  value,
  title,
  onCopy,
}: {
  label: string;
  value?: string;
  title: string;
  onCopy: (label: string, value: string) => void;
}) {
  if (!value) return null;
  return (
    <button
      type="button"
      title={`${title}（点击复制）`}
      onClick={() => onCopy(title, value)}
      className="inline-flex max-w-full items-center gap-1 rounded border border-idpxyz-border bg-idpxyz-sidebar px-1.5 py-0.5 text-[12px] hover:border-idpxyz-accent"
    >
      <span className="shrink-0 whitespace-nowrap text-idpxyz-breadcrumb">{label}</span>
      <span className="truncate font-mono text-idpxyz-text">{value}</span>
    </button>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
      <p className="text-[12px] text-idpxyz-breadcrumb">{label}</p>
      <p className="mt-0.5 break-words font-mono text-[12px] font-semibold leading-snug text-idpxyz-textBright">{children}</p>
    </div>
  );
}

/**
 * 集团与法人工作台的详情栏（票 admin-web-group-legal-entities/15）：形态照 idp-prism 采购订单的 PODetailPanel——
 * 头部（标识 + 种类 / 状态徽章 + 一行副题 + 引用签）· 主动作 + 「更多」菜单 + 关闭 · 生命周期条 · 关键事实条 · 页签。
 *
 * 一个事实在首屏只出现一处（票 group-legal-entities-uiux/06）：修订号与登记时间归事实条，国家、注册号与依据归引用签，
 * 参与方名称归副题；概要签只列这几处都没有的格，参与方身份那一行留着是因为它带复制。
 *
 * 内容只取行上已有的与两个既有区块（法人资料、修订历史各自取数），不为详情栏另开读口。生命周期条、横幅与「更多」菜单
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
  const [formOpenRequest, setFormOpenRequest] = useState(0);
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
              <h2 className="truncate font-mono text-[16px] font-semibold text-idpxyz-textBright">{row.legalEntityId}</h2>
              <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">{kind}</span>
              {statusBadge(identityStatusLabels, row.status)}
            </div>
            <p className="mt-0.5 text-[12px] text-idpxyz-breadcrumb">
              {row.partyNameKnown ? row.partyName : <UnknownPartyName />} ·{' '}
              <span className="whitespace-nowrap font-mono">{row.partyId}</span>
            </p>
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              <RefChip label="国家" value={identity?.country} title="注册国家 / 地区" onCopy={copy} />
              <RefChip label="注册号" value={identity?.numbers} title="终身注册号" onCopy={copy} />
              <RefChip label="依据" value={row.basis} title="登记依据" onCopy={copy} />
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button size="sm" variant="default" className={`${primaryActionClass} text-[12px]`} onClick={openProfileForm}>
              <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
              登记资料修订
            </Button>
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

        <LegalEntityLifecycle row={row} />

        <LegalEntityBanners row={row} />
      </div>

      {/* 事实条按栏宽在四列与两列之间二选一（票 group-legal-entities-uiux/10）：固定四列在窄栏里把「08:00:00」这样一个词拆成
          两半；按最小格宽自动分列又会在中间宽度排出三格加一格。minmax 的下限用 clamp 夹：栏宽过 32rem 时中间项为负、取下界
          25%，排四列；不到时中间项极大、取上界 50%，排两列——四格放得下每个词时才排四列。 */}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(clamp(calc(25%_-_0.625rem),calc((32rem_-_100%)_*_999),calc(50%_-_0.625rem)),1fr))] gap-2.5 border-b border-idpxyz-border px-5 py-3">
        <Fact label="当前修订">{identityRevisionLabel(row.revision)}</Fact>
        <Fact label="生效自">
          <Instant value={row.effectiveFrom} />
        </Fact>
        <Fact label="登记时间">
          <Instant value={row.registeredAt} />
        </Fact>
        <Fact label="停用时点">{row.deactivatedAt ? <Instant value={row.deactivatedAt} /> : '未停用'}</Fact>
      </div>

      <Tabs value={tab} onValueChange={(value) => selectTab(value as PanelTab)} className="flex min-h-0 flex-1 flex-col">
        <TabsList className="mx-5 mt-3 shrink-0">
          <TabsTrigger value="overview" className="text-idpxyz-breadcrumb">
            <IdCard className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            概要
          </TabsTrigger>
          <TabsTrigger value="profile" className="text-idpxyz-breadcrumb">
            <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            法人资料
          </TabsTrigger>
          <TabsTrigger value="history" className="text-idpxyz-breadcrumb">
            <History className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            身份修订
          </TabsTrigger>
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          <TabsContent value="overview" className="mt-0">
            <dl>
              <DetailRow label="业务参与方身份" mono onCopy={() => copy('参与方身份', row.partyId)}>
                {row.partyId}
              </DetailRow>
              <DetailRow label="身份更正依据" mono>
                {row.identityCorrectionBasis ?? <span className="text-idpxyz-breadcrumb">—</span>}
              </DetailRow>
              <DetailRow label="停用依据" mono>
                {row.deactivationBasis ?? <span className="text-idpxyz-breadcrumb">—</span>}
              </DetailRow>
              <DetailRow label="租户" mono>
                {row.tenantId}
              </DetailRow>
            </dl>
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
      </Tabs>
    </div>
  );
}
