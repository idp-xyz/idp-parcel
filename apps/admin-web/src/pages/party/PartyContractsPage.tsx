import { useEffect, useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listCustomerContracts,
  type CustomerContractListResponseBody,
  type CustomerContractRecord,
} from './api';
import { commercialStatusLabels, labelOf } from './presentation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['party-contracts'];

// 骨架期这里列过货主客户账户与责任法人两列。读面里没有它们：0012 的正文表只有
// rule_package_id 与 declared_at，版本壳上也不带客户或法人坐标，所以两列不上——
// 缺的是登记面，不是转写。它们由下面的页面说明如实交代，不留空列、不填假值。
const columns: ListColumn<CustomerContractRecord>[] = [
  {
    id: 'contract',
    header: '合同 / 版本',
    render: (row) => (
      <div className="min-w-48">
        <p className="font-mono font-medium text-idpxyz-text">{row.objectId}</p>
        <p className="mt-0.5 font-mono text-xs text-idpxyz-textMuted">{row.version}</p>
      </div>
    ),
  },
  {
    id: 'scope',
    header: '适用范围',
    className: 'font-mono text-xs',
    render: (row) => row.scope,
  },
  {
    id: 'status',
    header: '生命周期状态',
    render: (row) => labelOf(commercialStatusLabels, row.status),
  },
  {
    id: 'rule-package',
    header: '接单规则包',
    className: 'font-mono text-xs',
    // 正文未登记时不显示空白：空白读起来像「没引用规则包」，而合同正文一旦登记
    // 规则包必存（库上 NOT NULL），两者是不同的事实。
    render: (row) =>
      row.contentRegistered ? (
        row.rulePackageId
      ) : (
        <span className="font-sans text-idpxyz-textMuted">正文未登记</span>
      ),
  },
  {
    id: 'bindings',
    header: '接受前财务控制约定',
    className: 'min-w-64',
    render: (row) => <ControlBindings row={row} />,
  },
  {
    id: 'effective',
    header: '有效区间',
    className: 'min-w-64 font-mono text-xs',
    render: (row) => formatRange(row.effectiveStartsAt, row.effectiveEndsAt),
  },
  {
    id: 'published-at',
    header: '发布时间',
    className: 'min-w-44 font-mono text-xs',
    render: (row) => formatInstant(row.publishedAt),
  },
];

// 零绑定有两种，说法必须分开：没有正文行是「正文未登记」（去登记正文），有正文行
// 而零子行是「已登记，未作约定」（无事可做，且该范围答「不存在」而不是「不适用」）。
// 服务端为此专门给了 contentRegistered，页面照它分，不看数组长度。
function ControlBindings({ row }: { row: CustomerContractRecord }) {
  if (!row.contentRegistered) {
    return <span className="text-idpxyz-textMuted">正文未登记</span>;
  }
  if (row.bindings.length === 0) {
    return <span className="text-idpxyz-textMuted">已登记，未对任何费用范围作约定</span>;
  }
  return (
    <ul className="space-y-0.5">
      {row.bindings.map((binding) => (
        <li key={binding.chargeScope} className="text-xs">
          <span className="font-mono text-idpxyz-text">{binding.chargeScope}</span>
          <span className="text-idpxyz-textMuted"> · </span>
          {binding.policyId ? (
            <span className="font-mono text-idpxyz-text">{binding.policyId}</span>
          ) : (
            <span className="text-idpxyz-textMuted">
              不适用（{binding.inapplicabilityBasis}）
            </span>
          )}
        </li>
      ))}
    </ul>
  );
}

/**
 * 客户与合同（party-commercial）。行对象是客户合同版本：合同版本到期或被
 * 后续版本替代，不改变已经接受委托所保存的合同依据。查阅面，不设登记与
 * 发布动作——商业版本的草稿与发布生命周期归受控登记通道。
 */
export function PartyContractsPage() {
  const [search, setSearch] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [answer, setAnswer] = useState<ApiResult<CustomerContractListResponseBody> | null>(null);

  useEffect(() => {
    let cancelled = false;
    setAnswer(null);
    void listCustomerContracts().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadKey]);

  const contracts = answer?.kind === 'outcome' ? answer.body.contracts : [];
  const needle = search.trim().toLowerCase();
  // 过滤只在已取回的这一页数据上做，不下推成查询参数——那要改端点契约。
  const visibleContracts = needle
    ? contracts.filter((row) =>
        [row.objectId, row.version, row.scope, row.status, row.rulePackageId ?? ''].some((value) =>
          value.toLowerCase().includes(needle),
        ),
      )
    : contracts;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CustomerContractRecord>
      title={info.title}
      description={`${info.owner}——当前读面展示合同版本壳与已登记正文（接单规则包、按费用范围的财务控制约定）；货主客户账户与责任法人尚无登记面，不上列`}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索合同、版本、适用范围或规则包',
      }}
      filterSummary={
        // 计数只在拿到业务答案后显示：未配置态与错误态下报「0 份」会与状态区
        // 「这不是目录为空」直接矛盾（README 列表页上列通则第六条）。
        answer?.kind === 'outcome' ? `当前返回 ${contracts.length} 份合同版本` : undefined
      }
      columns={columns}
      rows={visibleContracts}
      rowKey={(row) => `${row.objectId}@${row.version}`}
      viewState={catalogueViewState(answer, contracts.length, retry, {
        module: info,
        endpoint: 'GET /commercial-customer-contracts',
        emptyTitle: '当前租户尚无客户合同版本',
        emptyDescription: '读取入口已配置，但目录为空；页面不会预置合同或控制约定。',
      })}
    />
  );
}
