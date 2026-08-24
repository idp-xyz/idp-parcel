import { useState } from 'react';
import { ListPageTemplate, type ListColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';

// 主责上下文与场景出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['customs-ports-paths'];

/**
 * 口岸与申报路径查阅面。词取两处原句：CONTEXT-MAP「customs-compliance ↔
 * network-routing」——关务提供合规候选区域、口岸、申报路径、限制及解除结果，
 * 路由只在合格候选中选择；customs-compliance CONTEXT.md 所有权句——本上下文
 * 拥有合规候选区域、口岸、申报路径和关务适用性判断。
 *
 * 三族对象在权威文档里只有所有权与协作句，没有字段级定义；各族栏目只取那
 * 两句出现过的词，不虚构字段。对象建模落地时以真实定义为准重谈，不得反过来
 * 把本页当已发布的查询 Schema。
 */
type PortPathRow = Record<string, string>;

function col(
  id: string,
  header: string,
  options?: { mono?: boolean; align?: 'left' | 'center' | 'right'; className?: string },
): ListColumn<PortPathRow> {
  return {
    id,
    header,
    align: options?.align,
    className: options?.className,
    render: (row) =>
      options?.mono ? <span className="font-mono text-[12px]">{row[id]}</span> : row[id],
  };
}

interface PortPathFamily {
  id: string;
  /** 文档原词。 */
  word: string;
  columns: ListColumn<PortPathRow>[];
}

const families: PortPathFamily[] = [
  {
    id: 'candidate-regions',
    word: '合规候选区域',
    columns: [
      col('code', '区域标识', { mono: true }),
      col('applicability', '关务适用性判断'),
      col('restrictions', '限制及解除结果（引用）', { mono: true }),
    ],
  },
  {
    id: 'ports',
    word: '口岸',
    columns: [
      col('code', '口岸标识', { mono: true }),
      col('region', '所属合规候选区域', { mono: true }),
      col('applicability', '关务适用性判断'),
      col('restrictions', '限制及解除结果（引用）', { mono: true }),
    ],
  },
  {
    id: 'declaration-paths',
    word: '申报路径',
    // 报关服务方入列的依据是改路硬句点名的三个变更维（关务区域、口岸、报关
    // 服务方）——路径换其中任何一维都必须重新请求关务适用性判断。
    columns: [
      col('code', '申报路径标识', { mono: true }),
      col('port', '口岸', { mono: true }),
      col('broker', '报关服务方'),
      col('applicability', '关务适用性判断'),
    ],
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

/**
 * 口岸与申报路径（customs-compliance）。页面只查阅、无任何选择动作——路由在
 * 合格候选中选择是 network-routing 的事；拟改路改变关务区域、口岸或报关服务
 * 方时，必须先请求新的关务适用性判断，不能沿用原申报或放行结果（CONTEXT-MAP
 * 原句），本页的适用性列即那次判断的查阅口。
 */
export function CustomsPortsPathsPage() {
  const [familyId, setFamilyId] = useState(families[0].id);
  const [keyword, setKeyword] = useState('');
  const family = families.find((candidate) => candidate.id === familyId) ?? families[0];

  return (
    <ListPageTemplate<PortPathRow>
      title={info.title}
      description={`${info.owner}——路由只在合格候选中选择；改区域、口岸或报关服务方须重新请求关务适用性判断`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按区域 / 口岸 / 申报路径标识检索',
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
      rowKey={(row) => `${row.code}`}
      viewState={{
        kind: 'unconfigured',
        title: '关务合规模块尚未接线',
        description: '合规候选区域、口岸与申报路径的域模型与持久化均未建（本页把规划占位补齐为查阅骨架），不发请求、不含未确认参数的默认值。',
        facts: {
          owner: info.owner,
          source: info.source,
          unlock: '三族对象建模落地、查询端点建成并经 ADR-0017 准入闸门放行后接线',
        },
      }}
    />
  );
}
