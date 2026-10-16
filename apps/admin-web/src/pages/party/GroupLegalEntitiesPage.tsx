import { useEffect, useRef, useState } from 'react';
import { AlertTriangle, ArrowLeft, Clock, Plus } from 'lucide-react';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
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
import { statusToneSurface, statusToneText } from '../../components/status-tone';
import { catalogueViewState } from '../catalogue-view';
import { listGroupLegalEntities, type GroupLegalEntityRecord } from './api';
import { useBoundTenant } from '../use-operator-binding';
import { groupLegalEntitiesViewOf, registerViewHash } from './group-legal-entities-address';
import { identityLayerAbsentNote, identityRevisionLabel, identityStatusLabels } from './presentation';
import { LegalEntityRegistrationForm } from './LegalEntityRegistrationForm';
import { identityLayerCellsOf } from './legal-entity-identity';
import { LegalEntityDetailPage } from './LegalEntityDetailPage';
import { LegalEntityDetailPanel } from './LegalEntityDetailPanel';
import { Instant, UnknownPartyName, statusBadge } from './detail-primitives';
import { useRegisterList } from './register-list';
import {
  countLegalEntities,
  defaultLegalEntitySort,
  effectiveShareOfInUse,
  filterLegalEntities,
  legalEntityCountSummary,
  legalEntityNeedsAttention,
  legalEntityNoMatchNote,
  legalEntityStatusFilterOptions,
  sortLegalEntities,
  type LegalEntityStatusFilter,
} from './legal-entity-list';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

const emptyDescription = '用页头「登记责任法人」登记第一个，或用受控 CLI parcel-commercial register-parties 灌入。';

// 行首圆点的颜色随身份状态；待补的行圆点闪，与参照页「逾期 / 暂挂」同一个记号。
const statusDotClass: Record<string, string> = {
  REGISTERED: 'bg-sky-400',
  EFFECTIVE: 'bg-emerald-400',
  DEACTIVATED: 'bg-gray-400',
};

// 身份两格照答复原样示出，国家在上、号在下；最新修订登记于身份层落地之前时如实写 identityLayerAbsentNote。
function IdentityLayerCell({ row }: { row: GroupLegalEntityRecord }) {
  const cells = identityLayerCellsOf(row);
  if (cells === null) return <span className="text-idpxyz-breadcrumb">{identityLayerAbsentNote}</span>;
  return (
    <div className="min-w-40">
      <p>{cells.country}</p>
      <p className="mt-0.5 text-idpxyz-breadcrumb">{cells.numbers}</p>
    </div>
  );
}

