import { useState, type ReactNode } from 'react';
import { AlertTriangle, Ban, ExternalLink, FileText, History, IdCard, MoreHorizontal, Copy, Users, X } from 'lucide-react';
import {
  Button,
  DropdownMenu,
  MenuItem,
  MenuSeparator,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@idpxyz/ui-primitives';
import type { GroupLegalEntityRecord } from './api';
import { identityLayerAbsentNote, identityStatusLabels, labelOf, legalEntityKindLabels, partyNameUnknownNote } from './presentation';
import { identityLayerCellsOf } from './legal-entity-identity';
import { legalEntityLifecycle, lifecycleConnectorReached, type LifecycleStageState } from './legal-entity-lifecycle';
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
      className="inline-flex max-w-full items-center gap-1 rounded border border-idpxyz-border bg-idpxyz-sidebar px-1.5 py-0.5 text-[10px] hover:border-idpxyz-accent"
    >
      <span className="text-idpxyz-textMuted">{label}</span>
      <span className="truncate font-mono text-idpxyz-text">{value}</span>
    </button>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
      <p className="text-[10px] text-idpxyz-textMuted">{label}</p>
      <p className="mt-0.5 break-words font-mono text-[12px] font-semibold leading-snug text-idpxyz-textBright">{children}</p>
    </div>
  );
}

const stageDotClass: Record<LifecycleStageState, string> = {
  done: 'border-emerald-500/50 bg-emerald-500/20 text-emerald-400',
  current: 'border-idpxyz-accent bg-idpxyz-activeItem text-idpxyz-accent',
  skipped: 'border-dashed border-idpxyz-border bg-transparent text-idpxyz-textMuted',
  pending: 'border-idpxyz-border bg-idpxyz-inputBg text-idpxyz-textMuted',
};

function Lifecycle({ row }: { row: GroupLegalEntityRecord }) {
  const stages = legalEntityLifecycle(row);
  if (stages === null) return null;
  return (
    <ol className="flex items-center" aria-label="身份生命周期">
      {stages.map((stage, index) => (
        <li key={stage.code} className="flex flex-1 items-center last:flex-none">
          <div className="flex flex-col items-center gap-1">
            <span
              className={`flex h-5 w-5 items-center justify-center rounded-full border text-[9px] font-semibold ${stageDotClass[stage.state]}`}
              aria-hidden="true"
            >
              {stage.state === 'done' ? '✓' : stage.state === 'skipped' ? '–' : index + 1}
            </span>
            <span
              className={`text-[9px] leading-none ${stage.state === 'current' ? 'font-medium text-idpxyz-textBright' : 'text-idpxyz-textMuted'}`}
            >
              {labelOf(identityStatusLabels, stage.code)}
              {stage.state === 'skipped' ? '（未经过）' : ''}
            </span>
          </div>
          {index < stages.length - 1 && (
            <span
              className={`mx-1.5 mb-3.5 h-px flex-1 ${
                lifecycleConnectorReached(stage, stages[index + 1]) ? 'bg-emerald-500/40' : 'bg-idpxyz-border'
              }`}
              aria-hidden="true"
            />
          )}
        </li>
      ))}
    </ol>
  );
}

