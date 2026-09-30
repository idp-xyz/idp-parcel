import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import {
  ListPageTemplate,
  decodeHashSegment,
  presentFields,
  useTabReturn,
  type InspectorContent,
  type ListColumn,
} from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import { formatInstant } from '../moment';
import {
  listGroupLegalEntities,
  type GroupLegalEntityListResponseBody,
  type GroupLegalEntityRecord,
} from './api';
import {
  identityLayerAbsentNote,
  identityStatusLabels,
  labelOf,
  partyNameUnknownNote,
} from './presentation';
import { LegalEntityRegistrationForm } from './LegalEntityRegistrationForm';
import { identityLayerCellsOf } from './legal-entity-identity';
import { LegalEntityDetailPage } from './LegalEntityDetailPage';
import { Instant, UnknownPartyName, statusBadge } from './detail-primitives';
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

function AbsentIdentityLayer() {
  return <span className="text-idpxyz-textMuted">{identityLayerAbsentNote}</span>;
}

// 身份两格照答复原样示出，国家在上、号在下；最新修订登记于身份层落地之前时如实写 identityLayerAbsentNote。
function IdentityLayerCell({ row }: { row: GroupLegalEntityRecord }) {
  const cells = identityLayerCellsOf(row);
  if (cells === null) return <AbsentIdentityLayer />;
  return (
    <div className="min-w-40">
      <p>{cells.country}</p>
      <p className="mt-0.5 text-idpxyz-textMuted">{cells.numbers}</p>
    </div>
  );
}

