import { Button } from '@idpxyz/ui-primitives';
import { useCopyToClipboard } from './detail-primitives';

/**
 * 详情栏底部的「技术详情」折叠区：标识、修订号、依据引用、租户这类只有排查或对日志时才用的字收在这里，默认收起。
 * 正文只讲业务（名称、角色、状态、有效期），这些字不再上列表主列与详情头部——读的人是运营，不是接口的调用方。
 *
 * 本区先用这份局部实现；地基作者的共享件（`src/vocabulary` 下同名同接口）落 main 后，各处只换导入路径。接口刻意只收
 * `{label, value}` 字符串对，与 components/states 里那份排查用的同形，换件时不改调用处。
 */
export interface TechnicalRow {
  label: string;
  value: string | undefined;
}

export function TechnicalDetails({ rows, className }: { rows: readonly TechnicalRow[]; className?: string }) {
  const copy = useCopyToClipboard();
  // 值为空的行不摆：可缺的格（停用依据、后继）缺席时不列一个「—」出来占位。
  const shown = rows.filter((row): row is { label: string; value: string } => (row.value ?? '').trim() !== '');
  if (shown.length === 0) return null;
  return (
    <details className={`rounded border border-idpxyz-border ${className ?? ''}`.trim()}>
      <summary className="cursor-pointer select-none px-3 py-2 text-[12px] text-idpxyz-breadcrumb">技术详情</summary>
      <dl className="border-t border-idpxyz-border px-3 py-1">
        {shown.map((row) => (
          <div key={row.label} className="flex items-start justify-between gap-2 border-b border-idpxyz-border py-1.5 last:border-b-0">
            <div className="min-w-0">
              <dt className="text-[11px] text-idpxyz-breadcrumb">{row.label}</dt>
              <dd className="break-all font-mono text-[12px] text-idpxyz-text">{row.value}</dd>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="shrink-0 text-[11px] text-idpxyz-breadcrumb"
              onClick={() => copy(row.label, row.value)}
            >
              复制
            </Button>
          </div>
        ))}
      </dl>
    </details>
  );
}
