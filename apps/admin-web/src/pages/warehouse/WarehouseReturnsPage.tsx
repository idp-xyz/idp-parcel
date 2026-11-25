import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WmsDocumentPage } from './document-page';
import { returnsModule, returnsTarget } from './return-address';
import { ReturnAdviceDocument } from './ReturnAdviceDocument';
import { ReturnAdviceList } from './ReturnAdviceList';
import { ReturnReceiptDocument } from './ReturnReceiptDocument';
import { ReturnReceiptList } from './ReturnReceiptList';
import { useDocumentParts, useReturnToList } from './use-document';
import { useQueueFilters } from './use-queue-filters';
import { useReturnsOverview } from './use-returns-overview';

/**
 * 退货：退货预报与退货到仓两种单据各占自己的标签，同一个模块按地址里单据键的第一段（种类词）分派；没有单据键就是列表，
 * 两张列表各是一个页签。到仓扫描仍在作业台，回库处置在到仓记录上直接做。
 */
export function WarehouseReturnsPage() {
  const parts = useDocumentParts();
  if (parts === null) return <ReturnsHome />;
  const target = returnsTarget(parts);
  switch (target.kind) {
    case 'advice':
      return <ReturnAdviceDocument adviceKey={target.id} />;
    case 'receipt':
      return <ReturnReceiptDocument factId={target.id} />;
    default:
      return <UnknownDocument word={target.word} />;
  }
}

function ReturnsHome() {
  const [tab, setTab] = useState('advices');
  // 两张列表共读一份概览，持有它的是页签外面这一层：切走的那一张列表会卸载，数据与筛选态放在这里，换回来还在、不重取。
  const overview = useReturnsOverview();
  const adviceFilters = useQueueFilters('ALL');
  const receiptFilters = useQueueFilters('AWAITING_DISPOSITION');

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="advices">退货预报</TabsTrigger>
          <TabsTrigger value="receipts">到仓与回库处置</TabsTrigger>
        </TabsList>

        <TabsContent value="advices" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ReturnAdviceList overview={overview} filters={adviceFilters} />
        </TabsContent>
        <TabsContent value="receipts" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ReturnReceiptList overview={overview} filters={receiptFilters} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

/** 地址里的种类词两种都不是：说清哪两种，不猜它想开哪一张。 */
function UnknownDocument({ word }: { word: string }) {
  const back = useReturnToList(returnsModule);
  return (
    <WmsDocumentPage
      kind="单据"
      title={word}
      error="地址里的种类词不认识：退货管理页的单据地址以「退货预报」或「退货到仓」开头，后面跟单据号。"
      onBack={back}
    >
      {null}
    </WmsDocumentPage>
  );
}
