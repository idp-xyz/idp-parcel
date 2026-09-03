import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState } from '../catalogue-view';
import {
  listCommercialPolicies,
  type CommercialPolicyKind,
  type CommercialPolicyListResponseBody,
} from './api';
import { kindColumns, rowsOf, type PolicyRow } from './policy-rows';
import { commercialPolicyKinds, policyKindLabels } from './presentation';

const info = moduleInfoById['commercial-policies'];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

// 各政策册独立请求、独立列形;同页切换不把各类对象折成一份「大配置」。行与列的转写在
// policy-rows.ts,本文件只管取数、切册与渲染。
export function CommercialPoliciesPage() {
  const [kind, setKind] = useState<CommercialPolicyKind>('ACCEPTANCE_RULE_PACKAGE');
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    kind: CommercialPolicyKind;
    answer: ApiResult<CommercialPolicyListResponseBody>;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listCommercialPolicies(kind).then((answer) => {
      if (!cancelled) setLoaded({ kind, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [kind, reloadKey]);

  const answer = loaded?.kind === kind ? loaded.answer : null;
  const rows = answer?.kind === 'outcome' ? rowsOf(answer.body) : [];
  const needle = search.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<PolicyRow>
      title={info.title}
      description={`${info.owner}——七类政策册分别查阅,重叠候选仍是适用冲突而非「同时生效」。接单规则包一栏另列挂在同一版本上的收寄资格与终局规则声明,授权规则一栏按请求方逐格列出取消授权,信用政策一栏的额度按金额或比例恰一上列`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索政策对象、范围或引用',
      }}
      filters={
        <>
          {commercialPolicyKinds.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === kind)}
              onClick={() => setKind(candidate)}
            >
              {policyKindLabels[candidate]}
            </button>
          ))}
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示,未配置态不报「0 条」(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome' ? `${policyKindLabels[kind]} ${rows.length} 条` : undefined
      }
      columns={kindColumns[kind]}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /commercial-policies?kind=${kind}`,
        emptyTitle: `当前租户尚无${policyKindLabels[kind]}`,
        emptyDescription: '读取入口已配置,但该政策册为空;页面不会生成默认政策。',
      })}
    />
  );
}
