import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { AlertTriangle, ArrowLeft, Ban, Clock, Handshake, MoreHorizontal, Plus, UserPlus } from 'lucide-react';
import { Button, ConfirmDialog, DropdownMenu, MenuItem, Tabs, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
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
import { primaryActionClass } from '../../components/primary-action';
import { statusToneText } from '../../components/status-tone';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import {
  listBusinessParties,
  listPartyRelationships,
  type BusinessPartyListResponseBody,
  type BusinessPartyRecord,
  type PartyRelationshipRecord,
} from './api';
import { useBoundTenant } from '../use-operator-binding';
import { useVisited } from './keep-alive-tabs';
import { identityStatusLabels, labelOf, partyRoleLabels, relationshipStatusLabels } from './presentation';
import { EffectiveShareBar, Instant, InstantRange, UnknownPartyName, filterSelectClass, statusBadge } from './detail-primitives';
import { useRegisterList } from './register-list';
import { BusinessPartyRegistrationForm } from './BusinessPartyRegistrationForm';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
import { IdentityDeactivationForm } from './IdentityDeactivationForm';
import {
  businessPartiesViewOf,
  businessPartyObjectHash,
  businessPartyRegisters,
  deactivationLinkHash,
  deactivationPresetOf,
  registerViewHash,
  withoutDeactivationPreset,
  type BusinessPartyRegister,
  type DeactivationPreset,
} from './business-parties-address';
import { BusinessPartyDetailPage } from './BusinessPartyDetailPage';
import { BusinessPartyDetailPanel } from './BusinessPartyDetailPanel';
import { PartyRelationshipDetailPanel } from './PartyRelationshipDetailPanel';
import {
  businessPartyCountSummary,
  businessPartyNoMatchNote,
  businessPartyStatusFilterOptions,
  countBusinessParties,
  defaultBusinessPartySort,
  filterBusinessParties,
  sortBusinessParties,
  type BusinessPartyStatusFilter,
} from './business-party-list';
import { effectiveShareOfInUse } from './legal-entity-list';
import {
  countPartyRelationships,
  defaultPartyRelationshipSort,
  filterPartyRelationships,
  partyRelationshipCountSummary,
  partyRelationshipNeedsAttention,
  partyRelationshipNoMatchNote,
  partyRelationshipRoleFilterOptions,
  partyRelationshipStatusFilterOptions,
  sortPartyRelationships,
  type PartyRelationshipRoleFilter,
  type PartyRelationshipStatusFilter,
} from './party-relationship-list';
import { useMayWriteRegistry } from '../use-operator-grants';
import { entryHint } from '../operator-grants';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['business-parties'];

/** 本页两本读册。身份册与关系册分册而不并表，理由见 BusinessPartiesPage 头注。 */
type Book = 'identities' | 'relationships';

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

// 身份本体册的列。停用时点与状态同格呈现：看这格的人要知道自何时起停用；停用依据归详情栏。登记时间上列是因为默认排序
// 按它排（票 09 裁决 1）——排的键看不见，操作者判不出「为什么这一条在最上面」。
const identityColumns: WorkbenchColumn<BusinessPartyRecord>[] = [
  {
    key: 'party',
    header: '参与方标识',
    sortKey: 'party',
    render: (row) => (
      <div className="flex min-w-0 items-center gap-1.5">
        <span
          className={`h-1.5 w-1.5 shrink-0 rounded-full ${identityStatusDotClass[row.status] ?? 'bg-idpxyz-textMuted'}`}
          aria-hidden="true"
        />
        {/* 标识取主题的链接色 text.link.hover：强调色 #007acc 落在画布与选中行上只有 3.7–4.2:1。 */}
        <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.partyId}>
          {row.partyId}
        </span>
        <span className="shrink-0 font-mono text-[12px] text-idpxyz-breadcrumb">r{row.revision}</span>
      </div>
    ),
  },
  {
    key: 'party-name',
    header: '名称',
    sortKey: 'party-name',
    // 详情栏开着时收起：名称在栏头副题上；留着会在窄列里逐字折行。
    hideWhenDetailOpen: true,
    render: (row) => <span className="text-idpxyz-textBright">{row.partyName}</span>,
  },
  {
    key: 'status',
    header: '状态',
    render: (row) => (
      <div className="flex flex-col items-start gap-1 whitespace-nowrap">
        {statusBadge(identityStatusLabels, row.status)}
        {row.deactivatedAt ? (
          <span className="font-mono text-[12px] text-idpxyz-breadcrumb">
            自 <Instant value={row.deactivatedAt} />
          </span>
        ) : null}
      </div>
    ),
  },
  {
    key: 'effective-from',
    header: '生效时点',
    sortKey: 'effective-from',
    // 已登记即还没到生效时点，配一个琥珀色时钟而不是把时刻标成琥珀色：琥珀色字落在选中行底色上不足 4.5:1
    // （票 group-legal-entities-uiux/08 评审），图标是非文字，门槛 3:1。
    render: (row) => (
      <span className="inline-flex items-center gap-1 font-mono text-idpxyz-text">
        {row.status === 'REGISTERED' ? (
          <Clock className={`h-3 w-3 shrink-0 ${statusToneText.warning}`} aria-label="尚未生效" />
        ) : null}
        <Instant value={row.effectiveFrom} />
      </span>
    ),
  },
  {
    key: 'basis',
    header: '登记依据',
    hideWhenDetailOpen: true,
    render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{row.basis}</span>,
  },
  {
    key: 'registered-at',
    header: '登记时间',
    sortKey: 'registered-at',
    hideWhenDetailOpen: true,
    render: (row) => (
      <span className="font-mono text-idpxyz-breadcrumb">
        <Instant value={row.registeredAt} />
      </span>
    ),
  },
];

