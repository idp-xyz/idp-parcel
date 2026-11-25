import type { ReturnReceiptRecord } from './api';
import { returnReceiptOutcomeLabels } from './presentation';
import { returnDestinationLabel, returnReceiptStatus } from './return-flow';
import { WmsBadge } from './wms-ui';

/**
 * 一笔到仓记录此刻在回库处置上的样子：记了去向的写去向，入了暂存没记的写待处置，转了待认领的写转待认领。
 * 列表的一列与两张单据页都用它，三处说同一句话。
 */
export function ReceiptStatusBadge({ receipt }: { receipt: Pick<ReturnReceiptRecord, 'outcome' | 'disposition'> }) {
  if (receipt.disposition) {
    const { destination } = receipt.disposition;
    return <WmsBadge code={destination} label={returnDestinationLabel(destination)} />;
  }
  const status = returnReceiptStatus(receipt);
  // 读口以后多答一种到仓结果时照它的原词写，不替它猜。
  if (status === 'OTHER') return <WmsBadge label={returnReceiptOutcomeLabels[receipt.outcome] ?? receipt.outcome} tone="neutral" />;
  return <WmsBadge code={status} />;
}
