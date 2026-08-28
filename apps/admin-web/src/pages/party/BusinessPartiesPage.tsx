import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listPartyRelationships,
  type PartyRelationshipListResponseBody,
  type PartyRelationshipRecord,
} from './api';
import { labelOf, partyRoleLabels, relationshipStatusLabels } from './presentation';

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

/**
 * 业务参与方（party-commercial）。行对象是参与方关系的最新登记修订：承运商、
 * 承运商代理商、转售商、聚合平台与渠道账号持有人都以「双方 + 角色 + 有效区间」
 * 的时态关系表达，代理关系不自动合并交易角色；关系登记按修订版本化不可覆盖。
 * 查阅面，不设登记动作——登记走 parcel-commercial 受控 CLI。
 */
export function BusinessPartiesPage() {
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
        emptyDescription: '读取入口已配置，但登记册为空；页面不会预置参与方或关系。',
      })}
    />
  );
}
