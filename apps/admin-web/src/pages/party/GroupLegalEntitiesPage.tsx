import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import {
  listGroupLegalEntities,
  type GroupLegalEntityListResponseBody,
  type GroupLegalEntityRecord,
} from './api';
import { identityStatusLabels, labelOf, legalEntityKindLabels } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['group-legal-entities'];

// 骨架期这里列过「运营集团租户」——ADR-0003 里那一级是配置与隔离边界本身，读面
// 本来就在单租户作用域内取数，整列同值没有信息，接线时按 live 页惯例撤下。
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
    id: 'kind',
    header: '对象类型',
    align: 'center',
    render: (row) => labelOf(legalEntityKindLabels, row.kind),
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
      <div>
        <p>{labelOf(identityStatusLabels, row.status)}</p>
        {row.deactivatedAt ? (
          <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">
            自 {formatInstant(row.deactivatedAt)}
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
    render: (row) => formatInstant(row.effectiveFrom),
  },
  {
    id: 'registered-at',
    header: '登记时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.registeredAt),
  },
];

/**
 * 集团与法人（party-commercial）。行对象是责任法人的最新登记修订：法人钉在稳定的
 * 业务参与方身份上（ADR-0003 三级边界的第二级），名称从参与方册转写；身份登记按
 * 修订版本化不可覆盖，停用形成新修订而不是删除。查阅面，不设登记动作——登记与
 * 停用走 parcel-commercial 受控 CLI。
 */
export function GroupLegalEntitiesPage() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<GroupLegalEntityListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listGroupLegalEntities().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const entities = answer?.kind === 'outcome' ? answer.body.entities : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleEntities = needle
    ? entities.filter((row) =>
        [row.legalEntityId, row.partyId, row.partyName ?? '', row.status].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : entities;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<GroupLegalEntityRecord>
      title={info.title}
      description={`${info.owner}——行对象是责任法人的最新登记修订，名称从参与方册转写`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索法人、参与方身份或名称',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 个」会与状态区
        // 「这不是目录为空」直接矛盾（README 列表页上列通则第六条）。
        answer?.kind === 'outcome' ? `当前返回 ${entities.length} 个责任法人` : undefined
      }
      columns={columns}
      rows={visibleEntities}
      rowKey={(row) => row.legalEntityId}
      viewState={catalogueViewState(answer, entities.length, retry, {
        module: info,
        endpoint: 'GET /commercial-group-legal-entities',
        emptyTitle: '当前租户尚无责任法人登记',
        emptyDescription: '读取入口已配置，但登记册为空；页面不会预置法人或参与方身份。',
      })}
    />
  );
}
