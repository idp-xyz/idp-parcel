import { Copy } from 'lucide-react';
import { useToast } from '@idpxyz/ui-theme-runtime';

import { parcelNoOf, shipmentNoOf } from './ref-no';

// 号码签组件:短写法上屏、整串随复制。纯函数在 ref-no.ts,这里只有 React。

/** 复制到剪贴板，成败都以 toast 反馈；不支持 clipboard 的上下文（非 https）如实说，不静默。 */
export function useCopyRef() {
  const { addToast } = useToast();
  return (label: string, value: string) => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      addToast({ type: 'warning', title: '无法复制', message: '当前页面不支持剪贴板（需 https 或 localhost）。' });
      return;
    }
    void navigator.clipboard.writeText(value).then(
      () => addToast({ type: 'success', title: '已复制', message: `${label}：${value}` }),
      (error: unknown) =>
        addToast({ type: 'error', title: '复制失败', message: error instanceof Error ? error.message : String(error) }),
    );
  };
}

/**
 * 等宽的号码签，带一个复制按钮。`label` 是复制反馈里称它的词（运单号 / 包裹号），`full` 是复制下来的整串，
 * 不给时与显示串相同。按钮上 stopPropagation：号码常摆在可点选的表行里，复制不该顺带选中那一行。
 */
export function RefNo({
  value,
  full,
  label,
  className,
}: {
  value: string;
  full?: string;
  label: string;
  className?: string;
}) {
  const copy = useCopyRef();
  const whole = full ?? value;
  return (
    <span className={`inline-flex items-center gap-1 ${className ?? ''}`.trim()}>
      <span className="font-mono" title={whole}>
        {value}
      </span>
      <button
        type="button"
        className="rounded p-0.5 text-idpxyz-textMuted opacity-60 hover:opacity-100 hover:text-idpxyz-text focus:outline-none focus:opacity-100"
        title={`复制${label}`}
        aria-label={`复制${label} ${whole}`}
        onClick={(event) => {
          event.stopPropagation();
          copy(label, whole);
        }}
      >
        <Copy size={12} />
      </button>
    </span>
  );
}

/** 包裹号签：短写法上屏，整串随复制。 */
export function ParcelNo({ parcelId, className }: { parcelId: string; className?: string }) {
  return <RefNo value={parcelNoOf(parcelId)} full={parcelId} label="包裹号" className={className} />;
}

/** 运单号签：短写法上屏，整串随复制。 */
export function ShipmentNo({ shipmentRequestId, className }: { shipmentRequestId: string; className?: string }) {
  return <RefNo value={shipmentNoOf(shipmentRequestId)} full={shipmentRequestId} label="运单号" className={className} />;
}
