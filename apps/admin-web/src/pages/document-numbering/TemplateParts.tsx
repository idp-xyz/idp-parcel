import { templateReading, templateSegmentsOf } from './rows';

// 把一份格式摆成「字面 + 部件签」：字面等宽原样，部件用中文小签（年、月、日、6 位流水）。模板原文本身进技术详情，
// 主列与右栏只给人读的这一份（ADR-0355 决定十一：内部写法进技术详情）。悬停给整句读法。

export function TemplateParts({ template, className }: { template: string; className?: string }) {
  const segments = templateSegmentsOf(template);
  if (segments.length === 0) return <span className="text-idpxyz-breadcrumb">—</span>;
  return (
    <span className={`inline-flex flex-wrap items-center gap-0.5 ${className ?? ''}`.trim()} title={templateReading(template)}>
      {segments.map((segment, index) =>
        segment.kind === 'literal' ? (
          <span key={index} className="whitespace-pre font-mono text-[12px] text-idpxyz-textBright">
            {segment.text}
          </span>
        ) : (
          <span
            key={index}
            className={`rounded border px-1 text-[11px] leading-4 ${
              segment.known
                ? 'border-idpxyz-border bg-idpxyz-hover text-idpxyz-text'
                : 'border-[color:var(--ds-status-warning-border)] font-mono text-idpxyz-text'
            }`}
          >
            {segment.label}
          </span>
        ),
      )}
    </span>
  );
}