// 参与方格：名称在上、标识在下；名称转写不到是写入门失败才会出现的悬空引用，如实标出，不补占位文本。
function PartyCell({ id, name, nameKnown }: { id: string; name: string | undefined; nameKnown: boolean }) {
  return (
    <div className="min-w-0">
      <div className="truncate text-idpxyz-textBright">{nameKnown ? name : <UnknownPartyName />}</div>
      <div className="font-mono text-[12px] text-idpxyz-breadcrumb">{id}</div>
    </div>
  );
}

// 关系册的列。骨架期的「方向」列撤下：方向由持有方 → 相对方的两列次序表达（CONTEXT：方向由「哪一方对哪一方持有该角色」
// 表达），单设一列没有第二份事实可载。终止时点与后继是状态格的内容：撤销或到期只影响生效边界后的新决定，看这格的人
// 需要知道边界在哪、被谁替代；终止依据归详情栏。
const relationshipColumns: WorkbenchColumn<PartyRelationshipRecord>[] = [
  {
    key: 'relationship',
    header: '关系标识',
    sortKey: 'relationship',
    render: (row) => (
      <div className="flex min-w-0 items-center gap-1.5">
        <span
          className={`h-1.5 w-1.5 shrink-0 rounded-full ${relationshipStatusDotClass[row.status] ?? 'bg-idpxyz-textMuted'} ${
            partyRelationshipNeedsAttention(row) ? 'animate-pulse' : ''
          }`}
          aria-hidden="true"
        />
        <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.relationshipId}>
          {row.relationshipId}
        </span>
        <span className="shrink-0 font-mono text-[12px] text-idpxyz-breadcrumb">r{row.revision}</span>
        {partyRelationshipNeedsAttention(row) && (
          <AlertTriangle className={`h-3 w-3 shrink-0 ${statusToneText.warning}`} aria-label="待查" />
        )}
      </div>
    ),
  },
  {
    key: 'holder',
    header: '持有方',
    sortKey: 'holder',
    render: (row) => <PartyCell id={row.holderId} name={row.holderName} nameKnown={row.holderNameKnown} />,
  },
  {
    key: 'counterparty',
    header: '相对方',
    sortKey: 'counterparty',
    render: (row) => <PartyCell id={row.counterpartyId} name={row.counterpartyName} nameKnown={row.counterpartyNameKnown} />,
  },
  {
    key: 'role',
    header: '角色',
    render: (row) => <span className="whitespace-nowrap text-idpxyz-text">{labelOf(partyRoleLabels, row.role)}</span>,
  },
  {
    key: 'status',
    header: '状态',
    render: (row) => (
      <div className="flex flex-col items-start gap-1 whitespace-nowrap">
        {statusBadge(relationshipStatusLabels, row.status)}
        {row.endedAt ? (
          <span className="font-mono text-[12px] text-idpxyz-breadcrumb">
            自 <Instant value={row.endedAt} />
          </span>
        ) : null}
        {row.successorId ? <span className="font-mono text-[12px] text-idpxyz-breadcrumb">→ {row.successorId}</span> : null}
      </div>
    ),
  },
  {
    key: 'validity',
    header: '有效区间',
    sortKey: 'effective-start',
    // 详情栏开着时收起：两端在栏的事实条上；这一格最宽，留着会把角色与状态挤出列表栏。
    hideWhenDetailOpen: true,
    render: (row) => (
      <span className="whitespace-nowrap font-mono text-idpxyz-text">
        <InstantRange from={row.effectiveStartsAt} to={row.effectiveEndsAt} />
      </span>
    ),
  },
  {
    key: 'scope',
    header: '适用范围',
    hideWhenDetailOpen: true,
    render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{row.scope}</span>,
  },
  {
    key: 'basis',
    header: '依据',
    hideWhenDetailOpen: true,
    render: (row) => <span className="font-mono text-idpxyz-breadcrumb">{row.basis}</span>,
  },
  {
    key: 'registered-at',
    header: '登记时间',
    sortKey: 'registered-at',
    hideWhenDetailOpen: true,
    render: (row) => (
      <span className="font-mono text-idpxyz-breadcrumb">
        <Instant value={row.registeredAt} />
      </span>
    ),
  },
];

