import { useEffect, useMemo, useRef, useState } from 'react';
import { useMarkUnsavedInput } from '../../templates/unsaved-input';
import { useOwnAddressChange } from '../../templates/use-own-address-change';
import { AlertTriangle, ArrowLeft, Clock, Info, Plus, X } from 'lucide-react';
import {
  Button,
  ConfirmDialog,
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@idpxyz/ui-primitives';
import {
  WorkbenchPageTemplate,
  toggleWorkbenchSort,
  useTabReturn,
  type WorkbenchColumn,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../components/primary-action';
import { statusToneSurface, statusToneText } from '../../components/status-tone';
import { catalogueViewState } from '../catalogue-view';
import { useAddressKeyword } from '../../templates/use-address-keyword';
import { listGroupLegalEntities, type GroupLegalEntityRecord } from './api';
import { useBoundTenant } from '../use-operator-binding';
import { groupLegalEntitiesViewOf, registerViewHash } from './group-legal-entities-address';
import { identityLayerAbsentNote } from './presentation';
import { LegalEntityRegistrationForm } from './LegalEntityRegistrationForm';
import {
  identityLayerCellsOf,
  registrationNumberTypeNamesOf,
  type RegistrationNumberTypeNameOf,
} from './legal-entity-identity';
import { LegalEntityDetailPage } from './LegalEntityDetailPage';
import { LegalEntityDetailPanel } from './LegalEntityDetailPanel';
import { filterSelectClass, Instant, UnknownPartyName } from './detail-primitives';
import { StatusPill } from '../../components/StatusPill';
import { useTabViewState } from '../../templates/workspace-view-state';
import { useRegisterList } from './register-list';
import {
  countLegalEntities,
  defaultLegalEntitySort,
  filterLegalEntities,
  legalEntityCountSummary,
  legalEntityNeedsAttention,
  legalEntityNoMatchNote,
  sortLegalEntities,
  type LegalEntityStatusFilter,
} from './legal-entity-list';
import { useMayWriteRegistry } from '../use-operator-grants';
import { CountryLabel } from '../refdata-labels';
import { entryHint } from '../operator-grants';
import { neighborsOf, prefetchLegalEntityReads, useCachedRegistrationNumberTypes } from './legal-entity-read-cache';

// 列表取回后预读右栏的前几行：一行三个很小的读，二十行是六十个请求，再多就只读选中行的邻行。
const prefetchedRowLimit = 20;

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

function emptyDescriptionOf(mayWrite: boolean): string {
  return `签约、开票都要用我们自己公司的名义。${entryHint(mayWrite, '用页头的「新增公司主体」登记第一个。')}`;
}

/** 「待处理」胶囊的悬停说明：判据在 legal-entity-list.ts 的 legalEntityNeedsAttention，这里只把它说成运营的话。 */
const attentionFilterTitle = '注册地或注册号缺失，或关联的往来单位查不到';

interface LegalEntityFilterItem {
  label: string;
  count: number;
  active: boolean;
  disabled?: boolean;
  /** 悬停说明；只有判据不自明的胶囊（待处理）给。 */
  title?: string;
  onClick: () => void;
}

function LegalEntityFilterButton({ item }: { item: LegalEntityFilterItem }) {
  return (
    <button
      type="button"
      onClick={item.onClick}
      aria-pressed={item.active}
      title={item.title}
      disabled={item.disabled ?? (item.count === 0 && !item.active)}
      className={`inline-flex h-[34px] items-center gap-1 whitespace-nowrap border-y-2 border-transparent text-[12px] leading-normal transition-colors focus-visible:border-b-idpxyz-accent focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50 ${
        item.active
          ? 'border-b-idpxyz-accent font-semibold text-idpxyz-textBright'
          : 'text-idpxyz-breadcrumb hover:text-idpxyz-textBright'
      }`}
    >
      <span>{item.label}</span>
      <span className="font-medium tabular-nums text-idpxyz-textBright">{item.count}家</span>
    </button>
  );
}

function LegalEntityStatusFilters({ items }: { items: LegalEntityFilterItem[] }) {
  return (
    <div
      role="group"
      aria-label="生命周期状态筛选"
      className="flex flex-wrap items-center gap-x-4 gap-y-2"
    >
      {items.map((item) => (
        <LegalEntityFilterButton key={item.label} item={item} />
      ))}
    </div>
  );
}

// 行首圆点的颜色随身份状态；待补的行圆点闪，与参照页「逾期 / 暂挂」同一个记号。
const statusDotClass: Record<string, string> = {
  REGISTERED: statusToneSurface.warning,
  EFFECTIVE: statusToneSurface.success,
  DEACTIVATED: 'bg-idpxyz-textMuted',
};

// 行里的编码与注册号不做点击复制（票 company-subject-ux/14 第 4 条）：行点击是选中、开详情栏，两种点击叠在一格上会冲突——
// 想复制号的人一点，详情栏切到了别的行；复制只在详情栏与对象页的头区做，那里号是 RefText。主列的编码同此。
//
// 身份两格号在上、类型名与国家小字在下，与主列「名称 / 编码」两层同形（票 company-subject-ux/08 第 6 条）：号才是要认的，
// 类型名（演示目录的长名）不该比号还显眼。国家经基础数据目录译成「中国（CN）」（码仍在括号里，查不到原样显示，
// ADR-0335 决定七），类型码经注册号类型目录译成「统一社会信用代码」（查法由页面读一次目录折成，译不出原样显码，
// legal-entity-identity.ts）；号本身照答复原样。最新修订登记于身份层落地之前时如实写 identityLayerAbsentNote。
function IdentityLayerCell({ row, typeNameOf }: { row: GroupLegalEntityRecord; typeNameOf: RegistrationNumberTypeNameOf }) {
  const cells = identityLayerCellsOf(row, typeNameOf);
  if (cells === null) return <span className="text-idpxyz-breadcrumb">{identityLayerAbsentNote}</span>;
  const country = cells.country === '' ? null : <CountryLabel code={cells.country} />;
  if (cells.numberCells.length === 0) {
    return <p className="min-w-40 text-[13px] leading-5 text-idpxyz-text">{country}</p>;
  }
  return (
    <div className="min-w-40">
      <p className="font-mono text-[13px] leading-5 text-idpxyz-text">{cells.numberCells.map((cell) => cell.number).join('；')}</p>
      <p className="mt-0.5 truncate text-[12px] leading-4 text-idpxyz-breadcrumb" title={cells.numbers}>
        {cells.numberCells.map((cell) => cell.typeName).join('；')}
        {country ? <> · {country}</> : null}
      </p>
    </div>
  );
}

// 骨架期这里列过「运营集团租户」——ADR-0003 里那一级是配置与隔离边界本身，读面本来就在单租户
// 作用域内取数，整列同值没有信息，接线时按 live 页惯例撤下。「对象类型」列同一判据撤下（票 01 第 3 条）：
// 本册今天只有 RESPONSIBLE_LEGAL_ENTITY 一格，种类进详情栏与对象页。
//
// 列表是函数不是常量：注册号那一列要带目录查法，目录是页面挂载后读回的；页面用 useMemo 只在查法变时重建。
//
// 宽屏下的列宽纪律（票 company-subject-ux/08 第 6 条）：前两列给定宽（模板只开放每列的 width，表格本身撑满列表区），
// 多出来的宽度就落到没定宽的状态与日期列上，它们的内容靠左、紧跟前一列，不再被前两列推到右边缘。主列内容另上限 22rem：
// 列表比各列之和还宽时浏览器会把定宽列也拉宽，名称那一格不跟着长。
function columnsOf(typeNameOf: RegistrationNumberTypeNameOf): WorkbenchColumn<GroupLegalEntityRecord>[] {
  return [
  {
    key: 'party',
    header: '公司主体',
    sortKey: 'party-name',
    width: '22rem',
    // 名称在参与方册上（法人不抄第二份）；转写不到那一格的话与判据在 presentation.ts 的 partyNameUnknownNote。
    // 主列把名称与法人标识并排成两层，方便扫描；参与方标识、修订号、登记依据仍进详情栏底部的「技术详情」。
    render: (row) => (
      <div className="flex min-w-44 max-w-[22rem] items-start gap-2">
        <span
          className={`mt-[7px] h-1.5 w-1.5 shrink-0 rounded-full ${statusDotClass[row.status] ?? 'bg-idpxyz-textMuted'} ${
            legalEntityNeedsAttention(row) ? 'animate-pulse' : ''
          }`}
          aria-hidden="true"
        />
        <span
          className="min-w-0 truncate text-[14px] font-semibold leading-5 text-idpxyz-textBright"
          title={row.partyNameKnown ? row.partyName : row.legalEntityId}
        >
          <span className="block truncate">{row.partyNameKnown ? row.partyName : <UnknownPartyName />}</span>
          <span className="mt-0.5 block truncate font-mono text-[12px] font-normal leading-4 text-idpxyz-breadcrumb">
            {row.legalEntityId}
          </span>
        </span>
        {legalEntityNeedsAttention(row) && (
          <AlertTriangle className={`mt-1 h-3 w-3 shrink-0 ${statusToneText.warning}`} aria-label="待处理" />
        )}
      </div>
    ),
  },
  {
    key: 'identity-layer',
    header: '注册地与注册号',
    minListWidth: 680,
    width: '20rem',
    render: (row) => <IdentityLayerCell row={row} typeNameOf={typeNameOf} />,
  },
  {
    key: 'status',
    header: '状态',
    // 已停用行标出停用时点：停用只自其时点起不再支持新的商业决定，时点是这格状态的内容而不是装饰。
    // 三个时刻都只显到日期（spec 公司主体 UX「统一用语」）：生效 / 登记 / 停用对运营是「哪一天」，完整时刻在悬停里。
    render: (row) => (
      <div className="flex flex-col items-start gap-1">
        <StatusPill code={row.status} />
        {row.deactivatedAt ? (
          <span className="font-mono text-[12px] text-idpxyz-breadcrumb">
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
    // 待生效配一个琥珀色时钟而不是把时刻标琥珀色：琥珀色字落在选中行与关注行的底色上不足 4.5:1（票 group-legal-entities-uiux/08
    // 评审），图标是非文字，门槛 3:1。
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
      <span className="font-mono text-idpxyz-breadcrumb">
        <Instant value={row.registeredAt} precision="day" />
      </span>
    ),
  },
  ];
}

/** 详情地址：hash 二段。壳层拿前两段作标签 id，对象页因此另开一张标签，列表留在原处。 */
function detailHash(legalEntityId: string): string {
  return `#/group-legal-entities/${encodeURIComponent(legalEntityId)}`;
}

/**
 * 集团与法人（party-commercial）。行对象是责任法人的最新登记修订：法人钉在稳定的业务参与方身份上
 * （ADR-0003 三级边界的第二级），名称从参与方册转写；身份登记按修订版本化不可覆盖，停用形成新修订而不是删除。
 *
 * 形态照 idp-prism 采购订单工作台（票 admin-web-group-legal-entities/15，用户令「完全参考」，取代 spec「不做」里
 * 「不改两签结构」那一条）：搜索栏旁的状态计数可点即筛、表头点排序、单击行在右栏开详情（可拖宽）、
 * 详情栏「更多」或对象地址开整页对象标签、↑/↓ 换行 Esc 收栏 `/` 检索。登记从页头主动作进登记视图，不再是第二个签。
 *
 * 参照页有而这里没有的，都是因为册上没有对应的事实或端点：分页（读口一页答完，下推归票 04）、批量勾选
 * （没有批量命令端点）、收货 / 证据 / 协作（法人册不承载）。指标与计数数的是已取回的这一页，只在拿到业务答案后显示——
 * 未配置态与错误态下报「0 个」会与状态区「这不是目录为空」直接矛盾（README 列表页上列通则）。
 *
 * 没有行级编辑或删除面：更正占下一个修订号翻旧插新，停用形成新修订。停用留在业务参与方页——那一页读得最全，
 * 快照收 deactivations 数组、kind 在每一项上，按身份切签就得让每页拒收非本页那种 kind，等于管理台编一条
 * 服务端没有的约束。墙降之前登记必然答 403「接入渠道未配置」，那是诚实答案。
 *
 * 列表状态（检索、筛选、排序、选中行）挂在页面上：进登记视图再回来还在。
 *
 * 登记视图住在地址上（`?register=1`，判读在 group-legal-entities-address.ts；票 group-legal-entities-uiux/04）：刷新仍停在
 * 登记视图，浏览器后退回列表。草稿有没提交的输入时，页内「返回列表」先确认，刷新与关页由 beforeunload 拦；浏览器后退
 * 不拦——拦它得改写历史栈，壳层按 hashchange 记每张标签的地址，改写会让两边对不上。
 */
export function GroupLegalEntitiesPage() {
  const { answer, retry, pending } = useRegisterList(listGroupLegalEntities);
  // 页名里的「集团」是 ADR-0003 三级边界的第一级，即本会话绑定的租户（票 group-legal-entities-uiux/11）。答不出就不显，
  // 不拿列表行上的 tenantId 顶：空册时那一格根本没有，有时有、有时没有的页头比不显更让人疑惑。
  const boundTenant = useBoundTenant();
  // 页名下不再写集团标识：一家公司一套系统，租户是谁不必告诉运营；绑定仍读着，壳层顶栏显公司名用它。
  void boundTenant;
  // 没有登记册配置写授予的操作者不摆登记与发布入口（ADR-0315）；摆不摆只是呈现，写口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const tabReturn = useTabReturn();
  // 渲列表、登记视图还是对象页由地址定：对象地址在壳层是自己的一张标签，本组件在那张标签里渲对象页。
  const [hash, setHash] = useState(() => window.location.hash);
  const view = groupLegalEntitiesViewOf(hash);
  // 本次挂载里是不是从列表压进登记视图的：是就用后退回列表，不再压一条；刷新或从别的标签回来直接落在登记视图时不是，
  // 那时后退一格不一定是列表，改用替换。
  const pushedRegister = useRef(false);
  const [registerUnsaved, setRegisterUnsaved] = useState(false);
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  // 检索词在地址里（`?q=`）：别处的引用链接带着它过来，筛到那一条；刷新与换标签回来也还在。
  const [search, setSearch] = useAddressKeyword();
  const [status, setStatus] = useTabViewState<LegalEntityStatusFilter>('legal-entities.status', 'ALL');
  const [registrationCountry, setRegistrationCountry] = useTabViewState('legal-entities.registration-country', 'ALL');
  const [attentionOnly, setAttentionOnly] = useTabViewState('legal-entities.attention-only', false);
  const [helpOpen, setHelpOpen] = useState(false);
  const helpButtonRef = useRef<HTMLButtonElement>(null);
  const [sort, setSort] = useTabViewState<WorkbenchSort>('legal-entities.sort', defaultLegalEntitySort);
  const [selectedKey, setSelectedKey] = useTabViewState<string | null>('legal-entities.selected', null);
  // 注册号类型目录读一次，给「注册地与注册号」列把类型码译成名。读不到（未配置、拒绝、故障）就按码显，不报错、不挡列表：
  // 译名是锦上添花，号本身在答复里。经页面内缓存读，再开这一页时译名与表同一帧出来。
  const typeCatalogue = useCachedRegistrationNumberTypes();
  const typeNameOf = useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
  const columns = useMemo(() => columnsOf(typeNameOf), [typeNameOf]);

  useOwnAddressChange(() => setHash(window.location.hash));

  // 刷新与关页时浏览器自己弹确认；只在登记视图且草稿有没提交的输入时挂。
  const guardUnload = view.kind === 'register' && registerUnsaved;
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

  const openRegister = () => {
    pushedRegister.current = true;
    window.location.hash = registerViewHash(window.location.hash, true);
  };
  const leaveRegister = () => {
    setConfirmingLeave(false);
    if (pushedRegister.current) {
      pushedRegister.current = false;
      window.history.back();
      return;
    }
    window.location.replace(registerViewHash(window.location.hash, false));
  };
  const requestLeaveRegister = () => (registerUnsaved ? setConfirmingLeave(true) : leaveRegister());

  // 列表没取到（加载中 / 未配置 / 出错）传 null：登记表单那边建议修订号一律为 1，不拿空数组冒充「册上没有」。
  const knownEntities = answer?.kind === 'outcome' ? answer.body.entities : null;
  const entities = knownEntities ?? [];
  const registrationCountries = useMemo(
    () =>
      [...new Set(entities.map((entity) => identityLayerCellsOf(entity)?.country).filter((country): country is string => Boolean(country)))].sort(),
    [entities],
  );

  // 右栏的三种读预先读好（legal-entity-read-cache.ts）：列表取回后先把前若干行读进页面内缓存，选中一行后再把它在当前
  // 排序下的上下邻行读好——点哪一行、方向键走到哪一行，右栏都直接显，不再先清空成「正在读取」再撑开。只在列表视图做；
  // 前若干行之外的行点到时照常现读（有占位轮廓，不塌）。
  useEffect(() => {
    if (view.kind !== 'list' || knownEntities === null) return;
    const timer = window.setTimeout(() => {
      for (const entity of knownEntities.slice(0, prefetchedRowLimit)) prefetchLegalEntityReads(entity);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [view.kind, knownEntities]);
  useEffect(() => {
    if (view.kind !== 'list' || selectedKey === null) return;
    const ordered = sortLegalEntities(filterLegalEntities(entities, { search, status, registrationCountry, attentionOnly }), sort);
    for (const entity of neighborsOf(ordered, (row) => row.legalEntityId, selectedKey)) prefetchLegalEntityReads(entity);
  }, [view.kind, selectedKey, entities, search, status, registrationCountry, attentionOnly, sort]);

  if (view.kind === 'object') {
    const row = entities.find((entity) => entity.legalEntityId === view.legalEntityId) ?? null;
    return (
      <LegalEntityDetailPage
        legalEntityId={view.legalEntityId}
        row={row}
        listAnswer={answer}
        retry={retry}
        onBack={() => tabReturn.returnTo('group-legal-entities')}
        typeNameOf={typeNameOf}
      />
    );
  }

  // 地址里的登记视图可能是别人发来的链接：没有登记写授予时不进，落回列表。
  if (view.kind === 'register' && mayWrite) {
    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        <div className="flex shrink-0 items-center gap-4 border-b border-idpxyz-border px-6 py-3">
          <Button variant="ghost" size="sm" className="text-[12px] text-idpxyz-breadcrumb" onClick={requestLeaveRegister}>
            <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
            返回列表
          </Button>
          <div className="min-w-0">
            <h1 className="text-[16px] font-semibold leading-tight text-idpxyz-textBright">新增公司主体</h1>
            <p className="truncate text-[12px] text-idpxyz-breadcrumb">
              我们自己签约、开票用的公司；填注册地与注册号，开票资料和联系人保存后在详情里补
            </p>
          </div>
        </div>
        <LegalEntityRegistrationForm
          knownEntities={knownEntities}
          onRegistered={retry}
          onUnsavedChange={setRegisterUnsaved}
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

  // 筛选与排序只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约（归票 04）。
  const visible = sortLegalEntities(filterLegalEntities(entities, { search, status, registrationCountry, attentionOnly }), sort);
  const answered = answer?.kind === 'outcome';
  const counts = countLegalEntities(entities);
  const toggleStatus = (code: LegalEntityStatusFilter) => setStatus(status === code ? 'ALL' : code);

  const allFilter: LegalEntityFilterItem | undefined = answered
    ? {
        label: '全部',
        count: counts.total,
        active: status === 'ALL' && !attentionOnly,
        disabled: false,
        onClick: () => {
          setStatus('ALL');
          setAttentionOnly(false);
        },
      }
    : undefined;
  const attentionFilter: LegalEntityFilterItem | undefined = answered
    ? {
        label: '待处理',
        count: counts.attention,
        active: attentionOnly,
        title: attentionFilterTitle,
        onClick: () => setAttentionOnly(!attentionOnly),
      }
    : undefined;
  const statusItems: LegalEntityFilterItem[] | undefined = answered
    ? [
        {
          label: '待生效',
          count: counts.byStatus.REGISTERED,
          active: status === 'REGISTERED',
          onClick: () => toggleStatus('REGISTERED'),
        },
        {
          label: '已生效',
          count: counts.byStatus.EFFECTIVE,
          active: status === 'EFFECTIVE',
          onClick: () => toggleStatus('EFFECTIVE'),
        },
        {
          label: '已停用',
          count: counts.byStatus.DEACTIVATED,
          active: status === 'DEACTIVATED',
          onClick: () => toggleStatus('DEACTIVATED'),
        },
      ]
    : undefined;
  const hasActiveFilters = search.length > 0 || status !== 'ALL' || registrationCountry !== 'ALL' || attentionOnly;
  const clearFilters = () => {
    setSearch('');
    setStatus('ALL');
    setRegistrationCountry('ALL');
    setAttentionOnly(false);
  };

  return (
    <Dialog open={helpOpen} onOpenChange={setHelpOpen}>
      <WorkbenchPageTemplate<GroupLegalEntityRecord>
      title={info.title}
      viewStateKey="legal-entities"
      subtitle="我们自己签约、开票、结算用的公司"
      onRefresh={retry}
      refreshing={pending}
      primaryAction={
        <>
          <Button
            ref={helpButtonRef}
            type="button"
            variant="ghost"
            size="sm"
            className="gap-1.5 text-[12px] text-idpxyz-accent"
            aria-haspopup="dialog"
            aria-expanded={helpOpen}
            onClick={() => setHelpOpen(true)}
          >
            <Info className="h-3.5 w-3.5" aria-hidden="true" />
            使用说明
          </Button>
          {mayWrite ? (
            <Button variant="default" className={primaryActionClass} onClick={openRegister}>
              <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
              新增公司主体
            </Button>
          ) : null}
        </>
      }
      search={{ value: search, onChange: setSearch, placeholder: '搜索公司名称、编码或注册号…' }}
      extraFilters={
        answered ? (
          <div className="flex flex-wrap items-center gap-x-5 gap-y-2">
            {allFilter ? <LegalEntityFilterButton item={allFilter} /> : null}
            {attentionFilter ? (
              <div role="group" aria-label="待处理事项筛选" className="flex items-center">
                <LegalEntityFilterButton item={attentionFilter} />
              </div>
            ) : null}
            {statusItems ? <LegalEntityStatusFilters items={statusItems} /> : null}
            <select
              className={filterSelectClass + ' h-[34px] leading-normal'}
              value={registrationCountry}
              aria-label="注册国家或地区"
              onChange={(event) => setRegistrationCountry(event.target.value)}
            >
              <option value="ALL">全部注册地</option>
              {registrationCountries.map((country) => (
                <option key={country} value={country}>
                  <CountryLabel code={country} />
                </option>
              ))}
            </select>
            {hasActiveFilters ? (
              <button
                type="button"
                className="inline-flex h-[34px] items-center whitespace-nowrap text-[12px] leading-normal text-idpxyz-breadcrumb hover:text-idpxyz-textBright focus-visible:outline-none focus-visible:underline"
                onClick={clearFilters}
              >
                清除
              </button>
            ) : null}
          </div>
        ) : undefined
      }
      toolbarRight={answered ? legalEntityCountSummary(entities.length, visible.length) : undefined}
      columns={columns}
      rows={visible}
      rowKey={(row) => row.legalEntityId}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      rowAttention={legalEntityNeedsAttention}
      sort={sort}
      onSort={(key) => setSort(toggleWorkbenchSort(sort, key))}
      noMatchText={legalEntityNoMatchNote}
      onClearFilters={clearFilters}
      renderDetail={(key) => {
        const row = visible.find((entity) => entity.legalEntityId === key);
        return row ? (
          <LegalEntityDetailPanel
            row={row}
            onClose={() => setSelectedKey(null)}
            onOpenObjectPage={() => {
              window.location.hash = detailHash(row.legalEntityId);
            }}
            onChanged={retry}
            typeNameOf={typeNameOf}
          />
        ) : null;
      }}
      // 筛出为空不是空态（票 01 裁决 1）：viewState 按总数判，表格区另显 noMatchText。
      viewState={catalogueViewState(answer, entities.length, retry, {
        module: info,
        endpoint: 'GET /commercial-group-legal-entities',
        emptyTitle: '还没有登记任何公司主体',
        emptyDescription: emptyDescriptionOf(mayWrite),
      })}
      />
      <DialogContent
        size="lg"
        className="max-h-[80vh] overflow-y-auto"
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          helpButtonRef.current?.focus();
        }}
      >
          <DialogHeader className="items-start">
            <div>
              <DialogTitle>使用说明</DialogTitle>
              <DialogDescription className="mt-1">
                公司主体是我们自己签约、开票、结算用的公司。
              </DialogDescription>
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="关闭使用说明"
              onClick={() => setHelpOpen(false)}
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </Button>
          </DialogHeader>
          {/* 全文是运营的话（spec 公司主体 UX「统一用语」）：不出现「责任法人」「修订」「身份」这些领域词，它们留在 CONTEXT 与技术详情。 */}
          <DialogBody className="space-y-4 text-[12px] leading-5 text-idpxyz-text">
            <section>
              <h3 className="font-semibold text-idpxyz-textBright">这里登记谁</h3>
              <p className="mt-1">
                这里登记的是我们自己开票、签约用的公司。客户、供应商在「往来单位」里登记。
              </p>
            </section>
            <section>
              <h3 className="font-semibold text-idpxyz-textBright">状态与待处理</h3>
              <ul className="mt-1 list-disc space-y-1 pl-4">
                <li>待生效：已经登记，生效日期还没到。</li>
                <li>已生效：现在可以用它签约、开票。</li>
                <li>已停用：从停用日期起不再用它签新约、开新票，之前的单据不受影响。</li>
                <li>待处理：{attentionFilterTitle}；打开详情看怎么补。</li>
              </ul>
            </section>
            <section>
              <h3 className="font-semibold text-idpxyz-textBright">资料怎么维护</h3>
              <p className="mt-1">
                注册地、注册号改了不覆盖旧的，每保存一次就多一版登记记录。开票抬头、监管登记号、收款账户、地址和联系人叫公司资料，在详情里点
                「更新公司资料」登记，也是一版一版留着；开票时按日期取当天生效的那一版。
              </p>
            </section>
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setHelpOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
