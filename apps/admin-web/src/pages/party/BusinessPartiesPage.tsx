import { useEffect, useMemo, useRef, useState, type MutableRefObject, type ReactNode } from 'react';
import { useMarkUnsavedInput } from '../../templates/unsaved-input';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { usePushedAddress } from '../../templates/use-pushed-address';
import { AlertTriangle, ArrowLeft, Ban, Clock, Handshake, Info, MoreHorizontal, Plus } from 'lucide-react';
import { Button, ConfirmDialog, DropdownMenu, MenuItem, Tabs, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import { toggleWorkbenchSort, useTabReturn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { moduleInfoById } from '../../navigation';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { hasUnsavedDraft, registrationGroup } from './unsaved-guard';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneText } from '../../components/status-tone';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { catalogueConditionsNarrow, type CatalogueConditions, type CataloguePage } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword, type CatalogueCursor } from '../use-catalogue-cursor';
import { pastFirstPage } from '../shipment-request/cursor-pager';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import {
  listBusinessParties,
  listCustomerAccounts,
  listGroupLegalEntities,
  listPartyRelationships,
  type BusinessPartyListResponseBody,
  type BusinessPartyRecord,
  type PartyRelationshipRecord,
} from './api';
import { useBoundTenant } from '../use-operator-binding';
import { useVisited } from './keep-alive-tabs';
import { StatusPill } from '../../components/StatusPill';
import { labelOf, partyRoleLabels, type RelationshipStatusCode } from './presentation';
import { Instant, UnknownPartyName, filterSelectClass } from './detail-primitives';
import { useRegisterList } from './register-list';
import { BusinessPartyRegistrationForm } from './BusinessPartyRegistrationForm';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
import { IdentityDeactivationForm } from './IdentityDeactivationForm';
import {
  businessPartiesViewOf,
  businessPartyObjectHash,
  businessPartyRegisters,
  deactivationPresetOf,
  identityEditHash,
  identityEditRequested,
  profileEditHash,
  profileEditRequested,
  registerViewHash,
  listAfterRegistrationHash,
  withoutDeactivationPreset,
  type BusinessPartyRegister,
  type DeactivationPreset,
} from './business-parties-address';
import type { BusinessPartyDraft } from './business-party-form';
import type { PartyRelationshipDraft } from './party-relationship-form';
import { BusinessPartyDetailPage } from './BusinessPartyDetailPage';
import { BusinessPartyDetailPanel } from './BusinessPartyDetailPanel';
import { BusinessPartyTypeTags } from './BusinessPartyHeaderParts';
import {
  AttentionItem,
  BusinessPartiesPageHead,
  BusinessPartiesViewTabs,
  PartyFilterChip,
  PartyTypeChipRow,
  type RefreshOutcome,
  type ViewTab,
} from './BusinessPartiesChrome';
import { PartyWorkbench, type PartyWorkbenchColumn } from './PartyWorkbench';
import { PartyTile } from './party-role-visuals';
import {
  formatCount,
  partyAttentionCount,
  partyFilteredFooterCountText,
  partyFooterCountText,
  partyRelationshipCellOf,
  partyStatusOptionsOf,
  partyTileKindOf,
  partyTypeChipsFit,
  partyTypeChipsOf,
  statusCountsUnderConditions,
  tallyPartyRelationships,
  type PartyRelationshipTallies,
} from './business-parties-workbench';
import { EmptyState } from '../../components/states';
import { CountryLabel } from '../refdata-labels';
import { PartyRelationshipDetailPanel } from './PartyRelationshipDetailPanel';
import {
  businessPartyIsOfType,
  businessPartyListConditions,
  businessPartyNoMatchNote,
  businessPartyRegistrationCountry,
  businessPartyTypeFilterOptionsOf,
  businessPartyTypeFilterWords,
  businessPartyTypeKpisWhenComplete,
  businessPartyTypeNoMatchNote,
  countBusinessParties,
  defaultBusinessPartySort,
  effectiveBusinessPartyTypeFilter,
  filterBusinessPartiesByType,
  indexPartyRoles,
  ourCompanyPartyIds,
  partiesPinnedByLegalEntities,
  partyRegistrationCountriesOf,
  partyTypeSourcesComplete,
  partyTypeUnreadNote,
  rolesOfParty,
  type BusinessPartyStatusFilter,
  type BusinessPartyTypeFilter,
  type OurCompanyIndex,
  type PartyRoleIndex,
} from './business-party-list';
import {
  countPartyRelationships,
  defaultPartyRelationshipSort,
  partyRelationshipListConditions,
  partyRelationshipNeedsAttention,
  partyRelationshipNoMatchNote,
  partyRelationshipRoleFilterOptions,
  partyRelationshipStatusFilterOptions,
  type PartyRelationshipRoleFilter,
  type PartyRelationshipStatusFilter,
} from './party-relationship-list';
import { PageOnlyNote } from '../visibility/page-only';
import { completeRegisterRows } from '../complete-register';
import { OurCompanyNetworkPanel } from './OurCompanyNetworkPanel';
import { matchesOurCompanySearch, ourCompanyNetworkRows, type OurCompanyNetworkRow, type RoleCount } from './our-company-network';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['business-parties'];

/**
 * 本页三个视图。身份册与关系册分册而不并表，理由见 BusinessPartiesPage 头注。「我方关联」不是第三本登记册，是以我方公司为
 * 主体把关系册重新组合一遍（our-company-network.ts，票 business-party-relationship-graph/02）。
 */
type Book = 'identities' | 'relationships' | 'network';

// 视图签右端两项待办在册读不全时的来源：各发一问只带状态条件的读，取答复的总数（business-parties-workbench.ts 的
// partyAttentionCount）。写成模块级函数：取数层按读函数本身认缓存（register-list.ts）。
const listCandidateRelationships = () =>
  listPartyRelationships(partyRelationshipListConditions('', 'ALL', 'CANDIDATE', defaultPartyRelationshipSort));
const listRegisteredParties = () => listBusinessParties(businessPartyListConditions('', 'REGISTERED', defaultBusinessPartySort));

// 关系状态的圆点：候选与已生效是在用的两格，三种终局一色——终局之间的分别由状态徽章说，圆点只说「还在不在用」。
const relationshipStatusDotClass: Record<string, string> = {
  CANDIDATE: 'bg-sky-400',
  EFFECTIVE: 'bg-emerald-400',
  EXPIRED: 'bg-gray-400',
  REVOKED: 'bg-gray-400',
  SUPERSEDED: 'bg-gray-400',
};

