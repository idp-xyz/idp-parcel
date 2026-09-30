import type { ReactNode } from 'react';
import { DetailPageTemplate, type DetailField, type DetailMeta, type TemplateViewState } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listLegalEntityRevisions,
  type GroupLegalEntityListResponseBody,
  type GroupLegalEntityRecord,
  type LegalEntityRevisionListResponseBody,
} from './api';
import { identityLayerAbsentNote, identityStatusLabels, labelOf, legalEntityKindLabels } from './presentation';
import { LegalEntityProfileSection } from './LegalEntityProfileSection';
import { identityLayerCellsOf } from './legal-entity-identity';
import { legalEntityRevisionTimeline, revisionHistoryNote } from './legal-entity-revisions';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import { Instant, UnknownPartyName, statusBadge } from './detail-primitives';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

/**
 * 对象页「时间线」签与工作台详情栏「修订历史」签交给修订历史区的那几样。判读在 legal-entity-revisions.ts。
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
  return <span className="text-idpxyz-textMuted">{text}</span>;
}

function identityCells(row: GroupLegalEntityRecord) {
  return identityLayerCellsOf(row);
}

function buildMeta(row: GroupLegalEntityRecord): DetailMeta[] {
  return [
    { label: '修订', value: <span className="font-mono">r{row.revision}</span> },
    { label: '种类', value: labelOf(legalEntityKindLabels, row.kind) },
    { label: '生效自', value: <Instant value={row.effectiveFrom} /> },
  ];
}

function buildBasicFields(row: GroupLegalEntityRecord): DetailField[] {
  const identity = identityCells(row);
  return [
    { label: '法人标识', value: <span className="font-mono">{row.legalEntityId}</span> },
    { label: '修订', value: <span className="font-mono">r{row.revision}</span> },
    { label: '种类', value: labelOf(legalEntityKindLabels, row.kind) },
    { label: '业务参与方身份', value: <span className="font-mono">{row.partyId}</span> },
    { label: '参与方名称', value: row.partyNameKnown ? row.partyName : <UnknownPartyName /> },
    {
      label: '注册国家 / 地区',
      value: identity ? <span className="font-mono">{identity.country}</span> : muted(identityLayerAbsentNote),
    },
    {
      label: '终身注册号',
      value: identity ? <span className="font-mono">{identity.numbers}</span> : muted(identityLayerAbsentNote),
    },
    {
      label: '身份更正依据',
      value: row.identityCorrectionBasis ? (
        <span className="font-mono">{row.identityCorrectionBasis}</span>
      ) : (
        muted('—')
      ),
    },
    { label: '登记依据', value: <span className="font-mono">{row.basis}</span> },
    { label: '生效自', value: <Instant value={row.effectiveFrom} /> },
    {
      label: '停用时点',
      value: row.deactivatedAt ? <Instant value={row.deactivatedAt} /> : muted('未停用'),
    },
    {
      label: '停用依据',
      value: row.deactivationBasis ? <span className="font-mono">{row.deactivationBasis}</span> : muted('—'),
    },
    { label: '登记时间', value: <Instant value={row.registeredAt} /> },
    { label: '租户', value: <span className="font-mono">{row.tenantId}</span> },
  ];
}

/**
 * 对象页的四态。没有按标识的读口：行从法人册列表里认。册已取回却没有这一标识，
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
 * 法人对象页（`#/group-legal-entities/<法人标识>`）。检查器只放行上已有的概要，
 * 修订历史与法人资料（含登记新的资料修订）在这里：历史进「时间线」签，资料与登记表单
 * 在「概要」签里单独成区，读和写不再挤在同一段滚动里。
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
  return (
    <DetailPageTemplate
      title="责任法人"
      identifier={row?.legalEntityId ?? legalEntityId}
      status={row ? statusBadge(identityStatusLabels, row.status) : undefined}
      description="查看法人身份登记、资料修订与历史依据。"
      headerActions={
        <button
          type="button"
          onClick={onBack}
          className="rounded border border-idpxyz-border px-3 py-1.5 text-[12px] text-idpxyz-textMuted hover:bg-idpxyz-hover"
        >
          返回列表
        </button>
      }
      meta={row ? buildMeta(row) : []}
      basicFields={row ? buildBasicFields(row) : []}
      sections={
        row
          ? [
              {
                id: 'profile',
                title: '法人资料与修订',
                description: '法人身份与法人资料分开登记；这里查看当前有效资料并登记下一笔资料修订。',
                content: <LegalEntityProfileSection key={row.legalEntityId} legalEntityId={row.legalEntityId} />,
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
      viewState={viewState}
    />
  );
}
