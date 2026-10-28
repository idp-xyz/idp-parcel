import { shortPrincipalRef, shortenEmbeddedPrincipalRefs } from './principal-ref';

/** 主体引用格：显短名，悬停给全文（title 里的全文可由浏览器选中复制）。 */
export function PrincipalRef({ value, className }: { value: string; className?: string }) {
  return (
    <span className={className ?? 'font-mono'} title={value}>
      {shortPrincipalRef(value)}
    </span>
  );
}

/** 文本里嵌着主体引用的格（如「授权类型@发行方#主体」）：显缩短后的文本，悬停给原文。 */
export function PrincipalText({ value, className }: { value: string; className?: string }) {
  return (
    <span className={className} title={value}>
      {shortenEmbeddedPrincipalRefs(value)}
    </span>
  );
}
