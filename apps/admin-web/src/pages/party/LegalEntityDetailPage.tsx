import { Fragment, useState, type ReactNode } from 'react';
import { FileText } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import {
  DetailPageTemplate,
  type DetailMeta,
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
import { identityLayerAbsentNote, partyNameUnknownNote } from './presentation';
import {
  LegalEntityBanners,
  LegalEntityLifecycle,
  LegalEntityMoreMenu,
  legalEntityHasBanners,
  legalEntityHeaderFacts,
} from './LegalEntityHeaderParts';
import { LegalEntityProfileSection } from './LegalEntityProfileSection';
import { useTabViewState } from '../../templates/workspace-view-state';
import { LegalEntityProfileSummary } from './LegalEntityProfileSummary';
import { LegalEntityContactsSection } from './LegalEntityContactsSection';
import { useCurrentLegalEntityProfile } from './CurrentLegalEntityProfile';
import { legalEntityContactCountOf } from './legal-entity-profile';
import { identityLayerCellsOf, type RegistrationNumberTypeNameOf } from './legal-entity-identity';
import { legalEntityRevisionTimeline, revisionHistoryNote } from './legal-entity-revisions';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import { LegalEntityReferencesSection, useLegalEntityReferences } from './LegalEntityReferencesSection';
import { profileStatusCellOf, referencedCount, type ProfileStatusCell } from './legal-entity-references';
import { useLegalEntityTabCounts } from './legal-entity-tab-counts';
import { DetailRow, RefText, useCopyToClipboard } from './detail-primitives';
import { CountryLabel } from '../refdata-labels';
import { StatusPill } from '../../components/StatusPill';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

/**
 * 对象页与工作台详情栏共用身份修订历史的判读。按注册号类型目录译名的那一份随页面传进来（票 01 备好的
 * `legalEntityRevisionTimeline` 第三参）；`load` 仍是模块级函数——只有它进 effect 依赖，每帧新造的壳不会触发重取。
 */
export function legalEntityRevisionHistoryOf(
  typeNameOf?: RegistrationNumberTypeNameOf,
): RevisionHistoryRegister<LegalEntityRevisionListResponseBody> {
  return {
    subject: '法人',
    endpoint: 'GET /commercial-group-legal-entities/{legalEntityId}/revisions',
    load: listLegalEntityRevisions,
    echoedSubjectId: (body) => body.legalEntityId,
    timelineOf: (body) => legalEntityRevisionTimeline(body.revisions, formatInstant, typeNameOf),
    noteOf: revisionHistoryNote,
  };
}

function muted(text: string): ReactNode {
  return <span className="text-idpxyz-breadcrumb">{text}</span>;
}

// 身份事实留在头部，当前资料归概要，资料版本与身份修订各用自己的历史链。

/**
 * 头区第二行的 meta，与详情栏头区同一形（票 company-subject-ux/08）：编码、注册地、注册号一行小字，号可点按复制；
 * 关键事实（legalEntityHeaderFacts）接在后面，生命周期小步进贴在行末。注册号的类型码经目录译成名（统一社会信用代码），
 * 目录没读到就按码显——译名是锦上添花，号本身在答复里；一行小字放得下类型名，就跟在号前面显而不藏进 title。
 *
 * 事实不再走模板的指标卡（summary 不传，模板允许）：卡在宽屏上值只占一角。模板在头区与签之间只留了指标卡那一格，
 * 一行小字只能接在 meta 行后面，放进签里又会与概要签的「当前资料 第 N 版」挨着重复。
 */
function buildMeta(
  row: GroupLegalEntityRecord,
  typeNameOf: RegistrationNumberTypeNameOf | undefined,
  copy: (label: string, value: string) => void,
  profileCell: ProfileStatusCell,
  referenced: number | null,
): DetailMeta[] {
  const identity = identityLayerCellsOf(row, typeNameOf);
  return [
    { label: '编码', value: <RefText value={row.legalEntityId} name="编码" onCopy={copy} /> },
    {
      label: '注册地',
      value: identity ? <CountryLabel code={identity.country} /> : muted(identityLayerAbsentNote),
    },
    {
      label: '注册号',
      value: identity
        ? identity.numberCells.map((cell, index) => (
            <Fragment key={`${cell.number}#${index}`}>
              {index > 0 ? '；' : null}
              {muted(cell.typeName)} <RefText value={cell.number} name={cell.typeName} onCopy={copy} />
            </Fragment>
          ))
        : muted(identityLayerAbsentNote),
    },
    ...legalEntityHeaderFacts(row, profileCell, referenced).map((fact) => ({
      label: fact.label,
      value: fact.title ? <span title={fact.title}>{fact.value}</span> : fact.value,
    })),
    { label: '生命周期', value: <LegalEntityLifecycle row={row} /> },
  ];
}

/** 技术详情的行：对象页与详情栏同一份。行名照旧用原词与原码，那是给技术人员对照的；往来单位档案是从首屏撤下来的那一格。 */
export function legalEntityTechnicalRows(row: GroupLegalEntityRecord): { label: string; value: string }[] {
  return [
    { label: '法人标识', value: row.legalEntityId },
    { label: '往来单位档案', value: row.partyNameKnown && row.partyName ? row.partyName : partyNameUnknownNote },
    { label: '参与方标识', value: row.partyId },
    { label: '身份修订号', value: String(row.revision) },
    { label: '登记依据', value: row.basis },
    { label: '身份更正依据', value: row.identityCorrectionBasis ?? '' },
    { label: '停用依据', value: row.deactivationBasis ?? '' },
    { label: '租户', value: row.tenantId },
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
 * 页头更新动作先激活资料版本，再展开表单并滚到它；受控页签保证从任何签都能到达编辑入口。
 */
export function LegalEntityDetailPage({
  legalEntityId,
  row,
  listAnswer,
  retry,
  onBack,
  typeNameOf,
}: {
  legalEntityId: string;
  /** 册上对得上的那一行；册已取回却没有则为 null。 */
  row: GroupLegalEntityRecord | null;
  listAnswer: ApiResult<GroupLegalEntityListResponseBody> | null;
  retry: () => void;
  onBack: () => void;
  /** 注册号类型目录的译名（列表页读一次传进来）；不给就按码显。 */
  typeNameOf?: RegistrationNumberTypeNameOf;
}) {
  const [tab, setTab] = useTabViewState<WorkspaceTabId>('legal-entity.object-tab', 'summary');
  // 旧对象页把身份历史放在时间线签，保留工作区已记住的历史查看位置。
  const activeTab = tab === 'timeline' ? 'identity-revisions' : tab;
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const viewState = viewStateOf(listAnswer, row, retry, mayWrite);
  const [formOpenRequest, setFormOpenRequest] = useState(0);
  const [profileRefresh, setProfileRefresh] = useState(0);
  // 身份登了下一修订、停用、资料更新落册——三件事都要让当前资料与签上的计数重读，一把钥匙两处转。
  const refreshKey = `${row?.revision}:${row?.status}:${profileRefresh}`;
  const profileQuery = useCurrentLegalEntityProfile(row?.legalEntityId, refreshKey);
  // 签上的计数（票 company-subject-ux/09）：模板按 count 字段摆；读不到就不给，不拿 0 冒充。
  const tabCounts = useLegalEntityTabCounts(row?.legalEntityId, refreshKey);
  const contactCount = legalEntityContactCountOf(
    profileQuery.answer?.kind === 'outcome' && profileQuery.answer.body.legalEntityId === row?.legalEntityId
      ? profileQuery.answer.body
      : undefined,
  );
  // 两册一页只读一次：首屏「被引用」与「关联」签用同一份答案。
  const references = useLegalEntityReferences(row?.legalEntityId ?? legalEntityId);
  const profileCell = profileStatusCellOf(profileQuery.answer, row?.legalEntityId ?? legalEntityId);
  const referenced = referencedCount(references.states);
  const copy = useCopyToClipboard();
  const openProfileForm = () => {
    setTab('profile-versions');
    setFormOpenRequest((count) => count + 1);
  };
  return (
    <DetailPageTemplate
      title={info.title}
      identifier={row ? (row.partyNameKnown ? row.partyName : partyNameUnknownNote) : legalEntityId}
      status={row ? <StatusPill code={row.status} /> : undefined}
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
                  更新公司资料
                </Button>
              ) : null}
              <LegalEntityMoreMenu row={row} onChanged={retry} />
            </>
          ) : null}
          <Button variant="outline" size="sm" className="text-[12px] text-idpxyz-breadcrumb" onClick={onBack}>
            返回列表
          </Button>
        </>
      }
      meta={row ? buildMeta(row, typeNameOf, copy, profileCell, referenced) : []}
      // 头区 meta 行已把身份事实与关键事实显完，概要卡没有自己的格：技术详情跟在当前资料读出之后（与详情栏概要签同位），
      // 停用来源单据跟在停用横幅下。传空数组——分签形态下模板按有没有字段判要不要画这张卡，空的不画。
      basicFields={[]}
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
                          {row.deactivationBasis ? (
                            <dl>
                              <DetailRow label="停用来源单据" mono>
                                {row.deactivationBasis}
                              </DetailRow>
                            </dl>
                          ) : null}
                        </div>
                      ),
                    },
                  ]
                : []),
            ]
          : undefined
      }
      tabs={
        row
          ? [
              {
                id: 'summary',
                // 正文最宽 52rem、靠左，与详情栏同一宽度纪律（票 company-subject-ux/08 第 4 条）；模板的内容列是 960px，
                // 读出在里面不撑满。
                content: (
                  <section className="bg-idpxyz-bg p-5">
                    <div className="max-w-[52rem]">
                      <LegalEntityProfileSummary query={profileQuery} />
                      <TechnicalDetails className="mt-4" rows={legalEntityTechnicalRows(row)} />
                    </div>
                  </section>
                ),
              },
              {
                id: 'contacts',
                count: contactCount,
                content: (
                  <section className="bg-idpxyz-bg p-5">
                    <LegalEntityContactsSection query={profileQuery} />
                  </section>
                ),
              },
              {
                id: 'profile-versions',
                count: tabCounts.profileRevisions,
                keepMounted: true,
                content: (
                  <section className="bg-idpxyz-bg p-5">
                    <LegalEntityProfileSection
                      key={row.legalEntityId}
                      legalEntityId={row.legalEntityId}
                      formOpenRequest={formOpenRequest}
                      collapseCurrentProfile
                      onProfileUpdated={() => setProfileRefresh((value) => value + 1)}
                    />
                  </section>
                ),
              },
              {
                id: 'identity-revisions',
                count: tabCounts.identityRevisions,
                content: (
                  <RevisionHistorySection
                    register={legalEntityRevisionHistoryOf(typeNameOf)}
                    subjectId={row.legalEntityId}
                    revision={row.revision}
                  />
                ),
              },
              {
                id: 'related',
                count: referenced ?? undefined,
                content: (
                  <section className="bg-idpxyz-bg p-5">
                    <LegalEntityReferencesSection legalEntityId={row.legalEntityId} references={references} />
                  </section>
                ),
              },
            ]
          : undefined
      }
      activeTab={activeTab}
      onActiveTabChange={setTab}
      viewState={viewState}
    />
  );
}
