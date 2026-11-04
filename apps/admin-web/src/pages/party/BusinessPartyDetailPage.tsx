import { useEffect, useState, type ReactNode } from 'react';
import { partyAnswersDomain } from './party-writes';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { BusinessPartyIdentityEditMode } from './BusinessPartyIdentityEditMode';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
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
import { PartyOverviewSection } from './PartyOverviewSection';
import { RevisionHistorySection, type RevisionHistoryRegister } from './RevisionHistorySection';
import { Instant } from './detail-primitives';
import { useRegistryWriteGrant } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['business-parties'];

/**
 * 对象页「时间线」签与工作台详情栏「身份与修订」签交给修订历史区的那几样（票 12 第 5 条立、票 13 第 9 条改用共用件）：
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
    // 不摆生命周期步进：状态药丸已经说了在哪一格，停用另有时点，步进是第二遍（用户 2026-11-03 裁定，公司主体两页同）。
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
            { label: '来源单据', value: row.basis },
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
 * 按分区版式摆（票 company-subject-ux/13）：概要与时间线一页纵排，时间线默认折起——修订链要取数，折着时标题后
 * 显当前第几版。指标带仍在头区，随滚动收起。
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
}) {
  const [tab, setTab] = useState<WorkspaceTabId>('summary');
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const grant = useRegistryWriteGrant();
  const mayWrite = grant.mayWrite;
  const viewState = viewStateOf(listAnswer, row, retry, mayWrite);
  // 查看者拿着编辑态地址进来（票 company-subject-section-edit/05）：授予已答出、明确没有写授予时清掉编辑键（替换地址）。加载期不动。
  useEffect(() => {
    if (identityEditing && grant.settled && !grant.mayWrite) onExitIdentityEdit();
  }, [identityEditing, grant.settled, grant.mayWrite, onExitIdentityEdit]);
  // 编辑态（票 business-party-edit-mode/02，照公司主体对象页）：概要分区换成「更正名称」表单，「保存 / 取消」钉在模板页脚；
  // 页头的「更正名称」「更多」收起。不是模态，「返回列表」有没提交的输入先问一句。
  const editing = identityEditing && mayWrite && row !== null;
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
  const relationshipRegistration =
    mayWrite && row && !editing
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
          {row && !editing ? (
            <>
              {mayWrite ? (
                <Button
                  size="sm"
                  variant="default"
                  className={`${primaryActionClass} text-[12px]`}
                  onClick={() => {
                    setRegisteringRelationship(false);
                    onEnterIdentityEdit();
                  }}
                >
                  更正名称
                </Button>
              ) : null}
              <BusinessPartyMoreMenu row={row} parties={listAnswer} onChanged={retry} />
            </>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            className="text-[12px] text-idpxyz-breadcrumb"
            onClick={() => {
              if (editing) {
                if (unsaved) setConfirming(() => () => onExitIdentityEdit(onBack));
                else onExitIdentityEdit(onBack);
              } else if (registeringRelationship && relationshipUnsaved) setConfirming(() => onBack);
              else onBack();
            }}
          >
            返回列表
          </Button>
        </>
      }
      footer={editing ? <div ref={setFooterSlot} /> : undefined}
      meta={row ? buildMeta(row) : []}
      summary={row ? buildSummary(row) : undefined}
      // 这张卡装的是停用的来源单据与技术详情；分区版式下「概要」已是分区标题，卡再叫概要就是上下两行同一个词。
      basicTitle="来源单据"
      basicFields={row && !editing ? buildBasicFields(row) : []}
      sections={
        row
          ? [
              ...(editing
                ? [
                    {
                      id: 'identity-edit',
                      title: '更正名称',
                      description: '改的是这家单位登记在册的名称；合作关系、合同与账户照旧指向它。',
                      content: (
                        <BusinessPartyIdentityEditMode
                          key={row.partyId}
                          row={row}
                          knownParties={knownParties}
                          footerSlot={footerSlot}
                          onLanded={(sent) => {
                            addToast({ type: 'success', title: `名称已更正${sent ? `，登记为${updateOrdinalLabel(Number(sent.revision))}` : ''}` });
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
                    relationshipRegistration={relationshipRegistration}
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
                collapsed: true,
                summary: `当前${updateOrdinalLabel(row.revision)}`,
                content: (
                  <section className="bg-idpxyz-bg p-5">
                    <RevisionHistorySection register={businessPartyRevisionHistory} subjectId={row.partyId} revision={row.revision} />
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
