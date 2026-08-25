import { useEffect, useState } from 'react';
import { moduleInfoById } from '../../navigation';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { ApiResult } from '../catalogue-api';
import { catalogueViewState, formatInstant, formatRange } from '../catalogue-view';
import {
  listNetworkCatalog,
  type NetworkCatalogFamily,
  type NetworkCatalogResponseBody,
} from './api';
import { adjustmentKindLabel, networkFamilyLabel, targetKindLabel } from './presentation';

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

interface CatalogFamily {
  id: NetworkCatalogFamily;
  columns: ListColumn<CatalogRow>[];
}

const families: CatalogFamily[] = [
  {
    id: 'node',
    columns: [
      col('code', '节点代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('businessTimezone', '业务时区', { mono: true }),
      col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
    ],
  },
  {
    id: 'connection',
    columns: [
      col('code', '连接代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('endpoints', '端点节点（按登记顺序）', { mono: true }),
      col('directed', '方向性'),
      col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
    ],
  },
  {
    id: 'line',
    columns: [
      col('code', '线路代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('connections', '组成连接（按序）', { mono: true }),
      col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
    ],
  },
  {
    id: 'service-area',
    columns: [
      col('code', '区域代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('includedRegions', '包含区域', { mono: true }),
      col('excludedRegions', '排除区域', { mono: true }),
      col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
    ],
  },
  {
    id: 'service-calendar',
    columns: [
      col('code', '日历代码', { mono: true }),
      col('version', '版本', { align: 'right', className: 'w-[72px]' }),
      col('timezone', '时区', { mono: true }),
      col('serviceDays', '服务日', { mono: true }),
      col('exceptionDates', '例外日期', { mono: true }),
      col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
    ],
  },
  {
    id: 'calendar-binding',
    columns: [
      col('targetKind', '适用对象族'),
      col('targetCode', '适用对象代码', { mono: true }),
      col('version', '绑定版本', { align: 'right', className: 'w-[80px]' }),
      col('calendar', '服务日历版本', { mono: true }),
      col('effective', '适用区间', { mono: true, className: 'min-w-64' }),
    ],
  },
  {
    id: 'availability-adjustment',
    columns: [
      col('targetKind', '适用对象族'),
      col('targetCode', '适用对象代码', { mono: true }),
      col('version', '调整版本', { align: 'right', className: 'w-[80px]' }),
      col('adjustmentKind', '调整类别'),
      col('window', '适用窗口', { mono: true, className: 'min-w-64' }),
      col('scopeReference', '适用范围依据', { mono: true }),
      col('reasonReference', '原因依据', { mono: true }),
    ],
  },
];

const chipClass = (active: boolean) =>
  `px-2.5 py-1 text-[12px] rounded border ${
    active
      ? 'border-idpxyz-accent text-idpxyz-accent'
      : 'border-idpxyz-border text-idpxyz-textMuted hover:bg-idpxyz-hover'
  }`;

function rowsOf(body: NetworkCatalogResponseBody): CatalogRow[] {
  switch (body.outcome) {
    case 'NODE_VERSIONS_LISTED':
      return body.versions.map((record) => ({
        key: `node:${record.code}@${record.version}`,
        values: {
          code: record.code,
          version: String(record.version),
          businessTimezone: record.businessTimezone,
          effective: formatRange(record.effectiveFrom, record.effectiveTo),
        },
      }));
    case 'CONNECTION_VERSIONS_LISTED':
      return body.versions.map((record) => ({
        key: `connection:${record.code}@${record.version}`,
        values: {
          code: record.code,
          version: String(record.version),
          endpoints: record.endpointNodeCodes.join(' → '),
          directed: record.directed ? '有向' : '无向',
          effective: formatRange(record.effectiveFrom, record.effectiveTo),
        },
      }));
    case 'LINE_VERSIONS_LISTED':
      return body.versions.map((record) => ({
        key: `line:${record.code}@${record.version}`,
        values: {
          code: record.code,
          version: String(record.version),
          connections: record.orderedConnectionCodes.join(' → '),
          effective: formatRange(record.effectiveFrom, record.effectiveTo),
        },
      }));
    case 'SERVICE_AREA_VERSIONS_LISTED':
      return body.versions.map((record) => ({
        key: `service-area:${record.code}@${record.version}`,
        values: {
          code: record.code,
          version: String(record.version),
          includedRegions: record.includedRegions.join('、'),
          excludedRegions: record.excludedRegions.join('、') || '—',
          effective: formatRange(record.effectiveFrom, record.effectiveTo),
        },
      }));
    case 'SERVICE_CALENDAR_VERSIONS_LISTED':
      return body.versions.map((record) => ({
        key: `service-calendar:${record.code}@${record.version}`,
        values: {
          code: record.code,
          version: String(record.version),
          timezone: record.timezone,
          serviceDays: record.serviceDays,
          exceptionDates: record.exceptionDates,
          effective: formatRange(record.effectiveFrom, record.effectiveTo),
        },
      }));
    case 'CALENDAR_BINDING_VERSIONS_LISTED':
      return body.versions.map((record) => ({
        key: `calendar-binding:${record.targetKind}:${record.targetCode}@${record.version}`,
        values: {
          targetKind: targetKindLabel(record.targetKind),
          targetCode: record.targetCode,
          version: String(record.version),
          calendar: `${record.calendarCode}@${record.calendarVersion}`,
          effective: formatRange(record.effectiveFrom, record.effectiveTo),
        },
      }));
    case 'AVAILABILITY_ADJUSTMENT_VERSIONS_LISTED':
      return body.versions.map((record) => ({
        key: `availability:${record.targetKind}:${record.targetCode}@${record.version}`,
        values: {
          targetKind: targetKindLabel(record.targetKind),
          targetCode: record.targetCode,
          version: String(record.version),
          adjustmentKind: adjustmentKindLabel(record.adjustmentKind),
          window: `${formatInstant(record.windowStart)} → ${
            record.windowEnd ? formatInstant(record.windowEnd) : '持续有效'
          }`,
          scopeReference: record.scopeReference,
          reasonReference: record.reasonReference,
        },
      }));
  }
}

export function NetworkCatalogPage() {
  const [familyId, setFamilyId] = useState<NetworkCatalogFamily>('node');
  const [keyword, setKeyword] = useState('');
  const [reloadKey, setReloadKey] = useState(0);
  const [loaded, setLoaded] = useState<{
    family: NetworkCatalogFamily;
    answer: ApiResult<NetworkCatalogResponseBody>;
  } | null>(null);
  const family = families.find((candidate) => candidate.id === familyId) ?? families[0];

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
  const rows = answer?.kind === 'outcome' ? rowsOf(answer.body) : [];
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
      description={`${info.owner}——逐族查阅版本原文，不选版、不折叠为路由判断`}
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
              {networkFamilyLabel(candidate.id)}
            </button>
          ))}
        </>
      }
      columns={family.columns}
      filterSummary={`${networkFamilyLabel(familyId)} ${rows.length} 个版本`}
      rows={visibleRows}
      rowKey={(row) => row.key}
      viewState={catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: `GET /network-catalog?family=${familyId}`,
        emptyTitle: `当前租户尚无${networkFamilyLabel(familyId)}版本`,
        emptyDescription: '读取入口已配置，但该目录族为空；页面不会借其他族的数据补位。',
      })}
    />
  );
}
