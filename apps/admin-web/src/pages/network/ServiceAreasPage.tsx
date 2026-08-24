import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['service-areas'];

/**
 * 服务区域与覆盖查阅面。词取 network-routing CONTEXT.md「服务区域」定义：
 * 带有版本和适用期的国家、行政区域、邮编范围或其他地理覆盖定义，用于把客户地址
 * 解析为候选收寄节点、交付节点或尾程注入节点。
 *
 * 与网络目录页的分工：目录页的 service-area 族只列七族公共的版本骨架；本页是
 * 服务区域的专页——版本视图之外还承载「区域 ↔ 节点」的覆盖关系视图。两页读的是
 * 同一族对象，不是两套数据。
 *
 * 两条边界（CONTEXT 原句）钉在这里：服务区域不取得客户地址所有权（客户地址由
 * parcel-shipment 保存，本上下文只保存服务区域、节点覆盖版本和当次解析依据）；
 * 也不直接证明逻辑可达（可达性是另一个判断，有自己的依据与生命周期）。
 */
type AreaRow = Record<string, string>;

function col(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ListColumn<AreaRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? <span className="font-mono text-[12px]">{row[id]}</span> : row[id],
  };
}

interface AreaView {
  id: string;
  word: string;
  columns: ListColumn<AreaRow>[];
}

const views: AreaView[] = [
  {
    id: 'area-versions',
    word: '服务区域版本',
    // 覆盖定义类别取 CONTEXT 定义列举的原词（国家、行政区域、邮编范围或其他地理
    // 覆盖定义）；具体覆盖内容列还不存在——地理覆盖定义属 PAR-NET-14 未登记项，
    // 照网络目录页先例只列版本骨架，不虚构内容列。
    columns: [
      col('code', '区域代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('coverageKind', '覆盖定义类别'),
      col('effectiveFrom', '生效自', { mono: true }),
      col('effectiveTo', '生效至', { mono: true }),
    ],
  },
  {
    id: 'node-coverage',
    word: '节点覆盖关系',
    // 覆盖角色取「服务区域」定义的三个解析目标原词：候选收寄节点、交付节点、
    // 尾程注入节点。节点、连接、线路的永久变化形成新版本不覆盖原版本（CONTEXT
    // 网络定义与临时可用性），覆盖关系因此带区域版本与生效区间两个轴。
    columns: [
      col('areaCode', '服务区域', { mono: true }),
      col('areaVersion', '区域版本', { align: 'right', className: 'w-[80px]' }),
      col('nodeCode', '物流节点', { mono: true }),
      col('coverageRole', '覆盖角色'),
      col('effectiveFrom', '生效自', { mono: true }),
      col('effectiveTo', '生效至', { mono: true }),
    ],
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

export function ServiceAreasPage() {
  const [viewId, setViewId] = useState(views[0].id);
  const [keyword, setKeyword] = useState('');
  const view = views.find((candidate) => candidate.id === viewId) ?? views[0];

  return (
    <ListPageTemplate<AreaRow>
      title={info.title}
      description={`${info.owner}——服务区域不取得客户地址所有权，也不直接证明逻辑可达。`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按区域代码或节点代码检索',
      }}
      filters={
        <>
          {views.map((candidate) => (
            <button
              key={candidate.id}
              type="button"
              className={chipClass(candidate.id === viewId)}
              onClick={() => setViewId(candidate.id)}
            >
              {candidate.word}
            </button>
          ))}
        </>
      }
      columns={view.columns}
      rows={[]}
      rowKey={(row) => `${row.code ?? row.areaCode}#${row.version ?? row.areaVersion}#${row.nodeCode ?? ''}`}
      viewState={{
        kind: 'unconfigured',
        title: '服务区域查询端点尚未建立',
        description: `覆盖定义内容属 PAR-NET-14 待登记实例，查询契约待建；本页不发请求、不含合成数据。场景出处：${info.source}`,
      }}
    />
  );
}
