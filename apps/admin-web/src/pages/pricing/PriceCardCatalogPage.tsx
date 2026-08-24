import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['price-card-catalog'];

/**
 * 价卡目录列表行。字段取 parcel-pricing CONTEXT.md「定价方案」「定价方案版本」定义与
 * 价卡版本登记册的比对/检索列；登记快照的方案全图（价表、规则工件）属详情区，接线时
 * 另行呈现。登记册行只增不改，本页没有任何改写动作。
 */
export interface PriceCardVersionRow {
  /** 定价方案标识。 */
  plan: string;
  /** 定价方案版本——一次受控发布形成的不可覆盖可执行版本。 */
  planVersion: string;
  /** 价格方向（BUY / SELL / INTERNAL）；三方向不因同名产品或渠道自动合并。 */
  direction: string;
  /** 计算目的；首发取值与方向一一配对，不得携带不匹配的组合。 */
  purpose: string;
  /** 适用期。 */
  applicablePeriod: string;
  /** 规范化版本。 */
  canonicalizationVersion: string;
  /** 版本内容摘要；展示截断归供数方，本页不改写摘要。 */
  contentDigest: string;
  /** 源文件身份（名称）；真文件外置，登记册只登名称与 SHA-256。 */
  sourceFile: string;
  /** 发布批准责任方。 */
  approvedBy: string;
}

// 规范化版本与内容摘要相邻成列——摘要只在同一规范化版本内可比、不携规范化版本的摘要
// 视为不完整（CONTEXT「版本内容摘要」，ADR-0014），呈现时不把两者拆散。
const columns: ListColumn<PriceCardVersionRow>[] = [
  { id: 'plan', header: '定价方案', className: 'font-mono', render: (row) => row.plan },
  { id: 'version', header: '方案版本', align: 'center', render: (row) => row.planVersion },
  { id: 'direction', header: '价格方向', align: 'center', className: 'w-[88px] font-mono', render: (row) => row.direction },
  { id: 'purpose', header: '计算目的', className: 'font-mono', render: (row) => row.purpose },
  { id: 'period', header: '适用期', render: (row) => row.applicablePeriod },
  { id: 'canon', header: '规范化版本', align: 'center', render: (row) => row.canonicalizationVersion },
  { id: 'digest', header: '版本内容摘要', className: 'font-mono', render: (row) => row.contentDigest },
  { id: 'source', header: '源文件身份', render: (row) => row.sourceFile },
  { id: 'approved', header: '发布批准责任方', render: (row) => row.approvedBy },
];

/**
 * 价卡目录：已登记定价方案版本的查阅/复核面。
 * 页面刻意没有「新建 / 登记」动作——价卡登记是治理动作，走受控登记口
 * （parcel-pricing-register，操作员在数据库网络内手跑），不进在线请求面；
 * 在这里放登记按钮等于把治理动作搬回在线面，与登记口的存在理由相悖。
 */
export function PriceCardCatalogPage() {
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  return (
    <ListPageTemplate<PriceCardVersionRow>
      title={info.title}
      description={info.owner}
      search={{
        value: search,
        onChange: setSearch,
        placeholder: '搜索定价方案 / 源文件身份',
      }}
      columns={columns}
      // 接线前无实例：价卡内容全属实例半边（PAR-SET-02/03 待提供），不造合成行。
      rows={[]}
      rowKey={(row) => `${row.plan}@${row.planVersion}`}
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
        description: `价卡登记走受控登记口，不经在线端点；本页是查阅面，其查询端点尚未建，不发请求、不含合成数据与未确认参数的默认值。场景出处：${info.source}`,
      }}
    />
  );
}
