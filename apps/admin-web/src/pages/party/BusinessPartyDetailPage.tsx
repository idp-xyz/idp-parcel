import { useEffect, useState, type ReactNode } from 'react';
import { partyAnswersDomain } from './party-writes';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { BusinessPartyIdentityEditMode } from './BusinessPartyIdentityEditMode';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
import { DetailPageTemplate, type DetailMeta, type TemplateViewState, type WorkspaceTabId } from '../../templates';
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
import { PartyOverviewSection, usePartyOverviewCount } from './PartyOverviewSection';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import { Instant } from './detail-primitives';
import { RefText } from './copy-affordance';
import { useBusinessPartyIdentificationMeta } from './BusinessPartyIdentificationRows';
import {
  BusinessPartyContactsReadout,
  BusinessPartyProfileEditMode,
  BusinessPartyProfileReadout,
  BusinessPartyProfileVersions,
  effectiveProfileOf,
  ourCompanyProfileNote,
  useBusinessPartyProfiles,
} from './BusinessPartyProfileSection';
import type { AnswerQuery } from '../../query/answers';
import type { BusinessPartyProfileRevisionListResponseBody } from './api';
import { useRegistryWriteGrant } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['business-parties'];

/**
 * 对象页「登记记录」签与工作台详情栏「登记记录」签交给修订历史区的那几样（修订历史区是两册共用件 RevisionHistorySection）：
 * 判读在 business-party-revisions.ts（两册共用的本体在 revision-timeline.ts）。
 */
export const businessPartyRevisionHistory: RevisionHistoryRegister<BusinessPartyRevisionListResponseBody> = {
  subject: '参与方',
  endpoint: 'GET /commercial-business-parties/{partyId}/revisions',
  load: listBusinessPartyRevisions,
  answerKey: (partyId, version) => [partyAnswersDomain, 'business-party', partyId, version, 'revisions'],
  echoedSubjectId: (body) => body.partyId,
  timelineOf: (body) => businessPartyRevisionTimeline(body.revisions, formatInstant),
  noteOf: businessPartyRevisionHistoryNote,
};

/** 「资料版本」分区折起时标题后那一句：此刻是第几版，或还没有。 */
function profileVersionsSummary(query: AnswerQuery<BusinessPartyProfileRevisionListResponseBody>, partyId: string): string | undefined {
  const { record, body } = effectiveProfileOf(query, partyId);
  if (body === null) return undefined;
  if (record) return `当前第 ${record.revision} 版`;
  return body.revisions.length ? '尚无生效版本' : '未登记';
}

function muted(text: string): ReactNode {
  return <span className="text-idpxyz-breadcrumb">{text}</span>;
}

// 版式照公司主体对象页（票 business-parties-object-page/01）：头区一行元信息、不摆大指标卡；分区走共用模板的固定签——「概要」是
// 往来单位资料（ADR-0369）的读出与技术详情，「联系人」「资料版本」读同一份资料修订链（我方公司不摆，资料登在公司主体上），
// 「登记记录」是身份修订链，「关联」是全景（合作关系、客户账户、公司主体、供应商协议、结算政策）。
// 身份这一层（名称、编码、识别信息、状态、生效与登记版次）只在头区出现一次，与右栏头区同一份（票 business-party-ux/03）；
// 当前来源单据在「登记记录」折起时的摘要里，排查用的标识与依据收进技术详情。

function buildMeta(row: BusinessPartyRecord, related: number | undefined, identification: DetailMeta[]): DetailMeta[] {
  return [
    { label: '编码', value: <RefText value={row.partyId} name="编码" /> },
    ...identification,
    { label: '生效', value: <Instant value={row.effectiveFrom} precision="day" /> },
    ...(row.deactivatedAt ? [{ label: '停用', value: <Instant value={row.deactivatedAt} precision="day" /> }] : []),
    { label: '登记', value: updateOrdinalLabel(row.revision) },
    { label: '关联', value: related === undefined ? muted('…') : `${related} 项` },
  ];
}

