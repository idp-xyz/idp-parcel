import { useState, type ReactNode } from 'react';
import { FileText } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
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
  listLegalEntityRevisions,
  type GroupLegalEntityListResponseBody,
  type GroupLegalEntityRecord,
  type LegalEntityRevisionListResponseBody,
} from './api';
import {
  identityLayerAbsentNote,
  partyNameUnknownNote,
  updateOrdinalLabel,
} from './presentation';
import {
  LegalEntityBanners,
  LegalEntityMoreMenu,
  legalEntityHasBanners,
} from './LegalEntityHeaderParts';
import { LegalEntityProfileSection } from './LegalEntityProfileSection';
import { LegalEntityProfileSummary } from './LegalEntityProfileSummary';
import { identityLayerCellsOf } from './legal-entity-identity';
import { legalEntityRevisionTimeline, revisionHistoryNote } from './legal-entity-revisions';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import { Instant } from './detail-primitives';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

/**
 * 对象页「时间线」签与工作台详情栏「身份修订」签交给修订历史区的那几样。判读在 legal-entity-revisions.ts。
 * 模块级常量——load 进 effect 依赖。
 */
export const legalEntityRevisionHistory: RevisionHistoryRegister<LegalEntityRevisionListResponseBody> = {
  subject: '法人',
  endpoint: 'GET /commercial-group-legal-entities/{legalEntityId}/revisions',
  load: listLegalEntityRevisions,
  echoedSubjectId: (body) => body.legalEntityId,
  timelineOf: (body) => legalEntityRevisionTimeline(body.revisions, formatInstant),
  noteOf: revisionHistoryNote,
};

function muted(text: string): ReactNode {
  return <span className="text-idpxyz-breadcrumb">{text}</span>;
}

// 对象页与工作台详情栏摆同一套头区（票 group-legal-entities-uiux/09）：详情栏的引用签（国家、注册号、依据）进头区第二行，
// 事实条四格进指标带，概要签包含同一份法人资料摘要，停用与待补横幅、资料修订各成一区，身份修订历史进
// 「时间线」签——签名受共用模板的固定词表约束（workspace-tabs.ts）。

function buildMeta(row: GroupLegalEntityRecord): DetailMeta[] {
  const identity = identityLayerCellsOf(row);
  return [
    {
      label: '国家',
      value: identity ? <span className="font-mono">{identity.country}</span> : muted(identityLayerAbsentNote),
    },
    {
      label: '注册号',
      value: identity ? <span className="font-mono">{identity.numbers}</span> : muted(identityLayerAbsentNote),
    },
  ];
}

function buildSummary(row: GroupLegalEntityRecord): DetailSummaryStat[] {
  return [
    { label: '变更', value: updateOrdinalLabel(row.revision) },
    { label: '生效时间', value: <Instant value={row.effectiveFrom} /> },
    { label: '登记时间', value: <Instant value={row.registeredAt} /> },
    { label: '停用时点', value: row.deactivatedAt ? <Instant value={row.deactivatedAt} /> : '未停用' },
  ];
}

/** 与详情栏概要签同一份格：头区与指标带已经显示的不再重复。 */
function buildBasicFields(row: GroupLegalEntityRecord): DetailField[] {
  return [
    { label: '对应的往来单位', value: row.partyNameKnown ? row.partyName : muted(partyNameUnknownNote) },
    {
      label: '停用的来源单据',
      value: row.deactivationBasis ? <span className="font-mono">{row.deactivationBasis}</span> : muted('未停用'),
    },
    {
      label: '技术详情',
      value: (
        <TechnicalDetails
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
      ),
    },
  ];
}

/**
 * 对象页的页面态。没有按标识的读口：行从法人册列表里认。册已取回却没有这一标识，
 * 答「册上没有这个法人」——不存在与其他租户的对象在这句话里不分，本页也不另发请求去分辨。
 */
