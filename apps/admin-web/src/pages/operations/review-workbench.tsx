import type { ReactNode } from 'react';
import type { ListColumn, WorkbenchColumn } from '../../templates';
import { toneForLabel } from '../../vocabulary/status';
import { parcelNoOf } from '../shipment-request/ref-no';
import { codeStatus } from '../visibility/exception-list';
import { RecordDetailPanel, techRows, type RowStatus } from '../network/route-workbench';
import { placeLabel, type PlaceNameIndex } from '../network/place-names';

// 网点作业与运输班次两页的册行是「键 + 一袋已转写好的格」（各册行形状互不相同，转写在各自的 rowsOf 里）。这里把两页沿用的
// 列定义接到工作台上：列定义里带 techPairs 的那一格不上表，挪进右栏的「技术详情」；第一格是行首，也是右栏的标题；其余格在右栏
// 里逐格成为一块事实。这样各册只维护一份列定义，表与右栏说的是同一组字。

export interface ReviewRow {
  key: string;
  values: Readonly<Record<string, string>>;
}

/** 一格列定义；techPairs 在场即这是「技术详情」那一格（名称 / 取哪个值），它不上表。 */
export type ReviewColumn<Row extends ReviewRow> = ListColumn<Row> & { techPairs?: readonly (readonly [string, string])[] };

/** 行状态从哪两格来：词（已译好）与码（查色调）；码缺席时按词反查色调。 */
export interface ReviewStatusSpec {
  labelKey: string;
  codeKey?: string;
  scope?: string;
}

export function reviewStatusOf(row: ReviewRow, spec: ReviewStatusSpec | undefined): RowStatus | undefined {
  if (!spec) return undefined;
  const word = row.values[spec.labelKey];
  if (!word) return undefined;
  const code = spec.codeKey ? row.values[spec.codeKey] : undefined;
  return code ? codeStatus(code, spec.scope) : { label: word, tone: toneForLabel(word) };
}

/**
 * 上表的列：去掉技术详情那一格；前两格与状态那一格常驻，其余在右栏打开时收起（右栏里都有）。状态格常驻是因为它就是指标即筛
 * 的那个词，收起了人就看不出筛的是什么。
 */
export function workbenchColumnsOf<Row extends ReviewRow>(
  columns: readonly ReviewColumn<Row>[],
  status?: ReviewStatusSpec,
): WorkbenchColumn<Row>[] {
  return columns
    .filter((column) => column.techPairs === undefined)
    .map((column, index) => ({
      key: column.id,
      header: column.header,
      right: column.align === 'right',
      hideWhenDetailOpen: index >= 2 && column.id !== status?.labelKey,
      render: column.render,
    }));
}

/** 一行的右栏：标题取行首那一格（包裹号去线格式前缀），其余格逐格成事实，技术详情取列定义里点名的那几格。 */
export function ReviewDetail<Row extends ReviewRow>({
  row,
  columns,
  status,
  subtitle,
  actions,
  children,
  onClose,
}: {
  row: Row;
  columns: readonly ReviewColumn<Row>[];
  status?: RowStatus;
  subtitle?: ReactNode;
  actions?: ReactNode;
  children?: ReactNode;
  onClose: () => void;
}) {
  const shown = columns.filter((column) => column.techPairs === undefined);
  const [identity, ...rest] = shown;
  const tech = columns.flatMap((column) => column.techPairs ?? []);
  return (
    <RecordDetailPanel
      title={identity ? parcelNoOf(row.values[identity.id] ?? '—') : '—'}
      status={status}
      subtitle={subtitle ?? (identity ? String(identity.header) : undefined)}
      facts={rest.map((column) => ({
        label: String(column.header),
        // 行上那一格是空串（这一行没有这件事）时说「—」，不留一块空白。
        value: row.values[column.id] === '' ? '—' : column.render(row),
      }))}
      technical={techRows(tech.map(([label, key]) => ({ label, value: row.values[key] })))}
      actions={actions}
      onClose={onClose}
    >
      {children}
    </RecordDetailPanel>
  );
}

/**
 * 把行上几格网点（或口岸）编码换成屏上写法：按此刻生效那一版查名，查不到即编码本身；原编码挪到同名加 `Code` 的那一格，
 * 由列定义的技术详情点名带出。只换上屏的格，行键与检索照旧认编码（检索两格都搜）。
 */
export function withPlaceNames<Row extends ReviewRow>(rows: readonly Row[], keys: readonly string[], names: PlaceNameIndex): Row[] {
  return rows.map((row) => {
    const values: Record<string, string> = { ...row.values };
    for (const key of keys) {
      const code = row.values[key];
      if (code === undefined || code === '') continue;
      values[key] = placeLabel(names, code);
      values[`${key}Code`] = code;
    }
    return { ...row, values };
  });
}