/** 概要底部的技术详情：行名照原词，给对照排查的人（company-subject-ux「统一用语」表末那条）。 */
function technicalRowsOf(row: BusinessPartyRecord) {
  return [
    { label: '参与方标识', value: row.partyId },
    { label: '当前修订号', value: String(row.revision) },
    { label: '登记依据', value: row.basis },
    ...(row.deactivationBasis ? [{ label: '停用依据', value: row.deactivationBasis }] : []),
    { label: '租户', value: row.tenantId },
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
 * 按分区版式摆（票 company-subject-ux/13；版式对齐公司主体，票 business-parties-object-page/01）：概要、登记记录、关联一页纵排，
 * 登记记录默认折起——修订链要取数，折着时标题后显当前第几版与来源单据。
 */
export function BusinessPartyDetailPage({
  partyId,
  row,
  listAnswer,
  retry,
  relationships,
  retryRelationships,
  onBack,
  identityEditing,
  onEnterIdentityEdit,
  onExitIdentityEdit,
  profileEditing,
  onEnterProfileEdit,
  onExitProfileEdit,
  ours: pinned,
  roles,
}: {
  partyId: string;
  /** 册上对得上的那一行；册已取回却没有则为 null。 */
  row: BusinessPartyRecord | null;
  listAnswer: ApiResult<BusinessPartyListResponseBody> | null;
  retry: () => void;
  relationships: ApiResult<PartyRelationshipListResponseBody> | null;
  retryRelationships: () => void;
  onBack: () => void;
  /** 「更正名称」的编辑态：由地址判出（business-parties-address.ts 的 identityEditRequested）。 */
  identityEditing: boolean;
  /** 进编辑态：调用方改地址。 */
  onEnterIdentityEdit: () => void;
  /** 出编辑态：调用方改地址（后退或替换，由它按历史判）；`then` 是退出后还要去的地方，给了就替换地址再走。 */
  onExitIdentityEdit: (then?: () => void) => void;
  /** 「更新资料」的编辑态（ADR-0369）：由地址判出（business-parties-address.ts 的 profileEditRequested）。 */
  profileEditing: boolean;
  onEnterProfileEdit: () => void;
  onExitProfileEdit: (then?: () => void) => void;
  /**
   * 法人册上有没有公司主体钉着它，已停用的也算（business-party-list.ts 的 partiesPinnedByLegalEntities）：编辑态不摆注册号，也不摆往来单位资料。
   * null 是法人册还没答出；没取到（未配置、出错）由调用方答 false，写口照旧在服务端判。
   */
  ours: boolean | null;
  /** 这家单位此刻持有的角色（列表页的 rolesOfParty）；概要按它判要不要提示客户补联系人。 */
  roles: readonly string[];
}) {
  const ours = pinned === true;
  const [tab, setTab] = useState<WorkspaceTabId>('summary');
  const related = usePartyOverviewCount(partyId, relationships);
  const identificationMeta = useBusinessPartyIdentificationMeta(row);
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const grant = useRegistryWriteGrant();
  const mayWrite = grant.mayWrite;
  const viewState = viewStateOf(listAnswer, row, retry, mayWrite);
  // 查看者拿着编辑态地址进来（票 company-subject-section-edit/05）：授予已答出、明确没有写授予时清掉编辑键（替换地址）。加载期不动。
  useEffect(() => {
    if (identityEditing && grant.settled && !grant.mayWrite) onExitIdentityEdit();
  }, [identityEditing, grant.settled, grant.mayWrite, onExitIdentityEdit]);
  useEffect(() => {
    if (profileEditing && grant.settled && !grant.mayWrite) onExitProfileEdit();
  }, [profileEditing, grant.settled, grant.mayWrite, onExitProfileEdit]);
  // 往来单位资料（ADR-0369）：概要、联系人与资料版本几处读同一份修订链。我方公司的资料在公司主体上，不读；法人册答出之前
  // 也先不读，免得先发一笔再认出是我方公司。
  const profiles = useBusinessPartyProfiles(partyId, false, pinned === false);
  // 编辑态（票 business-party-edit-mode/02，照公司主体对象页）：概要分区换成「更正名称」表单，「保存 / 取消」钉在模板页脚；
  // 页头的「更正名称」「更多」收起。不是模态，「返回列表」有没提交的输入先问一句。
  const editing = identityEditing && mayWrite && row !== null;
  const editingProfile = profileEditing && !identityEditing && mayWrite && row !== null && !ours;
  const anyEditing = editing || editingProfile;
  const exitEditing = (then?: () => void) => (editingProfile ? onExitProfileEdit(then) : onExitIdentityEdit(then));
  const [footerSlot, setFooterSlot] = useState<HTMLElement | null>(null);
  const [unsaved, setUnsaved] = useState(false);
  // 要确认才做的那件事（放弃编辑态、放弃关系登记、离开）；null 即没在问。
  const [confirming, setConfirming] = useState<(() => void) | null>(null);
  const { addToast } = useToast();
  const knownParties = listAnswer?.kind === 'outcome' ? listAnswer.body.parties : null;
  // 全景区「合作关系」块就地登记（票 business-party-edit-mode/03）；与编辑态互斥，一页一个写口在开。
  const [registeringRelationship, setRegisteringRelationship] = useState(false);
  const [relationshipUnsaved, setRelationshipUnsaved] = useState(false);
  const knownRelationships = relationships?.kind === 'outcome' ? relationships.body.relationships : null;
  const startProfileEdit = () => {
    setRegisteringRelationship(false);
    onEnterProfileEdit();
  };
  // 资料空状态与客户提示里的「更新资料」与页头那一枚同一个去处；查看者、我方公司与编辑态中不摆。
  const profileUpdate = mayWrite && !ours && !anyEditing ? startProfileEdit : undefined;
  const relationshipRegistration =
    mayWrite && row && !anyEditing
      ? {
          active: registeringRelationship,
          onOpen: () => setRegisteringRelationship(true),
          form: (
            <PartyRelationshipRegistrationForm
              key={row.partyId}
              knownRelationships={knownRelationships}
              parties={listAnswer}
              preset={{ holder: row.partyId }}
              onRegistered={() => {
                addToast({ type: 'success', title: '合作关系已登记' });
                setRelationshipUnsaved(false);
                setRegisteringRelationship(false);
                retryRelationships();
              }}
              onUnsavedChange={setRelationshipUnsaved}
              inline={{
                onCancel: () =>
                  relationshipUnsaved ? setConfirming(() => () => setRegisteringRelationship(false)) : setRegisteringRelationship(false),
              }}
            />
          ),
        }
      : undefined;
  return (
    <>
    <DetailPageTemplate
      title={info.title}
      identifier={row?.partyName ?? partyId}
      status={row ? <StatusPill code={row.status} /> : undefined}
      headerActions={
        <>
          {row && !anyEditing ? (
            <>
              {mayWrite && !ours ? (
                <Button size="sm" variant="default" className={`${primaryActionClass} type-body`} onClick={startProfileEdit}>
                  更新资料
                </Button>
              ) : null}
              {mayWrite ? (
                <Button
                  size="sm"
                  variant={ours ? 'default' : 'outline'}
                  className={ours ? `${primaryActionClass} type-body` : 'type-body text-idpxyz-breadcrumb'}
                  onClick={() => {
                    setRegisteringRelationship(false);
                    onEnterIdentityEdit();
                  }}
                >
                  更正登记信息
                </Button>
              ) : null}
              <BusinessPartyMoreMenu row={row} parties={listAnswer} onChanged={retry} />
            </>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            className="type-body text-idpxyz-breadcrumb"
            onClick={() => {
              if (anyEditing) {
                if (unsaved) setConfirming(() => () => exitEditing(onBack));
                else exitEditing(onBack);
              } else if (registeringRelationship && relationshipUnsaved) setConfirming(() => onBack);
              else onBack();
            }}
          >
            返回列表
          </Button>
        </>
      }
      footer={anyEditing ? <div ref={setFooterSlot} /> : undefined}
      meta={row ? buildMeta(row, related, identificationMeta) : []}
      basicFields={[]}
      sections={
        row
          ? [
              ...(editing
                ? [
                    {
                      id: 'identity-edit',
                      title: '更正登记信息',
                      description: '改的是这家单位登记在册的名称与识别信息；合作关系、合同与账户照旧指向它。',
                      content: (
                        <BusinessPartyIdentityEditMode
                          key={row.partyId}
                          row={row}
                          ours={ours}
                          knownParties={knownParties}
                          footerSlot={footerSlot}
                          onLanded={(sent) => {
                            addToast({ type: 'success', title: `登记信息已更正${sent ? `，登记为${updateOrdinalLabel(Number(sent.revision))}` : ''}` });
                            setUnsaved(false);
                            retry();
                            onExitIdentityEdit();
                          }}
                          onUnsavedChange={setUnsaved}
                          onCancel={() => (unsaved ? setConfirming(() => () => onExitIdentityEdit()) : onExitIdentityEdit())}
                        />
                      ),
                    },
                  ]
                : []),
              ...(editingProfile
                ? [
                    {
                      id: 'profile-edit',
                      title: '更新资料',
                      description: '联系人、地址、其他语言名称、税号、开票抬头与收款账户；登记为新的一版，历次版本都保留。',
                      content: (
                        <BusinessPartyProfileEditMode
                          key={row.partyId}
                          partyId={row.partyId}
                          registrationCountry={row.identityLayerRegistered ? row.registrationCountry : undefined}
                          footerSlot={footerSlot}
                          onLanded={(sent) => {
                            addToast({ type: 'success', title: `资料已更新${sent ? `，登记为第 ${sent.revision} 版` : ''}` });
                            setUnsaved(false);
                            onExitProfileEdit();
                          }}
                          onUnsavedChange={setUnsaved}
                          onCancel={() => (unsaved ? setConfirming(() => () => onExitProfileEdit()) : onExitProfileEdit())}
                          onRequestLeave={(href) =>
                            unsaved ? setConfirming(() => () => onExitProfileEdit(() => (window.location.hash = href))) : onExitProfileEdit(() => (window.location.hash = href))
                          }
                        />
                      ),
                    },
                  ]
                : []),
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
            ]
          : undefined
      }
      tabs={
        row
          ? [
              ...(anyEditing
                ? []
                : [
                    {
                      id: 'summary' as const,
                      content: (
                        <section className="bg-idpxyz-bg p-5">
                          {ours ? (
                            <p className="type-value leading-6 text-idpxyz-breadcrumb">{ourCompanyProfileNote}</p>
                          ) : (
                            <BusinessPartyProfileReadout
                              partyId={row.partyId}
                              query={profiles}
                              registrationCountry={row.identityLayerRegistered ? row.registrationCountry : undefined}
                              roles={roles}
                              onUpdate={profileUpdate}
                            />
                          )}
                          <TechnicalDetails className="mt-4" rows={technicalRowsOf(row)} />
                        </section>
                      ),
                    },
                  ]),
              ...(ours || anyEditing
                ? []
                : [
                    {
                      id: 'contacts' as const,
                      content: (
                        <section className="bg-idpxyz-bg p-5">
                          <BusinessPartyContactsReadout partyId={row.partyId} query={profiles} onUpdate={profileUpdate} />
                        </section>
                      ),
                    },
                  ]),
              ...(ours
                ? []
                : [
                    {
                      id: 'profile-versions' as const,
                      collapsed: true,
                      summary: profileVersionsSummary(profiles, row.partyId),
                      content: (
                        <section className="bg-idpxyz-bg p-5">
                          <BusinessPartyProfileVersions partyId={row.partyId} query={profiles} onUpdate={profileUpdate} />
                        </section>
                      ),
                    },
                  ]),
              {
                id: 'identity-revisions',
                collapsed: true,
                summary: (
                  <>
                    当前{updateOrdinalLabel(row.revision)} · 来源单据 <span className="font-mono">{row.basis}</span>
                  </>
                ),
                content: (
                  <section className="bg-idpxyz-bg p-5">
                    <RevisionHistorySection register={businessPartyRevisionHistory} subjectId={row.partyId} revision={row.revision} />
                  </section>
                ),
              },
              {
                id: 'related',
                count: related,
                // 默认展开：来往来单位对象页多半就是要看它和我们有哪些往来。
                collapsed: false,
                content: (
                  <section className="bg-idpxyz-bg p-5">
                    <PartyOverviewSection
                      partyId={row.partyId}
                      relationships={relationships}
                      retryRelationships={retryRelationships}
                      relationshipRegistration={relationshipRegistration}
                      onRelationshipEnded={mayWrite && !anyEditing ? retryRelationships : undefined}
                      accountWrites={mayWrite && !anyEditing ? { parties: listAnswer } : undefined}
                    />
                  </section>
                ),
              },
            ]
          : undefined
      }
      activeTab={tab}
      onActiveTabChange={setTab}
      layout="sections"
      viewState={viewState}
    />
    <ConfirmDialog
      renderFooter={confirmDialogFooter}
      open={confirming !== null}
      tone="warning"
      title="放弃修改？"
      message="有内容还没有保存，离开后填写的内容不会保留。"
      confirmLabel="放弃"
      cancelLabel="继续编辑"
      onConfirm={() => {
        const run = confirming;
        setConfirming(null);
        run?.();
      }}
      onCancel={() => setConfirming(null)}
    />
    </>
  );
}
