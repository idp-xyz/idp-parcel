import { CodeRef } from './CodeRef';

// 「技术详情」折叠区（票 party-commercial-uiux/18 立于 components/states，改版第一步抬到这里全站共用）：各种 ID、修订号、
// 来源键、依据引用、状态码、接口路径这类只有排查与对接时才用的字收进一格，默认收起。正文先说业务上怎么回事；要报障、
// 对日志或复制编号的人展开就有原名，不丢。
//
// 每行是「名称 / 值」，值走 CodeRef：等宽、点按复制。折起来的内容仍在标记里（<details> 不按需挂载），按码检索、测试
// 按字断言都不受影响。只引 react 与 CodeRef，node:test 里用 renderToStaticMarkup 钉得到。

export const TECHNICAL_DETAILS_TITLE = '技术详情';

export interface TechnicalRow {
  label: string;
  value: string;
}

/** 一次 HTTP 答复的两行：状态码与问题码；服务端随 4xx 给的理由散文另放正文，不进这里。 */
export function httpRows(status: number, code: string): TechnicalRow[] {
  return [
    { label: 'HTTP 状态', value: String(status) },
    { label: '问题码', value: code },
  ];
}

export interface TechnicalDetailsProps {
  rows: readonly TechnicalRow[];
  className?: string;
  /** 折叠标题；缺省「技术详情」。 */
  title?: string;
}

export function TechnicalDetails({ rows, className, title }: TechnicalDetailsProps) {
  const shown = rows.filter((row) => row.value.trim() !== '');
  if (shown.length === 0) return null;
  return (
    <details className={`text-[12px] ${className ?? ''}`.trim()}>
      <summary className="cursor-pointer select-none text-idpxyz-breadcrumb hover:text-idpxyz-text">
        {title ?? TECHNICAL_DETAILS_TITLE}
      </summary>
      <dl className="mt-1.5 space-y-1 text-left">
        {shown.map((row, index) => (
          <div key={`${row.label}-${index}`} className="flex items-start gap-2">
            <dt className="w-[7em] shrink-0 pt-0.5 text-idpxyz-breadcrumb">{row.label}</dt>
            <dd className="min-w-0 flex-1">
              <CodeRef value={row.value} title={row.label} wrap />
            </dd>
          </div>
        ))}
      </dl>
    </details>
  );
}
