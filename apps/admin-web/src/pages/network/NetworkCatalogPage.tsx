import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listNetworkCatalog,
  type NetworkCatalogFamily,
  type NetworkCatalogListResponseBody,
  type NetworkVersionRecord,
} from './api';
import {
  adjustmentKindLabels,
  familyLabels,
  labelOf,
  networkCatalogFamilies,
  targetKindLabels,
} from './presentation';

const info = moduleInfoById['network-catalog'];

interface CatalogRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

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
        <span className="font-mono text-[12px]">{row.values[id] ?? '—'}</span>
      ) : (
        row.values[id] ?? '—'
      ),
  };
}

// 列向与传输层各族行体同形(MCP-3 裁决总则:读面有的上列,读面没有的不上列)。
// 日历正文、策略正文等内容列今天不存在(PAR-NET-14),不发明列。
const familyColumns: Record<NetworkCatalogFamily, ListColumn<CatalogRow>[]> = {
  node: [
    col('code', '节点代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('businessTimezone', '业务时区', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  connection: [
    col('code', '连接代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('endpoints', '端点节点', { mono: true }),
    col('businessTimezone', '业务时区', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  line: [
    col('code', '线路代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('segments', '组成段(按序)', { mono: true, className: 'min-w-64' }),
    col('applicableScope', '适用范围', { mono: true }),
    col('businessTimezone', '业务时区', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  // 服务区域族由专页承担(裁决③),本页 chip 不含它;封闭集类型要求键在场,列表留空。
  'service-area': [],
  'service-calendar': [
    col('targetKind', '适用对象类别'),
    col('targetCode', '适用对象代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
  'availability-adjustment': [
    col('code', '调整代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('targetKind', '适用对象类别'),
    col('targetCode', '适用对象代码', { mono: true }),
    col('adjustmentKind', '调整种类'),
    col('source', '来源', { mono: true }),
    col('window', '生效/解除', { mono: true, className: 'min-w-64' }),
  ],
  'route-strategy': [
    col('code', '策略代码', { mono: true }),
    col('version', '版本', { align: 'right', className: 'w-[72px]' }),
    col('applicableScope', '适用范围', { mono: true }),
    col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
  ],
};

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

// 行体是七族字段并集上的可选字段(与 api.ts 的 NetworkVersionRecord 同形),这里按
// 当前族取值;缺席字段如实显「—」,不为区间完整编造时刻。
function rowValues(family: NetworkCatalogFamily, record: NetworkVersionRecord): CatalogRow {
  const effective = record.effectiveFrom
    ? formatRange(record.effectiveFrom, record.effectiveTo)
    : '—';
  switch (family) {
    case 'connection':
      return {
        key: `connection:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          endpoints: `${record.fromNode ?? '—'} → ${record.toNode ?? '—'}`,
          businessTimezone: record.businessTimezone ?? '—',
          effective,
        },
      };
    case 'line':
      return {
        key: `line:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          segments: (record.segments ?? []).join(' → '),
          applicableScope: record.applicableScope ?? '—',
          businessTimezone: record.businessTimezone ?? '—',
          effective,
        },
      };
    case 'service-calendar':
      return {
        key: `service-calendar:${record.targetKind}:${record.targetCode}@${record.version}`,
        values: {
          targetKind: labelOf(targetKindLabels, record.targetKind ?? ''),
          targetCode: record.targetCode ?? '—',
          version: String(record.version),
          effective,
        },
      };
    case 'availability-adjustment':
      return {
        key: `availability:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          targetKind: labelOf(targetKindLabels, record.targetKind ?? ''),
          targetCode: record.targetCode ?? '—',
          adjustmentKind: labelOf(adjustmentKindLabels, record.kind ?? ''),
          source: record.source ?? '—',
          window: `${record.effectiveAt ? formatInstant(record.effectiveAt) : '—'} → ${
            record.liftedAt ? formatInstant(record.liftedAt) : '未解除'
          }`,
        },
      };
    default:
      // node 与 route-strategy 同为「代码 + 版本 + 区间」骨架;route-strategy 另有
      // 适用范围列,值缺席时由列渲染兜「—」。
      return {
        key: `${family}:${record.code}@${record.version}`,
        values: {
          code: record.code ?? '—',
          version: String(record.version),
          businessTimezone: record.businessTimezone ?? '—',
          applicableScope: record.applicableScope ?? '—',
          effective,
        },
      };
  }
}

// 逐族查阅版本原文;chip 六族不含服务区域(MCP-3 裁决③,服务区域由专页承担)。
export function NetworkCatalogPage() {
  const [familyId, setFamilyId] = useState<NetworkCatalogFamily>('node');
  const [keyword, setKeyword] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    family: NetworkCatalogFamily;
    answer: ApiResult<NetworkCatalogListResponseBody>;
  } | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listNetworkCatalog(familyId).then((answer) => {
      if (!cancelled) setLoaded({ family: familyId, answer });
    });
    return () => {
      cancelled = true;
    };
  }, [familyId, reloadKey]);

  const answer = loaded?.family === familyId ? loaded.answer : null;
  const rows =
    answer?.kind === 'outcome'
      ? answer.body.versions.map((record) => rowValues(familyId, record))
      : [];
  const needle = keyword.trim().toLowerCase();
  const visibleRows = needle
    ? rows.filter((row) =>
        Object.values(row.values).some((value) => value.toLowerCase().includes(needle)),
      )
    : rows;
  const retry = () => setReloadKey((value) => value + 1);

  return (
    <ListPageTemplate<CatalogRow>
      title={info.title}
      description={`${info.owner}——逐族查阅版本原文,不选版、不折叠为路由判断`}
      search={{
        value: keyword,
        onChange: setKeyword,
        placeholder: '按代码或适用对象代码检索',
      }}
      filters={
        <>
          {networkCatalogFamilies.map((candidate) => (
            <button
              key={candidate}
              type="button"
              className={chipClass(candidate === familyId)}
              onClick={() => setFamilyId(candidate)}
            >
              {familyLabels[candidate]}
            </button>
          ))}
        </>
      }
      filterSummary={
        // 计数只在拿到业务答案后显示:未配置/错误态下「0 个版本」会与状态区
        // 「这不是目录为空」的说明自相矛盾(与 pricing 两页同一守卫)。
        answer?.kind === 'outcome'
          ? `${familyLabels[familyId]} ${rows.length} 个版本`
          : undefined
      }
      columns={familyColumns[familyId]}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /network-catalog?family=${familyId}`,
        emptyTitle: `当前租户尚无${familyLabels[familyId]}版本`,
        emptyDescription: '读取入口已配置,但该目录族为空;页面不会借其他族的数据补位。',
      })}
    />
  );
}
