import { useState } from 'react';
import { ClipboardCheck, PlayCircle, Plus, Scale, ShieldAlert } from 'lucide-react';
import { Button, Card, CardContent, CardHeader, CardTitle, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { ListPageTemplate, type ListColumn } from '../../templates';
import type { InspectorAction, InspectorContent } from '../../templates/inspector';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import { listInternalJobs, type InternalJobRecord } from './api';
import { internalOpsActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { useWarehouseLookups } from './use-catalogues';

const info = moduleInfoById['warehouse-counts-adjustments'];
const mono = 'whitespace-nowrap font-mono type-body leading-4';

const jobColumns: ListColumn<InternalJobRecord>[] = [
  { id: 'fact', header: '作业事实号', className: mono, render: (row) => row.factId },
  { id: 'kind', header: '作业种类', render: (row) => row.kind },
  { id: 'warehouse', header: '仓', className: mono, render: (row) => row.warehouse },
  { id: 'lines', header: '行数', className: mono, render: (row) => String(row.lines.length) },
  {
    id: 'diff',
    header: '异常/短少信号',
    render: (row) => {
      const issues = row.lines.filter((l) => l.shortOnBook > 0 || l.held > 0 || l.cartonMissing).length;
      return issues > 0 ? `${issues} 行带差异信号` : '正常';
    },
  },
  { id: 'occurred', header: '发生时刻', className: mono, render: (row) => formatInstant(row.occurredAt) },
];

/** 盘点与调整页：按「盘点任务 / 库存调整 / 作业差异审批」三签组织，告别大杂烩按钮墙。 */
export function WarehouseCountsAdjustmentsPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(internalOpsActionSpecs);
  const [tab, setTab] = useState('counts');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [keyword, setKeyword] = useState('');
  const settled = useSettledKeyword(keyword);

  const conditions: CatalogueConditions = { q: settled };
  const { answer, pagination, retry } = useCatalogueCursor(conditions, listInternalJobs);
  const jobs = answer?.kind === 'outcome' ? answer.body.jobs : [];

  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  const jobInspector = (row: InternalJobRecord): InspectorContent => {
    const actions: InspectorAction[] = [];
    if (targetMap.has('job-discrepancy-approve')) {
      actions.push({ label: '批准作业差异', onRun: () => openAction('job-discrepancy-approve', { discrepancyId: row.factId }) });
    }
    if (targetMap.has('job-discrepancy-reject')) {
      actions.push({ label: '驳回作业差异', onRun: () => openAction('job-discrepancy-reject', { discrepancyId: row.factId }) });
    }
    return {
      title: row.factId,
      subtitle: row.kind,
      sections: [
        {
          kind: 'summary',
          fields: [
            { label: '仓库', value: row.warehouse, mono: true },
            { label: '作业行数', value: String(row.lines.length), mono: true },
            { label: '发生时刻', value: formatInstant(row.occurredAt), mono: true },
            { label: '设备序号', value: `#${row.deviceSequence}`, mono: true },
          ],
        },
        ...(actions.length > 0 ? [{ kind: 'actions' as const, actions }] : []),
      ],
    };
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="counts">盘点任务</TabsTrigger>
          <TabsTrigger value="adjustments">库存调整</TabsTrigger>
          <TabsTrigger value="job-diffs">作业差异审批</TabsTrigger>
        </TabsList>

        <TabsContent value="counts" className="flex-1 overflow-y-auto p-6 data-[state=inactive]:hidden">
          <div className="max-w-[1040px] mx-auto space-y-6">
            <header className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h1 className="type-title font-bold text-idpxyz-textBright">盘点全流程</h1>
                <p className="mt-1 type-body text-idpxyz-textMuted">
                  创建盘点 → 开始盘点（锁定范围） → 在仓内作业台录入清点数 → 结束盘点出差异 → 审批盘点差异入账。
                </p>
              </div>
              {targetMap.has('count-create') ? (
                <Button type="button" className={primaryActionClass} onClick={() => openAction('count-create')}>
                  <Plus className="h-3.5 w-3.5 mr-1" />
                  新建盘点任务
                </Button>
              ) : null}
            </header>

            <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <PlayCircle className="h-4 w-4" />
                    <span className="font-mono type-caption font-bold">阶段 01</span>
                  </div>
                  <CardTitle className="type-section mt-1">下发与锁定</CardTitle>
                  <p className="type-body text-idpxyz-textMuted">开始盘点对所选范围落盘点锁；发现错单可作废未开始或进行中的任务。</p>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  {targetMap.has('count-start') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('count-start')}>
                      开始盘点（落锁）
                    </Button>
                  ) : null}
                  {targetMap.has('count-void') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('count-void')}>
                      作废盘点
                    </Button>
                  ) : null}
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <ClipboardCheck className="h-4 w-4" />
                    <span className="font-mono type-caption font-bold">阶段 02</span>
                  </div>
                  <CardTitle className="type-section mt-1">现场清点与结束</CardTitle>
                  <p className="type-body text-idpxyz-textMuted">在「仓内作业台」录入清点数，全部清点完成后在此结束盘点并比对账实差异。</p>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      window.location.hash = '#/warehouse-workstation';
                    }}
                  >
                    去作业台录清点数
                  </Button>
                  {targetMap.has('count-complete') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('count-complete')}>
                      结束盘点并出差异
                    </Button>
                  ) : null}
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-2">
                  <div className="flex items-center gap-2 text-idpxyz-accent">
                    <Scale className="h-4 w-4" />
                    <span className="font-mono type-caption font-bold">阶段 03</span>
                  </div>
                  <CardTitle className="type-section mt-1">盘点差异审批</CardTitle>
                  <p className="type-body text-idpxyz-textMuted">按租户库存调整批准职责规则批准或驳回盘点差异；批准后自动过账。</p>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  {targetMap.has('count-approve') ? (
                    <Button type="button" className={primaryActionClass} onClick={() => openAction('count-approve')}>
                      批准盘点差异
                    </Button>
                  ) : null}
                  {targetMap.has('count-reject') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('count-reject')}>
                      驳回盘点差异
                    </Button>
                  ) : null}
                </CardContent>
              </Card>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="adjustments" className="flex-1 overflow-y-auto p-6 data-[state=inactive]:hidden">
          <div className="max-w-[960px] mx-auto space-y-6">
            <header className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h1 className="type-title font-bold text-idpxyz-textBright">库存调整申请与审批</h1>
                <p className="mt-1 type-body text-idpxyz-textMuted">
                  除盘点差异外的日常库存盘盈盘亏调整。调增拒绝已停用库位；审批人校验遵循「仓配规则设置」里的批准职责规则。
                </p>
              </div>
              {targetMap.has('adjustment-submit') ? (
                <Button type="button" className={primaryActionClass} onClick={() => openAction('adjustment-submit')}>
                  <Plus className="h-3.5 w-3.5 mr-1" />
                  提交库存调整
                </Button>
              ) : null}
            </header>

            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="type-section">提交调整申请</CardTitle>
                  <p className="type-body text-idpxyz-textMuted">指定仓库、库位、货主、商品、调整量与原因；提交本身不改库存账。</p>
                </CardHeader>
                <CardContent>
                  {targetMap.has('adjustment-submit') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('adjustment-submit')}>
                      填写调整申请单
                    </Button>
                  ) : (
                    <p className="type-caption text-idpxyz-textMuted">需持有库存调整提交授予。</p>
                  )}
                </CardContent>
              </Card>

              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="type-section">审批与驳回</CardTitle>
                  <p className="type-body text-idpxyz-textMuted">核实调整号与原因后批准过账或驳回；如未配置批准职责规则，服务端会直接答未配置。</p>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  {targetMap.has('adjustment-approve') ? (
                    <Button type="button" className={primaryActionClass} onClick={() => openAction('adjustment-approve')}>
                      批准库存调整
                    </Button>
                  ) : null}
                  {targetMap.has('adjustment-reject') ? (
                    <Button type="button" variant="outline" onClick={() => openAction('adjustment-reject')}>
                      驳回库存调整
                    </Button>
                  ) : null}
                  <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-settings'; }}>
                    检查批准职责规则
                  </Button>
                </CardContent>
              </Card>
            </div>
          </div>
        </TabsContent>

        <TabsContent value="job-diffs" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<InternalJobRecord>
            moduleId="warehouse-counts-adjustments"
            title="作业差异审批"
            description="一线库内移动、形态调整、短少上报等作业产生的账面差异需复核批准或驳回。点右上角或单击行进入审批。"
            headerActions={
              <div className="flex gap-2">
                {targetMap.has('job-discrepancy-approve') ? (
                  <Button type="button" className={primaryActionClass} onClick={() => openAction('job-discrepancy-approve')}>
                    <ShieldAlert className="h-3.5 w-3.5 mr-1" />
                    批准作业差异
                  </Button>
                ) : null}
                {targetMap.has('job-discrepancy-reject') ? (
                  <Button type="button" variant="outline" onClick={() => openAction('job-discrepancy-reject')}>
                    驳回作业差异
                  </Button>
                ) : null}
              </div>
            }
            search={{ value: keyword, onChange: setKeyword, placeholder: '搜事实号、仓库或种类' }}
            columns={jobColumns}
            rows={jobs}
            rowKey={(row) => row.factId}
            inspector={jobInspector}
            pagination={pagination}
            emptyRowsNote="当前条件下没有库内作业记录"
            viewState={catalogueViewState(answer, jobs.length, retry, {
              module: info,
              endpoint: 'GET /warehouse-fulfillment-internal-jobs',
              emptyTitle: '还没有库内作业记录',
              emptyDescription: '在「仓内作业台」完成移库、报短少等作业后，记录会出现在此台账。',
              narrowed: settled.trim() !== '',
            })}
          />
        </TabsContent>
      </Tabs>

      <ActionDrawer
        moduleId="warehouse-counts-adjustments"
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => retry()}
      />
    </div>
  );
}
