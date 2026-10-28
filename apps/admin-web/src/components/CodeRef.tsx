import { useEffect, useRef, useState, type MouseEvent } from 'react';

// 编码引用格：等宽、截断、悬停给全文、点按复制。各种 ID、依据引用、版本号在界面上都长这样——它们从主列退进技术详情
// 之后仍要能一眼认出、一下复制。原先这件长在 pages/party/detail-primitives.tsx 的 RefChip 里，抬到这里全站共用，
// 那边的 RefChip 只剩一层包装。
//
// 复制成功在件内自己说一声「已复制」而不走 toast：技术详情里一行一个，toast 会在人连着复制几格时叠成一摞；要 toast 的
// 调用方（参与方页头部的引用签）经 onCopy 接自己的反馈。只引 react，node:test 里钉得到。
//
// 点击不冒泡：它常落在工作台的行里，冒上去会把行选中或收起。

export const COPIED_NOTE = '已复制';
export const COPY_HINT = '点击复制';

export interface CodeRefProps {
  value: string;
  /** 值前面的短标签（如「依据」），没有就只显值。 */
  label?: string;
  /** 悬停说明里的名字；缺省取 label，再缺省就只显值。 */
  title?: string;
  /** 折行而不截断——技术详情的列表里值要看全，表格列里要截断。 */
  wrap?: boolean;
  className?: string;
  /** 接管复制与反馈（如走 toast）；不给就由本件写剪贴板并在件内提示。 */
  onCopy?: (title: string, value: string) => void;
}

export function codeRefTitle(name: string | undefined, value: string): string {
  return name ? `${name}：${value}（${COPY_HINT}）` : `${value}（${COPY_HINT}）`;
}

export function CodeRef({ value, label, title, wrap = false, className, onCopy }: CodeRefProps) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<number | undefined>(undefined);
  useEffect(() => () => window.clearTimeout(timer.current), []);

  const name = title ?? label;
  const handleClick = (event: MouseEvent<HTMLButtonElement>) => {
    event.stopPropagation();
    if (onCopy) {
      onCopy(name ?? '编码', value);
      return;
    }
    if (typeof navigator === 'undefined' || !navigator.clipboard) return;
    void navigator.clipboard.writeText(value).then(
      () => {
        setCopied(true);
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => setCopied(false), 1500);
      },
      () => undefined,
    );
  };

  return (
    <button
      type="button"
      title={codeRefTitle(name, value)}
      onClick={handleClick}
      className={`inline-flex max-w-full items-center gap-1 rounded border border-idpxyz-border bg-idpxyz-sidebar px-1.5 py-0.5 text-left text-[12px] hover:border-idpxyz-accent ${className ?? ''}`.trim()}
    >
      {label ? <span className="shrink-0 whitespace-nowrap text-idpxyz-breadcrumb">{label}</span> : null}
      <span className={`min-w-0 font-mono text-idpxyz-text ${wrap ? 'break-all' : 'truncate'}`}>{value}</span>
      {copied ? (
        <span role="status" className="shrink-0 whitespace-nowrap text-idpxyz-breadcrumb">
          {COPIED_NOTE}
        </span>
      ) : null}
    </button>
  );
}