/** 登记入口与登记视图的用词。册名取读册自己的词，不为登记另造说法；「停用」取身份状态格里的封闭词（已停用）。 */
const registerBookLabels: Record<BusinessPartyRegister, string> = {
  'business-party': '参与方身份',
  'party-relationship': '参与方关系',
  'identity-deactivation': '身份停用',
};

const registerActionLabels: Record<BusinessPartyRegister, string> = {
  'business-party': '登记参与方身份',
  'party-relationship': '登记参与方关系',
  'identity-deactivation': '停用身份…',
};

const registerIcons: Record<BusinessPartyRegister, ReactNode> = {
  'business-party': <UserPlus className="mr-2 h-3.5 w-3.5" aria-hidden="true" />,
  'party-relationship': <Handshake className="mr-2 h-3.5 w-3.5" aria-hidden="true" />,
  'identity-deactivation': <Ban className="mr-2 h-3.5 w-3.5" aria-hidden="true" />,
};

const registerHeadings: Record<BusinessPartyRegister, { title: string; subtitle: string }> = {
  'business-party': {
    title: '登记参与方身份',
    subtitle: '每次登记形成新修订，历史不可覆盖；更正名称或依据就登记同一参与方的下一修订号',
  },
  'party-relationship': {
    title: '登记参与方关系',
    subtitle: '关系记谁对谁持有什么角色、在什么范围、多久；不带批准信息即登记为候选关系，批准另登一笔新修订',
  },
  'identity-deactivation': {
    title: '停用身份',
    subtitle: '停用形成新修订，自停用时点起不再支持新的商业决定；责任法人与货主客户账户也在这里停用',
  },
};

