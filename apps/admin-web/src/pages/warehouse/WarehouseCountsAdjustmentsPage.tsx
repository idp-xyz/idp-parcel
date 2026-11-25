import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { AdjustmentDocument } from './AdjustmentDocument';
import { AdjustmentList } from './AdjustmentList';
import { CountDocument } from './CountDocument';
import { CountList } from './CountList';
import { countsAdjustmentsModule, countsAdjustmentsTarget } from './counts-adjustments-address';
import { WmsDocumentPage } from './document-page';
import { JobDiscrepancyDocument } from './JobDiscrepancyDocument';
import { JobDiscrepancyList } from './JobDiscrepancyList';
import { useDocumentParts, useReturnToList } from './use-document';
import { useQueueFilters } from './use-queue-filters';

/**
 * 盘点与调整：盘点、库存调整与作业差异三种单据各占自己的标签，同一个模块按地址里单据键的第一段（种类词）分派；
 * 没有单据键就是列表，三张列表各是一个页签。
 */
export function WarehouseCountsAdjustmentsPage() {
  const parts = useDocumentParts();
  if (parts === null) return <CountsAdjustmentsHome />;
  const target = countsAdjustmentsTarget(parts);
  switch (target.kind) {
    case 'count':
      return <CountDocument countId={target.id} />;
    case 'adjustment':
      return <AdjustmentDocument adjustmentId={target.id} />;
    case 'discrepancy':
      return <JobDiscrepancyDocument discrepancyId={target.id} />;
    default:
      return <UnknownDocument word={target.word} />;
  }
}

function CountsAdjustmentsHome() {
  const [tab, setTab] = useState('counts');
  // 页签切走时那一张列表会卸载，筛选态放在这里，换回来还在。
  const countFilters = useQueueFilters('ALL');
  const adjustmentFilters = useQueueFilters('PENDING');
  const discrepancyFilters = useQueueFilters('PENDING');

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="counts">盘点任务</TabsTrigger>
          <TabsTrigger value="adjustments">库存调整</TabsTrigger>
          <TabsTrigger value="job-diffs">作业差异审批</TabsTrigger>
        </TabsList>

        <TabsContent value="counts" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <CountList filters={countFilters} />
        </TabsContent>
        <TabsContent value="adjustments" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <AdjustmentList filters={adjustmentFilters} />
        </TabsContent>
        <TabsContent value="job-diffs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <JobDiscrepancyList filters={discrepancyFilters} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

/** 地址里的种类词三种都不是：说清哪三种，不猜它想开哪一张。 */
function UnknownDocument({ word }: { word: string }) {
  const back = useReturnToList(countsAdjustmentsModule);
  return (
    <WmsDocumentPage
      kind="单据"
      title={word}
      error="地址里的种类词不认识：盘点与调整页的单据地址以「盘点」「调整」或「作业差异」开头，后面跟单据号。"
      onBack={back}
    >
      {null}
    </WmsDocumentPage>
  );
}
