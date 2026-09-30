import { useEffect, useState } from 'react';
import { AlertTriangle, ArrowLeft, Plus } from 'lucide-react';
import { Button, Progress } from '@idpxyz/ui-primitives';
import {
  WorkbenchPageTemplate,
  decodeHashSegment,
  toggleWorkbenchSort,
  useTabReturn,
  type WorkbenchChip,
  type WorkbenchColumn,
  type WorkbenchKpi,
  type WorkbenchSort,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import { catalogueViewState } from '../catalogue-view';
import { listGroupLegalEntities, type GroupLegalEntityRecord } from './api';
import { identityLayerAbsentNote, identityStatusLabels } from './presentation';
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
  if (cells === null) return <span className="text-idpxyz-textMuted">{identityLayerAbsentNote}</span>;
  return (
    <div className="min-w-40">
      <p>{cells.country}</p>
      <p className="mt-0.5 text-idpxyz-textMuted">{cells.numbers}</p>
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
        <span className="truncate font-mono font-medium text-idpxyz-accent">{row.legalEntityId}</span>
        <span className="shrink-0 font-mono text-[9px] text-idpxyz-textMuted">r{row.revision}</span>
        {legalEntityNeedsAttention(row) && (
          <AlertTriangle className="h-3 w-3 shrink-0 text-orange-400" aria-label="待补" />
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
        <div className="font-mono text-[10px] text-idpxyz-textMuted">{row.partyId}</div>
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
      <div className="flex flex-col items-start gap-1">
        {statusBadge(identityStatusLabels, row.status)}
        {row.deactivatedAt ? (
          <span className="font-mono text-[10px] text-idpxyz-textMuted">
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
    // 已登记即还没到生效时点，时刻标琥珀色——与参照页「预计到货」临近那一格同一个提示。
    render: (row) => (
      <span className={`font-mono ${row.status === 'REGISTERED' ? 'font-medium text-amber-400' : 'text-idpxyz-text'}`}>
        <Instant value={row.effectiveFrom} />
      </span>
    ),
  },
  {
    key: 'basis',
    header: '登记依据',
    hideWhenDetailOpen: true,
    render: (row) => <span className="font-mono text-idpxyz-textMuted">{row.basis}</span>,
  },
  {
    key: 'registered-at',
    header: '登记时间',
    sortKey: 'registered-at',
    hideWhenDetailOpen: true,
    render: (row) => (
      <span className="font-mono text-idpxyz-textMuted">
        <Instant value={row.registeredAt} />
      </span>
    ),
  },
];

/** 详情地址：hash 二段。壳层拿前两段作标签 id，对象页因此另开一张标签，列表留在原处。 */
function detailHash(legalEntityId: string): string {
  return `#/group-legal-entities/${encodeURIComponent(legalEntityId)}`;
}

/** 查询串不进对象标识：`?q=` 是列表检索词，粘在第二段上会认错行。 */
function selectedIdFromHash(): string | null {
  const path = window.location.hash.replace(/^#\/?/, '').split('?')[0];
  const [moduleId, objectId] = path.split('/');
  return moduleId === 'group-legal-entities' && objectId ? decodeHashSegment(objectId) : null;
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
 */
export function GroupLegalEntitiesPage() {
  const { answer, retry, pending } = useRegisterList(listGroupLegalEntities);
  const tabReturn = useTabReturn();
  // 渲列表还是对象页由 hash 二段定：对象地址在壳层是自己的一张标签，本组件在那张标签里渲对象页。
  const [objectId, setObjectId] = useState<string | null>(selectedIdFromHash);
  const [view, setView] = useState<'list' | 'register'>('list');
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<LegalEntityStatusFilter>('ALL');
  const [attentionOnly, setAttentionOnly] = useState(false);
  const [sort, setSort] = useState<WorkbenchSort>(defaultLegalEntitySort);
  const [selectedKey, setSelectedKey] = useState<string | null>(null);

  useEffect(() => {
    const onHashChange = () => setObjectId(selectedIdFromHash());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);

  // 列表没取到（加载中 / 未配置 / 出错）传 null：登记表单那边建议修订号一律为 1，不拿空数组冒充「册上没有」。
  const knownEntities = answer?.kind === 'outcome' ? answer.body.entities : null;
  const entities = knownEntities ?? [];

  if (objectId !== null) {
    const row = entities.find((entity) => entity.legalEntityId === objectId) ?? null;
    return (
      <LegalEntityDetailPage
        legalEntityId={objectId}
        row={row}
        listAnswer={answer}
        retry={retry}
        onBack={() => tabReturn.returnTo('group-legal-entities')}
      />
    );
  }

  if (view === 'register') {
    return (
      <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
        <div className="flex shrink-0 items-center gap-4 border-b border-idpxyz-border px-6 py-3">
          <Button variant="ghost" size="sm" onClick={() => setView('list')}>
            <ArrowLeft className="mr-1.5 h-4 w-4" aria-hidden="true" />
            返回列表
          </Button>
          <div className="min-w-0">
            <h1 className="text-[16px] font-semibold leading-tight text-idpxyz-textBright">登记责任法人</h1>
            <p className="truncate text-[11px] text-idpxyz-textMuted">
              每次登记形成新修订，历史不可覆盖；更正身份就登记同一法人的下一修订号
            </p>
          </div>
        </div>
        <LegalEntityRegistrationForm knownEntities={knownEntities} onRegistered={retry} />
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
          tone: counts.attention > 0 ? 'text-orange-400' : undefined,
          icon: counts.attention > 0 ? <AlertTriangle className="h-3 w-3 text-orange-400" aria-hidden="true" /> : undefined,
          active: attentionOnly,
          onClick: () => setAttentionOnly(!attentionOnly),
        },
        {
          label: '已登记未生效',
          value: counts.byStatus.REGISTERED,
          tone: counts.byStatus.REGISTERED > 0 ? 'text-amber-400' : undefined,
          active: status === 'REGISTERED',
          onClick: () => toggleStatus('REGISTERED'),
        },
        {
          label: '已生效',
          value: counts.byStatus.EFFECTIVE,
          tone: 'text-sky-400',
          active: status === 'EFFECTIVE',
          onClick: () => toggleStatus('EFFECTIVE'),
        },
        {
          label: '在用中已生效',
          value: '',
          custom:
            effectiveShare === null ? (
              <span className="text-[12px] font-semibold leading-none text-idpxyz-textMuted">—</span>
            ) : (
              <div className="flex items-center gap-1.5">
                <Progress value={effectiveShare} size="sm" variant={effectiveShare >= 100 ? 'success' : 'default'} className="w-24" />
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
      subtitle="责任法人是对外签约、开票与结算的经营主体 · 每次登记形成新修订，历史不可覆盖"
      kpis={kpis}
      onRefresh={retry}
      refreshing={pending}
      primaryAction={
        <Button onClick={() => setView('register')}>
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
