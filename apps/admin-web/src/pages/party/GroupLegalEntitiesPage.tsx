import { useEffect, useState, type ReactNode } from 'react';
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
import { StatusBadgeFor, domainStatusTones, type DomainStatus } from '../../domain/status';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listGroupLegalEntities,
  listLegalEntityRevisions,
  type GroupLegalEntityListResponseBody,
  type GroupLegalEntityRecord,
  type LegalEntityRevisionListResponseBody,
} from './api';
import { identityStatusLabels, labelOf, legalEntityKindLabels, problemNote } from './presentation';
import { LegalEntityRegistrationForm } from './LegalEntityRegistrationForm';
import { legalEntityRevisionTimeline, revisionHistoryNote } from './legal-entity-revisions';
import { DetailRow, Instant, filterSelectClass, useCopyToClipboard } from './detail-primitives';
import { useRegisterList } from './register-list';
import {
  filterLegalEntities,
  legalEntityCountSummary,
  legalEntityNoMatchNote,
  legalEntitySortOptions,
  legalEntityStatusFilterOptions,
  sortLegalEntities,
  type LegalEntitySortKey,
  type LegalEntityStatusFilter,
} from './legal-entity-list';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

// 身份状态词表词按共享词表着色；词表没收录的码（服务端新增一格时）原样示码、不猜色调——
// 归进某个既有中文说法会让一种新答案冒充另一种。断言只桥接类型边界，词同源于 CONTEXT 原词。
function statusBadge(status: string): ReactNode {
  const word = labelOf(identityStatusLabels, status);
  return word in domainStatusTones ? <StatusBadgeFor status={word as DomainStatus} /> : word;
}

