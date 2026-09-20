import { useEffect, useState } from 'react';
import {
  Button,
  Drawer,
  DrawerBody,
  DrawerHeader,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
  Timeline,
} from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { chipClass } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listBusinessParties,
  listBusinessPartyRevisions,
  listPartyRelationships,
  type BusinessPartyListResponseBody,
  type BusinessPartyRecord,
  type BusinessPartyRevisionListResponseBody,
  type PartyRelationshipListResponseBody,
  type PartyRelationshipRecord,
} from './api';
import {
  identityStatusLabels,
  labelOf,
  partyNameUnknownNote,
  partyRoleLabels,
  problemNote,
  relationshipStatusLabels,
} from './presentation';
import { businessPartyRevisionHistoryNote, businessPartyRevisionTimeline } from './business-party-revisions';
import {
  DetailRow,
  Instant,
  InstantRange,
  UnknownPartyName,
  filterSelectClass,
  statusBadge,
  useCopyToClipboard,
} from './detail-primitives';
import { useRegisterList } from './register-list';
import { BusinessPartyRegistrationForm } from './BusinessPartyRegistrationForm';
import { PartyRelationshipRegistrationForm } from './PartyRelationshipRegistrationForm';
import { IdentityDeactivationForm } from './IdentityDeactivationForm';
import {
  businessPartyCountSummary,
  businessPartyNoMatchNote,
  businessPartySortOptions,
  businessPartyStatusFilterOptions,
  filterBusinessParties,
  sortBusinessParties,
  type BusinessPartySortKey,
  type BusinessPartyStatusFilter,
} from './business-party-list';
import {
  filterPartyRelationships,
  partyRelationshipCountSummary,
  partyRelationshipNoMatchNote,
  partyRelationshipRoleFilterOptions,
  partyRelationshipSortOptions,
  partyRelationshipStatusFilterOptions,
  sortPartyRelationships,
  type PartyRelationshipRoleFilter,
  type PartyRelationshipSortKey,
  type PartyRelationshipStatusFilter,
} from './party-relationship-list';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['business-parties'];

// 参与方格：标识必列，名称从参与方册转写；nameKnown 为假是写入门失败才会出现的
// 悬空引用，如实标出，不补占位文本。
function partyCell(id: string, name: string | undefined, nameKnown: boolean) {
  return (
    <div className="min-w-36">
      <p className="font-mono text-xs text-idpxyz-textMuted">{id}</p>
      {nameKnown ? (
        <p className="mt-0.5 font-medium text-idpxyz-text">{name}</p>
      ) : (
        <p className="mt-0.5 text-xs text-idpxyz-textMuted">{partyNameUnknownNote}</p>
      )}
    </div>
  );
}

