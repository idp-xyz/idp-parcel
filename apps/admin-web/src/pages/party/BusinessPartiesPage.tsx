import { useEffect, useState } from 'react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { MultiRegistrationPanel, type RegistrationTarget } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  commercialRegistrationEndpoints,
  listBusinessParties,
  listPartyRelationships,
  partyIdentityOutcomeLabels,
  registerCommercial,
  type BusinessPartyListResponseBody,
  type BusinessPartyRecord,
  type PartyRelationshipListResponseBody,
  type PartyRelationshipRecord,
} from './api';
import {
  identityStatusLabels,
  labelOf,
  partyRoleLabels,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
  relationshipStatusLabels,
} from './presentation';

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
        <p className="mt-0.5 text-xs text-idpxyz-textMuted">参与方册查无此身份</p>
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
    render: (row) => formatRange(row.effectiveStartsAt, row.effectiveEndsAt),
  },
  {
    id: 'status',
    header: '状态',
    align: 'center',
    // 撤销/替代的时点、依据与后继是这格状态的内容：撤销或到期只影响生效边界后
    // 的新决定，看这格的人需要知道边界在哪、依据是什么、被谁替代。
    render: (row) => (
      <div>
        <p>{labelOf(relationshipStatusLabels, row.status)}</p>
        {row.endedAt ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">
            自 {formatInstant(row.endedAt)}
          </p>
        ) : null}
        {row.endBasis ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.endBasis}</p>
        ) : null}
        {row.successorId ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">→ {row.successorId}</p>
        ) : null}
      </div>
    ),
  },
];

// 身份本体册的列。停用两件（时点 + 依据）与状态同格呈现：看这格的人要知道自何时起
// 停用、依据是什么；只显示一个「已停用」说不出这两样。
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
    id: 'basis',
    header: '依据',
    className: 'font-mono text-xs',
    render: (row) => row.basis,
  },
  {
    id: 'effective-from',
    header: '生效时点',
    className: 'font-mono text-xs',
    render: (row) => formatInstant(row.effectiveFrom),
  },
  {
    id: 'status',
    header: '状态',
    align: 'center',
    render: (row) => (
      <div>
        <p>{labelOf(identityStatusLabels, row.status)}</p>
        {row.deactivatedAt ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">
            自 {formatInstant(row.deactivatedAt)}
          </p>
        ) : null}
        {row.deactivationBasis ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.deactivationBasis}</p>
        ) : null}
      </div>
    ),
  },
];

/**
 * 参与方身份本体册。它与关系册同页分签，而不是并进关系表：一个参与方既可以不是法人、
 * 也可以不在任何关系里，被停用的那种恰恰如此——身份生命周期的「已登记」与「已停用」
 * 两格只有在这一册上才有实例可显（票 admin-remainder-mechanism-batch/01 的补格裁定）。
 */
function BusinessPartyIdentitiesTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<BusinessPartyListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listBusinessParties().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const parties = answer?.kind === 'outcome' ? answer.body.parties : [];
  const needle = search.trim().toLowerCase();
  const visibleParties = needle
    ? parties.filter((row) =>
        [row.partyId, row.partyName, row.status].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : parties;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<BusinessPartyRecord>
      title={info.title}
      description={`${info.owner}——行对象是角色中立的参与方身份本体的最新登记修订，状态按装载时点导出`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索参与方标识、名称或状态',
      }}
      filterSummary={
        answer?.kind === 'outcome' ? `当前返回 ${parties.length} 个参与方身份` : undefined
      }
      columns={identityColumns}
      rows={visibleParties}
      rowKey={(row) => row.partyId}
      viewState={catalogueViewState(answer, parties.length, retry, {
        module: info,
        endpoint: 'GET /commercial-business-parties',
        emptyTitle: '当前租户尚无参与方身份登记',
        emptyDescription:
          '读取入口已配置，但登记册为空；页面不会预置参与方。登记可走本页「登记」签，或受控 CLI parcel-commercial register-parties。',
      })}
    />
  );
}