// 骨架期这里列过「运营集团租户」——ADR-0003 里那一级是配置与隔离边界本身，读面本来就在单租户
// 作用域内取数，整列同值没有信息，接线时按 live 页惯例撤下。「对象类型」列同一判据撤下（票 01 第 3 条）：
// 本册今天只有 RESPONSIBLE_LEGAL_ENTITY 一格，种类进详情栏与对象页。
const columns: WorkbenchColumn<GroupLegalEntityRecord>[] = [
  {
    key: 'legal-entity',
    header: '法人标识',
    sortKey: 'legal-entity',
    render: (row) => (
      <div className="flex min-w-0 items-center gap-1.5">
        <span
          className={`h-1.5 w-1.5 shrink-0 rounded-full ${statusDotClass[row.status] ?? 'bg-idpxyz-textMuted'} ${
            legalEntityNeedsAttention(row) ? 'animate-pulse' : ''
          }`}
          aria-hidden="true"
        />
        {/* 标识取主题的链接色 text.link.hover：强调色 #007acc 落在画布与选中行上只有 3.7–4.2:1。 */}
        <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]" title={row.legalEntityId}>
          {row.legalEntityId}
        </span>
        <span className="shrink-0 font-mono text-[12px] text-idpxyz-breadcrumb">{identityRevisionLabel(row.revision)}</span>
        {legalEntityNeedsAttention(row) && (
          <AlertTriangle className={`h-3 w-3 shrink-0 ${statusToneText.warning}`} aria-label="待补" />
        )}
      </div>
    ),
  },
  {
    key: 'party',
    header: '责任主体',
    sortKey: 'party-name',
    hideWhenDetailOpen: true,
    // 名称在参与方册上（法人不抄第二份）；转写不到那一格的话与判据在 presentation.ts 的 partyNameUnknownNote。
    render: (row) => (
      <div>
        <div className="text-idpxyz-textBright">{row.partyNameKnown ? row.partyName : <UnknownPartyName />}</div>
        <div className="font-mono text-[12px] text-idpxyz-breadcrumb">{row.partyId}</div>
      </div>
    ),
  },
  {
    key: 'identity-layer',
    header: '注册身份',
    hideWhenDetailOpen: true,
    render: (row) => <IdentityLayerCell row={row} />,
  },
  {
    key: 'status',
    header: '状态',
    // 已停用行标出停用时点：停用只自其时点起不再支持新的商业决定，时点是这格状态的内容而不是装饰。
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
    header: '生效自',
    sortKey: 'effective-from',
    // 已登记即还没到生效时点，与参照页「预计到货」临近那一格同一个提示。参照页把时刻本身标琥珀色，这里改配一个琥珀色
    // 时钟：琥珀色字落在选中行与关注行的底色上不足 4.5:1（票 group-legal-entities-uiux/08 评审），图标是非文字，门槛 3:1。
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

/**
 * 「在用中已生效」的占比条。不用上游 Progress：它不转发 aria-*，进度条起不了名字（axe aria-progressbar-name），轨道的
 * `bg-idpxyz-border/50` 在本仓也生不出来（主题色是裸 CSS 变量，见 components/primary-action.ts）。
 */
function EffectiveShareBar({ share }: { share: number }) {
  return (
    <div
      role="progressbar"
      aria-label="在用法人中已生效的占比"
      aria-valuenow={share}
      aria-valuemin={0}
      aria-valuemax={100}
      className="h-1 w-24 overflow-hidden rounded-full bg-idpxyz-border"
    >
      <div
        className={`h-full rounded-full ${share >= 100 ? statusToneSurface.success : 'bg-idpxyz-accent'}`}
        style={{ width: `${share}%` }}
      />
    </div>
  );
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
 * 「不改两签结构」那一条）：命令头指标可点即筛、状态胶囊带计数、表头点排序、单击行在右栏开详情（可拖宽）、
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
  const tabReturn = useTabReturn();
  // 渲列表、登记视图还是对象页由地址定：对象地址在壳层是自己的一张标签，本组件在那张标签里渲对象页。
  const [hash, setHash] = useState(() => window.location.hash);
  const view = groupLegalEntitiesViewOf(hash);
  // 本次挂载里是不是从列表压进登记视图的：是就用后退回列表，不再压一条；刷新或从别的标签回来直接落在登记视图时不是，
  // 那时后退一格不一定是列表，改用替换。
  const pushedRegister = useRef(false);
  const [registerUnsaved, setRegisterUnsaved] = useState(false);
  const [confirmingLeave, setConfirmingLeave] = useState(false);
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<LegalEntityStatusFilter>('ALL');
  const [attentionOnly, setAttentionOnly] = useState(false);
  const [sort, setSort] = useState<WorkbenchSort>(defaultLegalEntitySort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  useEffect(() => {
    const onHashChange = () => setHash(window.location.hash);
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  // 刷新与关页时浏览器自己弹确认；只在登记视图且草稿有没提交的输入时挂。
  const guardUnload = view.kind === 'register' && registerUnsaved;
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

  if (view.kind === 'object') {
    const row = entities.find((entity) => entity.legalEntityId === view.legalEntityId) ?? null;
    return (
      <LegalEntityDetailPage
        legalEntityId={view.legalEntityId}
        row={row}
        listAnswer={answer}
        retry={retry}
        onBack={() => tabReturn.returnTo('group-legal-entities')}
      />
    );
  }

  if (view.kind === 'register') {
    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        <div className="flex shrink-0 items-center gap-4 border-b border-idpxyz-border px-6 py-3">
          <Button variant="ghost" size="sm" className="text-[12px] text-idpxyz-breadcrumb" onClick={requestLeaveRegister}>
            <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
            返回列表
          </Button>
          <div className="min-w-0">
            <h1 className="text-[16px] font-semibold leading-tight text-idpxyz-textBright">登记责任法人</h1>
            <p className="truncate text-[12px] text-idpxyz-breadcrumb">
              每次登记形成新修订，历史不可覆盖；更正身份就登记同一法人的下一修订号
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
          title="离开登记视图？"
          message="这份登记还没有提交，离开后填写的内容不会保留。"
          confirmLabel="离开"
          cancelLabel="继续填写"
          onConfirm={leaveRegister}
          onCancel={() => setConfirmingLeave(false)}
        />
      </div>
    );
  }

  // 筛选与排序只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约（归票 04）。
  const visible = sortLegalEntities(filterLegalEntities(entities, { search, status, attentionOnly }), sort);
  const answered = answer?.kind === 'outcome';
  const counts = countLegalEntities(entities);
  const effectiveShare = effectiveShareOfInUse(counts);
  const toggleStatus = (code: LegalEntityStatusFilter) => setStatus(status === code ? 'ALL' : code);

  const kpis: WorkbenchKpi[] | undefined = answered
    ? [
        {
          label: '待补',
          value: counts.attention,
          tone: counts.attention > 0 ? statusToneText.warning : undefined,
          icon:
            counts.attention > 0 ? <AlertTriangle className={`h-3 w-3 ${statusToneText.warning}`} aria-hidden="true" /> : undefined,
          active: attentionOnly,
          onClick: () => setAttentionOnly(!attentionOnly),
        },
        {
          label: '已登记未生效',
          value: counts.byStatus.REGISTERED,
          tone: counts.byStatus.REGISTERED > 0 ? statusToneText.warning : undefined,
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
          label: '在用中已生效',
          value: '',
          custom:
            effectiveShare === null ? (
              <span className="text-[12px] font-semibold leading-none text-idpxyz-breadcrumb">—</span>
            ) : (
              <div className="flex items-center gap-1.5">
                <EffectiveShareBar share={effectiveShare} />
                <span className="text-[12px] font-semibold leading-none text-idpxyz-textBright">{effectiveShare}%</span>
              </div>
            ),
        },
      ]
    : undefined;

  const statusChips: WorkbenchChip[] | undefined = answered
    ? legalEntityStatusFilterOptions
        .filter(
          (option) =>
            option.value === 'ALL' ||
            option.value === status ||
            counts.byStatus[option.value as keyof typeof counts.byStatus] > 0,
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
    <WorkbenchPageTemplate<GroupLegalEntityRecord>
      title={info.title}
      subtitle={`${boundTenant ? `集团 ${boundTenant} · ` : ''}责任法人是对外签约、开票与结算的经营主体 · 每次登记形成新修订，历史不可覆盖`}
      kpis={kpis}
      onRefresh={retry}
      refreshing={pending}
      primaryAction={
        <Button variant="default" className={primaryActionClass} onClick={openRegister}>
          <Plus className="mr-1.5 h-4 w-4" aria-hidden="true" />
          登记责任法人
        </Button>
      }
      search={{ value: search, onChange: setSearch, placeholder: '搜索法人、参与方身份或名称…' }}
      statusChips={statusChips}
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
      onClearFilters={() => {
        setSearch('');
        setStatus('ALL');
        setAttentionOnly(false);
      }}
      renderDetail={(key) => {
        const row = visible.find((entity) => entity.legalEntityId === key);
        return row ? (
          <LegalEntityDetailPanel
            row={row}
            onClose={() => setSelectedKey(null)}
            onOpenObjectPage={() => {
              window.location.hash = detailHash(row.legalEntityId);
            }}
          />
        ) : null;
      }}
      // 筛出为空不是空态（票 01 裁决 1）：viewState 按总数判，表格区另显 noMatchText。
      viewState={catalogueViewState(answer, entities.length, retry, {
        module: info,
        endpoint: 'GET /commercial-group-legal-entities',
        emptyTitle: '当前租户尚无责任法人登记',
        emptyDescription,
      })}
    />
  );
}
