// 「技术详情」折叠（票 party-commercial-uiux/18）：把服务端状态码、错误码、接口路径、场景出处这类只有排查时才用的字收进一格，
// 默认收起。正文先说业务上怎么回事、下一步做什么；要报障或对日志的人展开就有原名，不丢。
//
// 自成一个文件、只引 react，理由同 section-error.tsx：node:test 里要用 renderToStaticMarkup 钉它，引 @idpxyz/* 的组件钉不到。
// 折起来的内容仍在标记里（<details> 不渲染成按需挂载），所以按码检索、测试按字断言都不受影响。

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

export function TechnicalDetails({ rows, className }: { rows: readonly TechnicalRow[]; className?: string }) {
  const shown = rows.filter((row) => row.value.trim() !== '');
  if (shown.length === 0) return null;
  return (
    <details className={`mt-1 text-[11px] ${className ?? ''}`.trim()}>
      <summary className="cursor-pointer select-none opacity-80">技术详情</summary>
      <dl className="mt-1 space-y-0.5 text-left">
        {shown.map((row, index) => (
          <div key={`${row.label}-${index}`} className="flex gap-2">
            <dt className="shrink-0 opacity-80">{row.label}</dt>
            <dd className="font-mono break-all">{row.value}</dd>
          </div>
        ))}
      </dl>
    </details>
  );
}
