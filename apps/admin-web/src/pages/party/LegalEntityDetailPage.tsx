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
  identityRevisionLabel,
  identityStatusLabels,
  partyNameUnknownNote,
} from './presentation';
import {
  LegalEntityBanners,
  LegalEntityLifecycle,
  LegalEntityMoreMenu,
  legalEntityHasBanners,
} from './LegalEntityHeaderParts';
import { LegalEntityProfileSection } from './LegalEntityProfileSection';
import { identityLayerCellsOf } from './legal-entity-identity';
import { legalEntityRevisionTimeline, revisionHistoryNote } from './legal-entity-revisions';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import { Instant, statusBadge } from './detail-primitives';

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
// 事实条四格加生命周期条进指标带，概要签的格与详情栏概要签同一份，停用与待补横幅、法人资料各成一区，身份修订历史进
// 「时间线」签——签名受共用模板的固定词表约束（workspace-tabs.ts），法人资料没有对应的词，留在概要签里成区。

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
    { label: '依据', value: <span className="font-mono">{row.basis}</span> },
  ];
}

function buildSummary(row: GroupLegalEntityRecord): DetailSummaryStat[] {
  return [
    { label: '当前修订', value: <span className="font-mono">{identityRevisionLabel(row.revision)}</span> },
    { label: '生效自', value: <Instant value={row.effectiveFrom} /> },
    { label: '登记时间', value: <Instant value={row.registeredAt} /> },
    { label: '停用时点', value: row.deactivatedAt ? <Instant value={row.deactivatedAt} /> : '未停用' },
    { label: '生命周期', value: <LegalEntityLifecycle row={row} /> },
  ];
}

/** 与详情栏概要签同一份格：头区与指标带已经显示的不再重复。 */
function buildBasicFields(row: GroupLegalEntityRecord): DetailField[] {
  return [
    { label: '业务参与方身份', value: <span className="font-mono">{row.partyId}</span> },
    {
      label: '身份更正依据',
      value: row.identityCorrectionBasis ? <span className="font-mono">{row.identityCorrectionBasis}</span> : muted('—'),
    },
    {
      label: '停用依据',
      value: row.deactivationBasis ? <span className="font-mono">{row.deactivationBasis}</span> : muted('—'),
    },
    { label: '租户', value: <span className="font-mono">{row.tenantId}</span> },
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
): TemplateViewState {
  if (answer === null) return { kind: 'loading' };
  if (answer.kind === 'outcome') {
    return row
      ? { kind: 'ready' }
      : {
          kind: 'empty',
          title: '册上没有这个法人',
          description: '当前租户的责任法人册里没有这个标识。不存在与其他租户的对象在本页同一句话，本页不作区分。',
        };
  }
  return catalogueViewState(answer, 1, retry, {
    module: info,
    endpoint: 'GET /commercial-group-legal-entities',
    emptyTitle: '当前租户尚无责任法人登记',
    emptyDescription: '用列表页头「登记责任法人」登记第一个，或用受控 CLI parcel-commercial register-parties 灌入。',
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
  const viewState = viewStateOf(listAnswer, row, retry);
  const [tab, setTab] = useState<WorkspaceTabId>('summary');
  const [formOpenRequest, setFormOpenRequest] = useState(0);
  const openProfileForm = () => {
    setTab('summary');
    setFormOpenRequest((count) => count + 1);
  };
  return (
    <DetailPageTemplate
      title="责任法人"
      identifier={row?.legalEntityId ?? legalEntityId}
      status={row ? statusBadge(identityStatusLabels, row.status) : undefined}
      description={row ? `${row.partyNameKnown ? row.partyName : partyNameUnknownNote} · ${row.partyId}` : undefined}
      headerActions={
        <>
          {row ? (
            <>
              <Button
                size="sm"
                variant="default"
                className={`${primaryActionClass} text-[12px]`}
                onClick={openProfileForm}
              >
                <FileText className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
                登记资料修订
              </Button>
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
                id: 'profile',
                title: '法人资料',
                description: '法人身份与法人资料分开登记；这里查看当前有效资料与资料修订历史，并登记下一笔资料修订。',
                content: (
                  <LegalEntityProfileSection
                    key={row.legalEntityId}
                    legalEntityId={row.legalEntityId}
                    formOpenRequest={formOpenRequest}
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