/**
 * 业务参与方（party-commercial）。三签：身份本体册、关系册与登记签。
 *
 * 关系行对象是参与方关系的最新登记修订：承运商、承运商代理商、转售商、聚合平台与渠道
 * 账号持有人都以「双方 + 角色 + 有效区间」的时态关系表达，代理关系不自动合并交易角色；
 * 关系登记按修订版本化不可覆盖。两册的状态代数不同——身份状态按时点导出、关系状态是
 * 登记进来的事实，所以分签而不是并表。
 */
function PartyRelationshipsTable() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<PartyRelationshipListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listPartyRelationships().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const relationships = answer?.kind === 'outcome' ? answer.body.relationships : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleRelationships = needle
    ? relationships.filter((row) =>
        [
          row.relationshipId,
          row.holderId,
          row.holderName ?? '',
          row.counterpartyId,
          row.counterpartyName ?? '',
          row.role,
          row.status,
        ].some((value) => value.toLowerCase().includes(needle)),
      )
    : relationships;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<PartyRelationshipRecord>
      title={info.title}
      description={`${info.owner}——行对象是参与方关系的最新登记修订，双方名称从参与方册转写`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索关系、参与方或角色',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 段」会与状态区
        // 「这不是目录为空」直接矛盾（README 列表页上列通则第六条）。
        answer?.kind === 'outcome' ? `当前返回 ${relationships.length} 段参与方关系` : undefined
      }
      columns={columns}
      rows={visibleRelationships}
      rowKey={(row) => row.relationshipId}
      viewState={catalogueViewState(answer, relationships.length, retry, {
        module: info,
        endpoint: 'GET /commercial-party-relationships',
        emptyTitle: '当前租户尚无参与方关系登记',
        emptyDescription:
          '读取入口已配置，但登记册为空；页面不会预置参与方或关系。登记可走本页「登记」签，或受控 CLI parcel-commercial register-parties。',
      })}
    />
  );
}

/**
 * 登记签装的三本册（ADR-0085，票 admin-write-faces/02 切片 02c）。前两本与本页两张读签
 * 一一对应，选册按钮的词取读签自己的词，不为登记签另造说法。
 *
 * 第三本停用摆在本页，理由是本页读得见它刚写进去的那两件：停用快照每项必填 basis，而三个
 * 身份读面里只有身份本体册把停用时点与停用依据都渲染出来——法人册那格只有时点。登在哪里
 * 看得见结果就摆哪里，这是「写签跟着读签走」在本册上的落法。
 *
 * **不按 kind 切成两签**：快照收的是 deactivations 数组，kind 在每一项上、tenantId 在整批
 * 上，一次提交本来就可以同时停一个参与方与一个法人。切签就得让每页拒收非本页那种 kind，
 * 那是管理台编一条服务端没有的约束——页面不教一条不真的规则。
 *
 * 「登记签不比读签多铺一册」那条没有被触发：它禁的是同一册在两处都能登、其中一处看不见
 * 结果，而这里是一册一处登、读面分三处。另两处的去处由停用那条 snapshotHint 末句说出，
 * 披露义务已在词表里尽过，不在这里补第二遍。
 */
const registrationTargets: RegistrationTarget[] = (
  [
    ['business-party', '参与方身份'],
    ['party-relationship', '参与方关系'],
    // 「停用」取身份状态格里的封闭词（已停用），不为登记签另造一个动词。
    ['identity-deactivation', '身份停用'],
  ] as const
).map(([kind, label]) => ({
  id: kind,
  label,
  title: registrationTitles[kind],
  endpoint: `POST ${commercialRegistrationEndpoints[kind]}`,
  snapshotHint: registrationSnapshotHints[kind],
  submit: (snapshot: unknown) => registerCommercial(kind, snapshot),
  // 三册共用一份答案代数（服务端交回同一个 PartyRegistryOutcome），所以这一格在这里给一次
  // 而不是逐册各抄；其中`已停用`与`册上没有这一个身份`两格只可能来自停用那一册。
  outcomeLabels: partyIdentityOutcomeLabels,
}));

export function BusinessPartiesPage() {
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
          <BusinessPartyIdentitiesTable />
        </TabsContent>
        <TabsContent
          value="relationships"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <PartyRelationshipsTable />
        </TabsContent>
        <TabsContent
          value="register"
          className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden"
        >
          <MultiRegistrationPanel
            moduleId="business-parties"
            targets={registrationTargets}
            problemNote={problemNote}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