function viewStateOf(
  answer: ApiResult<GroupLegalEntityListResponseBody> | null,
  row: GroupLegalEntityRecord | null,
  retry: () => void,
  mayWrite: boolean,
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind === 'outcome') {
    return row
      ? { kind: 'ready' }
      : {
          kind: 'empty',
          title: '没有这个公司主体',
          description: '本租户没有登记过这个法人标识。请检查链接或标识是否正确。',
        };
  }
  return catalogueViewState(answer, 1, retry, {
    module: info,
    endpoint: 'GET /commercial-group-legal-entities',
    emptyTitle: '还没有登记任何公司主体',
    emptyDescription: `签约、开票都要以公司主体的名义。${entryHint(mayWrite, '回到列表，用页头的「新增公司主体」登记第一个。')}`,
  });
}

/**
 * 法人对象页（`#/group-legal-entities/<法人标识>`）。结构同工作台详情栏（见上）；主动作「登记资料修订」先把人带回概要签
 * 再展开法人资料区的登记表单并滚到它——人可能正停在时间线签上，所以签由本页记，经模板的受控签口交进去。
 */
export function LegalEntityDetailPage({
  legalEntityId,
  row,
  listAnswer,
  retry,
  onBack,
}: {
  legalEntityId: string;
  /** 册上对得上的那一行；册已取回却没有则为 null。 */
  row: GroupLegalEntityRecord | null;
  listAnswer: ApiResult<GroupLegalEntityListResponseBody> | null;
  retry: () => void;
  onBack: () => void;
}) {
  const [tab, setTab] = useState<WorkspaceTabId>('summary');
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const viewState = viewStateOf(listAnswer, row, retry, mayWrite);
  const [formOpenRequest, setFormOpenRequest] = useState(0);
  const [profileRefresh, setProfileRefresh] = useState(0);
  const openProfileForm = () => {
    setTab('summary');
    setFormOpenRequest((count) => count + 1);
  };
  return (
    <DetailPageTemplate
      title={info.title}
      identifier={row ? (row.partyNameKnown ? row.partyName : partyNameUnknownNote) : legalEntityId}
      status={row ? <StatusPill code={row.status} /> : undefined}
      description={row ? `编码 ${row.legalEntityId}` : undefined}
      headerActions={
        <>
          {row ? (
            <>
              {mayWrite ? (
                <Button
                  size="sm"
                  variant="default"
                  className={`${primaryActionClass} text-[12px]`}
                  onClick={openProfileForm}
                >
                  <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                  登记资料修订
                </Button>
              ) : null}
              <LegalEntityMoreMenu row={row} />
            </>
          ) : null}
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
              ...(legalEntityHasBanners(row)
                ? [
                    {
                      id: 'attention',
                      title: '需要处理',
                      content: (
                        <div className="flex flex-col gap-2">
                          <LegalEntityBanners row={row} />
                        </div>
                      ),
                    },
                  ]
                : []),
              {
                id: 'profile-summary',
                title: '法人资料摘要',
                description: '当前时点的资料状态与开票可用性；资料修订历史和登记入口见下方开票资料。',
                content: (
                  <LegalEntityProfileSummary
                    key={`${row.legalEntityId}:${row.revision}:${row.status}`}
                    legalEntityId={row.legalEntityId}
                    refreshKey={profileRefresh}
                  />
                ),
              },
              {
                id: 'profile',
                title: '开票资料',
                description: '法人身份与法人资料分开登记；这里查看当前有效资料与资料修订历史，并登记下一笔资料修订。',
                content: (
                  <LegalEntityProfileSection
                    key={row.legalEntityId}
                    legalEntityId={row.legalEntityId}
                    formOpenRequest={formOpenRequest}
                    collapseCurrentProfile
                    onProfileUpdated={() => setProfileRefresh((value) => value + 1)}
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
                  <RevisionHistorySection
                    register={legalEntityRevisionHistory}
                    subjectId={row.legalEntityId}
                    revision={row.revision}
                  />
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