/** 每本读册页头主动作登记的那一本：身份册登身份，关系册登关系；另两本收进旁边的「更多」。 */
const primaryRegisterOf: Record<Book, BusinessPartyRegister> = {
  identities: 'business-party',
  relationships: 'party-relationship',
};

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
  onPartiesChanged,
  onRelationshipsChanged,
  preset,
  reportUnsaved,
}: {
  register: BusinessPartyRegister;
  /** 页面持有的参与方列表答案：关系表单双方候选与停用表单参与方那一册都取它，不各自再读（票 13 第 5 条）。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  knownParties: readonly BusinessPartyRecord[] | null;
  knownRelationships: readonly PartyRelationshipRecord[] | null;
  onPartiesChanged: () => void;
  onRelationshipsChanged: () => void;
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
            onRegistered={onPartiesChanged}
            onUnsavedChange={reportUnsaved['business-party']}
          />
        </div>
      ) : null}
      {seen('party-relationship') ? (
        <div className={register === 'party-relationship' ? 'contents' : 'hidden'}>
          <PartyRelationshipRegistrationForm
            knownRelationships={knownRelationships}
            parties={parties}
            onRegistered={onRelationshipsChanged}
            onUnsavedChange={reportUnsaved['party-relationship']}
          />
        </div>
      ) : null}
      {seen('identity-deactivation') ? (
        <div className={register === 'identity-deactivation' ? 'contents' : 'hidden'}>
          <IdentityDeactivationForm
            parties={parties}
            onDeactivated={onPartiesChanged}
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
 * 业务参与方（party-commercial）。两本读册各成一个工作台：参与方身份本体册与参与方关系册。
 *
 * 身份本体册与关系册同页分册而不并进关系表：一个参与方既可以不是法人、也可以不在任何关系里，被停用的那种恰恰如此——
 * 身份生命周期的「已登记」与「已停用」两格只有在身份册上才有实例可显（票 admin-remainder-mechanism-batch/01 的补格裁定）。
 * 关系行对象是参与方关系的最新登记修订：承运商、承运商代理商、转售商、聚合平台与渠道账号持有人都以「双方 + 角色 +
 * 有效区间」的时态关系表达，代理关系不自动合并交易角色；关系登记按修订版本化不可覆盖。两册的状态代数不同——身份状态
 * 按时点导出、关系状态是登记进来的事实，所以分册而不是并表。
 *
 * 形态照集团与法人页（GroupLegalEntitiesPage，票 business-parties-uiux/01，用户令「参考集团与法人实现」）：命令头指标可点即筛、
 * 状态胶囊带计数、表头点排序、单击行在右栏开详情（可拖宽）、详情栏「更多」或对象地址开整页对象标签、↑/↓ 换行 Esc 收栏
 * `/` 检索。两册之间用页顶一条签切换，签上带各册总数；两册的检索、筛选、排序与选中行都挂在页面上，换册再换回来还在。
 * 没有用工作台模板的插槽放册切换：模板的命令头是一册的指标，册切换得在它上面。
 *
 * 登记从页头主动作进登记视图，不再是第三个签：主动作登当前读册那一本，另两本收进旁边的「更多」。登记视图住在地址上
 * （`?register=<册>`，判读在 business-parties-address.ts）：刷新仍停在登记视图、换册只换键值不压历史，浏览器后退回列表。
 * 草稿有没提交的输入时，页内「返回列表」先确认，刷新与关页由 beforeunload 拦；浏览器后退不拦——拦它得改写历史栈，壳层按
 * hashchange 记每张标签的地址，改写会让两边对不上。
 *
 * 页面持有两册的列表状态（票 10 第 5 条）：登记视图从这份答案取修订号建议与双方候选，登记册答 REGISTERED / DEACTIVATED 时
 * 触发对应读册重取；两册各自一份重取序号——登一段关系不必重取参与方册，反过来也一样。
 */
export function BusinessPartiesPage() {
  const parties = useRegisterList(listBusinessParties);
  const relationships = useRegisterList(listPartyRelationships);
  // 页名下的「集团」是 ADR-0003 三级边界的第一级，即本会话绑定的租户；答不出就不显，不拿列表行上的 tenantId 顶。
  const boundTenant = useBoundTenant();
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const tabReturn = useTabReturn();
  // 渲列表、登记视图还是对象页由地址定：对象地址在壳层是自己的一张标签，本组件在那张标签里渲对象页。
  const [hash, setHash] = useState(() => window.location.hash);
  const view = businessPartiesViewOf(hash);
  // 本次挂载里是不是从列表压进登记视图的：是就用后退回列表，不再压一条；刷新、深链或从别的标签回来直接落在登记视图时
  // 不是，那时后退一格不一定是列表，改用替换。
  const pushedRegister = useRef(false);
  // 深链（法人页「停用此责任法人…」、本页详情栏「停用此参与方…」）带来的停用预填：读到就交给停用表单，然后从地址里去掉，
  // 免得换标签回来、页面重新挂载时再预填一次。这一页的标签已开着时地址变了也认：壳层不重挂已开的标签。
  const [preset, setPreset] = useState<DeactivationPreset | null>(null);
  const [unsaved, setUnsaved] = useState<Record<BusinessPartyRegister, boolean>>(noUnsaved);
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const [book, setBook] = useState<Book>('identities');

  // 身份册的列表状态。检索词放在地址 `?q=`：别处的引用链接带着它过来，这一册挂载时已经筛到那一行。
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useState<BusinessPartyStatusFilter>('ALL');
  const [sort, setSort] = useState<WorkbenchSort>(defaultBusinessPartySort);
  const [selectedParty, setSelectedParty] = useState<string | null>(null);
  // 关系册的列表状态。检索留在页内：停用入口与引用链接定位的是身份这一册。
  const [relationshipSearch, setRelationshipSearch] = useState('');
  const [role, setRole] = useState<PartyRelationshipRoleFilter>('ALL');
  const [relationshipStatus, setRelationshipStatus] = useState<PartyRelationshipStatusFilter>('ALL');
  const [attentionOnly, setAttentionOnly] = useState(false);
  const [relationshipSort, setRelationshipSort] = useState<WorkbenchSort>(defaultPartyRelationshipSort);
  const [selectedRelationship, setSelectedRelationship] = useState<string | null>(null);

  useEffect(() => {
    // 地址是位置的唯一权威。停用预填读到就收下并从地址去掉（理由见 withoutDeactivationPreset），之后视图按去掉后的地址判。
    const read = () => {
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
    read();
    window.addEventListener('hashchange', read);
    return () => window.removeEventListener('hashchange', read);
  }, []);

  const anyUnsaved = Object.values(unsaved).some(Boolean);
  // 刷新与关页时浏览器自己弹确认；只在登记视图且有表单报了没提交的输入时挂。表单自己也挂了一份（useUnsavedGuard），
  // 停用表单关掉了它（预填不算输入），这里按表单对着带预填的基线报上来的那一格再兜一次。
  const guardUnload = view.kind === 'register' && anyUnsaved;
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
    pushedRegister.current = true;
    window.location.hash = registerViewHash(window.location.hash, register);
  };
  const openDeactivation = (target: DeactivationPreset) => {
    pushedRegister.current = view.kind !== 'register';
    window.location.hash = deactivationLinkHash(target);
  };
  const leaveRegister = () => {
    setConfirmingLeave(false);
    setPreset(null);
    if (pushedRegister.current) {
      pushedRegister.current = false;
      window.history.back();
      return;
    }
    window.location.replace(registerViewHash(window.location.hash, null));
  };
  const requestLeaveRegister = () => (anyUnsaved ? setConfirmingLeave(true) : leaveRegister());

  // 列表没取到（加载中 / 未配置 / 出错）传 null：表单那边建议修订号一律为 1，不拿空数组冒充「册上没有」。
  const knownParties = parties.answer?.kind === 'outcome' ? parties.answer.body.parties : null;
  const knownRelationships = relationships.answer?.kind === 'outcome' ? relationships.answer.body.relationships : null;
  const partyRows = knownParties ?? [];
  const relationshipRows = knownRelationships ?? [];

  if (view.kind === 'object') {
    const row = partyRows.find((party) => party.partyId === view.partyId) ?? null;
    return (
      <BusinessPartyDetailPage
        partyId={view.partyId}
        row={row}
        listAnswer={parties.answer}
        retry={parties.retry}
        relationships={relationships.answer}
        retryRelationships={relationships.retry}
        onBack={() => tabReturn.returnTo('business-parties')}
      />
    );
  }

  // 地址里的登记视图可能是别人发来的链接：没有登记写授予时不进，落回列表。
  if (view.kind === 'register' && mayWrite) {
    const heading = registerHeadings[view.register];
    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        <div className="flex shrink-0 flex-wrap items-center gap-4 border-b border-idpxyz-border px-6 py-3">
          <Button variant="ghost" size="sm" className="text-[12px] text-idpxyz-breadcrumb" onClick={requestLeaveRegister}>
            <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
            返回列表
          </Button>
          <div className="min-w-0">
            <h1 className="text-[16px] font-semibold leading-tight text-idpxyz-textBright">{heading.title}</h1>
            <p className="truncate text-[12px] text-idpxyz-breadcrumb">{heading.subtitle}</p>
          </div>
          {/* 三本册的切换：换册只换地址键值（openRegister），切走的表单留着挂载。 */}
          <Tabs
            value={view.register}
            onValueChange={(next) => openRegister(next as BusinessPartyRegister)}
            className="ml-auto shrink-0 gap-0"
          >
            <TabsList aria-label="选择登记册">
              {businessPartyRegisters.map((id) => (
                <TabsTrigger key={id} value={id} className="text-[12px] text-idpxyz-breadcrumb">
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
          onPartiesChanged={parties.retry}
          onRelationshipsChanged={relationships.retry}
          preset={preset}
          reportUnsaved={reportUnsaved}
        />
        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={confirmingLeave}
          tone="warning"
          title="离开登记视图？"
          message="有登记还没有提交，离开后填写的内容不会保留。"
          confirmLabel="离开"
          cancelLabel="继续填写"
          onConfirm={leaveRegister}
          onCancel={() => setConfirmingLeave(false)}
        />
      </div>
    );
  }

  const partiesAnswered = parties.answer?.kind === 'outcome';
  const relationshipsAnswered = relationships.answer?.kind === 'outcome';
  const tenantPrefix = boundTenant ? `集团 ${boundTenant} · ` : '';
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
  const bookStrip = (
    <Tabs value={book} onValueChange={(next) => setBook(next as Book)} className="shrink-0 gap-0">
      <TabsList className="shrink-0 px-4" aria-label="选择读册">
        <TabsTrigger value="identities">
          参与方身份
          {partiesAnswered ? <span className="ml-1.5 font-mono text-[12px] text-idpxyz-breadcrumb">{partyRows.length}</span> : null}
        </TabsTrigger>
        <TabsTrigger value="relationships">
          参与方关系
          {relationshipsAnswered ? (
            <span className="ml-1.5 font-mono text-[12px] text-idpxyz-breadcrumb">{relationshipRows.length}</span>
          ) : null}
        </TabsTrigger>
      </TabsList>
    </Tabs>
  );

  if (book === 'identities') {
    // 筛选与排序只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约（归票 04）。
    const visible = sortBusinessParties(filterBusinessParties(partyRows, { search, status }), sort);
    const counts = countBusinessParties(partyRows);
    const effectiveShare = effectiveShareOfInUse(counts);
    const toggleStatus = (code: BusinessPartyStatusFilter) => setStatus(status === code ? 'ALL' : code);

    const kpis: WorkbenchKpi[] | undefined = partiesAnswered
      ? [
          {
            label: '已登记未生效',
            value: counts.byStatus.REGISTERED,
            tone: counts.byStatus.REGISTERED > 0 ? statusToneText.warning : undefined,
            icon:
              counts.byStatus.REGISTERED > 0 ? <Clock className={`h-3 w-3 ${statusToneText.warning}`} aria-hidden="true" /> : undefined,
            active: status === 'REGISTERED',
            onClick: () => toggleStatus('REGISTERED'),
          },
          {
            label: '已生效',
            value: counts.byStatus.EFFECTIVE,
            tone: statusToneText.info,
            active: status === 'EFFECTIVE',
            onClick: () => toggleStatus('EFFECTIVE'),
          },
          {
            label: '已停用',
            value: counts.byStatus.DEACTIVATED,
            active: status === 'DEACTIVATED',
            onClick: () => toggleStatus('DEACTIVATED'),
          },
          {
            label: '在用中已生效',
            value: '',
            custom:
              effectiveShare === null ? (
                <span className="text-[12px] font-semibold leading-none text-idpxyz-breadcrumb">—</span>
              ) : (
                <div className="flex items-center gap-1.5">
                  <EffectiveShareBar share={effectiveShare} label="在用参与方身份中已生效的占比" />
                  <span className="text-[12px] font-semibold leading-none text-idpxyz-textBright">{effectiveShare}%</span>
                </div>
              ),
          },
        ]
      : undefined;

    const statusChips: WorkbenchChip[] | undefined = partiesAnswered
      ? businessPartyStatusFilterOptions
          .filter(
            (option) =>
              option.value === 'ALL' || option.value === status || counts.byStatus[option.value as keyof typeof counts.byStatus] > 0,
          )
          .map((option) => ({
            id: option.value,
            label: option.label,
            count: option.value === 'ALL' ? counts.total : counts.byStatus[option.value as keyof typeof counts.byStatus],
            active: status === option.value,
            onClick: () => (option.value === 'ALL' ? setStatus('ALL') : toggleStatus(option.value)),
          }))
      : undefined;

    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        {bookStrip}
        <WorkbenchPageTemplate<BusinessPartyRecord>
          title={info.title}
          subtitle={`${tenantPrefix}业务参与方是与本网络发生商业往来的对象——货主、承运商、代理、转售商 · 每次登记形成新修订，历史不可覆盖`}
          kpis={kpis}
          onRefresh={parties.retry}
          refreshing={parties.pending}
          primaryAction={primaryAction}
          search={{ value: search, onChange: setSearch, placeholder: '搜索参与方标识、名称或状态…' }}
          statusChips={statusChips}
          toolbarRight={partiesAnswered ? businessPartyCountSummary(partyRows.length, visible.length) : undefined}
          columns={identityColumns}
          rows={visible}
          rowKey={(row) => row.partyId}
          selectedKey={selectedParty}
          onSelect={setSelectedParty}
          sort={sort}
          onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
          noMatchText={businessPartyNoMatchNote}
          onClearFilters={() => {
            setSearch('');
            setStatus('ALL');
          }}
          renderDetail={(key) => {
            const row = visible.find((party) => party.partyId === key);
            return row ? (
              <BusinessPartyDetailPanel
                row={row}
                relationships={relationships.answer}
                retryRelationships={relationships.retry}
                onClose={() => setSelectedParty(null)}
                onOpenObjectPage={() => {
                  window.location.hash = businessPartyObjectHash(row.partyId);
                }}
                onDeactivate={() => openDeactivation({ kind: 'BUSINESS_PARTY', id: row.partyId })}
              />
            ) : null;
          }}
          // 筛出为空不是空态（票 01 裁决 1）：viewState 按总数判，表格区另显 noMatchText。
          viewState={catalogueViewState(parties.answer, partyRows.length, parties.retry, {
            module: info,
            endpoint: 'GET /commercial-business-parties',
            emptyTitle: '当前租户尚无参与方身份登记',
            emptyDescription: `还没有登记任何业务参与方；本页不会预置参与方。${entryHint(mayWrite, '用页头的「登记参与方身份」登记第一个。')}`,
          })}
        />
      </div>
    );
  }

  const visible = sortPartyRelationships(
    filterPartyRelationships(relationshipRows, { search: relationshipSearch, role, status: relationshipStatus, attentionOnly }),
    relationshipSort,
  );
  const counts = countPartyRelationships(relationshipRows);
  const toggleRelationshipStatus = (code: PartyRelationshipStatusFilter) =>
    setRelationshipStatus(relationshipStatus === code ? 'ALL' : code);

  const kpis: WorkbenchKpi[] | undefined = relationshipsAnswered
    ? [
        {
          label: '待查',
          value: counts.attention,
          tone: counts.attention > 0 ? statusToneText.warning : undefined,
          icon: counts.attention > 0 ? <AlertTriangle className={`h-3 w-3 ${statusToneText.warning}`} aria-hidden="true" /> : undefined,
          active: attentionOnly,
          onClick: () => setAttentionOnly(!attentionOnly),
        },
        {
          label: '候选关系',
          value: counts.byStatus.CANDIDATE,
          tone: counts.byStatus.CANDIDATE > 0 ? statusToneText.warning : undefined,
          active: relationshipStatus === 'CANDIDATE',
          onClick: () => toggleRelationshipStatus('CANDIDATE'),
        },
        {
          label: '已生效',
          value: counts.byStatus.EFFECTIVE,
          tone: statusToneText.info,
          active: relationshipStatus === 'EFFECTIVE',
          onClick: () => toggleRelationshipStatus('EFFECTIVE'),
        },
      ]
    : undefined;

  const statusChips: WorkbenchChip[] | undefined = relationshipsAnswered
    ? partyRelationshipStatusFilterOptions
        .filter(
          (option) =>
            option.value === 'ALL' ||
            option.value === relationshipStatus ||
            counts.byStatus[option.value as keyof typeof counts.byStatus] > 0,
        )
        .map((option) => ({
          id: option.value,
          label: option.label,
          count: option.value === 'ALL' ? counts.total : counts.byStatus[option.value as keyof typeof counts.byStatus],
          active: relationshipStatus === option.value,
          onClick: () => (option.value === 'ALL' ? setRelationshipStatus('ALL') : toggleRelationshipStatus(option.value)),
        }))
    : undefined;

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
      {bookStrip}
      <WorkbenchPageTemplate<PartyRelationshipRecord>
        title={info.title}
        subtitle={`${tenantPrefix}参与方关系记谁对谁持有什么角色、在什么范围、多久 · 撤销、到期与替代只影响边界之后的新决定`}
        kpis={kpis}
        onRefresh={relationships.retry}
        refreshing={relationships.pending}
        primaryAction={primaryAction}
        search={{ value: relationshipSearch, onChange: setRelationshipSearch, placeholder: '搜索关系、参与方或角色…' }}
        statusChips={statusChips}
        // 角色六格并排成胶囊太长，留作下拉；与状态胶囊叠加。
        extraFilters={
          relationshipsAnswered ? (
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
          ) : undefined
        }
        toolbarRight={relationshipsAnswered ? partyRelationshipCountSummary(relationshipRows.length, visible.length) : undefined}
        columns={relationshipColumns}
        rows={visible}
        rowKey={(row) => row.relationshipId}
        selectedKey={selectedRelationship}
        onSelect={setSelectedRelationship}
        rowAttention={partyRelationshipNeedsAttention}
        sort={relationshipSort}
        onSort={(key) => setRelationshipSort(toggleWorkbenchSort(relationshipSort, key))}
        noMatchText={partyRelationshipNoMatchNote}
        onClearFilters={() => {
          setRelationshipSearch('');
          setRole('ALL');
          setRelationshipStatus('ALL');
          setAttentionOnly(false);
        }}
        renderDetail={(key) => {
          const row = visible.find((relationship) => relationship.relationshipId === key);
          return row ? (
            <PartyRelationshipDetailPanel row={row} onClose={() => setSelectedRelationship(null)} onEnded={relationships.retry} />
          ) : null;
        }}
        // 筛出为空不是空态（票 01 裁决 1）：筛「候选关系」筛没了显 noMatchText，不显「0 段」的空态。
        viewState={catalogueViewState(relationships.answer, relationshipRows.length, relationships.retry, {
          module: info,
          endpoint: 'GET /commercial-party-relationships',
          emptyTitle: '当前租户尚无参与方关系登记',
          emptyDescription: `还没有登记任何参与方关系；本页不会预置参与方或关系。${entryHint(mayWrite, '用页头的「登记参与方关系」登记第一段。')}`,
        })}
      />
    </div>
  );
}
