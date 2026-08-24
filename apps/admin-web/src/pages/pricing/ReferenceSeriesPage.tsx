import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['reference-series'];

/**
 * 计价参考序列版本行。字段取 parcel-pricing CONTEXT.md「计价参考序列」词条与登记
 * 不变量：登记一期取值是一次来源事实断言，必须携带来源标识、取值凭证、登记责任方
 * 与生效区间。逐期取值与取值凭证属详情区，接线时另行呈现。
 */
export interface ReferenceSeriesVersionRow {
  /** 序列来源标识（承运商公布费率、财务侧牌价等外部来源）。 */
  series: string;
  /** 种类：燃油费率或汇率——CONTEXT 点名的两个实例。 */
  kind: string;
  /** 序列版本。取值更正形成新版本、不追溯改写，所以版本列是行身份的一半。 */
  seriesVersion: string;
  /** 生效区间。 */
  effectiveRange: string;
  /** 汇率口径（牌价类型、取值时点规则、加点规则），由商业价格政策版本化声明；燃油序列为空。 */
  quoteBasis: string;
  /** 证据等级：任何一期缺可复核凭证即断言强度，只准隔离验证，不得支撑生产金额。 */
  evidenceGrade: string;
  /** 更正关系（更正自某版本）；非更正版本为空。 */
  corrects: string;
  /** 登记责任方——转抄与登记错误由其承担，口径错误由声明口径的商业价格政策承担。 */
  registrant: string;
}

// 汇率口径单列而不并入种类——不接受未声明口径的裸汇率是 CONTEXT 硬句，
// 口径列空着的汇率行本身就是复核发现，值得一眼可见。
const columns: ListColumn<ReferenceSeriesVersionRow>[] = [
  { id: 'series', header: '序列来源标识', className: 'font-mono', render: (row) => row.series },
  { id: 'kind', header: '种类', align: 'center', className: 'w-[88px]', render: (row) => row.kind },
  { id: 'version', header: '序列版本', align: 'center', render: (row) => row.seriesVersion },
  { id: 'range', header: '生效区间', render: (row) => row.effectiveRange },
  { id: 'basis', header: '汇率口径', render: (row) => row.quoteBasis },
  { id: 'grade', header: '证据等级', align: 'center', render: (row) => row.evidenceGrade },
  { id: 'corrects', header: '更正自', className: 'font-mono', render: (row) => row.corrects },
  { id: 'registrant', header: '登记责任方', render: (row) => row.registrant },
];

/**
 * 计价参考序列：已登记序列版本的查阅/复核面。
 * 页面刻意没有「登记 / 修改」动作——序列登记走受控登记口（parcel-pricing-register），
 * 且数值由外部产生：计价只登记不生产数值、不选定商业口径（ADR-0013），
 * 页面提供改数入口会把这两条所有权都画错。
 */
export function ReferenceSeriesPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<ReferenceSeriesVersionRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索序列来源标识',
      }}
      columns={columns}
      // 接线前无实例：两个序列的真实期次是 PAR-SET-11 实例（待提供是常态），不造合成行。
      rows={[]}
      rowKey={(row) => `${row.series}@${row.seriesVersion}`}
      pagination={{
        page,
        pageSize,
        total: 0,
        onPageChange: setPage,
        onPageSizeChange: setPageSize,
      }}
      viewState={{
        kind: 'unconfigured',
        title: '查询端点尚未接线',
        description: `序列登记走受控登记口，不经在线端点；本页是查阅面，其查询端点尚未建，不发请求、不含合成数据与未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