/**
 * 集团与法人工作台的详情栏（票 admin-web-group-legal-entities/15）：形态照 idp-prism 采购订单的 PODetailPanel——
 * 头部（标识 + 种类 / 状态 / 修订徽章 + 一行副题 + 引用签）· 主动作 + 「更多」菜单 + 关闭 · 生命周期条 · 关键事实条 · 页签。
 *
 * 内容只取行上已有的与两个既有区块（法人资料、修订历史各自取数），不为详情栏另开读口。动作只摆本产品真有的：
 * 登记资料修订在「法人资料」签里；停用照页头注释的分工留在业务参与方页，这里只给去那一页的路。
 * 参照页的收货、证据、协作三签在法人册上没有对应的事实，不摆。
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
              <span className="rounded bg-sky-500/10 px-1.5 py-0.5 text-[10px] font-medium text-sky-400">{kind}</span>
              {statusBadge(identityStatusLabels, row.status)}
              <span className="rounded bg-gray-500/10 px-1.5 py-0.5 font-mono text-[10px] text-gray-400">r{row.revision}</span>
            </div>
            <p className="mt-0.5 text-[11px] text-idpxyz-textMuted">
              {row.partyNameKnown ? row.partyName : <UnknownPartyName />} · <span className="font-mono">{row.partyId}</span> · 登记于{' '}
              <Instant value={row.registeredAt} />
            </p>
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              <RefChip label="国家" value={identity?.country} title="注册国家 / 地区" onCopy={copy} />
              <RefChip label="注册号" value={identity?.numbers} title="终身注册号" onCopy={copy} />
              <RefChip label="依据" value={row.basis} title="登记依据" onCopy={copy} />
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Button size="sm" onClick={openProfileForm}>
              <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
              登记资料修订
            </Button>
            <DropdownMenu
              align="right"
              trigger={
                <Button variant="outline" size="sm" className="px-2" aria-label="更多动作">
                  <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
                </Button>
              }
            >
              <MenuItem onSelect={onOpenObjectPage}>
                <ExternalLink className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
                在新标签打开对象页
              </MenuItem>
              <MenuItem onSelect={() => copy('法人标识', row.legalEntityId)}>
                <Copy className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
                复制法人标识
              </MenuItem>
              <MenuSeparator />
              <MenuItem
                onSelect={() => {
                  window.location.hash = '#/business-parties';
                }}
              >
                <Users className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
                到业务参与方页停用 <span className="ml-1 font-mono text-idpxyz-textMuted">{row.partyId}</span>
              </MenuItem>
            </DropdownMenu>
            <Button variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </div>
        </div>

        <Lifecycle row={row} />

        {row.deactivatedAt && (
          <div className="flex items-center gap-2 rounded border border-red-500/30 bg-red-500/10 px-2.5 py-1.5 text-[11px] text-red-400">
            <Ban className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>
              自 <Instant value={row.deactivatedAt} /> 起停用，不再支持新的商业决定
              {row.deactivationBasis ? ` — 依据 ${row.deactivationBasis}` : ''}
            </span>
          </div>
        )}
        {/* 两件待补各报各的处置（判据在 legal-entity-list.ts legalEntityNeedsAttention）：身份层缺格登下一修订能补；
            参与方悬空是写入门失败留下的，登新修订补不上。 */}
        {!row.identityLayerRegistered && (
          <div className="flex items-center gap-2 rounded border border-orange-500/30 bg-orange-500/10 px-2.5 py-1.5 text-[11px] text-orange-400">
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>待补：身份层两格{identityLayerAbsentNote}。登记同一法人的下一修订补齐（列表页头「登记责任法人」）。</span>
          </div>
        )}
        {!row.partyNameKnown && (
          <div className="flex items-center gap-2 rounded border border-orange-500/30 bg-orange-500/10 px-2.5 py-1.5 text-[11px] text-orange-400">
            <AlertTriangle className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            <span>
              待查：{partyNameUnknownNote}（{row.partyId}）。这是写入门失败留下的悬空，登记新修订补不上，需到业务参与方页核对参与方册。
            </span>
          </div>
        )}
      </div>

      <div className="grid grid-cols-4 gap-2.5 border-b border-idpxyz-border px-5 py-3">
        <Fact label="当前修订">r{row.revision}</Fact>
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
          <TabsTrigger value="overview">
            <IdCard className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            概要
          </TabsTrigger>
          <TabsTrigger value="profile">
            <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            法人资料
          </TabsTrigger>
          <TabsTrigger value="history">
            <History className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            修订历史
          </TabsTrigger>
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          <TabsContent value="overview" className="mt-0">
            <dl>
              <DetailRow label="法人标识" mono onCopy={() => copy('法人标识', row.legalEntityId)}>
                {row.legalEntityId}
              </DetailRow>
              <DetailRow label="种类">{kind}</DetailRow>
              <DetailRow label="业务参与方身份" mono onCopy={() => copy('参与方身份', row.partyId)}>
                {row.partyId}
              </DetailRow>
              <DetailRow label="参与方名称">{row.partyNameKnown ? row.partyName : <UnknownPartyName />}</DetailRow>
              <DetailRow label="注册国家 / 地区" mono>
                {identity ? identity.country : <span className="text-idpxyz-textMuted">{identityLayerAbsentNote}</span>}
              </DetailRow>
              <DetailRow label="终身注册号" mono>
                {identity ? identity.numbers : <span className="text-idpxyz-textMuted">{identityLayerAbsentNote}</span>}
              </DetailRow>
              <DetailRow label="身份更正依据" mono>
                {row.identityCorrectionBasis ?? <span className="text-idpxyz-textMuted">—</span>}
              </DetailRow>
              <DetailRow label="登记依据" mono>
                {row.basis}
              </DetailRow>
              <DetailRow label="停用依据" mono>
                {row.deactivationBasis ?? <span className="text-idpxyz-textMuted">—</span>}
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
