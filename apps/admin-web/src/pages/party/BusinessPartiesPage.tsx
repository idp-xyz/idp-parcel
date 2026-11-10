import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { useMarkUnsavedInput } from '../../templates/unsaved-input';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { usePushedAddress } from '../../templates/use-pushed-address';
import { AlertTriangle, ArrowLeft, Ban, Clock, Handshake, MoreHorizontal, Plus, UserPlus } from 'lucide-react';
import { Button, ConfirmDialog, DropdownMenu, MenuItem, Tabs, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { useToast } from '@idpxyz/ui-theme-runtime';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  useTabReturn,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchKpi,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { hasUnsavedDraft, registrationGroup } from './unsaved-guard';
import { primaryActionClass } from '../../components/primary-action';
import { chipClass } from '../../components/registration';
import { statusToneText } from '../../components/status-tone';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { catalogueConditionsNarrow, type CatalogueConditions, type CataloguePage } from '../catalogue-query';
import { useCatalogueCursor, useSettledKeyword, type CatalogueCursor } from '../use-catalogue-cursor';
import { CursorPager, pastFirstPage } from '../shipment-request/cursor-pager';
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
import { labelOf, partyRoleLabels, type IdentityStatusCode, type RelationshipStatusCode } from './presentation';
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
import { CountryLabel } from '../refdata-labels';
import { PartyRelationshipDetailPanel } from './PartyRelationshipDetailPanel';
import {
  businessPartyListConditions,
  businessPartyNoMatchNote,
  businessPartyStatusFilterOptions,
  businessPartyRegistrationCountry,
  businessPartyTypeFilterOptions,
  businessPartyTypeKpis,
  countBusinessParties,
  defaultBusinessPartySort,
  filterBusinessPartiesByType,
  indexPartyRoles,
  ourCompanyPartyIds,
  partiesPinnedByLegalEntities,
  partyRegistrationCountriesOf,
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
 * 本页三本读册。身份册与关系册分册而不并表，理由见 BusinessPartiesPage 头注。「我方关联」不是第三本登记册，是以我方公司为
 * 主体把关系册重新组合一遍（our-company-network.ts，票 business-party-relationship-graph/02）。
 */
type Book = 'identities' | 'relationships' | 'network';

// 行首圆点的颜色随身份状态，与法人页同一个记号。
const identityStatusDotClass: Record<string, string> = {
  REGISTERED: 'bg-sky-400',
  EFFECTIVE: 'bg-emerald-400',
  DEACTIVATED: 'bg-gray-400',
};

// 关系状态的圆点：候选与已生效是在用的两格，三种终局一色——终局之间的分别由状态徽章说，圆点只说「还在不在用」。
const relationshipStatusDotClass: Record<string, string> = {
  CANDIDATE: 'bg-sky-400',
  EFFECTIVE: 'bg-emerald-400',
  EXPIRED: 'bg-gray-400',
  REVOKED: 'bg-gray-400',
  SUPERSEDED: 'bg-gray-400',
};

// 身份本体册的列。主列只留运营要看的：名称、类型（我方公司与持有的角色，成标签）、注册地、状态、生效时点；标识、修订号、登记依据
// 进详情栏底部的「技术详情」。停用时点与状态同格呈现：看这格的人要知道自何时起停用。登记时间上列是因为默认排序按它排（票 09
// 裁决 1）——排的键看不见，操作者判不出「为什么这一条在最上面」。类型列要关系册与法人册才算得出，所以列表随两份索引生成。
// 列随列表可用宽度增减，照公司主体列表（GroupLegalEntitiesPage 的 columnsOf）：名称、类型、状态三列总在，注册地、生效时间与
// 登记时间各有一个宽度门槛，随分栏拖动与窗口缩放收放，不再只看详情栏开没开；注册地的门槛最低——跨境往来先要认国家，比日期
// 常用（票 business-party-ux/04）。前两列给定宽，多出来的宽度落到状态与日期列上；名称格另上限 22rem，列表比各列之和还宽时
// 名称不跟着长。时刻只显到日（「哪一天」），完整时刻在悬停里。表头可排的只有生效时间与登记时间：排序下推到读口
// （business-party-list.ts 的 businessPartySortColumns），读口不按名称排。
function identityColumnsOf(
  roles: PartyRoleIndex,
  ours: OurCompanyIndex,
  countries: ReadonlyMap<string, string>,
): WorkbenchColumn<BusinessPartyRecord>[] {
  return [
    {
      key: 'party-name',
      header: '名称',
      width: '22rem',
      render: (row) => (
        <div className="flex min-w-44 max-w-[22rem] items-center gap-1.5">
          <span
            className={`h-1.5 w-1.5 shrink-0 rounded-full ${identityStatusDotClass[row.status] ?? 'bg-idpxyz-textMuted'}`}
            aria-hidden="true"
          />
          {/* 名称取主题的链接色 text.link.hover：强调色 #007acc 落在画布与选中行上只有 3.7–4.2:1。 */}
          {/* 名称下带编码（票 business-parties-object-page/04，照公司主体列表）：编码是对外对账、填单时认的那一串。 */}
          <span className="flex min-w-0 flex-col">
            <span className="truncate font-medium text-[color:var(--ds-text-link-hover)]">{row.partyName}</span>
            <span className="truncate font-mono type-caption text-idpxyz-breadcrumb">{row.partyId}</span>
          </span>
        </div>
      ),
    },
    {
      key: 'type',
      header: '类型',
      width: '12rem',
      render: (row) => <BusinessPartyTypeTags ours={ours.has(row.partyId)} roles={rolesOfParty(roles, row.partyId)} />,
    },
    {
      key: 'country',
      header: '注册地',
      minListWidth: 880,
      render: (row) => {
        const country = businessPartyRegistrationCountry(row, countries);
        return country ? (
          <span className="whitespace-nowrap text-idpxyz-text">
            <CountryLabel code={country} />
          </span>
        ) : (
          <span className="text-idpxyz-breadcrumb">—</span>
        );
      },
    },
    {
      key: 'status',
      header: '状态',
      render: (row) => (
        <div className="flex flex-col items-start gap-1 whitespace-nowrap">
          <StatusPill code={row.status} />
          {row.deactivatedAt ? (
            <span className="font-mono type-body text-idpxyz-breadcrumb">
              自 <Instant value={row.deactivatedAt} precision="day" />
            </span>
          ) : null}
        </div>
      ),
    },
    {
      key: 'effective-from',
      header: '生效时间',
      sortKey: 'effective-from',
      minListWidth: 960,
      // 待生效配一个琥珀色时钟而不是把时刻标成琥珀色：琥珀色字落在选中行底色上不足 4.5:1
      // （票 group-legal-entities-uiux/08 评审），图标是非文字，门槛 3:1。
      render: (row) => (
        <span className="inline-flex items-center gap-1 whitespace-nowrap font-mono text-idpxyz-text">
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
      sortKey: 'registered-at',
      minListWidth: 1240,
      render: (row) => (
        <span className="whitespace-nowrap font-mono text-idpxyz-breadcrumb">
          <Instant value={row.registeredAt} precision="day" />
        </span>
      ),
    },
  ];
}

// 参与方格：只显名称，标识给悬停与技术详情；名称转写不到是写入门失败才会出现的悬空引用，如实标出并亮灯，不补占位文本。
// 宽度夹在一个区间里：不设上限，长名称会把整表撑到 1280 宽放不下；不设下限，窄栏里会逐字折行。
function PartyCell({ id, name, nameKnown }: { id: string; name: string | undefined; nameKnown: boolean }) {
  return (
    <div className="flex min-w-32 max-w-44 items-center gap-1.5">
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
// 读口（party-relationship-list.ts 的 partyRelationshipSortColumns），读口不按双方排。
const relationshipColumns: WorkbenchColumn<PartyRelationshipRecord>[] = [
  {
    key: 'holder',
    header: '单位',
    render: (row) => (
      <div className="flex items-center gap-1.5">
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
    render: (row) => <span className="whitespace-nowrap text-idpxyz-text">{labelOf(partyRoleLabels, row.role)}</span>,
  },
  {
    key: 'counterparty',
    header: '对方',
    render: (row) => <PartyCell id={row.counterpartyId} name={row.counterpartyName} nameKnown={row.counterpartyNameKnown} />,
  },
  {
    key: 'status',
    header: '状态',
    // 终止时点那一行任其折成两行：整格不折行会在有终止行的册上把这一列撑到两百像素。
    render: (row) => (
      <div className="flex flex-col items-start gap-1">
        <StatusPill code={row.status} />
        {row.endedAt ? (
          <span className="font-mono type-body text-idpxyz-breadcrumb">
            自 <Instant value={row.endedAt} precision="day" />
          </span>
        ) : null}
      </div>
    ),
  },
  {
    key: 'validity',
    header: '有效期',
    sortKey: 'effective-start',
    // 随列表可用宽度收放（判据同往来单位册的日期列）：这一格最宽，窄了会把双方与状态挤出列表栏；两端另在栏的事实条上。
    minListWidth: 960,
    // 起止各占一行、只显到日：排成一行、带到秒要两百多像素。无终点说「长期有效」；完整时刻在悬停里。
    render: (row) => (
      <div className="flex flex-col whitespace-nowrap font-mono text-idpxyz-text">
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

const registerIcons: Record<BusinessPartyRegister, ReactNode> = {
  'business-party': <UserPlus className="mr-2 h-3.5 w-3.5" aria-hidden="true" />,
  'party-relationship': <Handshake className="mr-2 h-3.5 w-3.5" aria-hidden="true" />,
  'identity-deactivation': <Ban className="mr-2 h-3.5 w-3.5" aria-hidden="true" />,
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

/** 每本读册页头主动作登记的那一本：身份册登身份，关系册登关系；另两本收进旁边的「更多」。 */
const primaryRegisterOf: Record<Book, BusinessPartyRegister> = {
  identities: 'business-party',
  relationships: 'party-relationship',
  network: 'party-relationship',
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

/** 签上的总数取不带条件那一问答复的 page.total：那一问没有收窄，数的就是整册；没答出或读口不给总数就不显。 */
function registerTotalOf<Body extends { page: CataloguePage }>(answer: ApiResult<Body> | null): number | null {
  return answer?.kind === 'outcome' ? answer.body.page.total : null;
}

/**
 * 业务参与方（party-commercial）。两本读册各成一个工作台：参与方身份本体册与参与方关系册。
 *
 * 身份本体册与关系册同页分册而不并进关系表：一个参与方既可以不是法人、也可以不在任何关系里，被停用的那种恰恰如此——
 * 身份生命周期的「已登记」与「已停用」两格只有在身份册上才有实例可显（票 admin-remainder-mechanism-batch/01 的补格裁定）。
 * 关系行对象是参与方关系的最新登记修订：承运商、承运商代理商、转售商、聚合平台与渠道账号持有人都以「双方 + 角色 +
 * 有效区间」的时态关系表达，代理关系不自动合并交易角色；关系登记按修订版本化不可覆盖。两册的状态代数不同——身份状态
 * 按时点导出、关系状态是登记进来的事实，所以分册而不是并表。
 *
 * 形态照集团与法人页（GroupLegalEntitiesPage，票 business-parties-uiux/01，用户令「参考集团与法人实现」）：命令头指标可点即筛、
 * 状态胶囊带计数、表头点排序、单击行在右栏开详情（可拖宽）、详情栏「更多」或对象地址开整页对象标签、↑/↓ 换行 Esc 收栏 `/` 检索。各册之间用
 * 页顶一条签切换，签上带各册总数；摆法同客户与合同页（PartyContractsPage）：签下面的整块——副标题、指标、主动作、工具条与
 * 列表——都随所选的册换，签不属于其中任何一行。签不摆在命令头标题旁：那样它的横向位置随各册副标题的长短移动，宽屏上又与
 * 标题、指标挤成一行，分不出哪个是切换、哪个是数（票 business-party-ux/10）。两册的检索、筛选、排序与选中行
 * 都挂在页面上，换册再换回来还在。
 *
 * 两册的读口都在 ADR-0144 下（票 catalogue-read-pagination-rollout/11、14）：检索词、状态、角色与表头排序下推
 * （business-party-list.ts 的 businessPartyListConditions、party-relationship-list.ts 的 partyRelationshipListConditions），
 * 列表按游标翻页，换条件回第一页；游标只在列表视图里挂（CatalogueCursorScope），换册、进登记视图或对象页再回来是重新挂上、
 * 回到第一页。类型与待查是页面从别的册推出来的，读口筛不了，在到手的那一页上筛。指标与胶囊计数只在整册一页装得下时摆
 * （complete-register.ts 的 completeRegisterRows）：那时按整册数的是真数，页内筛也是精确的；装不下时一页里数出来的不是册上的
 * 数，计数不摆、指标格收起，类型与待查改成标「只筛本页」的控件，当前条件下共几条由翻页条说。册切换不进 kpis：那是一册的指标，
 * 册切换是页面级的。
 *
 * 登记从页头主动作进登记视图，不再是第三个签：主动作登当前读册那一本，另两本收进旁边的「更多」。登记视图住在地址上
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
  const identityColumns = useMemo(
    () => identityColumnsOf(roleIndex, ourCompanies, entityCountries),
    [roleIndex, ourCompanies, entityCountries],
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
  const completeParties = completeRegisterRows(parties.answer?.kind === 'outcome' ? parties.answer.body : null, (body) => body.parties);
  const completeRelationships = completeRegisterRows(
    relationships.answer?.kind === 'outcome' ? relationships.answer.body : null,
    (body) => body.relationships,
  );
  // 页名下不再写集团标识：一家公司一套系统，租户是谁不必告诉运营；绑定仍读着，壳层顶栏显公司名用它。
  void boundTenant;
  const primaryRegister = primaryRegisterOf[book];
  const primaryAction = mayWrite ? (
    <>
      <Button variant="default" className={primaryActionClass} onClick={() => openRegister(primaryRegister)}>
        <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
        {registerActionLabels[primaryRegister]}
      </Button>
      <DropdownMenu
        align="right"
        trigger={
          <Button variant="outline" className="px-2 text-idpxyz-breadcrumb" aria-label="更多登记入口">
            <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
          </Button>
        }
      >
        {businessPartyRegisters
          .filter((id) => id !== primaryRegister)
          .map((id) => (
            <MenuItem key={id} onSelect={() => openRegister(id)}>
              {registerIcons[id]}
              {registerActionLabels[id]}
            </MenuItem>
          ))}
      </DropdownMenu>
    </>
  ) : undefined;

  // 两册的签上带总数：总数只在拿到业务答案后显示——未配置态与错误态下报「0」会与状态区「这不是目录为空」直接矛盾。
  // 签条在工作台模板上面独占一条（理由在本组件头注）。左内边距是为了让第一枚签的字与下面的页标题、检索框、表格首列落在同一条
  // 竖线上。
  const bookStrip = (
    <Tabs value={book} onValueChange={(next) => setBook(next as Book)} className="shrink-0 gap-0">
      <TabsList className="shrink-0 px-2" aria-label="选择读册">
        <TabsTrigger value="identities">
          往来单位
          {partyTotal !== null ? <span className="ml-1.5 font-mono type-body text-idpxyz-breadcrumb">{partyTotal}</span> : null}
        </TabsTrigger>
        <TabsTrigger value="relationships">
          合作关系
          {relationshipTotal !== null ? (
            <span className="ml-1.5 font-mono type-body text-idpxyz-breadcrumb">{relationshipTotal}</span>
          ) : null}
        </TabsTrigger>
        <TabsTrigger value="network">
          我方关联
          {knownEntities !== null ? <span className="ml-1.5 font-mono type-body text-idpxyz-breadcrumb">{networkRows.length}</span> : null}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );

  if (book === 'network') {
    const visible = networkRows.filter((row) => matchesOurCompanySearch(row, networkSearch));
    const relationshipsAnswered = knownRelationships !== null;
    const networkColumns: WorkbenchColumn<OurCompanyNetworkRow>[] = [
      {
        key: 'company',
        header: '我方公司',
        render: (row) => (
          <div className="min-w-0">
            <div className="truncate text-idpxyz-text">{row.partyName ?? <UnknownPartyName />}</div>
            <div className="truncate font-mono type-caption text-idpxyz-breadcrumb">{row.partyId}</div>
          </div>
        ),
      },
      { key: 'toward', header: '对它担任角色的单位', render: (row) => roleCountsText(row.towardIt, relationshipsAnswered) },
      { key: 'from', header: '它对其担任角色', hideWhenDetailOpen: true, render: (row) => roleCountsText(row.fromIt, relationshipsAnswered) },
      {
        key: 'candidates',
        header: '待确认',
        right: true,
        render: (row) => (relationshipsAnswered && row.candidates > 0 ? <span className="font-mono">{row.candidates}</span> : '—'),
      },
      {
        key: 'entities',
        header: '公司主体',
        minListWidth: 760,
        render: (row) => <span className="font-mono type-caption text-idpxyz-breadcrumb">{row.legalEntityIds.join('、')}</span>,
      },
    ];
    // 合成节点与「已结束」那句点了：切到关系册、按这家公司的编码检索，看它名下的全部关系。
    const showRelationshipsOf = (partyId: string) => {
      setBook('relationships');
      setRelationshipSearch(partyId);
      setRole('ALL');
      setRelationshipStatus('ALL');
      setAttentionOnly(false);
      setSelectedRelationship(null);
    };
    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        {bookStrip}
        <WorkbenchPageTemplate<OurCompanyNetworkRow>
          title={info.title}
          subtitle="以我方每家公司为主体：谁对它担任什么角色、它又对谁担任什么角色；选一家在右边看它的关系图"
          onRefresh={() => {
            entities.retry();
            relationships.retry();
          }}
          refreshing={entities.pending || relationships.pending}
          primaryAction={primaryAction}
          search={{ value: networkSearch, onChange: setNetworkSearch, placeholder: '搜我方公司名称、编码或公司主体' }}
          toolbarRight={
            knownEntities !== null ? (
              <span className="type-body text-idpxyz-breadcrumb">
                共 {networkRows.length} 家我方公司
                {completeRelationships === null && relationshipsAnswered ? '；合作关系册不止一页，段数只按第一页数' : ''}
              </span>
            ) : undefined
          }
          columns={networkColumns}
          rows={visible}
          rowKey={(row) => row.partyId}
          selectedKey={selectedOurCompany}
          onSelect={setSelectedOurCompany}
          noMatchText="没有匹配的我方公司"
          onClearFilters={() => setNetworkSearch('')}
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
          viewState={catalogueViewState(entities.answer, networkRows.length, entities.retry, {
            module: info,
            endpoint: 'GET /commercial-group-legal-entities',
            emptyTitle: '还没有在用的我方公司',
            emptyDescription: '我方公司在「公司主体」页登记；登记之后，这里按公司列出各自的合作关系。',
            narrowed: networkSearch.trim() !== '',
          })}
        />
      </div>
    );
  }

  if (book === 'identities') {
    const conditions = businessPartyListConditions(settledSearch, status, sort);
    const toggleStatus = (code: BusinessPartyStatusFilter) => setStatus(status === code ? 'ALL' : code);
    const toggleType = (code: BusinessPartyTypeFilter) => setType(type === code ? 'ALL' : code);
    // 整册一页装得下时按整册计数（complete-register.ts）：类型指标格可点即筛（票 business-party-ux/05），胶囊带计数、计数为零的
    // 状态不摆；类型筛选这时是精确的。装不下就 null：不显计数，类型筛选改成下拉、标「只筛本页」。
    const counts = completeParties === null ? null : countBusinessParties(completeParties, roleIndex, ourCompanies);
    // 指标只按类型分格（票 business-party-ux/05）：运营问的是「有几家客户、几家承运商」，不是「有几条已登记修订」。哪几格、
    // 什么序在 businessPartyTypeKpis；状态（待生效、已停用）不在这里，归下面的状态胶囊——一个筛选器不开两个入口。
    const kpis: WorkbenchKpi[] | undefined = counts
      ? businessPartyTypeKpis(counts).map((cell) => ({
          label: cell.label,
          value: cell.count,
          // 「暂无合作关系」不是一种角色，数值不上语气色；我方公司与各角色是。
          tone: cell.type === 'NONE' ? undefined : statusToneText.info,
          active: type === cell.type,
          onClick: () => toggleType(cell.type),
        }))
      : undefined;
    // 状态胶囊下推成读口的 status 维。没有计数时常摆：筛空了也要能点回「全部」。
    const statusChips: WorkbenchChip[] = businessPartyStatusFilterOptions
      .filter(
        (option) =>
          counts === null || option.value === 'ALL' || option.value === status || counts.byStatus[option.value as IdentityStatusCode] > 0,
      )
      .map((option) => ({
        id: option.value,
        label: option.label,
        count: counts === null ? undefined : option.value === 'ALL' ? counts.total : counts.byStatus[option.value as IdentityStatusCode],
        active: status === option.value,
        onClick: () => (option.value === 'ALL' ? setStatus('ALL') : toggleStatus(option.value)),
      }));
    // 类型由关系册、客户账户册与法人册推出来（business-party-list.ts 文件头），读口没有这一维：整册装不下时只筛到手的这一页，
    // 控件上写明；装得下时类型的入口是上面的指标格。
    const typeFilter = counts ? undefined : (
      <>
        <select
          className={filterSelectClass}
          value={type}
          aria-label="类型（只筛本页）"
          title="类型由合作关系、客户账户与公司主体推出来，读口筛不了：只在当前这一页上筛，翻页后照新的一页再筛。"
          onChange={(event) => setType(event.target.value as BusinessPartyTypeFilter)}
        >
          {businessPartyTypeFilterOptions.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        <PageOnlyNote>类型只筛本页</PageOnlyNote>
      </>
    );

    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        {bookStrip}
        <CatalogueCursorScope conditions={conditions} load={listBusinessParties}>
          {({ answer, pagination, retry }) => {
            const pageRows = answer?.kind === 'outcome' ? answer.body.parties : [];
            const visible = filterBusinessPartiesByType(pageRows, type, roleIndex, ourCompanies);
            return (
              <WorkbenchPageTemplate<BusinessPartyRecord>
                title={info.title}
                // 副标题只说这一册装的是谁，不带操作说明：宽屏上指标与标题同行，副标题一长，指标那一组就排不进这一行，
                // 连同页头动作一起被挤到下一行。
                subtitle="与我们有业务往来的客户、承运商、代理等单位"
                kpis={kpis}
                // 刷新连法人册一起重取：刚在「公司主体」页登完的公司，回到这里按一下就该显成「我方公司」。
                onRefresh={() => {
                  retry();
                  parties.retry();
                  entities.retry();
                }}
                refreshing={answer === null}
                primaryAction={primaryAction}
                search={{ value: search, onChange: setSearch, placeholder: '名称、编码、注册号或外部编码' }}
                statusChips={statusChips}
                extraFilters={typeFilter}
                toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
                columns={identityColumns}
                rows={visible}
                rowKey={(row) => row.partyId}
                selectedKey={selectedParty}
                onSelect={selectParty}
                sort={sort}
                onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
                noMatchText={
                  counts === null && type !== 'ALL' && pageRows.length > 0
                    ? '这一页上没有这一类单位；类型只筛本页，不看其余几页，翻页看看'
                    : businessPartyNoMatchNote
                }
                onClearFilters={() => {
                  setSearch('');
                  setStatus('ALL');
                  setType('ALL');
                }}
                renderDetail={(key) => {
                  const row = visible.find((party) => party.partyId === key);
                  return row ? (
                    <BusinessPartyDetailPanel
                      row={row}
                      roles={rolesOfParty(roleIndex, row.partyId)}
                      ours={ourCompanies.has(row.partyId)}
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
                // 筛出为空不是空态（票 01 裁决 1）：带着下推条件或翻过页的零行报 ready，表格区另显 noMatchText。
                viewState={catalogueViewState(answer, pageRows.length, retry, {
                  module: info,
                  endpoint: 'GET /commercial-business-parties',
                  emptyTitle: '还没有登记任何往来单位',
                  emptyDescription: `客户、承运商、代理都从这里登记。${entryHint(mayWrite, '用页头的「新增往来单位」登记第一家。')}`,
                  narrowed: catalogueConditionsNarrow(conditions) || pastFirstPage(pagination),
                })}
              />
            );
          }}
        </CatalogueCursorScope>
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

  const relationshipConditions = partyRelationshipListConditions(settledRelationshipSearch, role, relationshipStatus, relationshipSort);
  const toggleRelationshipStatus = (code: PartyRelationshipStatusFilter) =>
    setRelationshipStatus(relationshipStatus === code ? 'ALL' : code);
  // 判据同身份册（complete-register.ts）：整册一页装得下时指标与胶囊带计数、「待查」指标格可点即筛且是精确的；装不下就不显计数，
  // 待查改成标「只筛本页」的开关。
  const relationshipCounts = completeRelationships === null ? null : countPartyRelationships(completeRelationships);
  const relationshipKpis: WorkbenchKpi[] | undefined = relationshipCounts
    ? [
        {
          label: '待查',
          value: relationshipCounts.attention,
          tone: relationshipCounts.attention > 0 ? statusToneText.warning : undefined,
          icon:
            relationshipCounts.attention > 0 ? <AlertTriangle className={`h-3 w-3 ${statusToneText.warning}`} aria-hidden="true" /> : undefined,
          active: attentionOnly,
          onClick: () => setAttentionOnly(!attentionOnly),
        },
        {
          label: '待确认',
          value: relationshipCounts.byStatus.CANDIDATE,
          tone: relationshipCounts.byStatus.CANDIDATE > 0 ? statusToneText.warning : undefined,
          active: relationshipStatus === 'CANDIDATE',
          onClick: () => toggleRelationshipStatus('CANDIDATE'),
        },
        {
          label: '已生效',
          value: relationshipCounts.byStatus.EFFECTIVE,
          tone: statusToneText.info,
          active: relationshipStatus === 'EFFECTIVE',
          onClick: () => toggleRelationshipStatus('EFFECTIVE'),
        },
      ]
    : undefined;
  // 状态胶囊与角色下拉下推成读口的 status 与 role 维。没有计数时常摆：筛空了也要能点回「全部」。
  const relationshipStatusChips: WorkbenchChip[] = partyRelationshipStatusFilterOptions
    .filter(
      (option) =>
        relationshipCounts === null ||
        option.value === 'ALL' ||
        option.value === relationshipStatus ||
        relationshipCounts.byStatus[option.value as RelationshipStatusCode] > 0,
    )
    .map((option) => ({
      id: option.value,
      label: option.label,
      count:
        relationshipCounts === null
          ? undefined
          : option.value === 'ALL'
            ? relationshipCounts.total
            : relationshipCounts.byStatus[option.value as RelationshipStatusCode],
      active: relationshipStatus === option.value,
      onClick: () => (option.value === 'ALL' ? setRelationshipStatus('ALL') : toggleRelationshipStatus(option.value)),
    }));
  const relationshipFilters = (
    <>
      {/* 角色六格并排成胶囊太长，留作下拉；与状态胶囊叠加。 */}
      <select
        className={filterSelectClass}
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
      {/* 待查由名称转写推出来（partyRelationshipNeedsAttention），读口没有这一维：整册装不下时只筛到手的这一页；装得下时入口是指标格。 */}
      {relationshipCounts ? null : (
      <button
        type="button"
        className={chipClass(attentionOnly)}
        aria-pressed={attentionOnly}
        title="双方有名称查不到的合作关系。读口筛不了这一项，只在当前这一页上筛，翻页后照新的一页再筛。"
        onClick={() => setAttentionOnly(!attentionOnly)}
      >
        待查（只筛本页）
      </button>
      )}
    </>
  );

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
      {bookStrip}
      <CatalogueCursorScope conditions={relationshipConditions} load={listPartyRelationships}>
        {({ answer, pagination, retry }) => {
          const pageRows = answer?.kind === 'outcome' ? answer.body.relationships : [];
          const visible = attentionOnly ? pageRows.filter(partyRelationshipNeedsAttention) : pageRows;
          return (
            <WorkbenchPageTemplate<PartyRelationshipRecord>
              title={info.title}
              subtitle="谁对谁担任什么角色、合作多久；到期或撤销后，新的业务不再按这段关系办"
              kpis={relationshipKpis}
              onRefresh={() => {
                retry();
                relationships.retry();
              }}
              refreshing={answer === null}
              primaryAction={primaryAction}
              search={{ value: relationshipSearch, onChange: setRelationshipSearch, placeholder: '搜索单位名称、编码或关系编号…' }}
              statusChips={relationshipStatusChips}
              extraFilters={relationshipFilters}
              toolbarRight={pagination ? <CursorPager pagination={pagination} /> : undefined}
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
              onClearFilters={() => {
                setRelationshipSearch('');
                setRole('ALL');
                setRelationshipStatus('ALL');
                setAttentionOnly(false);
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
              // 筛出为空不是空态（票 01 裁决 1）：带着下推条件或翻过页的零行报 ready，表格区另显 noMatchText。
              viewState={catalogueViewState(answer, pageRows.length, retry, {
                module: info,
                endpoint: 'GET /commercial-party-relationships',
                emptyTitle: '还没有登记任何合作关系',
                emptyDescription: `谁是我们的客户、谁是承运商，都由合作关系说。${entryHint(mayWrite, '用页头的「建立合作关系」登记第一段。')}`,
                narrowed: catalogueConditionsNarrow(relationshipConditions) || pastFirstPage(pagination),
              })}
            />
          );
        }}
      </CatalogueCursorScope>
    </div>
  );
}