// 身份本体册的列（票 business-party-redesign/02 的版式）。主列只留运营要看的：单位（图标块按类型取，名称，名称下是编码与
// 注册地）、类型（我方公司与持有的角色，成标签）、合作关系（它担任角色的在用关系有几段，其中待确认几段）、状态、生效时间、
// 登记时间；修订号与登记依据进详情栏底部的「技术详情」。注册地不单占一列：登了的不多，一列大半是「—」。停用时点与状态同格：
// 看这格的人要知道自何时起停用。登记时间上列是因为默认排序按它排（票 09 裁决 1）——排的键看不见，操作者判不出「为什么
// 这一条在最上面」。
// 各列定宽，多出来的宽度落在行尾，数据靠左成一块（PartyWorkbench.tsx）；列表窄了先收登记时间，再收生效时间与合作关系
// （两个时间列的门槛就是连它在内各列定宽之和：放得下就摆，行尾的箭头让给它），单位、类型、状态三列总在；右栏开着、三列也放不下时类型一列先收窄，名称那一列后让。时刻只显到日，完整时刻在悬停里。表头可排的只有生效时间与登记时间：排序下推到读口
// （business-party-list.ts 的 businessPartySortColumns），读口不按名称排。
// 类型与合作关系两列要关系册、客户账户册与法人册才算得出；它们没读全时（typeSourcesComplete 为假）表头旁带一个提示记号，
// 推不出来的格显「—」而不是「暂无合作关系」（票 01）。
function identityColumnsOf(
  roles: PartyRoleIndex,
  ours: OurCompanyIndex,
  countries: ReadonlyMap<string, string>,
  tallies: PartyRelationshipTallies,
  typeSourcesComplete: boolean,
): PartyWorkbenchColumn<BusinessPartyRecord>[] {
  // 推不出来的格：册读全了是真没有，画一个弱色的「—」；没读全只是没读到，同一个记号、另带说明（票 01）。
  const absent = typeSourcesComplete ? (
    <span className="text-idpxyz-breadcrumb">—</span>
  ) : (
    <span className="text-idpxyz-breadcrumb" title={partyTypeUnreadNote}>
      <span aria-hidden="true">—</span>
      <span className="sr-only">{partyTypeUnreadNote}</span>
    </span>
  );
  const unreadHint = typeSourcesComplete ? null : (
    <>
      {/* 警示色只落在图标上：图标是非文字，门槛 3:1；字仍是表头自己的颜色。 */}
      <Info className={`h-3 w-3 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
      <span className="sr-only">{partyTypeUnreadNote}</span>
    </>
  );
  return [
    {
      key: 'party',
      header: '单位',
      width: 380,
      render: (row) => {
        const country = businessPartyRegistrationCountry(row, countries);
        return (
          <div className="flex min-w-0 items-center gap-2.5">
            <PartyTile kind={partyTileKindOf(ours.has(row.partyId), rolesOfParty(roles, row.partyId))} />
            {/* 名称下一行是编码（对外对账、填单时认的那一串），登了注册地就跟在后面；没登的不占一格「—」。 */}
            <span className="flex min-w-0 flex-col">
              <span className="truncate type-section leading-5 text-idpxyz-textBright">{row.partyName}</span>
              <span className="truncate type-caption leading-4 text-idpxyz-breadcrumb">
                <span className="font-mono">{row.partyId}</span>
                {country ? (
                  <>
                    {' · '}
                    <CountryLabel code={country} />
                  </>
                ) : null}
              </span>
            </span>
          </div>
        );
      },
    },
    {
      key: 'type',
      header: (
        <span className="inline-flex items-center gap-1" title={typeSourcesComplete ? undefined : partyTypeUnreadNote}>
          类型
          {unreadHint}
        </span>
      ),
      width: 240,
      narrowWidth: 180,
      skeleton: 96,
      render: (row) => (
        <BusinessPartyTypeTags ours={ours.has(row.partyId)} roles={rolesOfParty(roles, row.partyId)} unread={!typeSourcesComplete} />
      ),
    },
    {
      key: 'relationships',
      header: (
        <span className="inline-flex items-center gap-1" title={typeSourcesComplete ? undefined : partyTypeUnreadNote}>
          合作关系
          {unreadHint}
        </span>
      ),
      width: 130,
      minListWidth: 900,
      skeleton: 44,
      render: (row) => {
        const cell = partyRelationshipCellOf(tallies, row.partyId, ours.has(row.partyId));
        if (cell === null) return absent;
        return (
          <span className="flex flex-col whitespace-nowrap">
            <span className="tabular-nums text-idpxyz-text">{formatCount(cell.count)} 段</span>
            {/* 待确认用色点带出来，字仍是弱色：琥珀色字落在选中行底色上不足 4.5:1，色点是非文字。 */}
            {cell.candidates > 0 ? (
              <span className="inline-flex items-center gap-1 type-caption leading-4 text-idpxyz-breadcrumb">
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-amber-500" aria-hidden="true" />
                {formatCount(cell.candidates)} 段待确认
              </span>
            ) : null}
          </span>
        );
      },
    },
    {
      key: 'status',
      header: '状态',
      width: 120,
      skeleton: 64,
      render: (row) => (
        <div className="flex flex-col items-start gap-0.5 whitespace-nowrap">
          <StatusPill code={row.status} />
          {row.deactivatedAt ? (
            <span className="type-caption leading-4 text-idpxyz-breadcrumb">
              自 <Instant value={row.deactivatedAt} precision="day" />
            </span>
          ) : null}
        </div>
      ),
    },
    {
      key: 'effective-from',
      header: '生效时间',
      width: 120,
      skeleton: 76,
      sortKey: 'effective-from',
      minListWidth: 990,
      // 待生效配一个琥珀色时钟而不是把时刻标成琥珀色：琥珀色字落在选中行底色上不足 4.5:1
      // （票 group-legal-entities-uiux/08 评审），图标是非文字，门槛 3:1。
      render: (row) => (
        <span className="inline-flex items-center gap-1 whitespace-nowrap tabular-nums text-idpxyz-text">
          {row.status === 'REGISTERED' ? (
            <Clock className={`h-3 w-3 shrink-0 ${statusToneText.warning}`} aria-label="尚未生效" />
          ) : null}
          <Instant value={row.effectiveFrom} precision="day" />
        </span>
      ),
    },
    {
      key: 'registered-at',
      header: '登记时间',
      width: 120,
      skeleton: 76,
      sortKey: 'registered-at',
      minListWidth: 1110,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums text-idpxyz-breadcrumb">
          <Instant value={row.registeredAt} precision="day" />
        </span>
      ),
    },
  ];
}

// 参与方格：只显名称，标识给悬停与技术详情；名称转写不到是写入门失败才会出现的悬空引用，如实标出并亮灯，不补占位文本。
// 宽度由列定（PartyWorkbench 的定宽列），长名称在格里截断。
function PartyCell({ id, name, nameKnown }: { id: string; name: string | undefined; nameKnown: boolean }) {
  return (
    <div className="flex min-w-0 items-center gap-1.5">
      {!nameKnown && <AlertTriangle className={`h-3 w-3 shrink-0 ${statusToneText.warning}`} aria-label="待查" />}
      <span className="truncate text-idpxyz-textBright" title={nameKnown ? `${name}（${id}）` : id}>
        {nameKnown ? name : <UnknownPartyName />}
      </span>
    </div>
  );
}

// 关系册的列。主列只留双方、角色、状态与有效期；关系标识、修订号、适用范围、依据与后继进详情栏底部的「技术详情」。方向由
// 持有方 → 相对方的两列次序表达（CONTEXT：方向由「哪一方对哪一方持有该角色」表达），单设一列没有第二份事实可载。
// 终止时点是状态格的内容：撤销或到期只影响生效边界后的新决定，看这格的人需要知道边界在哪。表头只有有效期可排：排序下推到
// 读口（party-relationship-list.ts 的 partyRelationshipSortColumns），读口不按双方排。列表窄了先收有效期：这一格最宽，
// 两端另在详情栏的事实条上；右栏开着时角色、对方、状态三列各收窄一些，好给持有方的名称留出位置，再窄状态一列也收。
const relationshipColumns: PartyWorkbenchColumn<PartyRelationshipRecord>[] = [
  {
    key: 'holder',
    header: '单位',
    width: 280,
    render: (row) => (
      <div className="flex min-w-0 items-center gap-1.5">
        <span
          className={`h-1.5 w-1.5 shrink-0 rounded-full ${relationshipStatusDotClass[row.status] ?? 'bg-idpxyz-textMuted'} ${
            partyRelationshipNeedsAttention(row) ? 'animate-pulse' : ''
          }`}
          aria-hidden="true"
        />
        <PartyCell id={row.holderId} name={row.holderName} nameKnown={row.holderNameKnown} />
      </div>
    ),
  },
  {
    key: 'role',
    header: '角色',
    width: 140,
    narrowWidth: 100,
    skeleton: 60,
    render: (row) => {
      const label = labelOf(partyRoleLabels, row.role);
      return (
        <span className="block truncate text-idpxyz-text" title={label}>
          {label}
        </span>
      );
    },
  },
  {
    key: 'counterparty',
    header: '对方',
    width: 280,
    narrowWidth: 170,
    skeleton: 150,
    render: (row) => <PartyCell id={row.counterpartyId} name={row.counterpartyName} nameKnown={row.counterpartyNameKnown} />,
  },
  {
    key: 'status',
    header: '状态',
    width: 130,
    narrowWidth: 120,
    // 右栏开着、列表窄到四列收到最窄也放不下时这一列不摆：行首的圆点仍说在不在用，选中的那一段的状态在右栏里。
    minListWidth: 570,
    skeleton: 64,
    render: (row) => (
      <div className="flex flex-col items-start gap-0.5 whitespace-nowrap">
        <StatusPill code={row.status} />
        {row.endedAt ? (
          <span className="type-caption leading-4 text-idpxyz-breadcrumb">
            自 <Instant value={row.endedAt} precision="day" />
          </span>
        ) : null}
      </div>
    ),
  },
  {
    key: 'validity',
    header: '有效期',
    width: 150,
    skeleton: 84,
    sortKey: 'effective-start',
    minListWidth: 960,
    // 起止各占一行、只显到日：排成一行、带到秒要两百多像素。无终点说「长期有效」；完整时刻在悬停里。
    render: (row) => (
      <div className="flex flex-col whitespace-nowrap tabular-nums leading-4 text-idpxyz-text">
        <span>
          <Instant value={row.effectiveStartsAt} precision="day" />
        </span>
        <span className="text-idpxyz-breadcrumb">
          → {row.effectiveEndsAt ? <Instant value={row.effectiveEndsAt} precision="day" /> : '长期有效'}
        </span>
      </div>
    ),
  },
];

/** 登记入口与登记视图的用词。册名取读册自己的词，不为登记另造说法；「停用」取身份状态格里的封闭词（已停用）。 */
const registerBookLabels: Record<BusinessPartyRegister, string> = {
  'business-party': '往来单位',
  'party-relationship': '合作关系',
  'identity-deactivation': '停用',
};

const registerActionLabels: Record<BusinessPartyRegister, string> = {
  'business-party': '新增往来单位',
  'party-relationship': '建立合作关系',
  'identity-deactivation': '停用…',
};

const registerHeadings: Record<BusinessPartyRegister, { title: string; subtitle: string }> = {
  'business-party': {
    title: '新增往来单位',
    subtitle: '先登记名称与编码；它是客户还是承运商，之后在「建立合作关系」里按角色登记',
  },
  'party-relationship': {
    title: '建立合作关系',
    subtitle: '记录谁对谁担任什么角色、从什么时候开始；没有批准信息的先作为待确认关系',
  },
  'identity-deactivation': {
    title: '停用',
    subtitle: '停用后不再按它办新业务，已有记录保留；公司主体与客户账户也在这里停用',
  },
};

/** 「我方关联」一行里按角色的段数：「客户 2 · 承运商代理 1」；关系册没答出显「—」，不拿 0 冒充「没有」。 */
function roleCountsText(counts: readonly RoleCount[], answered: boolean): string {
  if (!answered) return '—';
  if (counts.length === 0) return '—';
  return counts.map(({ role, count }) => `${labelOf(partyRoleLabels, role)} ${count}`).join(' · ');
}

/**
 * 登记视图里的三张表单（ADR-0085，票 admin-write-faces/02 切片 02c；自票 admin-web-group-legal-entities/10 起三册都是逐字段
 * 表单，ADR-0101 决定八自裁，JSON 快照签降为各表单底部的折叠区）。
 *
 * 第三本停用摆在本页，理由是本页读得见它刚写进去的那两件：停用每项必填 basis，而三个身份读面里只有身份本体册把
 * 停用时点与停用依据都渲染出来——法人册那格只有时点。登在哪里看得见结果就摆哪里，这是「写签跟着读签走」在本册
 * 上的落法。
 *
 * **不按 kind 切成两册**：停用口一个命令带种类，法人与客户账户的停用也走它。切册就得让每页拒收非本页那种 kind，
 * 那是管理台编一条服务端没有的约束——页面不教一条不真的规则。停用表单里种类是一格下拉，别处读面上结果的去处由
 * 格下那句说出。
 *
 * 换册即换表单：各册草稿是各自表单的内部状态，三册的格互不相容，一册的草稿从不带进另一册。访问过的表单切走后仍挂着、
 * 只藏起来，切回来草稿还在（票 party-commercial-uiux/09）：此前切走即卸载，切到别的册查一眼再回来，填了一半的整张表单
 * 就没了。离开登记视图才卸载，卸载前页面按三张表单报上来的「有没提交的输入」先确认。
 */
function RegisterForms({
  register,
  parties,
  knownParties,
  knownRelationships,
  onPartyRegistered,
  onRelationshipRegistered,
  onDeactivated,
  preset,
  reportUnsaved,
}: {
  register: BusinessPartyRegister;
  /** 页面持有的参与方列表答案：关系表单双方候选与停用表单参与方那一册都取它，不各自再读（票 13 第 5 条）。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  knownParties: readonly BusinessPartyRecord[] | null;
  knownRelationships: readonly PartyRelationshipRecord[] | null;
  /** 两张登记表单落册后的回调，带着送出的草稿（JSON 镜像那条路为 null）；页面据它重取并选中刚登的那一行。 */
  onPartyRegistered: (sent: BusinessPartyDraft | null) => void;
  onRelationshipRegistered: (sent: PartyRelationshipDraft | null) => void;
  onDeactivated: () => void;
  /** 深链带来的停用预填。 */
  preset: DeactivationPreset | null;
  reportUnsaved: Record<BusinessPartyRegister, (unsaved: boolean) => void>;
}) {
  const seen = useVisited(register);
  return (
    <>
      {seen('business-party') ? (
        <div className={register === 'business-party' ? 'contents' : 'hidden'}>
          <BusinessPartyRegistrationForm
            knownParties={knownParties}
            onRegistered={onPartyRegistered}
            onUnsavedChange={reportUnsaved['business-party']}
          />
        </div>
      ) : null}
      {seen('party-relationship') ? (
        <div className={register === 'party-relationship' ? 'contents' : 'hidden'}>
          <PartyRelationshipRegistrationForm
            knownRelationships={knownRelationships}
            parties={parties}
            onRegistered={onRelationshipRegistered}
            onUnsavedChange={reportUnsaved['party-relationship']}
          />
        </div>
      ) : null}
      {seen('identity-deactivation') ? (
        <div className={register === 'identity-deactivation' ? 'contents' : 'hidden'}>
          <IdentityDeactivationForm
            parties={parties}
            onDeactivated={onDeactivated}
            preset={preset}
            onUnsavedChange={reportUnsaved['identity-deactivation']}
          />
        </div>
      ) : null}
    </>
  );
}

const noUnsaved: Record<BusinessPartyRegister, boolean> = {
  'business-party': false,
  'party-relationship': false,
  'identity-deactivation': false,
};

/**
 * 一册的游标那一页只在列表视图里取：钩子挂在这一层，页面先按地址判出对象页或登记视图、提前返回时不渲它，也就不白取一页。
 * `load` 要是模块级的读口函数（useCatalogueCursor 的约定）。
 */
function CatalogueCursorScope<C extends CatalogueConditions, Body extends { page: CataloguePage }>({
  conditions,
  load,
  children,
}: {
  conditions: C;
  load: (conditions: C, after: string | null) => Promise<ApiResult<Body>>;
  children: (cursor: CatalogueCursor<Body>) => ReactNode;
}) {
  return <>{children(useCatalogueCursor(conditions, load))}</>;
}

/** 游标那一页此刻的样子，页头的刷新钮要的那两样：在不在途、答成了没有。 */
interface CursorRefreshState {
  pending: boolean;
  outcome: RefreshOutcome;
}

/**
 * 把游标那一页的重取、在途与结果报给页头的刷新钮。页头与视图签挂在三个视图之上、换视图不重挂（键盘在签上换视图，焦点才留得
 * 在签上），而游标挂在视图里面（CatalogueCursorScope）——页头够不着它，由视图报上去。不画任何东西。
 */
function CursorRefreshReport({
  retry,
  pending,
  outcome,
  retryRef,
  onReport,
}: {
  retry: () => void;
  pending: boolean;
  outcome: RefreshOutcome;
  retryRef: MutableRefObject<() => void>;
  onReport: (state: CursorRefreshState) => void;
}) {
  // retry 每次渲染都是新闭包（带着这一页的游标与页面上别的重取），每次渲染后都换上最新的一个。
  useEffect(() => {
    retryRef.current = retry;
  });
  const ok = outcome.ok;
  const message = outcome.ok ? undefined : outcome.message;
  useEffect(() => {
    onReport({ pending, outcome: ok ? { ok: true } : { ok: false, message } });
  }, [pending, ok, message, onReport]);
  return null;
}

/** 页头刷新钮收尾时报什么：答出来了（空册也是答出来了）报成功，别的把状态件的说明带上。在途时报什么都不会被用到。 */
function refreshOutcomeOf(viewState: TemplateViewState): RefreshOutcome {
  if (viewState.kind === 'ready' || viewState.kind === 'empty' || viewState.kind === 'loading') return { ok: true };
  return { ok: false, message: viewState.description };
}

/** 签上的总数取不带条件那一问答复的 page.total：那一问没有收窄，数的就是整册；没答出或读口不给总数就不显。 */
function registerTotalOf<Body extends { page: CataloguePage }>(answer: ApiResult<Body> | null): number | null {
  return answer?.kind === 'outcome' ? answer.body.page.total : null;
}

/**
 * 业务参与方（party-commercial）。两本读册各成一个视图：参与方身份本体册（单位名录）与参与方关系册（合作关系）。
 *
 * 身份本体册与关系册同页分册而不并进关系表：一个参与方既可以不是法人、也可以不在任何关系里，被停用的那种恰恰如此——
 * 身份生命周期的「已登记」与「已停用」两格只有在身份册上才有实例可显（票 admin-remainder-mechanism-batch/01 的补格裁定）。
 * 关系行对象是参与方关系的最新登记修订：承运商、承运商代理商、转售商、聚合平台与渠道账号持有人都以「双方 + 角色 +
 * 有效区间」的时态关系表达，代理关系不自动合并交易角色；关系登记按修订版本化不可覆盖。两册的状态代数不同——身份状态
 * 按时点导出、关系状态是登记进来的事实，所以分册而不是并表。
 *
 * 版式（票 business-party-redesign/02）：页头一行——页名、一句说明、刷新与登记入口；下面一行视图签——单位名录、合作关系、
 * 我方关联，签上带各自的总数，右端是待办（几段合作关系待确认、几家单位尚未生效，点了带着筛选跳过去）；再下面是各视图自己的
 * 工具条、表与表底翻页条（PartyWorkbench.tsx）。页头与视图签三个视图共用、换视图不重挂；各视图的检索、筛选、排序与选中行
 * 都挂在页面上，换视图再换回来还在。表头点排序、单击行在右栏开详情（可拖宽）、详情栏「更多」或对象地址开整页对象标签、
 * ↑/↓ 换行 Esc 收栏 `/` 检索。
 *
 * 两册的读口都在 ADR-0144 下（票 catalogue-read-pagination-rollout/11、14）：检索词、状态、角色与表头排序下推
 * （business-party-list.ts 的 businessPartyListConditions、party-relationship-list.ts 的 partyRelationshipListConditions），
 * 列表按游标翻页，换条件回第一页；游标只在列表视图里挂（CatalogueCursorScope），换视图、进登记视图或对象页再回来是重新挂上、
 * 回到第一页。类型与待查是页面从别的册推出来的，读口筛不了，在到手的那一页上筛。筛选项上的数只在整册一页装得下时摆
 * （complete-register.ts 的 completeRegisterRows）：那时按整册数的是真数，页内筛也是精确的；摆出来的数随别的条件走、不随
 * 这一项自己走（选了角色，各状态只数这个角色的），当前条件的那一问还没答出就先不摆，不拿 0 占位；装不下时一页里数出来的不是
 * 册上的数，数不摆，类型与待查改成标「只筛本页」的控件，当前条件下共几条由表底说。名录的类型另有一道：推类型用的那几本册要都
 * 读全，带数的类型芯片与「暂无合作关系」才作数，否则类型格推不出来的显「—」并标明（business-party-list.ts 文件头）。
 *
 * 登记从页头进登记视图：「建立合作关系」「新增往来单位」两枚按钮位置固定、不随视图换，停用收进旁边的「更多」。登记视图住在地址上
 * （`?register=<册>`，判读在 business-parties-address.ts）：刷新仍停在登记视图、换册只换键值不压历史，浏览器后退回列表。
 * 草稿有没提交的输入时，页内「返回列表」先确认，刷新与关页由 beforeunload 拦；浏览器后退不拦——拦它得改写历史栈，壳层按
 * hashchange 记每张标签的地址，改写会让两边对不上。
 *
 * 页面另持有两册不带条件的那一页（票 10 第 5 条）：登记视图从这份答案取修订号建议与双方候选，类型列的角色索引、对象页与
 * 签上的总数也取它；登记册答 REGISTERED / DEACTIVATED 时触发对应读册重取，两册各自一份重取序号——登一段关系不必重取参与方册，
 * 反过来也一样。
 */
export function BusinessPartiesPage() {
  const parties = useRegisterList(listBusinessParties);
  const relationships = useRegisterList(listPartyRelationships);
  // 法人册只为类型列的「我方公司」读（business-party-list.ts 文件头）；读不到就没有一行按我方公司显，列表照常。
  const entities = useRegisterList(listGroupLegalEntities);
  // 客户账户册只为类型列的「客户」读（ADR-0367 决定四）：账户的所属客户持有客户角色，不要求先登客户关系；读不到就只按关系算。
  const accounts = useRegisterList(listCustomerAccounts);
  // 页名下的「集团」是 ADR-0003 三级边界的第一级，即本会话绑定的租户；答不出就不显，不拿列表行上的 tenantId 顶。
  const boundTenant = useBoundTenant();
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const { addToast } = useToast();
  const tabReturn = useTabReturn();
  // 渲列表、登记视图还是对象页由地址定：对象地址在壳层是自己的一张标签，本组件在那张标签里渲对象页。
  const [hash, setHash] = useState(() => window.location.hash);
  const view = businessPartiesViewOf(hash);
  // 从列表压进登记视图的用后退回列表，不再压一条；刷新、深链、或压完之后地址被别处动过的（切过标签）那时后退一格不一定是
  // 列表，改用替换——判法在 usePushedAddress。
  const pushedRegister = usePushedAddress();
  // 对象页「更正名称」的编辑态（票 business-party-edit-mode/02）与登记视图同一套历史收口：自己压进去的用后退退出，刷新或
  // 直接落在编辑态的用替换；退出后还要去别处时替换再走，并同步 setHash——紧接着切标签时 hashchange 回调已不归本标签，
  // useOwnAddressChange 会把它滤掉。
  const pushedIdentityEdit = usePushedAddress();
  const pushedProfileEdit = usePushedAddress();
  const enterProfileEdit = () => pushedProfileEdit.push(profileEditHash(window.location.hash, true));
  const exitProfileEdit = (then?: () => void) => {
    if (then === undefined && pushedProfileEdit.back()) return;
    pushedProfileEdit.forget();
    const next = profileEditHash(window.location.hash, false);
    window.location.replace(next);
    setHash(next);
    then?.();
  };
  const enterIdentityEdit = () => pushedIdentityEdit.push(identityEditHash(window.location.hash, true));
  const exitIdentityEdit = (then?: () => void) => {
    if (then === undefined && pushedIdentityEdit.back()) return;
    pushedIdentityEdit.forget();
    const next = identityEditHash(window.location.hash, false);
    window.location.replace(next);
    setHash(next);
    then?.();
  };
  // 深链（法人页「停用此责任法人…」、本页详情栏「停用此参与方…」）带来的停用预填：读到就交给停用表单，然后从地址里去掉，
  // 免得换标签回来、页面重新挂载时再预填一次。这一页的标签已开着时地址变了也认：壳层不重挂已开的标签。
  const [preset, setPreset] = useState<DeactivationPreset | null>(null);
  const [unsaved, setUnsaved] = useState<Record<BusinessPartyRegister, boolean>>(noUnsaved);
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const [book, setBook] = useState<Book>('identities');

  // 身份册的列表状态。检索词放在地址 `?q=`：别处的引用链接带着它过来，这一册挂载时已经筛到那一行。
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<BusinessPartyStatusFilter>('ALL');
  const [type, setType] = useState<BusinessPartyTypeFilter>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultBusinessPartySort);
  const [selectedParty, setSelectedParty] = useState<string | null>(null);
  // 详情栏全景签里的关系登记不是模态（票 business-party-edit-mode/03）：换行、关栏会把表单连草稿一起卸掉，有没提交的输入时先问一句。
  // 登记簿按组查（unsaved-guard.ts）：列表视图里挂着的关系表单只有详情栏这一份。
  const [pendingSelect, setPendingSelect] = useState<{ key: string | null } | null>(null);
  const selectParty = (key: string | null) => {
    // 右栏关联签里就地开着的两种表单（登记合作关系、新增客户账户）有没提交的输入时，换行先问一句。
    if (key !== selectedParty && (hasUnsavedDraft(registrationGroup('party-relationship')) || hasUnsavedDraft(registrationGroup('customer-account'))))
      setPendingSelect({ key });
    else setSelectedParty(key);
  };
  // 关系册的列表状态。检索留在页内：停用入口与引用链接定位的是身份这一册。
  const [relationshipSearch, setRelationshipSearch] = useState('');
  const [role, setRole] = useState<PartyRelationshipRoleFilter>('ALL');
  const [relationshipStatus, setRelationshipStatus] = useState<PartyRelationshipStatusFilter>('ALL');
  const [attentionOnly, setAttentionOnly] = useState(false);
  const [relationshipSort, setRelationshipSort] = useState<WorkbenchSort>(defaultPartyRelationshipSort);
  const [selectedRelationship, setSelectedRelationship] = useState<string | null>(null);
  // 「我方关联」一册：检索只在手上这几行里找（our-company-network.ts 的 matchesOurCompanySearch）。
  const [networkSearch, setNetworkSearch] = useState('');
  const [selectedOurCompany, setSelectedOurCompany] = useState<string | null>(null);
  // 发出去的检索词等它停稳（useSettledKeyword）；框里的字照旧逐字跟手。钩子不能挂在下面的提前返回之后，两册的都在这里取。
  const settledSearch = useSettledKeyword(search);
  const settledRelationshipSearch = useSettledKeyword(relationshipSearch);

  // 地址是位置的唯一权威。停用预填读到就收下并从地址去掉（理由见 withoutDeactivationPreset），之后视图按去掉后的地址判。
  // 只读本标签名下的地址（useOwnAddressChange）：藏着的这一页若也读，会把别的标签地址上的预填剥掉。
  const readAddress = () => {
    const current = window.location.hash;
    const next = deactivationPresetOf(current);
    if (next === null) {
      setHash(current);
      return;
    }
    setPreset(next);
    const cleaned = withoutDeactivationPreset(current);
    window.history.replaceState(window.history.state, '', cleaned);
    setHash(cleaned);
  };
  // 挂载时读一次：首个地址是打开本标签的那一个，必在本标签名下；此后经 useOwnAddressChange。
  useEffect(readAddress, []);
  useOwnAddressChange(readAddress);

  const anyUnsaved = Object.values(unsaved).some(Boolean);
  // 刷新与关页时浏览器自己弹确认；只在登记视图且有表单报了没提交的输入时挂。表单自己也挂了一份（useUnsavedGuard），
  // 停用表单关掉了它（预填不算输入），这里按表单对着带预填的基线报上来的那一格再兜一次。
  const guardUnload = view.kind === 'register' && anyUnsaved;
  useMarkUnsavedInput(guardUnload);
  useEffect(() => {
    if (!guardUnload) return;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [guardUnload]);

  // 三张表单各一个稳定的回调：表单在卸载时报 false，回调换了引用会在每次渲染多报一轮。
  const reportUnsaved = useMemo(() => {
    const entries = businessPartyRegisters.map(
      (id) =>
        [
          id,
          (next: boolean) => setUnsaved((previous) => (previous[id] === next ? previous : { ...previous, [id]: next })),
        ] as const,
    );
    return Object.fromEntries(entries) as Record<BusinessPartyRegister, (unsaved: boolean) => void>;
  }, []);

  const replaceHash = (next: string) => {
    window.history.replaceState(window.history.state, '', next);
    setHash(next);
  };
  const openRegister = (register: BusinessPartyRegister) => {
    // 已在登记视图里换册只换键值、不压历史：三张表单是一个视图的三页，后退一次就该回列表。
    if (view.kind === 'register') {
      replaceHash(registerViewHash(window.location.hash, register));
      return;
    }
    pushedRegister.push(registerViewHash(window.location.hash, register));
  };
  const leaveRegister = () => {
    setConfirmingLeave(false);
    setPreset(null);
    if (pushedRegister.back()) return;
    window.location.replace(registerViewHash(window.location.hash, null));
  };
  const requestLeaveRegister = () => (anyUnsaved ? setConfirmingLeave(true) : leaveRegister());
  // 登完就去看结果（票 business-party-ux/06）：回列表、选中刚登的那一行，右栏随之打开。离开登记视图会把表单连它的答复区一起
  // 卸掉，所以先报一句 toast，成功不至于没有任何文字反馈。地址用替换而不是后退——后退会带回进登记视图之前那一条地址上的
  // 检索词，刚登的那一行可能正被它筛掉；状态与类型筛选同理由 selectRegistered 清掉。别的表单还有没提交的输入时不自动离开
  // （离开会把那份草稿连表单一起卸掉），只报 toast 与重取，人自己走。JSON 镜像那条路交回 null：不知道登了谁，只回列表。
  const showRegistered = (title: string, otherUnsaved: boolean, selectRegistered: (() => void) | null) => {
    addToast({ type: 'success', title });
    if (otherUnsaved) return;
    setConfirmingLeave(false);
    setPreset(null);
    pushedRegister.forget();
    window.location.replace(listAfterRegistrationHash(window.location.hash));
    selectRegistered?.();
  };
  const onPartyRegistered = (sent: BusinessPartyDraft | null) => {
    parties.retry();
    showRegistered(
      '往来单位已登记',
      unsaved['party-relationship'] || unsaved['identity-deactivation'],
      sent &&
        (() => {
          setBook('identities');
          setStatus('ALL');
          setType('ALL');
          setSelectedParty(sent.partyId);
        }),
    );
  };
  const onRelationshipRegistered = (sent: PartyRelationshipDraft | null) => {
    relationships.retry();
    showRegistered(
      '合作关系已登记',
      unsaved['business-party'] || unsaved['identity-deactivation'],
      sent &&
        (() => {
          setBook('relationships');
          setRelationshipSearch('');
          setRole('ALL');
          setRelationshipStatus('ALL');
          setAttentionOnly(false);
          setSelectedRelationship(sent.relationshipId);
        }),
    );
  };

  // 列表没取到（加载中 / 未配置 / 出错）传 null：表单那边建议修订号一律为 1，不拿空数组冒充「册上没有」。
  const knownParties = parties.answer?.kind === 'outcome' ? parties.answer.body.parties : null;
  const knownRelationships = relationships.answer?.kind === 'outcome' ? relationships.answer.body.relationships : null;
  // 类型列按关系册、客户账户册与法人册算（business-party-list.ts 文件头）；哪一册没取到，那一册先不贡献，取到后自动补上。
  const knownEntities = entities.answer?.kind === 'outcome' ? entities.answer.body.entities : null;
  const knownAccounts = accounts.answer?.kind === 'outcome' ? accounts.answer.body.accounts : null;
  const roleIndex = useMemo(() => indexPartyRoles(knownRelationships, knownAccounts), [knownRelationships, knownAccounts]);
  const ourCompanies = useMemo(() => ourCompanyPartyIds(knownEntities), [knownEntities]);
  const pinnedParties = useMemo(() => partiesPinnedByLegalEntities(knownEntities), [knownEntities]);
  const networkRows = useMemo(() => ourCompanyNetworkRows(knownEntities, knownRelationships ?? []), [knownEntities, knownRelationships]);
  const entityCountries = useMemo(() => partyRegistrationCountriesOf(knownEntities), [knownEntities]);
  // 不带条件的那一页若已是整册就交出它的行（complete-register.ts）：筛选项上的数、视图签右端的待办都按它数；不是就 null。
  const completeParties = completeRegisterRows(parties.answer?.kind === 'outcome' ? parties.answer.body : null, (body) => body.parties);
  const completeRelationships = completeRegisterRows(
    relationships.answer?.kind === 'outcome' ? relationships.answer.body : null,
    (body) => body.relationships,
  );
  // 推类型用的那几本册读全了没有（business-party-list.ts 文件头）。没读全——多于一页、还没答出、没读成——类型一列就只是
  // 读到的那一部分：推不出类型的格显「—」，按类型的数不摆，「暂无合作关系」这一项不给筛。
  const typeSourcesComplete = partyTypeSourcesComplete([
    completeRelationships,
    completeRegisterRows(accounts.answer?.kind === 'outcome' ? accounts.answer.body : null, (body) => body.accounts),
    completeRegisterRows(entities.answer?.kind === 'outcome' ? entities.answer.body : null, (body) => body.entities),
  ]);
  // 视图签右端两项待办的数，册读不全时才另问（读全了从手上的行数）；只在列表视图里问。
  const candidateRelationships = useRegisterList(listCandidateRelationships, {
    enabled: view.kind === 'list' && knownRelationships !== null && completeRelationships === null,
  });
  const registeredParties = useRegisterList(listRegisteredParties, {
    enabled: view.kind === 'list' && knownParties !== null && completeParties === null,
  });
  // 名录与关系两个视图的那一页游标挂在视图里面，页头的刷新钮经这两样够到它（CursorRefreshReport）。
  const cursorRetry = useRef<() => void>(() => {});
  const [cursorRefresh, setCursorRefresh] = useState<CursorRefreshState>({ pending: true, outcome: { ok: true } });
  const relationshipTallies = useMemo(() => tallyPartyRelationships(knownRelationships), [knownRelationships]);
  const identityColumns = useMemo(
    () => identityColumnsOf(roleIndex, ourCompanies, entityCountries, relationshipTallies, typeSourcesComplete),
    [roleIndex, ourCompanies, entityCountries, relationshipTallies, typeSourcesComplete],
  );

  if (view.kind === 'object') {
    // 对象页的行从不带条件的那一页里认（BusinessPartyDetailPage 的 viewStateOf），与登记表单的候选同一份。
    const row = knownParties?.find((party) => party.partyId === view.partyId) ?? null;
    return (
      <BusinessPartyDetailPage
        partyId={view.partyId}
        row={row}
        listAnswer={parties.answer}
        retry={parties.retry}
        relationships={relationships.answer}
        retryRelationships={relationships.retry}
        onBack={() => tabReturn.returnTo('business-parties')}
        identityEditing={identityEditRequested(hash)}
        onEnterIdentityEdit={enterIdentityEdit}
        onExitIdentityEdit={exitIdentityEdit}
        profileEditing={profileEditRequested(hash)}
        onEnterProfileEdit={enterProfileEdit}
        onExitProfileEdit={exitProfileEdit}
        ours={entities.answer == null ? null : pinnedParties.has(view.partyId)}
        roles={rolesOfParty(roleIndex, view.partyId)}
      />
    );
  }

  // 地址里的登记视图可能是别人发来的链接：没有登记写授予时不进，落回列表。
  if (view.kind === 'register' && mayWrite) {
    const heading = registerHeadings[view.register];
    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        <div className="flex shrink-0 flex-wrap items-center gap-4 border-b border-idpxyz-border px-6 py-3">
          <Button variant="ghost" size="sm" className="type-body text-idpxyz-breadcrumb" onClick={requestLeaveRegister}>
            <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
            返回列表
          </Button>
          <div className="min-w-0">
            <h1 className="type-title font-semibold leading-tight text-idpxyz-textBright">{heading.title}</h1>
            <p className="truncate type-body text-idpxyz-breadcrumb">{heading.subtitle}</p>
          </div>
          {/* 三本册的切换：换册只换地址键值（openRegister），切走的表单留着挂载。 */}
          <Tabs
            value={view.register}
            onValueChange={(next) => openRegister(next as BusinessPartyRegister)}
            className="ml-auto shrink-0 gap-0"
          >
            <TabsList aria-label="选择登记册">
              {businessPartyRegisters.map((id) => (
                <TabsTrigger key={id} value={id} className="type-body text-idpxyz-breadcrumb">
                  {registerBookLabels[id]}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
        </div>
        <RegisterForms
          register={view.register}
          parties={parties.answer}
          knownParties={knownParties}
          knownRelationships={knownRelationships}
          onPartyRegistered={onPartyRegistered}
          onRelationshipRegistered={onRelationshipRegistered}
          onDeactivated={parties.retry}
          preset={preset}
          reportUnsaved={reportUnsaved}
        />
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirmingLeave}
          tone="warning"
          title="离开这一页？"
          message="有内容还没有保存，离开后填写的内容不会保留。"
          confirmLabel="离开"
          cancelLabel="继续填写"
          onConfirm={leaveRegister}
          onCancel={() => setConfirmingLeave(false)}
        />
      </div>
    );
  }

  const partyTotal = registerTotalOf(parties.answer);
  const relationshipTotal = registerTotalOf(relationships.answer);
  // 页名下不写集团标识：一家公司一套系统，租户是谁不必告诉运营；绑定仍读着，壳层顶栏显公司名用它。
  void boundTenant;

  // 写入口位置固定、不随视图换：人记的是位置。没有登记写授予时整组不摆（ADR-0315）。一家往来单位都没有时关系无从建起，
  // 「建立合作关系」点不动并说明原因。
  const noParties = partyTotal === 0;
  const headActions = mayWrite ? (
    <>
      <Button
        variant="outline"
        disabled={noParties}
        title={noParties ? '先登记往来单位，才能建立合作关系' : undefined}
        onClick={() => openRegister('party-relationship')}
      >
        <Handshake className="mr-1.5 h-4 w-4" aria-hidden="true" />
        {registerActionLabels['party-relationship']}
      </Button>
      <Button variant="default" className={primaryActionClass} onClick={() => openRegister('business-party')}>
        <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
        {registerActionLabels['business-party']}
      </Button>
      <DropdownMenu
        align="right"
        trigger={
          <Button variant="outline" className="px-2 text-idpxyz-breadcrumb" aria-label="更多登记入口">
            <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
          </Button>
        }
      >
        <MenuItem onSelect={() => openRegister('identity-deactivation')}>
          <Ban className="mr-2 h-3.5 w-3.5" aria-hidden="true" />
          {registerActionLabels['identity-deactivation']}
        </MenuItem>
      </DropdownMenu>
    </>
  ) : undefined;

  // 签上的数只在拿到业务答案后摆：未配置态与错误态下报「0」会与状态区「这不是目录为空」直接矛盾。
  const viewTabs: ViewTab<Book>[] = [
    { id: 'identities', label: '单位名录', count: partyTotal },
    { id: 'relationships', label: '合作关系', count: relationshipTotal },
    { id: 'network', label: '我方关联', count: knownEntities !== null ? networkRows.length : null },
  ];

  // 视图签右端的待办：为零或数不出就不出现；点了带着对应的状态筛选落到那个视图，别的筛选清掉——留着的话落过去可能一行都没有。
  const candidateCount = partyAttentionCount(
    completeRelationships,
    (row) => row.status === 'CANDIDATE',
    registerTotalOf(candidateRelationships.answer),
  );
  const registeredCount = partyAttentionCount(completeParties, (row) => row.status === 'REGISTERED', registerTotalOf(registeredParties.answer));
  const showCandidateRelationships = () => {
    setBook('relationships');
    setRelationshipSearch('');
    setRole('ALL');
    setRelationshipStatus('CANDIDATE');
    setAttentionOnly(false);
  };
  const showRegisteredParties = () => {
    setBook('identities');
    setSearch('');
    setStatus('REGISTERED');
    setType('ALL');
  };
  const attention =
    candidateCount || registeredCount ? (
      <>
        {candidateCount ? (
          <AttentionItem onClick={showCandidateRelationships}>{formatCount(candidateCount)} 段合作关系待确认</AttentionItem>
        ) : null}
        {registeredCount ? <AttentionItem onClick={showRegisteredParties}>{formatCount(registeredCount)} 家单位尚未生效</AttentionItem> : null}
      </>
    ) : undefined;

  // 每个视图交出两样：页头刷新钮要的那一组（怎么重取、在不在途、答成了没有），与视图签下面的那一块。
  let refresh: { run: () => void; pending: boolean; outcome: RefreshOutcome };
  let body: ReactNode;

  if (book === 'network') {
    const visible = networkRows.filter((row) => matchesOurCompanySearch(row, networkSearch));
    const relationshipsAnswered = knownRelationships !== null;
    // 我方公司、对它担任角色的单位、待确认三列总在；列表窄了先收公司主体，再收「它对其担任角色」（右栏开着时就是这样）。
    // 公司名称这一列给得宽：这个视图一行就是一家公司，名称是唯一认得出它的东西。
    const networkColumns: PartyWorkbenchColumn<OurCompanyNetworkRow>[] = [
      {
        key: 'company',
        header: '我方公司',
        width: 340,
        render: (row) => (
          <div className="flex min-w-0 items-center gap-2.5">
            <PartyTile kind="OURS" />
            <span className="flex min-w-0 flex-col">
              <span className="truncate type-section leading-5 text-idpxyz-textBright">{row.partyName ?? <UnknownPartyName />}</span>
              <span className="truncate font-mono type-caption leading-4 text-idpxyz-breadcrumb">{row.partyId}</span>
            </span>
          </div>
        ),
      },
      {
        key: 'toward',
        header: '对它担任角色的单位',
        width: 280,
        narrowWidth: 200,
        skeleton: 140,
        render: (row) => <span className="block truncate text-idpxyz-text">{roleCountsText(row.towardIt, relationshipsAnswered)}</span>,
      },
      {
        key: 'from',
        header: '它对其担任角色',
        width: 280,
        skeleton: 140,
        minListWidth: 990,
        render: (row) => <span className="block truncate text-idpxyz-text">{roleCountsText(row.fromIt, relationshipsAnswered)}</span>,
      },
      {
        key: 'candidates',
        header: '待确认',
        width: 90,
        narrowWidth: 64,
        skeleton: 24,
        render: (row) =>
          relationshipsAnswered && row.candidates > 0 ? (
            <span className="tabular-nums text-idpxyz-text">{formatCount(row.candidates)}</span>
          ) : (
            <span className="text-idpxyz-breadcrumb">—</span>
          ),
      },
      {
        key: 'entities',
        header: '公司主体',
        width: 200,
        skeleton: 110,
        minListWidth: 1190,
        render: (row) => <span className="block truncate font-mono type-caption text-idpxyz-breadcrumb">{row.legalEntityIds.join('、')}</span>,
      },
    ];
    // 合成节点与「已结束」那句点了：切到合作关系视图、按这家公司的编码检索，看它名下的全部关系。
    const showRelationshipsOf = (partyId: string) => {
      setBook('relationships');
      setRelationshipSearch(partyId);
      setRole('ALL');
      setRelationshipStatus('ALL');
      setAttentionOnly(false);
      setSelectedRelationship(null);
    };
    const viewState = catalogueViewState(entities.answer, networkRows.length, entities.retry, {
      module: info,
      endpoint: 'GET /commercial-group-legal-entities',
      emptyTitle: '还没有在用的我方公司',
      emptyDescription: '我方公司在「公司主体」页登记；登记之后，这里按公司列出各自的合作关系。',
      narrowed: networkSearch.trim() !== '',
    });
    refresh = {
      run: () => {
        entities.retry();
        relationships.retry();
      },
      pending: entities.pending || relationships.pending,
      outcome: refreshOutcomeOf(viewState),
    };
    const networkNote = '以我方每家公司为主体：谁对它担任什么角色、它又对谁担任什么角色；选一家在右边看它的关系图';
    body = (
      <PartyWorkbench<OurCompanyNetworkRow>
        key="network"
        viewStateKey="business-parties.network"
        search={{ value: networkSearch, onChange: setNetworkSearch, placeholder: '搜我方公司名称、编码或公司主体' }}
        filters={() => (
          <>
            {/* 段数不全的那一句排在前面、不参与截断：它是提醒，后面那句只是说明。 */}
            {completeRelationships === null && relationshipsAnswered ? <PageOnlyNote>合作关系册不止一页，段数只按第一页数</PageOnlyNote> : null}
            <span className="min-w-0 truncate type-caption text-idpxyz-breadcrumb" title={networkNote}>
              {networkNote}
            </span>
          </>
        )}
        columns={networkColumns}
        rows={visible}
        rowKey={(row) => row.partyId}
        selectedKey={selectedOurCompany}
        onSelect={setSelectedOurCompany}
        noMatchText="没有匹配的我方公司"
        onClearFilters={() => setNetworkSearch('')}
        // 检索只在手上这几行里找，摆出来几家就是几家。
        footer={{ countText: partyFooterCountText(visible.length, visible.length, '家') }}
        renderDetail={(key) => {
          const row = networkRows.find((candidate) => candidate.partyId === key);
          return row ? (
            <OurCompanyNetworkPanel
              row={row}
              relationshipsComplete={completeRelationships !== null}
              onClose={() => setSelectedOurCompany(null)}
              onShowRelationships={() => showRelationshipsOf(row.partyId)}
            />
          ) : null;
        }}
        viewState={viewState}
      />
    );
  } else if (book === 'identities') {
    const conditions = businessPartyListConditions(settledSearch, status, sort);
    // 整册一页装得下时按整册计数（complete-register.ts）；装不下就 null，筛选项上不摆数。
    const counts = completeParties === null ? null : countBusinessParties(completeParties, roleIndex, ourCompanies);
    // 按类型的数还要推类型用的册都读全才作数（businessPartyTypeKpisWhenComplete，票 01）：作数时类型是一排带数的芯片，
    // 运营问的「有几家客户、几家承运商」就在芯片上，一个筛选只留这一个入口；不作数时类型改由下拉筛，下拉自己写明原因
    // （businessPartyTypeFilterWords）。
    const typeCountsUsable = businessPartyTypeKpisWhenComplete(counts, typeSourcesComplete) !== null;
    const typeWords = businessPartyTypeFilterWords(completeParties !== null, typeSourcesComplete);
    // 没读全时「暂无合作关系」不给筛，记着的这一项当作没筛（effectiveBusinessPartyTypeFilter）。
    const shownType = effectiveBusinessPartyTypeFilter(type, typeSourcesComplete);
    // 类型这一件等名录与推类型的三本册都答过一次再摆：名录没答成时它筛不了任何东西；别的册还在途时摆出来的只会是「只筛本页」
    // 的下拉，一两百毫秒后又换成芯片。答过之后重取在途照旧显上一份（register-list.ts），刷新时这一件不会消失再出现。
    const typeControlReady = knownParties !== null && relationships.answer !== null && accounts.answer !== null && entities.answer !== null;
    const narrowed = search.trim() !== '' || status !== 'ALL' || shownType !== 'ALL';
    const clearFilters = () => {
      setSearch('');
      setStatus('ALL');
      setType('ALL');
    };
    const emptyTitle = '还没有登记任何往来单位';
    const emptyDescription = `客户、承运商、代理都从这里登记。${entryHint(mayWrite, '用页头的「新增往来单位」登记第一家。')}`;
    refresh = { run: () => cursorRetry.current(), ...cursorRefresh };
    body = (
      <CatalogueCursorScope key="identities" conditions={conditions} load={listBusinessParties}>
        {({ answer, pagination, retry }) => {
          const pageRows = answer?.kind === 'outcome' ? answer.body.parties : [];
          const visible = filterBusinessPartiesByType(pageRows, shownType, roleIndex, ourCompanies);
          // 筛出为空不是空态（票 01 裁决 1）：带着下推条件或翻过页的零行报 ready，表格区另显 noMatchText。
          const viewState = catalogueViewState(answer, pageRows.length, retry, {
            module: info,
            endpoint: 'GET /commercial-business-parties',
            emptyTitle,
            emptyDescription,
            narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
          });
          // 当前条件下答回的行；这一问还没答出或没答成就是 null——筛选项上不拿 0 占位。
          const answeredRows = answer?.kind === 'outcome' ? pageRows : null;
          // 芯片上的数看当前条件下答回的行，摆哪几枚看整册（business-parties-workbench.ts 的 partyTypeChipsOf）。
          const typeChips =
            typeCountsUsable && counts !== null
              ? partyTypeChipsOf(counts, answeredRows && countBusinessParties(answeredRows, roleIndex, ourCompanies), shownType)
              : null;
          const statusOptions = partyStatusOptionsOf({
            whole: completeParties,
            conditioned: answeredRows,
            keyword: settledSearch,
            status,
            ofType: (row) => businessPartyIsOfType(row, shownType, roleIndex, ourCompanies),
          });
          return (
            <>
              <CursorRefreshReport
                // 刷新连法人册一起重取：刚在「公司主体」页登完的公司，回到这里按一下就该显成「我方公司」。
                retry={() => {
                  retry();
                  parties.retry();
                  entities.retry();
                }}
                pending={answer === null}
                outcome={refreshOutcomeOf(viewState)}
                retryRef={cursorRetry}
                onReport={setCursorRefresh}
              />
              <PartyWorkbench<BusinessPartyRecord>
                viewStateKey="business-parties.identities"
                search={{ value: search, onChange: setSearch, placeholder: '搜索名称、编码、注册号或外部编码' }}
                filters={(toolbarWidth) => (
                  <>
                    <select
                      className={`${filterSelectClass} shrink-0`}
                      value={status}
                      aria-label="状态"
                      onChange={(event) => setStatus(event.target.value as BusinessPartyStatusFilter)}
                    >
                      {statusOptions.map((option) => (
                        <option key={option.value} value={option.value}>
                          {`状态：${option.label}${option.count !== null ? `（${formatCount(option.count)}）` : ''}`}
                        </option>
                      ))}
                    </select>
                    {!typeControlReady ? null : typeChips !== null ? (
                      // 放得下摆一排芯片，放不下收成下拉，不折成第二行（partyTypeChipsFit）。
                      partyTypeChipsFit(toolbarWidth, typeChips) ? (
                        <PartyTypeChipRow chips={typeChips} onPick={(next) => setType(next === shownType ? 'ALL' : next)} />
                      ) : (
                        <select
                          className={`${filterSelectClass} shrink-0`}
                          value={shownType}
                          aria-label="类型"
                          onChange={(event) => setType(event.target.value as BusinessPartyTypeFilter)}
                        >
                          {typeChips.map((chip) => (
                            <option key={chip.type} value={chip.type} disabled={chip.disabled}>
                              {`类型：${chip.label}${chip.count !== null ? `（${formatCount(chip.count)}）` : ''}`}
                            </option>
                          ))}
                        </select>
                      )
                    ) : typeWords !== null ? (
                      <>
                        <select
                          className={`${filterSelectClass} shrink-0`}
                          value={shownType}
                          aria-label={typeWords.label}
                          title={typeWords.title}
                          onChange={(event) => setType(event.target.value as BusinessPartyTypeFilter)}
                        >
                          {businessPartyTypeFilterOptionsOf(typeSourcesComplete).map((option) => (
                            <option key={option.value} value={option.value}>
                              {option.label}
                            </option>
                          ))}
                        </select>
                        <PageOnlyNote>{typeWords.note}</PageOnlyNote>
                      </>
                    ) : null}
                    {narrowed ? <ClearFiltersLink onClick={clearFilters} /> : null}
                  </>
                )}
                columns={identityColumns}
                rows={visible}
                rowKey={(row) => row.partyId}
                selectedKey={selectedParty}
                onSelect={selectParty}
                sort={sort}
                onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
                noMatchText={
                  shownType !== 'ALL' && pageRows.length > 0
                    ? businessPartyTypeNoMatchNote(completeParties !== null, typeSourcesComplete)
                    : businessPartyNoMatchNote
                }
                onClearFilters={clearFilters}
                footer={{
                  countText: partyFilteredFooterCountText({
                    exact: typeCountsUsable,
                    total: pagination?.total ?? null,
                    pageRows: pageRows.length,
                    shown: visible.length,
                    pageFiltered: shownType !== 'ALL',
                    unit: '家',
                  }),
                  pagination,
                }}
                renderDetail={(key) => {
                  const row = visible.find((party) => party.partyId === key);
                  return row ? (
                    <BusinessPartyDetailPanel
                      row={row}
                      roles={rolesOfParty(roleIndex, row.partyId)}
                      ours={ourCompanies.has(row.partyId)}
                      typeUnread={!typeSourcesComplete}
                      pinned={entities.answer == null ? null : pinnedParties.has(row.partyId)}
                      relationships={relationships.answer}
                      retryRelationships={relationships.retry}
                      onClose={() => selectParty(null)}
                      onOpenObjectPage={() => {
                        window.location.hash = businessPartyObjectHash(row.partyId);
                      }}
                      parties={parties.answer}
                      // 游标这一页不在取数层里，参与方写口的整域作废碰不到它：写后由这里重取。
                      onChanged={() => {
                        parties.retry();
                        retry();
                      }}
                    />
                  ) : null;
                }}
                viewState={viewState}
                // 空册不是死胡同：有登记权限时说明下面另给一枚「新增往来单位」，主按钮仍只有页头那一枚。
                stateOverride={
                  mayWrite
                    ? {
                        empty: () => (
                          <EmptyState
                            title={emptyTitle}
                            description={emptyDescription}
                            action={
                              <Button variant="outline" onClick={() => openRegister('business-party')}>
                                <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
                                {registerActionLabels['business-party']}
                              </Button>
                            }
                          />
                        ),
                      }
                    : undefined
                }
              />
            </>
          );
        }}
      </CatalogueCursorScope>
    );
  } else {
    const relationshipConditions = partyRelationshipListConditions(settledRelationshipSearch, role, relationshipStatus, relationshipSort);
    const toggleRelationshipStatus = (code: PartyRelationshipStatusFilter) =>
      setRelationshipStatus(relationshipStatus === code ? 'ALL' : code);
    // 判据同名录（complete-register.ts）：整册一页装得下时状态芯片与「待查」带数、待查筛得准；装不下就不摆数，待查标「只筛本页」。
    const relationshipCounts = completeRelationships === null ? null : countPartyRelationships(completeRelationships);
    // 状态与角色下推成读口的 status 与 role 维。摆哪几枚状态看整册：有数时整册里一段都没有的状态不摆，摆出来的不随角色与
    // 检索词增减；没有数时全摆：筛空了也要能点回「全部」。每枚上的数另算，随角色与检索词走（下面的 statusCountOf）。
    const statusChips = partyRelationshipStatusFilterOptions.filter(
      (option) =>
        relationshipCounts === null ||
        option.value === 'ALL' ||
        option.value === relationshipStatus ||
        relationshipCounts.byStatus[option.value as RelationshipStatusCode] > 0,
    );
    const narrowed = relationshipSearch.trim() !== '' || role !== 'ALL' || relationshipStatus !== 'ALL' || attentionOnly;
    const clearFilters = () => {
      setRelationshipSearch('');
      setRole('ALL');
      setRelationshipStatus('ALL');
      setAttentionOnly(false);
    };
    refresh = { run: () => cursorRetry.current(), ...cursorRefresh };
    body = (
      <CatalogueCursorScope key="relationships" conditions={relationshipConditions} load={listPartyRelationships}>
        {({ answer, pagination, retry }) => {
          const pageRows = answer?.kind === 'outcome' ? answer.body.relationships : [];
          const visible = attentionOnly ? pageRows.filter(partyRelationshipNeedsAttention) : pageRows;
          // 筛出为空不是空态（票 01 裁决 1）：带着下推条件或翻过页的零行报 ready，表格区另显 noMatchText。
          const viewState = catalogueViewState(answer, pageRows.length, retry, {
            module: info,
            endpoint: 'GET /commercial-party-relationships',
            emptyTitle: '还没有登记任何合作关系',
            emptyDescription: `谁是我们的客户、谁是承运商，都由合作关系说。${entryHint(mayWrite, '用页头的「建立合作关系」登记第一段。')}`,
            narrowed: catalogueConditionsNarrow(relationshipConditions) || pastFirstPage(pagination),
          });
          // 当前条件下答回的行；这一问还没答出或没答成就是 null——筛选项上不拿 0 占位。
          const answeredRows = answer?.kind === 'outcome' ? pageRows : null;
          // 状态芯片上的数随角色与检索词走、不随状态自己走，与名录的状态下拉同一条规矩（statusCountsUnderConditions）。
          const statusCountOf = statusCountsUnderConditions({
            whole: completeRelationships,
            conditioned: answeredRows,
            keyword: settledRelationshipSearch,
            status: relationshipStatus,
            matchesOthers: (row) => role === 'ALL' || row.role === role,
          });
          // 待查的数看当前条件（检索词、角色、状态）下答回的行里有几段：整册一页装得下时，带条件答回的那一页也是全的。
          const attentionCount =
            completeRelationships === null || answeredRows === null ? null : answeredRows.filter(partyRelationshipNeedsAttention).length;
          return (
            <>
              <CursorRefreshReport
                retry={() => {
                  retry();
                  relationships.retry();
                }}
                pending={answer === null}
                outcome={refreshOutcomeOf(viewState)}
                retryRef={cursorRetry}
                onReport={setCursorRefresh}
              />
              <PartyWorkbench<PartyRelationshipRecord>
                viewStateKey="business-parties.relationships"
                search={{ value: relationshipSearch, onChange: setRelationshipSearch, placeholder: '搜索单位名称、编码或关系编号' }}
                filters={() => (
                  <>
                    <select
                      className={`${filterSelectClass} shrink-0`}
                      value={role}
                      aria-label="角色"
                      onChange={(event) => setRole(event.target.value as PartyRelationshipRoleFilter)}
                    >
                      {partyRelationshipRoleFilterOptions.map((option) => (
                        <option key={option.value} value={option.value}>
                          {option.label}
                        </option>
                      ))}
                    </select>
                    <div role="group" aria-label="状态" className="flex shrink-0 items-center gap-1.5">
                      {statusChips.map((option) => {
                        const count = statusCountOf(option.value);
                        const active = relationshipStatus === option.value;
                        return (
                          <PartyFilterChip
                            key={option.value}
                            label={option.label}
                            count={count}
                            active={active}
                            // 这个角色、这个检索词下一段都没有的状态留在原位、点不动；「全部」总点得动。
                            disabled={count === 0 && !active && option.value !== 'ALL'}
                            onClick={() => (option.value === 'ALL' ? setRelationshipStatus('ALL') : toggleRelationshipStatus(option.value))}
                          />
                        );
                      })}
                    </div>
                    <span className="mx-0.5 h-4 w-px shrink-0 bg-idpxyz-border" aria-hidden="true" />
                    {/* 待查由名称转写推出来（partyRelationshipNeedsAttention），读口没有这一维：整册装不下时只筛到手的这一页。 */}
                    <PartyFilterChip
                      label={relationshipCounts ? '待查' : '待查（只筛本页）'}
                      count={attentionCount}
                      active={attentionOnly}
                      disabled={attentionCount === 0 && !attentionOnly}
                      title={
                        relationshipCounts
                          ? '双方有名称查不到的合作关系'
                          : '双方有名称查不到的合作关系。读口筛不了这一项，只在当前这一页上筛，翻页后照新的一页再筛。'
                      }
                      onClick={() => setAttentionOnly(!attentionOnly)}
                    />
                    {narrowed ? <ClearFiltersLink onClick={clearFilters} /> : null}
                  </>
                )}
                columns={relationshipColumns}
                rows={visible}
                rowKey={(row) => row.relationshipId}
                selectedKey={selectedRelationship}
                onSelect={setSelectedRelationship}
                rowAttention={partyRelationshipNeedsAttention}
                sort={relationshipSort}
                onSort={(key) => setRelationshipSort(toggleWorkbenchSort(relationshipSort, key))}
                noMatchText={
                  relationshipCounts === null && attentionOnly && pageRows.length > 0
                    ? '这一页上没有待查的合作关系；待查只筛本页，不看其余几页，翻页看看'
                    : partyRelationshipNoMatchNote
                }
                onClearFilters={clearFilters}
                footer={{
                  countText: partyFilteredFooterCountText({
                    exact: relationshipCounts !== null,
                    total: pagination?.total ?? null,
                    pageRows: pageRows.length,
                    shown: visible.length,
                    pageFiltered: attentionOnly,
                    unit: '段',
                  }),
                  pagination,
                }}
                renderDetail={(key) => {
                  const row = visible.find((relationship) => relationship.relationshipId === key);
                  return row ? (
                    <PartyRelationshipDetailPanel
                      row={row}
                      onClose={() => setSelectedRelationship(null)}
                      // 游标这一页不在取数层里：结束关系之后连同不带条件的那一页一起重取。
                      onEnded={() => {
                        relationships.retry();
                        retry();
                      }}
                    />
                  ) : null;
                }}
                viewState={viewState}
              />
            </>
          );
        }}
      </CatalogueCursorScope>
    );
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
      <BusinessPartiesPageHead
        title={info.title}
        description="客户、供应商、承运代理等与我们有业务往来的单位，以及它们与我方公司之间的合作关系"
        onRefresh={refresh.run}
        refreshing={refresh.pending}
        refreshOutcome={refresh.outcome}
        actions={headActions}
      />
      <BusinessPartiesViewTabs tabs={viewTabs} value={book} onChange={setBook} attention={attention} />
      {body}
      <ConfirmDialog
        renderFooter={confirmDialogFooter}
        open={pendingSelect !== null}
        tone="warning"
        title="放弃修改？"
        message="合作关系登记还没有保存，换行或关闭详情后填写的内容不会保留。"
        confirmLabel="放弃"
        cancelLabel="继续编辑"
        onConfirm={() => {
          const next = pendingSelect?.key ?? null;
          setPendingSelect(null);
          setSelectedParty(next);
        }}
        onCancel={() => setPendingSelect(null)}
      />
    </div>
  );
}

/** 工具条行尾的「清除筛选」：带着任何一项筛选时才出现。 */
function ClearFiltersLink({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="shrink-0 whitespace-nowrap type-caption text-[color:var(--ds-text-link-hover)] hover:underline"
    >
      清除筛选
    </button>
  );
}
