import { shortPrincipalRef } from './principal-ref';

/** 主体引用格：显短名，悬停给全文（title 里的全文可由浏览器选中复制）。 */
export function PrincipalRef({ value, className }: { value: string; className?: string }) {
  return (
    <span className={className ?? 'font-mono'} title={value}>
      {shortPrincipalRef(value)}
    </span>
  );
}