// 骨架期这里列过「运营集团租户」——ADR-0003 里那一级是配置与隔离边界本身，读面本来就在单租户
// 作用域内取数，整列同值没有信息，接线时按 live 页惯例撤下。「对象类型」列同一判据撤下（票 01 第 3 条）：
// 本册今天只有 RESPONSIBLE_LEGAL_ENTITY 一格，种类进详情抽屉。
const columns: ListColumn<GroupLegalEntityRecord>[] = [
  {
    id: 'legal-entity',
    header: '法人标识 / 修订',
    render: (row) => (
      <div className="min-w-40">
        <p className="font-mono font-medium text-idpxyz-text">{row.legalEntityId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">r{row.revision}</p>
      </div>
    ),
  },
  {
    id: 'name',
    header: '名称',
    // 名称在参与方册上（法人不抄第二份）；partyNameKnown 为假是写入门失败才会出现
    // 的悬空引用，如实标出让人去查写侧，不补占位文本冒充名称。
    render: (row) =>
      row.partyNameKnown ? (
        row.partyName
      ) : (
        <span className="text-idpxyz-textMuted">参与方册查无此身份</span>
      ),
  },
  {
    id: 'party-identity',
    header: '业务参与方身份',
    className: 'font-mono text-xs',
    render: (row) => row.partyId,
  },
  {
    id: 'status',
    header: '身份状态',
    align: 'center',
    // 已停用行标出停用时点：停用只自其时点起不再支持新的商业决定，时点是这格
    // 状态的内容而不是装饰。
    render: (row) => (
      <div className="flex flex-col items-center gap-0.5">
        {statusBadge(row.status)}
        {row.deactivatedAt ? (
          <p className="font-mono text-xs text-idpxyz-textMuted">
            自 <Instant value={row.deactivatedAt} />
          </p>
        ) : null}
      </div>
    ),
  },
  {
    id: 'basis',
    header: '登记依据',
    className: 'font-mono text-xs',
    render: (row) => row.basis,
  },
  {
    id: 'effective-from',
    header: '生效自',
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
 * 抽屉「修订历史」区（票 03 第 5 条）：按法人取整条修订链，纵向时间线；判读在 legal-entity-revisions.ts，
 * 这里只摆。每次换行重取，未回的旧请求按 cancelled 丢；答案顶层回显的法人标识再核一次，对不上就不摆——
 * 摆一段别的法人的历史比空着更坏。
 *
 * 读口墙前照旧显未配置：403 是「今天没有问到」，不是「这个法人没有历史」，两句续办不同（前者去配渠道，
 * 后者去查写侧），措辞把这一格点出来；不用 UnconfiguredState 大块——抽屉里一段区，一句话够。
 */
function LegalEntityRevisionHistory({ legalEntityId }: { legalEntityId: string }) {
  const [answer, setAnswer] = useState<ApiResult<LegalEntityRevisionListResponseBody> | null>(null);
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listLegalEntityRevisions(legalEntityId).then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [legalEntityId, reloadKey]);

  const retry = () => setReloadKey((value) => value + 1);
  const note = 'mt-1 text-[12px] text-idpxyz-textMuted';

  if (answer === null) {
    return <p className={note}>正在读取修订历史…</p>;
  }
  if (answer.kind === 'unconfigured') {
    return (
      <p className={note}>
        访问通道尚未配置：修订历史读口（GET /commercial-group-legal-entities/{'{legalEntityId}'}/revisions）当前
        不可用（403）。这不是「这个法人没有历史」——今天没有问到；配置该上下文的访问通道后重新打开抽屉。
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
  if (answer.body.legalEntityId !== legalEntityId) {
    return (
      <p className={note}>
        答案回显的法人（{answer.body.legalEntityId}）与所问（{legalEntityId}）不符，已丢弃。
        <Button variant="ghost" size="sm" className="ml-2" onClick={retry}>
          重试
        </Button>
      </p>
    );
  }

  const items = legalEntityRevisionTimeline(answer.body.revisions, formatInstant);
  return (
    <>
      <p className={note}>{revisionHistoryNote(items.length)}</p>
      {items.length > 0 ? <Timeline className="mt-3" items={items} /> : null}
    </>
  );
}

/**
 * 行详情抽屉（票 01 第 5 条）：列全字段，含表上撤下的种类与租户、停用两件；「修订历史」区自票 03 起取真数据。
 */
function LegalEntityDrawer({
  row,
  onClose,
}: {
  row: GroupLegalEntityRecord | null;
  onClose: () => void;
}) {
  const copy = useCopyToClipboard();
  return (
    <Drawer open={row !== null} onOpenChange={(open) => (open ? undefined : onClose())} aria-label="法人详情">
      {row ? (
        <>
          <DrawerHeader>
            <div className="flex items-center gap-2">
              <span className="font-mono font-medium text-idpxyz-text">{row.legalEntityId}</span>
              <span className="font-mono text-xs text-idpxyz-textMuted">r{row.revision}</span>
              {statusBadge(row.status)}
            </div>
          </DrawerHeader>
          <DrawerBody>
            <dl>
              <DetailRow label="法人标识" mono onCopy={() => copy('法人标识', row.legalEntityId)}>
                {row.legalEntityId}
              </DetailRow>
              <DetailRow label="修订" mono>
                r{row.revision}
              </DetailRow>
              <DetailRow label="种类">{labelOf(legalEntityKindLabels, row.kind)}</DetailRow>
              <DetailRow label="业务参与方身份" mono onCopy={() => copy('业务参与方身份', row.partyId)}>
                {row.partyId}
              </DetailRow>
              <DetailRow label="参与方名称">
                {row.partyNameKnown ? row.partyName : <span className="text-idpxyz-textMuted">参与方册查无此身份</span>}
              </DetailRow>
              <DetailRow label="身份状态">{statusBadge(row.status)}</DetailRow>
              <DetailRow label="登记依据" mono onCopy={() => copy('登记依据', row.basis)}>
                {row.basis}
              </DetailRow>
              <DetailRow label="生效自" mono>
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
              <LegalEntityRevisionHistory legalEntityId={row.legalEntityId} />
            </section>
          </DrawerBody>
        </>
      ) : null}
    </Drawer>
  );
}

/**
 * 集团与法人（party-commercial）。行对象是责任法人的最新登记修订：法人钉在稳定的
 * 业务参与方身份上（ADR-0003 三级边界的第二级），名称从参与方册转写；身份登记按
 * 修订版本化不可覆盖，停用形成新修订而不是删除。
 *
 * 列表状态由页面持有再传进来：登记签要读已取回的列表给修订号建议、登记成功后要触发重取，
 * 两签共享同一份答案而不各取一次。
 */
function GroupLegalEntitiesTable({
  answer,
  retry,
}: {
  answer: ApiResult<GroupLegalEntityListResponseBody> | null;
  retry: () => void;
}) {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<LegalEntityStatusFilter>('ALL');
  const [sort, setSort] = useState<LegalEntitySortKey>('registered-desc');
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const entities = answer?.kind === 'outcome' ? answer.body.entities : [];
  // 筛选与排序只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约（归票 04）。
  const visibleEntities = sortLegalEntities(filterLegalEntities(entities, { search, status }), sort);
  // 抽屉按标识重找行而不是存整行：列表重取后行内容以新答案为准，行没了抽屉随之关。
  const selected = selectedId === null ? null : entities.find((row) => row.legalEntityId === selectedId) ?? null;

  return (
    <>
      <ListPageTemplate<GroupLegalEntityRecord>
        title={info.title}
        description="责任法人是对外签约、开票与结算的经营主体；每次登记形成新修订，历史不可覆盖"
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索法人、参与方身份或名称',
        }}
        filters={
          <>
            <select
              className={filterSelectClass}
              value={status}
              aria-label="身份状态"
              onChange={(event) => setStatus(event.target.value as LegalEntityStatusFilter)}
            >
              {legalEntityStatusFilterOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
            <select
              className={filterSelectClass}
              value={sort}
              aria-label="排序"
              onChange={(event) => setSort(event.target.value as LegalEntitySortKey)}
            >
              {legalEntitySortOptions.map((option) => (
                <option key={option.value} value={option.value}>
                  {option.label}
                </option>
              ))}
            </select>
          </>
        }
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 个」会与状态区「这不是目录为空」直接矛盾
          // （README 列表页上列通则）。总数与当前显示数分开报（票 01 裁决 1）。
          answer?.kind === 'outcome' ? legalEntityCountSummary(entities.length, visibleEntities.length) : undefined
        }
        columns={columns}
        rows={visibleEntities}
        rowKey={(row) => row.legalEntityId}
        onRowClick={(row) => setSelectedId(row.legalEntityId)}
        // 筛出为空不是空态（裁决 1）：viewState 按总数判，表格区另显一行。
        emptyRowsNote={legalEntityNoMatchNote}
        viewState={catalogueViewState(answer, entities.length, retry, {
          module: info,
          endpoint: 'GET /commercial-group-legal-entities',
          emptyTitle: '当前租户尚无责任法人登记',
          emptyDescription: '在「登记法人」签登记第一个，或用受控 CLI parcel-commercial register-parties 灌入。',
        })}
      />
      <LegalEntityDrawer row={selected} onClose={() => setSelectedId(null)} />
    </>
  );
}

/**
 * 集团与法人：查阅法人册，外加登记签（ADR-0085，票 admin-write-faces/02 切片 02c）。
 *
 * 登记签只装法人身份登记一册。停用不摆这里，判据是本页读不全它写进去的东西：停用快照每项
 * 必填 basis，而本页身份状态那格只显停用时点、没有依据，业务参与方页的身份本体册两件都显
 * 得出来，停用签因此摆在那一页（写签跟着读签走）。
 *
 * 不是「一个口跨三种身份所以哪都不能摆」——那条路走不通的是**按身份切签**：快照收的是
 * deactivations 数组、kind 在每一项上，切开就得让每页拒收非本页那种 kind，等于管理台编一条
 * 服务端没有的约束。跨读面本身不是禁令，读得最全的那一页收下它才是。
 *
 * 也没有行级编辑或删除面：身份登记按修订版本化不可覆盖，更正占下一个修订号翻旧插新，
 * 停用形成新修订，所以本签只有登记一个动作。墙降之前它必然答 403「接入渠道未配置」，
 * 那是诚实答案；墙降当天在装配点换真 Intake 即点亮，本页一行不用改。
 *
 * 登记签自票 admin-web-group-legal-entities/02 起是逐字段表单（ADR-0101 决定八自裁），JSON 快照签
 * 降为表单里的折叠区。
 */
export function GroupLegalEntitiesPage() {
  const { answer, retry } = useRegisterList(listGroupLegalEntities);
  // 列表没取到（加载中 / 未配置 / 出错）传 null：表单那边建议修订号一律为 1，不拿空数组冒充「册上没有」。
  const knownEntities = answer?.kind === 'outcome' ? answer.body.entities : null;

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="entities" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="entities">集团与法人</TabsTrigger>
          <TabsTrigger value="register">登记法人</TabsTrigger>
        </TabsList>
        <TabsContent
          value="entities"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <GroupLegalEntitiesTable answer={answer} retry={retry} />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <LegalEntityRegistrationForm knownEntities={knownEntities} onRegistered={retry} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
