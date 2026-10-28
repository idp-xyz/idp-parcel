import { useState } from 'react';
import { IdCard, Network, X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import type { ApiResult } from '../catalogue-api';
import type { BusinessPartyRecord, PartyRelationshipListResponseBody } from './api';
import { identityStatusLabels, labelOf, partyRoleLabels, partyTypeFallbackLabel, updateOrdinalLabel } from './presentation';
import { TechnicalDetails } from './TechnicalDetails';
import { BusinessPartyBanners, BusinessPartyMoreMenu } from './BusinessPartyHeaderParts';
import { LegalEntityLifecycle } from './LegalEntityHeaderParts';
import { PartyOverviewSection } from './PartyOverviewSection';
import { businessPartyRevisionHistory } from './BusinessPartyDetailPage';
import { RevisionHistorySection } from './RevisionHistorySection';
import { DetailRow, Fact, FactStrip, Instant, statusBadge } from './detail-primitives';

type PanelTab = 'overview' | 'identity';

/**
 * 业务参与方工作台身份册的详情栏（票 business-parties-uiux/01）：形态照 LegalEntityDetailPanel——头部（标识 + 状态徽章 +
 * 名称一行 + 引用签）· 「更多」菜单 + 关闭 · 生命周期条 · 停用横幅 · 事实条 · 两签。
 *
 * 一个事实在首屏只出现一处（票 group-legal-entities-uiux/06）：更新次数、生效时点、登记时间与停用时点归事实条，名称是标题，
 * 编码是副题；标识、修订号、登记依据、租户这类排查用的字收进「变更记录」签底部的「技术详情」，不再占头部。
 *
 * 两签沿票 operator-workspace-gaps/01：「全景」（默认）回答这个参与方对我们是什么，「身份与修订」装剩下的格与修订历史。
 * 去过的签留着挂载（隐藏而不卸载）：全景一开就读四册、修订历史读一条链，每切回来一次就重取一次会闪一次加载态；只留
 * 「去过的」，方向键逐行翻时不为没看的签取数。签不按参与方重置：↑/↓ 逐行翻时停在哪一签就一直看哪一签，与法人栏同一做法。
 */
export function BusinessPartyDetailPanel({
  row,
  roles,
  relationships,
  retryRelationships,
  onClose,
  onOpenObjectPage,
  onDeactivate,
}: {
  row: BusinessPartyRecord;
  /** 这个参与方在关系册上持有的角色（business-party-list.ts 的 indexPartyRoles），头部按它称呼；空即「往来单位」。 */
  roles: readonly string[];
  /** 关系册答案随页面持有、交给全景签，与关系册工作台同一份，不另取。 */
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
  onClose: () => void;
  onOpenObjectPage: () => void;
  /** 「停用此参与方…」：页面借它记下这次是从列表压进登记视图的，再改地址。 */
  onDeactivate: () => void;
}) {
  const [tab, setTab] = useState<PanelTab>('overview');
  const [visited, setVisited] = useState<ReadonlySet<PanelTab>>(() => new Set<PanelTab>(['overview']));
  const selectTab = (next: PanelTab) => {
    setTab(next);
    setVisited((previous) => (previous.has(next) ? previous : new Set(previous).add(next)));
  };

  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`业务参与方 ${row.partyId}`}>
      <div className="space-y-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="truncate text-[16px] font-semibold text-idpxyz-textBright">{row.partyName}</h2>
              {statusBadge(identityStatusLabels, row.status)}
            </div>
            {/* 类型按持有的角色称呼；一个角色都没有就说「往来单位」，不留空。 */}
            <div className="mt-1 flex flex-wrap items-center gap-1.5">
              {roles.length === 0 ? (
                <span className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">
                  {partyTypeFallbackLabel}
                </span>
              ) : (
                roles.map((role) => (
                  <span key={role} className="rounded bg-idpxyz-hover px-1.5 py-0.5 text-[12px] font-medium text-idpxyz-breadcrumb">
                    {labelOf(partyRoleLabels, role)}
                  </span>
                ))
              )}
              <span className="font-mono text-[12px] text-idpxyz-breadcrumb">编码 {row.partyId}</span>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <BusinessPartyMoreMenu row={row} onOpenObjectPage={onOpenObjectPage} onDeactivate={onDeactivate} />
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

      <Tabs value={tab} onValueChange={(value) => selectTab(value as PanelTab)} className="flex min-h-0 flex-1 flex-col">
        <TabsList className="mx-5 mt-3 shrink-0">
          <TabsTrigger value="overview" className="text-idpxyz-breadcrumb">
            <Network className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            全景
          </TabsTrigger>
          <TabsTrigger value="identity" className="text-idpxyz-breadcrumb">
            <IdCard className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
            变更记录
          </TabsTrigger>
        </TabsList>
        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          <TabsContent
            value="overview"
            forceMount={visited.has('overview') || undefined}
            className="mt-0 data-[state=inactive]:hidden"
          >
            <PartyOverviewSection partyId={row.partyId} relationships={relationships} retryRelationships={retryRelationships} />
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
              <h3 className="text-[12px] font-medium text-idpxyz-text">历次更新</h3>
              <RevisionHistorySection register={businessPartyRevisionHistory} subjectId={row.partyId} revision={row.revision} />
            </section>
            <TechnicalDetails
              className="mt-4"
              rows={[
                { label: '参与方标识', value: row.partyId },
                { label: '当前修订号', value: String(row.revision) },
                { label: '登记依据', value: row.basis },
                { label: '停用依据', value: row.deactivationBasis },
                { label: '租户', value: row.tenantId },
              ]}
            />
          </TabsContent>
        </div>
      </Tabs>
    </div>
  );
}
