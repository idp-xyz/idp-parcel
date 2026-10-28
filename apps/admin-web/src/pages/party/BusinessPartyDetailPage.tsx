import { useState, type ReactNode } from 'react';
import { Button } from '@idpxyz/ui-primitives';
import {
  DetailPageTemplate,
  type DetailField,
  type DetailMeta,
  type DetailSummaryStat,
  type TemplateViewState,
  type WorkspaceTabId,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listBusinessPartyRevisions,
  type BusinessPartyListResponseBody,
  type BusinessPartyRecord,
  type BusinessPartyRevisionListResponseBody,
  type PartyRelationshipListResponseBody,
} from './api';
import { updateOrdinalLabel } from './presentation';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { StatusPill } from '../../components/StatusPill';
import { businessPartyRevisionHistoryNote, businessPartyRevisionTimeline } from './business-party-revisions';
import { BusinessPartyBanners, BusinessPartyMoreMenu, businessPartyHasBanners } from './BusinessPartyHeaderParts';
import { LegalEntityLifecycle } from './LegalEntityHeaderParts';
import { PartyOverviewSection } from './PartyOverviewSection';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import { Instant } from './detail-primitives';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['business-parties'];

/**
 * 对象页「时间线」签与工作台详情栏「身份与修订」签交给修订历史区的那几样（票 12 第 5 条立、票 13 第 9 条改用共用件）：
 * 判读在 business-party-revisions.ts（两册共用的本体在 revision-timeline.ts）。模块级常量——load 进 effect 依赖。
 */
export const businessPartyRevisionHistory: RevisionHistoryRegister<BusinessPartyRevisionListResponseBody> = {
  subject: '参与方',
  endpoint: 'GET /commercial-business-parties/{partyId}/revisions',
  load: listBusinessPartyRevisions,
  echoedSubjectId: (body) => body.partyId,
  timelineOf: (body) => businessPartyRevisionTimeline(body.revisions, formatInstant),
  noteOf: businessPartyRevisionHistoryNote,
};

function muted(text: string): ReactNode {
  return <span className="text-idpxyz-breadcrumb">{text}</span>;
}

// 对象页与工作台详情栏摆同一套头区（票 group-legal-entities-uiux/09 的做法）：详情栏的引用签（依据）进头区第二行，
// 事实条四格加生命周期条进指标带，概要签的格与详情栏「身份与修订」签同一份，停用横幅成「需要处理」区，全景成区，
// 身份修订历史进「时间线」签——签名受共用模板的固定词表约束（workspace-tabs.ts），全景没有对应的词，留在概要签里成区。

function buildMeta(row: BusinessPartyRecord): DetailMeta[] {
  return [{ label: '编码', value: <span className="font-mono">{row.partyId}</span> }];
}

function buildSummary(row: BusinessPartyRecord): DetailSummaryStat[] {
  return [
    { label: '变更', value: updateOrdinalLabel(row.revision) },
    { label: '生效时间', value: <Instant value={row.effectiveFrom} /> },
    { label: '登记时间', value: <Instant value={row.registeredAt} /> },
    { label: '停用时点', value: row.deactivatedAt ? <Instant value={row.deactivatedAt} /> : '未停用' },
    { label: '生命周期', value: <LegalEntityLifecycle row={row} /> },
  ];
}

/** 与详情栏「变更记录」签同一份格：头区与指标带已经显示的不再重复；标识、修订号、依据、租户收进技术详情。 */
function buildBasicFields(row: BusinessPartyRecord): DetailField[] {
  return [
    {
      label: '停用的来源单据',
      value: row.deactivationBasis ? <span className="font-mono">{row.deactivationBasis}</span> : muted('未停用'),
    },
    {
      label: '技术详情',
      value: (
        <TechnicalDetails
          rows={[
            { label: '参与方标识', value: row.partyId },
            { label: '当前修订号', value: String(row.revision) },
            { label: '登记依据', value: row.basis },
            { label: '停用依据', value: row.deactivationBasis ?? '' },
            { label: '租户', value: row.tenantId },
          ]}
        />
      ),
    },
  ];
}

/**
 * 对象页的页面态。没有按标识的读口：行从参与方册列表里认。册已取回却没有这一标识，
 * 答「册上没有这个参与方」——不存在与其他租户的对象在这句话里不分，本页也不另发请求去分辨。
 */
function viewStateOf(
  answer: ApiResult<BusinessPartyListResponseBody> | null,
  row: BusinessPartyRecord | null,
  retry: () => void,
  mayWrite: boolean,
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind === 'outcome') {
    return row
      ? { kind: 'ready' }
      : {
          kind: 'empty',
          title: '没有这家单位',
          description: '没有登记过这个编码。请检查链接或编码是否正确。',
        };
  }
  return catalogueViewState(answer, 1, retry, {
    module: info,
    endpoint: 'GET /commercial-business-parties',
    emptyTitle: '还没有登记任何往来单位',
    emptyDescription: `客户、承运商、代理都从这里登记。${entryHint(mayWrite, '回到列表，用页头的「新增往来单位」登记第一家。')}`,
  });
}

/**
 * 参与方对象页（`#/business-parties/<参与方标识>`）。结构同工作台详情栏（BusinessPartyDetailPanel）。关系册答案由列表页
 * 持有、传进来（与关系册同一份），全景区不另取。签由本页记、经模板的受控签口交进去，与法人对象页同一做法。
 */
export function BusinessPartyDetailPage({
  partyId,
  row,
  listAnswer,
  retry,
  relationships,
  retryRelationships,
  onBack,
}: {
  partyId: string;
  /** 册上对得上的那一行；册已取回却没有则为 null。 */
  row: BusinessPartyRecord | null;
  listAnswer: ApiResult<BusinessPartyListResponseBody> | null;
  retry: () => void;
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
  onBack: () => void;
}) {
  const [tab, setTab] = useState<WorkspaceTabId>('summary');
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const viewState = viewStateOf(listAnswer, row, retry, mayWrite);
  return (
    <DetailPageTemplate
      title={info.title}
      identifier={row?.partyName ?? partyId}
      status={row ? <StatusPill code={row.status} /> : undefined}
      headerActions={
        <>
          {row ? <BusinessPartyMoreMenu row={row} /> : null}
          <Button variant="outline" size="sm" className="text-[12px] text-idpxyz-breadcrumb" onClick={onBack}>
            返回列表
          </Button>
        </>
      }
      meta={row ? buildMeta(row) : []}
      summary={row ? buildSummary(row) : undefined}
      basicTitle="概要"
      basicFields={row ? buildBasicFields(row) : []}
      sections={
        row
          ? [
              ...(businessPartyHasBanners(row)
                ? [
                    {
                      id: 'attention',
                      title: '需要处理',
                      content: (
                        <div className="flex flex-col gap-2">
                          <BusinessPartyBanners row={row} />
                        </div>
                      ),
                    },
                  ]
                : []),
              {
                id: 'overview',
                title: '全景',
                description: '这家单位对我们是什么：合作关系、客户账户、公司主体、供应商协议与结算政策。',
                content: (
                  <PartyOverviewSection
                    partyId={row.partyId}
                    relationships={relationships}
                    retryRelationships={retryRelationships}
                  />
                ),
              },
            ]
          : undefined
      }
      tabs={
        row
          ? [
              {
                id: 'timeline',
                content: (
                  <RevisionHistorySection register={businessPartyRevisionHistory} subjectId={row.partyId} revision={row.revision} />
                ),
              },
            ]
          : undefined
      }
      activeTab={tab}
      onActiveTabChange={setTab}
      viewState={viewState}
    />
  );
}
