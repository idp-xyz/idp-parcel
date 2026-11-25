import { useState } from 'react';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WmsDocumentPage } from './document-page';
import { FreezeDocument } from './FreezeDocument';
import { FreezeList } from './FreezeList';
import { InstructionDocument } from './InstructionDocument';
import { InstructionList } from './InstructionList';
import { InternalJobDocument } from './InternalJobDocument';
import { InternalJobList } from './InternalJobList';
import { processingModule, processingTarget } from './processing-address';
import { useDocumentParts, useReturnToList } from './use-document';
import { useQueueFilters } from './use-queue-filters';

/**
 * 加工、换标、冻结与库内作业：指令、冻结单与库内作业三种单据各占自己的标签，同一个模块按地址里单据键的第一段（种类词）分派；
 * 没有单据键就是列表，三张列表各是一个页签。指令和冻结是台账，执行仍去作业台。
 */
export function WarehouseProcessingPage() {
  const parts = useDocumentParts();
  if (parts === null) return <ProcessingHome />;
  const target = processingTarget(parts);
  switch (target.kind) {
    case 'instruction':
      return <InstructionDocument instructionId={target.id} />;
    case 'freeze':
      return <FreezeDocument freezeId={target.id} />;
    case 'internalJob':
      return <InternalJobDocument factId={target.id} />;
    default:
      return <UnknownDocument word={target.word} />;
  }
}

function ProcessingHome() {
  const [tab, setTab] = useState('instructions');
  // 页签切走时那一张列表会卸载，筛选态放在这里，换回来还在。
  const instructionFilters = useQueueFilters('PENDING');
  const freezeFilters = useQueueFilters('FROZEN');
  // 库内作业没有状态，这里的状态位不用，只用检索词、仓、种类与排序。
  const jobFilters = useQueueFilters('ALL');

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="instructions">加工与换标指令</TabsTrigger>
          <TabsTrigger value="freeze">冻结与解冻</TabsTrigger>
          <TabsTrigger value="jobs">库内作业台账</TabsTrigger>
        </TabsList>

        <TabsContent value="instructions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <InstructionList filters={instructionFilters} />
        </TabsContent>
        <TabsContent value="freeze" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <FreezeList filters={freezeFilters} />
        </TabsContent>
        <TabsContent value="jobs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <InternalJobList filters={jobFilters} />
        </TabsContent>
      </Tabs>
    </div>
  );
}

/** 地址里的种类词三种都不是：说清哪三种，不猜它想开哪一张。 */
function UnknownDocument({ word }: { word: string }) {
  const back = useReturnToList(processingModule);
  return (
    <WmsDocumentPage
      kind="单据"
      title={word}
      error="地址里的种类词不认识：加工与换标页的单据地址以「指令」「冻结」或「库内作业」开头，后面跟单据号。"
      onBack={back}
    >
      {null}
    </WmsDocumentPage>
  );
}