// 骨架期的「方向」列撤下：方向由持有方→相对方的两列次序表达（CONTEXT：方向由
// 「哪一方对哪一方持有该角色」表达），单设一列没有第二份事实可载。
const columns: ListColumn<PartyRelationshipRecord>[] = [
  {
    id: 'relationship',
    header: '关系标识 / 修订',
    render: (row) => (
      <div className="min-w-36">
        <p className="font-mono font-medium text-idpxyz-text">{row.relationshipId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">r{row.revision}</p>
      </div>
    ),
  },
  {
    id: 'holder',
    header: '持有方',
    render: (row) => partyCell(row.holderId, row.holderName, row.holderNameKnown),
  },
  {
    id: 'counterparty',
    header: '相对方',
    render: (row) => partyCell(row.counterpartyId, row.counterpartyName, row.counterpartyNameKnown),
  },
  {
    id: 'role',
    header: '角色',
    align: 'center',
    render: (row) => labelOf(partyRoleLabels, row.role),
  },
  {
    id: 'scope',
    header: '适用范围',
    className: 'font-mono text-xs',
    render: (row) => row.scope,
  },
  {
    id: 'basis',
    header: '依据',
    className: 'font-mono text-xs',
    render: (row) => row.basis,
  },
  {
    id: 'validity',
    header: '有效区间',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => <InstantRange from={row.effectiveStartsAt} to={row.effectiveEndsAt} />,
  },
  {
    id: 'status',
    header: '状态',
    align: 'center',
    // 撤销/替代的时点、依据与后继是这格状态的内容：撤销或到期只影响生效边界后
    // 的新决定，看这格的人需要知道边界在哪、依据是什么、被谁替代。
    render: (row) => (
      <div className="flex flex-col items-center gap-0.5">
        {statusBadge(relationshipStatusLabels, row.status)}
        {row.endedAt ? (
          <p className="font-mono text-xs text-idpxyz-textMuted">
            自 <Instant value={row.endedAt} />
          </p>
        ) : null}
        {row.endBasis ? <p className="font-mono text-xs text-idpxyz-textMuted">{row.endBasis}</p> : null}
        {row.successorId ? <p className="font-mono text-xs text-idpxyz-textMuted">→ {row.successorId}</p> : null}
      </div>
    ),
  },
];

// 身份本体册的列。停用两件（时点 + 依据）与状态同格呈现：看这格的人要知道自何时起
// 停用、依据是什么；只显示一个「已停用」说不出这两样。登记时间上列是因为默认排序按它排（裁决 1）——
// 排的键看不见，操作者判不出「为什么这一条在最上面」。
const identityColumns: ListColumn<BusinessPartyRecord>[] = [
  {
    id: 'party',
    header: '参与方标识 / 修订',
    render: (row) => (
      <div className="min-w-36">
        <p className="font-mono font-medium text-idpxyz-text">{row.partyId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">r{row.revision}</p>
      </div>
    ),
  },
  { id: 'name', header: '名称', render: (row) => row.partyName },
  {
    id: 'status',
    header: '状态',
    align: 'center',
    render: (row) => (
      <div className="flex flex-col items-center gap-0.5">
        {statusBadge(identityStatusLabels, row.status)}
        {row.deactivatedAt ? (
          <p className="font-mono text-xs text-idpxyz-textMuted">
            自 <Instant value={row.deactivatedAt} />
          </p>
        ) : null}
        {row.deactivationBasis ? (
          <p className="font-mono text-xs text-idpxyz-textMuted">{row.deactivationBasis}</p>
        ) : null}
      </div>
    ),
  },
  {
    id: 'basis',
    header: '依据',
    className: 'font-mono text-xs',
    render: (row) => row.basis,
  },
  {
    id: 'effective-from',
    header: '生效时点',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => <Instant value={row.effectiveFrom} />,
  },
  {
    id: 'registered-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => <Instant value={row.registeredAt} />,
  },
];

/**
 * 抽屉「修订历史」区（票 12 第 5 条）：按参与方取整条修订链，纵向时间线；判读在 business-party-revisions.ts
 * （两册共用的本体在 revision-timeline.ts），这里只摆。形状照 GroupLegalEntitiesPage 的 LegalEntityRevisionHistory：
 * 每次换行重取，未回的旧请求按 cancelled 丢；答案顶层回显的参与方标识再核一次，对不上就不摆——摆一段别的参与方
 * 的历史比空着更坏。
 *
 * 读口墙前照旧显未配置：403 是「今天没有问到」，不是「这个参与方没有历史」，两句续办不同（前者去配渠道，
 * 后者去查写侧），措辞把这一格点出来；不用 UnconfiguredState 大块——抽屉里一段区，一句话够。
 */
function BusinessPartyRevisionHistory({ partyId }: { partyId: string }) {
  const [answer, setAnswer] = useState<ApiResult<BusinessPartyRevisionListResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listBusinessPartyRevisions(partyId).then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [partyId, reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const note = 'mt-1 text-[12px] text-idpxyz-textMuted';

  if (answer === null) {
    return <p className={note}>正在读取修订历史…</p>;
  }
  if (answer.kind === 'unconfigured') {
    return (
      <p className={note}>
        访问通道尚未配置：修订历史读口（GET /commercial-business-parties/{'{partyId}'}/revisions）当前不可用（403）。
        这不是「这个参与方没有历史」——今天没有问到；配置该上下文的访问通道后重新打开抽屉。
      </p>
    );
  }
  if (answer.kind === 'callerProblem') {
    return (
      <p className={note}>
        调用方式问题（HTTP {answer.status}）：{problemNote(answer.code)}
      </p>
    );
  }
  if (answer.kind === 'noAnswer' || answer.kind === 'transport') {
    return (
      <p className={note}>
        {answer.kind === 'noAnswer'
          ? `服务端未形成答案（HTTP ${answer.status}）：${problemNote(answer.code)}`
          : `无法连接主数据读取服务：${answer.message}`}
        <Button variant="ghost" size="sm" className="ml-2" onClick={retry}>
          重试
        </Button>
      </p>
    );
  }
  if (answer.body.partyId !== partyId) {
    return (
      <p className={note}>
        答案回显的参与方（{answer.body.partyId}）与所问（{partyId}）不符，已丢弃。
        <Button variant="ghost" size="sm" className="ml-2" onClick={retry}>
          重试
        </Button>
      </p>
    );
  }

  const items = businessPartyRevisionTimeline(answer.body.revisions, formatInstant);
  return (
    <>
      <p className={note}>{businessPartyRevisionHistoryNote(items.length)}</p>
      {items.length > 0 ? <Timeline className="mt-3" items={items} /> : null}
    </>
  );
}

/**
 * 身份行详情抽屉（票 09 第 4 条）：列全字段，含表上没有的租户；「修订历史」区自票 12 起取真数据。
 */
function BusinessPartyDrawer({ row, onClose }: { row: BusinessPartyRecord | null; onClose: () => void }) {
  const copy = useCopyToClipboard();
  return (
    <Drawer open={row !== null} onOpenChange={(open) => (open ? undefined : onClose())} aria-label="参与方身份详情">
      {row ? (
        <>
          <DrawerHeader>
            <div className="flex items-center gap-2">
              <span className="font-mono font-medium text-idpxyz-text">{row.partyId}</span>
              <span className="font-mono text-xs text-idpxyz-textMuted">r{row.revision}</span>
              {statusBadge(identityStatusLabels, row.status)}
            </div>
          </DrawerHeader>
          <DrawerBody>
            <dl>
              <DetailRow label="参与方标识" mono onCopy={() => copy('参与方标识', row.partyId)}>
                {row.partyId}
              </DetailRow>
              <DetailRow label="修订" mono>
                r{row.revision}
              </DetailRow>
              <DetailRow label="名称">{row.partyName}</DetailRow>
              <DetailRow label="状态">{statusBadge(identityStatusLabels, row.status)}</DetailRow>
              <DetailRow label="依据" mono onCopy={() => copy('依据', row.basis)}>
                {row.basis}
              </DetailRow>
              <DetailRow label="生效时点" mono>
                <Instant value={row.effectiveFrom} />
              </DetailRow>
              <DetailRow label="停用时点" mono>
                {row.deactivatedAt ? <Instant value={row.deactivatedAt} /> : <span className="text-idpxyz-textMuted">未停用</span>}
              </DetailRow>
              <DetailRow label="停用依据" mono>
                {row.deactivationBasis ?? <span className="text-idpxyz-textMuted">—</span>}
              </DetailRow>
              <DetailRow label="登记时间" mono>
                <Instant value={row.registeredAt} />
              </DetailRow>
              <DetailRow label="租户" mono>
                {row.tenantId}
              </DetailRow>
            </dl>
            <section className="mt-4">
              <h3 className="text-[12px] font-medium text-idpxyz-text">修订历史</h3>
              {/* key 带上列表行的最新修订号：登记签在抽屉开着时给同一参与方登了下一笔（或停用），列表重取后行的
                  revision 变了，历史区随之重挂重取——只按 partyId 记依赖会让它继续显上一条链。 */}
              <BusinessPartyRevisionHistory key={`${row.partyId}#r${row.revision}`} partyId={row.partyId} />
            </section>
          </DrawerBody>
        </>
      ) : null}
    </Drawer>
  );
}

/**
 * 关系行详情抽屉（票 09 第 4 条）：列全字段。撤销 / 到期 / 替代的时点、依据、后继各占一格——它们是「边界之后的新决定
 * 不再依据这段关系」的内容，不折进状态词里。
 */
function PartyRelationshipDrawer({ row, onClose }: { row: PartyRelationshipRecord | null; onClose: () => void }) {
  const copy = useCopyToClipboard();
  return (
    <Drawer open={row !== null} onOpenChange={(open) => (open ? undefined : onClose())} aria-label="参与方关系详情">
      {row ? (
        <>
          <DrawerHeader>
            <div className="flex items-center gap-2">
              <span className="font-mono font-medium text-idpxyz-text">{row.relationshipId}</span>
              <span className="font-mono text-xs text-idpxyz-textMuted">r{row.revision}</span>
              {statusBadge(relationshipStatusLabels, row.status)}
            </div>
          </DrawerHeader>
          <DrawerBody>
            <dl>
              <DetailRow label="关系标识" mono onCopy={() => copy('关系标识', row.relationshipId)}>
                {row.relationshipId}
              </DetailRow>
              <DetailRow label="修订" mono>
                r{row.revision}
              </DetailRow>
              <DetailRow label="持有方" mono onCopy={() => copy('持有方', row.holderId)}>
                {row.holderId}
              </DetailRow>
              <DetailRow label="持有方名称">{row.holderNameKnown ? row.holderName : <UnknownPartyName />}</DetailRow>
              <DetailRow label="相对方" mono onCopy={() => copy('相对方', row.counterpartyId)}>
                {row.counterpartyId}
              </DetailRow>
              <DetailRow label="相对方名称">
                {row.counterpartyNameKnown ? row.counterpartyName : <UnknownPartyName />}
              </DetailRow>
              <DetailRow label="角色">{labelOf(partyRoleLabels, row.role)}</DetailRow>
              <DetailRow label="适用范围" mono>
                {row.scope}
              </DetailRow>
              <DetailRow label="依据" mono onCopy={() => copy('依据', row.basis)}>
                {row.basis}
              </DetailRow>
              <DetailRow label="有效区间" mono>
                <InstantRange from={row.effectiveStartsAt} to={row.effectiveEndsAt} />
              </DetailRow>
              <DetailRow label="状态">{statusBadge(relationshipStatusLabels, row.status)}</DetailRow>
              <DetailRow label="终止时点" mono>
                {row.endedAt ? <Instant value={row.endedAt} /> : <span className="text-idpxyz-textMuted">未终止</span>}
              </DetailRow>
              <DetailRow label="终止依据" mono>
                {row.endBasis ?? <span className="text-idpxyz-textMuted">—</span>}
              </DetailRow>
              <DetailRow label="后继" mono>
                {row.successorId ?? <span className="text-idpxyz-textMuted">—</span>}
              </DetailRow>
              <DetailRow label="登记时间" mono>
                <Instant value={row.registeredAt} />
              </DetailRow>
              <DetailRow label="租户" mono>
                {row.tenantId}
              </DetailRow>
            </dl>
          </DrawerBody>
        </>
      ) : null}
    </Drawer>
  );
}

/**
 * 参与方身份本体册。它与关系册同页分签，而不是并进关系表：一个参与方既可以不是法人、
 * 也可以不在任何关系里，被停用的那种恰恰如此——身份生命周期的「已登记」与「已停用」
 * 两格只有在这一册上才有实例可显（票 admin-remainder-mechanism-batch/01 的补格裁定）。
 *
 * 列表状态由页面持有再传进来（票 10 第 5 条，GroupLegalEntitiesPage 的做法）：登记签要读已取回的列表给修订号建议、
 * 登记成功后要触发重取，两签共享同一份答案而不各取一次。
 */
function BusinessPartyIdentitiesTable({
  answer,
  retry,
}: {
  answer: ApiResult<BusinessPartyListResponseBody> | null;
  retry: () => void;
}) {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<BusinessPartyStatusFilter>('ALL');
  const [sort, setSort] = useState<BusinessPartySortKey>('registered-desc');
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const parties = answer?.kind === 'outcome' ? answer.body.parties : [];
  // 筛选与排序只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约（归票 04）。
  const visibleParties = sortBusinessParties(filterBusinessParties(parties, { search, status }), sort);
  // 抽屉按标识重找行而不是存整行：列表重取后行内容以新答案为准，行没了抽屉随之关。
  const selected = selectedId === null ? null : parties.find((row) => row.partyId === selectedId) ?? null;

  return (
    <>
      <ListPageTemplate<BusinessPartyRecord>
        title={info.title}
        description="业务参与方是与本网络发生商业往来的对象——货主、承运商、代理、转售商；一个参与方可以同时是法人，也可以不在任何关系里。每次登记形成新修订，历史不可覆盖"
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索参与方标识、名称或状态',
        }}
        filters={
          <>
            <select
              className={filterSelectClass}
              value={status}
              aria-label="状态"
              onChange={(event) => setStatus(event.target.value as BusinessPartyStatusFilter)}
            >
              {businessPartyStatusFilterOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <select
              className={filterSelectClass}
              value={sort}
              aria-label="排序"
              onChange={(event) => setSort(event.target.value as BusinessPartySortKey)}
            >
              {businessPartySortOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </>
        }
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 个」会与状态区「这不是目录为空」直接矛盾
          // （README 列表页上列通则第六条）。总数与当前显示数分开报（票 01 裁决 1）。
          answer?.kind === 'outcome' ? businessPartyCountSummary(parties.length, visibleParties.length) : undefined
        }
        columns={identityColumns}
        rows={visibleParties}
        rowKey={(row) => row.partyId}
        onRowClick={(row) => setSelectedId(row.partyId)}
        // 筛出为空不是空态（票 01 裁决 1）：viewState 按总数判，表格区另显一行。
        emptyRowsNote={businessPartyNoMatchNote}
        viewState={catalogueViewState(answer, parties.length, retry, {
          module: info,
          endpoint: 'GET /commercial-business-parties',
          emptyTitle: '当前租户尚无参与方身份登记',
          emptyDescription: '读取入口已配置，但登记册为空；页面不会预置参与方。在「登记」签登记第一个。',
        })}
      />
      <BusinessPartyDrawer row={selected} onClose={() => setSelectedId(null)} />
    </>
  );
}

/**
 * 业务参与方（party-commercial）。三签：身份本体册、关系册与登记签。
 *
 * 关系行对象是参与方关系的最新登记修订：承运商、承运商代理商、转售商、聚合平台与渠道
 * 账号持有人都以「双方 + 角色 + 有效区间」的时态关系表达，代理关系不自动合并交易角色；
 * 关系登记按修订版本化不可覆盖。两册的状态代数不同——身份状态按时点导出、关系状态是
 * 登记进来的事实，所以分签而不是并表。列表状态由页面持有再传进来，理由同身份册那张表。
 */
function PartyRelationshipsTable({
  answer,
  retry,
}: {
  answer: ApiResult<PartyRelationshipListResponseBody> | null;
  retry: () => void;
}) {
  const [search, setSearch] = useState('');
  const [role, setRole] = useState<PartyRelationshipRoleFilter>('ALL');
  const [status, setStatus] = useState<PartyRelationshipStatusFilter>('ALL');
  const [sort, setSort] = useState<PartyRelationshipSortKey>('effective-start-desc');
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const relationships = answer?.kind === 'outcome' ? answer.body.relationships : [];
  // 筛选与排序只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约（归票 04）。
  const visibleRelationships = sortPartyRelationships(
    filterPartyRelationships(relationships, { search, role, status }),
    sort,
  );
  const selected =
    selectedId === null ? null : relationships.find((row) => row.relationshipId === selectedId) ?? null;

  return (
    <>
      <ListPageTemplate<PartyRelationshipRecord>
        title={info.title}
        description="参与方关系记谁对谁持有什么角色、在什么范围、多久；撤销、到期与替代只影响边界之后的新决定"
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索关系、参与方或角色',
        }}
        filters={
          <>
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
            <select
              className={filterSelectClass}
              value={status}
              aria-label="状态"
              onChange={(event) => setStatus(event.target.value as PartyRelationshipStatusFilter)}
            >
              {partyRelationshipStatusFilterOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <select
              className={filterSelectClass}
              value={sort}
              aria-label="排序"
              onChange={(event) => setSort(event.target.value as PartyRelationshipSortKey)}
            >
              {partyRelationshipSortOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </>
        }
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 段」会与状态区
          // 「这不是目录为空」直接矛盾（README 列表页上列通则第六条）。总数与当前显示数分开报（票 01 裁决 1）。
          answer?.kind === 'outcome'
            ? partyRelationshipCountSummary(relationships.length, visibleRelationships.length)
            : undefined
        }
        columns={columns}
        rows={visibleRelationships}
        rowKey={(row) => row.relationshipId}
        onRowClick={(row) => setSelectedId(row.relationshipId)}
        // 筛出为空不是空态（票 01 裁决 1）：筛「候选关系」筛没了显这一行，不显「0 段」的空态。
        emptyRowsNote={partyRelationshipNoMatchNote}
        viewState={catalogueViewState(answer, relationships.length, retry, {
          module: info,
          endpoint: 'GET /commercial-party-relationships',
          emptyTitle: '当前租户尚无参与方关系登记',
          emptyDescription: '读取入口已配置，但登记册为空；页面不会预置参与方或关系。在「登记」签登记第一个。',
        })}
      />
      <PartyRelationshipDrawer row={selected} onClose={() => setSelectedId(null)} />
    </>
  );
}

/**
 * 登记签装的三本册（ADR-0085，票 admin-write-faces/02 切片 02c；自票 admin-web-group-legal-entities/10 起三册都是
 * 逐字段表单，ADR-0101 决定八自裁，JSON 快照签降为各表单底部的折叠区）。前两本与本页两张读签一一对应，选册按钮
 * 的词取读签自己的词，不为登记签另造说法。
 *
 * 第三本停用摆在本页，理由是本页读得见它刚写进去的那两件：停用每项必填 basis，而三个身份读面里只有身份本体册把
 * 停用时点与停用依据都渲染出来——法人册那格只有时点。登在哪里看得见结果就摆哪里，这是「写签跟着读签走」在本册
 * 上的落法。
 *
 * **不按 kind 切成两签**：停用口一个命令带种类，法人与客户账户的停用也走它。切签就得让每页拒收非本页那种 kind，
 * 那是管理台编一条服务端没有的约束——页面不教一条不真的规则。停用表单里种类是一格下拉，别处读面上结果的去处由
 * 格下那句说出。
 *
 * 换册即换表单：各册草稿是各自表单的内部状态，切走再切回从空白起——三册的格互不相容，留着上一册的草稿没有可以
 * 「带过去」的东西，与 MultiRegistrationPanel 换册清草稿是同一条纪律。
 */
type RegisterId = 'business-party' | 'party-relationship' | 'identity-deactivation';

const registers: { id: RegisterId; label: string }[] = [
  { id: 'business-party', label: '参与方身份' },
  { id: 'party-relationship', label: '参与方关系' },
  // 「停用」取身份状态格里的封闭词（已停用），不为登记签另造一个动词。
  { id: 'identity-deactivation', label: '身份停用' },
];

function RegistrationTab({
  parties,
  knownParties,
  knownRelationships,
  onPartiesChanged,
  onRelationshipsChanged,
}: {
  /** 页面持有的参与方列表答案：关系表单双方候选与停用表单参与方那一册都取它，不各自再读（票 13 第 5 条）。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  knownParties: readonly BusinessPartyRecord[] | null;
  knownRelationships: readonly PartyRelationshipRecord[] | null;
  onPartiesChanged: () => void;
  onRelationshipsChanged: () => void;
}) {
  const [selected, setSelected] = useState<RegisterId>('business-party');
  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      <div className="flex flex-wrap items-center gap-2 px-4 pt-4">
        {registers.map((register) => (
          <button
            key={register.id}
            type="button"
            className={chipClass(register.id === selected)}
            onClick={() => setSelected(register.id)}
          >
            {register.label}
          </button>
        ))}
      </div>
      {selected === 'business-party' ? (
        <BusinessPartyRegistrationForm knownParties={knownParties} onRegistered={onPartiesChanged} />
      ) : selected === 'party-relationship' ? (
        <PartyRelationshipRegistrationForm
          knownRelationships={knownRelationships}
          parties={parties}
          onRegistered={onRelationshipsChanged}
        />
      ) : (
        <IdentityDeactivationForm parties={parties} onDeactivated={onPartiesChanged} />
      )}
    </div>
  );
}

/**
 * 页面持有两张读签的列表状态（票 10 第 5 条）：登记签从这份答案取修订号建议，登记册答 REGISTERED / DEACTIVATED 时
 * 触发对应读签重取。两册各自一份重取序号——登一段关系不必重取参与方册，反过来也一样。
 */
export function BusinessPartiesPage() {
  const parties = useRegisterList(listBusinessParties);
  const relationships = useRegisterList(listPartyRelationships);
  // 列表没取到（加载中 / 未配置 / 出错）传 null：表单那边建议修订号一律为 1，不拿空数组冒充「册上没有」。
  const knownParties = parties.answer?.kind === 'outcome' ? parties.answer.body.parties : null;
  const knownRelationships =
    relationships.answer?.kind === 'outcome' ? relationships.answer.body.relationships : null;

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="identities" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="identities">参与方身份</TabsTrigger>
          <TabsTrigger value="relationships">参与方关系</TabsTrigger>
          <TabsTrigger value="register">登记</TabsTrigger>
        </TabsList>
        <TabsContent
          value="identities"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <BusinessPartyIdentitiesTable answer={parties.answer} retry={parties.retry} />
        </TabsContent>
        <TabsContent
          value="relationships"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <PartyRelationshipsTable answer={relationships.answer} retry={relationships.retry} />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <RegistrationTab
            parties={parties.answer}
            knownParties={knownParties}
            knownRelationships={knownRelationships}
            onPartiesChanged={parties.retry}
            onRelationshipsChanged={relationships.retry}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
