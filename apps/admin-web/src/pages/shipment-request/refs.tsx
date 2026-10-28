import { useToast } from '@idpxyz/ui-theme-runtime';
import { CodeRef } from '../../components/CodeRef';
import { parcelNoOf, shipmentNoOf } from './ref-no';

// 号码签:短写法上屏、整串随复制。纯函数在 ref-no.ts,这里只有 React。
// 签本身是全站共用的 CodeRef;这里只多做一件事——显示去前缀的短号,复制时交整串(查日志、对协议都用整串),
// 所以复制经 onCopy 接管、反馈走 toast。

/** 复制到剪贴板,成败都以 toast 反馈;不支持 clipboard 的上下文(非 https)如实说,不静默。 */
export function useCopyRef() {
  const { addToast } = useToast();
  return (label: string, value: string) => {
    if (typeof navigator === 'undefined' || !navigator.clipboard) {
      addToast({ type: 'warning', title: '无法复制', message: '当前页面不支持剪贴板(需 https 或 localhost)。' });
      return;
    }
    void navigator.clipboard.writeText(value).then(
      () => addToast({ type: 'success', title: '已复制', message: `${label}:${value}` }),
      (error: unknown) =>
        addToast({ type: 'error', title: '复制失败', message: error instanceof Error ? error.message : String(error) }),
    );
  };
}

function RefNo({ value, full, label, className }: { value: string; full: string; label: string; className?: string }) {
  const copy = useCopyRef();
  return <CodeRef value={value} title={label} className={className} onCopy={(name) => copy(name, full)} />;
}

/** 包裹号签:短写法上屏,整串随复制。 */
export function ParcelNo({ parcelId, className }: { parcelId: string; className?: string }) {
  return <RefNo value={parcelNoOf(parcelId)} full={parcelId} label="包裹号" className={className} />;
}

/** 运单号签:短写法上屏,整串随复制。 */
export function ShipmentNo({ shipmentRequestId, className }: { shipmentRequestId: string; className?: string }) {
  return <RefNo value={shipmentNoOf(shipmentRequestId)} full={shipmentRequestId} label="运单号" className={className} />;
}
