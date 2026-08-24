import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['network-catalog'];

/**
 * 网络目录查阅面。七族对象取 network-routing CONTEXT.md 原词：物流节点、网络连接、
 * 线路、服务区域、服务日历、网络可用性调整、路由策略版本；族的划分与登记口
 * cmd/parcel-network-register 的 -kind 七族一一对应。
 *
 * 登记走受控 CLI（治理动作，不是在线请求面），本页只查阅、无登记动作。查询端点
 * 未建，数据区如实呈现「未配置」态，不发请求、不含合成数据。
 *
 * 行形状不在此复刻：七族真实字段的权威是登记口的七个载荷形状（nodePayload 等，
 * 见 cmd/parcel-network-register），查询契约落地时以其为准重谈；本页行用列 id
 * 索引的字符串占位，栏目语义全在列头与注释。
 */
type CatalogRow = Record<string, string>;

function col(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ListColumn<CatalogRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? (
        <span className="font-mono text-[12px]">{row[id]}</span>
      ) : (
        row[id]
      ),
  };
}

interface CatalogFamily {
  /** 与登记口 -kind 的族名一致，接线时按此族请求查询。 */
  id: string;
  /** CONTEXT.md 原词。 */
  word: string;
  columns: ListColumn<CatalogRow>[];
}

// 版本与生效区间是七族共有的版本骨架；「生效至」缺席表示无终点（登记口用指针
// 表达「不在场」，呈现侧同样不得把零时刻当无终点）。
const families: CatalogFamily[] = [
  {
    id: 'node',
    word: '物流节点',
    columns: [
      col('code', '节点代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('businessTimezone', '业务时区', { mono: true }),
      col('effectiveFrom', '生效自', { mono: true }),
      col('effectiveTo', '生效至', { mono: true }),
    ],
  },
  {
    id: 'connection',
    word: '网络连接',
    // 网络连接是有向拓扑关系(CONTEXT):方向由自节点/至节点两列表达,不折叠成一格。
    columns: [
      col('code', '连接代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('fromNode', '自节点', { mono: true }),
      col('toNode', '至节点', { mono: true }),
      col('businessTimezone', '业务时区', { mono: true }),
      col('effectiveFrom', '生效自', { mono: true }),
      col('effectiveTo', '生效至', { mono: true }),
    ],
  },
  {
    id: 'line',
    word: '线路',
    // 线路由一个或多个网络连接按明确顺序组成(CONTEXT),组成连接列按序呈现。
    columns: [
      col('code', '线路代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('segments', '组成连接（按序）', { mono: true }),
      col('applicableScope', '适用范围'),
      col('businessTimezone', '业务时区', { mono: true }),
      col('effectiveFrom', '生效自', { mono: true }),
      col('effectiveTo', '生效至', { mono: true }),
    ],
  },
  {
    id: 'service-area',
    word: '服务区域',
    // 地理覆盖定义的列还不存在(PAR-NET-14,登记口同注):今天登的就是版本骨架,
    // 本族栏目照实只有骨架,不虚构覆盖列。
    columns: [
      col('code', '区域代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('effectiveFrom', '生效自', { mono: true }),
      col('effectiveTo', '生效至', { mono: true }),
    ],
  },
  {
    id: 'service-calendar',
    word: '服务日历',
    // 服务日历按适用对象(节点、网络连接或线路)登记;营业日/节假日/服务窗口/截单
    // 条件的内容列还不存在(PAR-NET-14),同上只列版本骨架。
    columns: [
      col('targetKind', '适用对象族'),
      col('targetCode', '适用对象代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('effectiveFrom', '生效自', { mono: true }),
      col('effectiveTo', '生效至', { mono: true }),
    ],
  },
  {
    id: 'availability-adjustment',
    word: '网络可用性调整',
    // 调整必须记录来源、范围、生效时间和解除时间(CONTEXT),四者各占一列;
    // 解除时间缺席即仍然生效,解除只恢复候选资格。
    columns: [
      col('code', '调整代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('targetKind', '适用对象族'),
      col('targetCode', '适用对象代码', { mono: true }),
      col('kind', '调整类别'),
      col('source', '来源'),
      col('effectiveAt', '生效时间', { mono: true }),
      col('liftedAt', '解除时间', { mono: true }),
    ],
  },
  {
    id: 'route-strategy',
    word: '路由策略版本',
    // 策略规则正文的列还不存在(PAR-NET-14),只列版本骨架。
    columns: [
      col('code', '策略代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('applicableScope', '适用范围'),
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

export function NetworkCatalogPage() {
  const [familyId, setFamilyId] = useState(families[0].id);
  const [keyword, setKeyword] = useState('');
  const family = families.find((candidate) => candidate.id === familyId) ?? families[0];

  return (
    <ListPageTemplate<CatalogRow>
      title={info.title}
      description={`${info.owner} · 登记走受控 CLI（cmd/parcel-network-register），本页只查阅、无登记动作。`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按代码或适用对象代码检索',
      }}
      filters={
        <>
          {families.map((candidate) => (
            <button
              key={candidate.id}
              type="button"
              className={chipClass(candidate.id === familyId)}
              onClick={() => setFamilyId(candidate.id)}
            >
              {candidate.word}
            </button>
          ))}
        </>
      }
      columns={family.columns}
      rows={[]}
      rowKey={(row) => `${row.code ?? row.targetCode}#${row.version}`}
      viewState={{
        kind: 'unconfigured',
        title: '网络与路由模块尚未接线',
        description:
          '登记口已可写入七族版本骨架，但查阅面的查询契约待建；本页不发请求、不含合成数据。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '网络目录查询契约建成并经 ADR-0017 准入闸门放行后接线；登记动作留在受控 CLI（cmd/parcel-network-register）',
        },
      }}
    />
  );
}