// 骨架期这里列过「运营集团租户」——ADR-0003 里那一级是配置与隔离边界本身，读面本来就在单租户
// 作用域内取数，整列同值没有信息，接线时按 live 页惯例撤下。「对象类型」列同一判据撤下（票 01 第 3 条）：
// 本册今天只有 RESPONSIBLE_LEGAL_ENTITY 一格，种类进检查器概要与对象页。
const columns: ListColumn<GroupLegalEntityRecord>[] = [
  {
    id: 'legal-entity',
    header: '法人标识',
    className: 'min-w-44',
    render: (row) => (
      <div>
        <p className="font-mono text-[13px] font-semibold text-idpxyz-text">{row.legalEntityId}</p>
        <p className="mt-1 text-[11px] text-idpxyz-textMuted">当前修订 · <span className="font-mono">r{row.revision}</span></p>
      </div>
    ),
  },
  {
    id: 'name',
    header: '责任主体',
    className: 'min-w-48',
    hideWhenMasterDetailOpen: true,
    // 名称在参与方册上（法人不抄第二份）；转写不到那一格的话与判据在 presentation.ts 的 partyNameUnknownNote。
    render: (row) => (
      <div>
        <p className="text-[13px] font-medium text-idpxyz-text">{row.partyNameKnown ? row.partyName : <UnknownPartyName />}</p>
        <p className="mt-1 font-mono text-[11px] text-idpxyz-textMuted">{row.partyId}</p>
      </div>
    ),
  },
  {
    id: 'identity-layer',
    header: '注册身份',
    className: 'min-w-48',
    hideWhenMasterDetailOpen: true,
    render: (row) => <IdentityLayerCell row={row} />,
  },
  {
    id: 'status',
    header: '状态',
    align: 'center',
    className: 'min-w-28',
    // 已停用行标出停用时点：停用只自其时点起不再支持新的商业决定，时点是这格
    // 状态的内容而不是装饰。
    render: (row) => (
      <div className="flex flex-col items-center gap-1">
        {statusBadge(identityStatusLabels, row.status)}
        {row.deactivatedAt ? (
          <p className="font-mono text-[11px] text-idpxyz-textMuted">
            自 <Instant value={row.deactivatedAt} />
          </p>
        ) : null}
      </div>
    ),
  },
  {
    id: 'effective-from',
    header: '生效自',
    className: 'min-w-44 font-mono text-xs',
    hideWhenMasterDetailOpen: true,
    render: (row) => <Instant value={row.effectiveFrom} />,
  },
  {
    id: 'basis',
    header: '登记依据',
    className: 'min-w-44 font-mono text-xs',
    hideWhenMasterDetailOpen: true,
    render: (row) => row.basis,
  },
  {
    id: 'registered-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs text-idpxyz-textMuted',
    hideWhenMasterDetailOpen: true,
    render: (row) => <Instant value={row.registeredAt} />,
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
 * 右侧检查器只放行上已有的概要（契约：不发第二个请求，概要 ≤ 8 格）。
 * 身份状态只进「状态」一节，不在概要里再写一遍。停用两件未停用时不占格。
 * 登记依据、登记时间、租户、修订号进默认折叠的审计。修订历史与法人资料要另取数，
 * 不进检查器，走「打开详情」。业务参与方页没有按标识的对象地址，链落到模块页，标识写在链的词上。
 */
function inspectorOf(row: GroupLegalEntityRecord): InspectorContent {
  const identity = identityLayerCellsOf(row);
  return {
    title: '法人',
    subtitle: row.legalEntityId,
    sections: [
      {
        kind: 'summary',
        fields: presentFields([
          { label: '参与方身份', value: row.partyId, mono: true },
          { label: '注册国家 / 地区', value: identity?.country ?? identityLayerAbsentNote, mono: true },
          { label: '终身注册号', value: identity?.numbers ?? identityLayerAbsentNote, mono: true },
          { label: '生效自', value: formatInstant(row.effectiveFrom), mono: true },
          row.deactivatedAt
            ? { label: '停用时点', value: formatInstant(row.deactivatedAt), mono: true }
            : null,
          row.deactivationBasis
            ? { label: '停用依据', value: row.deactivationBasis, mono: true }
            : null,
        ]),
      },
      {
        kind: 'status',
        items: [{ label: '身份状态', word: labelOf(identityStatusLabels, row.status) }],
      },
      {
        kind: 'actions',
        actions: [
          {
            label: '打开详情',
            onRun: () => {
              window.location.hash = detailHash(row.legalEntityId);
            },
          },
        ],
      },
      {
        kind: 'related',
        links: [{ label: `业务参与方 ${row.partyId}`, hash: '#/business-parties' }],
      },
      {
        kind: 'audit',
        fields: presentFields([
          { label: '登记依据', value: row.basis, mono: true },
          { label: '登记时间', value: formatInstant(row.registeredAt), mono: true },
          { label: '租户', value: row.tenantId, mono: true },
          { label: '修订号', value: `r${row.revision}`, mono: true },
        ]),
      },
    ],
  };
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
  previewId,
  onPreviewChange,
}: {
  answer: ApiResult<GroupLegalEntityListResponseBody> | null;
  retry: () => void;
  previewId: string | null;
  onPreviewChange: (id: string | null) => void;
}) {
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState<LegalEntityStatusFilter>('ALL');
  const [sort, setSort] = useState<LegalEntitySortKey>('registered-desc');

  const entities = answer?.kind === 'outcome' ? answer.body.entities : [];
  // 筛选与排序只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约（归票 04）。
  const visibleEntities = sortLegalEntities(filterLegalEntities(entities, { search, status }), sort);

  return (
    <ListPageTemplate<GroupLegalEntityRecord>
        title={info.title}
        description="责任法人是对外签约、开票与结算的经营主体；每次登记形成新修订，历史不可覆盖"
        search={{
          value: search,
          onChange: setSearch,
          placeholder: '搜索法人、参与方身份或名称',
        }}
        filters={
          <select
            className="h-6 shrink-0 rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 text-[11px] text-idpxyz-text outline-none focus-visible:ring-1 focus-visible:ring-idpxyz-accent/35"
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
        }
        sort={{
          options: [...legalEntitySortOptions],
          value: sort,
          onChange: (value) => setSort(value as LegalEntitySortKey),
        }}
        filterSummary={
          // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 个」会与状态区「这不是目录为空」直接矛盾
          // （README 列表页上列通则）。总数与当前显示数分开报（票 01 裁决 1）。
          answer?.kind === 'outcome' ? legalEntityCountSummary(entities.length, visibleEntities.length) : undefined
        }
        masterDetail={{
          selectedKey: previewId,
          onSelect: onPreviewChange,
          renderDetail: (entity) => (
            <LegalEntityDetailPage
              legalEntityId={entity.legalEntityId}
              row={entity}
              listAnswer={answer}
              retry={retry}
              onBack={() => onPreviewChange(null)}
            />
          ),
        }}
        columns={columns}
        rows={visibleEntities}
        rowKey={(row) => row.legalEntityId}
        // 单击进检查器，双击开对象页：表留在左边，翻行时右栏跟着换，不再用抽屉盖住列表。
        inspector={inspectorOf}
        onRowOpen={(row) => {
          window.location.hash = detailHash(row.legalEntityId);
        }}
        // 筛出为空不是空态（裁决 1）：viewState 按总数判，表格区另显一行。
        emptyRowsNote={legalEntityNoMatchNote}
        viewState={catalogueViewState(answer, entities.length, retry, {
          module: info,
          endpoint: 'GET /commercial-group-legal-entities',
          emptyTitle: '当前租户尚无责任法人登记',
          emptyDescription: '在「登记法人」签登记第一个，或用受控 CLI parcel-commercial register-parties 灌入。',
        })}
      />
  );
}

/**
 * 集团与法人：查阅法人册，外加登记签（ADR-0085，票 admin-write-faces/02 切片 02c）。
 *
 * 登记签只装法人身份登记一册。停用不摆这里：写签跟着读得最全的那一页走，业务参与方页的
 * 身份本体册停用时点与依据两件都显。本页列表的身份状态格只标停用时点；对象页会列出依据，
 * 停用签仍留在那一页，不在本签再摆一份。
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
  const tabReturn = useTabReturn();
  // 渲列表还是对象页由 hash 二段定：对象地址在壳层是自己的一张标签，本组件在那张标签里渲对象页。
  const [selectedId, setSelectedId] = useState<string | null>(selectedIdFromHash);
  const [previewId, setPreviewId] = useState<string | null>(null);
  useEffect(() => {
    const onHashChange = () => setSelectedId(selectedIdFromHash());
    window.addEventListener('hashchange', onHashChange);
    return () => window.removeEventListener('hashchange', onHashChange);
  }, []);
  // 列表没取到（加载中 / 未配置 / 出错）传 null：表单那边建议修订号一律为 1，不拿空数组冒充「册上没有」。
  const knownEntities = answer?.kind === 'outcome' ? answer.body.entities : null;

  if (selectedId !== null) {
    const row = knownEntities?.find((entity) => entity.legalEntityId === selectedId) ?? null;
    return (
      <LegalEntityDetailPage
        legalEntityId={selectedId}
        row={row}
        listAnswer={answer}
        retry={retry}
        onBack={() => tabReturn.returnTo('group-legal-entities')}
      />
    );
  }

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs defaultValue="entities" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="entities">责任法人目录</TabsTrigger>
          <TabsTrigger value="register">登记责任法人</TabsTrigger>
        </TabsList>
        <TabsContent
          value="entities"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <GroupLegalEntitiesTable answer={answer} retry={retry} previewId={previewId} onPreviewChange={setPreviewId} />
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
