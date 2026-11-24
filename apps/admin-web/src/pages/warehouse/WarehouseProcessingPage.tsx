import { useState } from 'react';
import { Lock, Plus, Snowflake, Tag, Wrench } from 'lucide-react';
import { Button, Card, CardContent, CardHeader, CardTitle, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { listInternalJobs, type InternalJobRecord } from './api';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-processing'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const jobColumns: ListColumn<InternalJobRecord>[] = [
  { id: 'fact', header: '事实号', className: mono, render: (row) => row.factId },
  { id: 'kind', header: '作业种类', render: (row) => row.kind },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouse },
  { id: 'lines', header: '明细行', className: mono, render: (row) => String(row.lines.length) },
  { id: 'seq', header: '设备序号', className: mono, render: (row) => `#${row.deviceSequence}` },
  { id: 'occurred', header: '发生时刻', className: mono, render: (row) => formatInstant(row.occurredAt) },
];

/** 加工与换标页：整合加工组装/拆解指令、换标指令、冻结解冻与次品转良品，以及库内作业台账。 */
export function WarehouseProcessingPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(internalOpsActionSpecs);
  const [tab, setTab] = useState('instructions');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);

  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listInternalJobs);
  const jobs = answer?.kind === 'outcome' ? answer.body.jobs : [];

  const openAction = (id: string) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const jobInspector = (row: InternalJobRecord): InspectorContent => ({
    title: row.factId,
    subtitle: row.kind,
    sections: [
      {
        kind: 'summary',
        fields: [
          { label: '仓库', value: row.warehouse, mono: true },
          { label: '行数', value: String(row.lines.length), mono: true },
          { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
          { label: '收到时刻', value: formatInstant(row.receivedAt), mono: true },
        ],
      },
      { kind: 'audit', fields: [{ label: '设备序号', value: `#${row.deviceSequence}`, mono: true }] },
    ],
  });

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="instructions">加工与换标指令</TabsTrigger>
          <TabsTrigger value="freeze">冻结与次品恢复</TabsTrigger>
          <TabsTrigger value="jobs">库内作业台账</TabsTrigger>
        </TabsList>

        <TabsContent value="instructions" className="flex-1 overflow-y-auto p-6 data-[state=inactive]:hidden">
          <div className="max-w-[1040px] mx-auto space-y-6">
            <header className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h1 className="type-title font-bold text-idpxyz-textBright">加工与换标指令</h1>
                <p className="mt-1 type-body text-idpxyz-textMuted">
                  由运营在此下发或作废加工与换标指令；仓内现场执行请在「仓内作业台 → 库内与盘点」完成。
                </p>
              </div>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  window.location.hash = '#/warehouse-workstation';
                }}
              >
                去作业台执行加工 / 换标
              </Button>
            </header>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <Wrench className="h-4 w-4" />
                    <CardTitle className="type-section">组装 / 拆解加工指令</CardTitle>
                  </div>
                  <p className="type-body text-idpxyz-textMuted">
                    按组合商品定义组装（ASSEMBLE）或拆解（DISASSEMBLE）。现场执行入库时拒绝停用库位。
                  </p>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  {targetMap.has('processing-instruction') ? (
                    <Button type="button" className={primaryActionClass} onClick={() => openAction('processing-instruction')}>
                      <Plus className="h-3.5 w-3.5 mr-1" />
                      下发加工指令
                    </Button>
                  ) : null}
                  {targetMap.has('processing-void') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('processing-void')}>
                      作废加工指令
                    </Button>
                  ) : null}
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <Tag className="h-4 w-4" />
                    <CardTitle className="type-section">换标指令</CardTitle>
                  </div>
                  <p className="type-body text-idpxyz-textMuted">
                    将库内指定商品改贴新别名标签（可指定库位、批号与旧别名）。执行后新别名即生效并入账。
                  </p>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  {targetMap.has('relabel-instruction') ? (
                    <Button type="button" className={primaryActionClass} onClick={() => openAction('relabel-instruction')}>
                      <Plus className="h-3.5 w-3.5 mr-1" />
                      下发换标指令
                    </Button>
                  ) : null}
                  {targetMap.has('relabel-void') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('relabel-void')}>
                      作废换标指令
                    </Button>
                  ) : null}
                </CardContent>
              </Card>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="freeze" className="flex-1 overflow-y-auto p-6 data-[state=inactive]:hidden">
          <div className="max-w-[1040px] mx-auto space-y-6">
            <header>
              <h1 className="type-title font-bold text-idpxyz-textBright">库存冻结与状态恢复</h1>
              <p className="mt-1 type-body text-idpxyz-textMuted">
                也可以在「实时库存」直接对单条库存格或整箱点「冻结 / 次转良」，自动带出仓库、库位与商品编码。
              </p>
            </header>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <Snowflake className="h-4 w-4" />
                    <CardTitle className="type-section">冻结与解冻</CardTitle>
                  </div>
                  <p className="type-body text-idpxyz-textMuted">冻结商品格或整箱使其不可被出库分配；按冻结号随时解冻。</p>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  {targetMap.has('stock-freeze') ? (
                    <Button type="button" className={primaryActionClass} onClick={() => openAction('stock-freeze')}>
                      冻结库存
                    </Button>
                  ) : null}
                  {targetMap.has('stock-unfreeze') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('stock-unfreeze')}>
                      解冻库存
                    </Button>
                  ) : null}
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <Lock className="h-4 w-4" />
                    <CardTitle className="type-section">次品转良品</CardTitle>
                  </div>
                  <p className="type-body text-idpxyz-textMuted">经复检确认可恢复销售的次品库存，在此登记恢复为良品。</p>
                </CardHeader>
                <CardContent>
                  {targetMap.has('condition-restore') ? (
                    <Button type="button" className={primaryActionClass} onClick={() => openAction('condition-restore')}>
                      登记次品转良品
                    </Button>
                  ) : (
                    <p className="type-caption text-idpxyz-textMuted">需持有次品转良品授予。</p>
                  )}
                </CardContent>
              </Card>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="jobs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<InternalJobRecord>
            moduleId="warehouse-processing"
            title="库内作业台账"
            description="一线在作业台提交的上架、移库、整箱拆箱、状态调整与报短少事实记录。"
            search={{ value: keyword, onChange: setKeyword, placeholder: '搜事实号、仓库或作业种类' }}
            columns={jobColumns}
            rows={jobs}
            rowKey={(row) => row.factId}
            inspector={jobInspector}
            pagination={pagination}
            emptyRowsNote="当前条件下没有匹配的库内作业"
            viewState={catalogueViewState(answer, jobs.length, retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-internal-jobs',
              emptyTitle: '还没有库内作业记录',
              emptyDescription: '在「仓内作业台」提交库内作业后，记录会自动出现在此台账。',
              narrowed: settled.trim() !== '',
            })}
          />
        </TabsContent>
      </Tabs>

      <ActionDrawer
        moduleId="warehouse-processing"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => retry()}
      />
    </div>
  );
}
