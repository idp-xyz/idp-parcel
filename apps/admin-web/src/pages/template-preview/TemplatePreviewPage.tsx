import { useMemo, useState } from 'react';
import { Ban, FlaskConical, FolderPlus, Link2, Search } from 'lucide-react';
import { useToast } from '@idpxyz/ui-theme-runtime';
import {
  Button,
  Tabs,
  TabsList,
  TabsTrigger,
  TabsContent,
} from '@idpxyz/ui-primitives';
import { StatusBadge, FilterChip } from '@idpxyz/ui-patterns';
import {
  ListPageTemplate,
  DetailPageTemplate,
  ReviewFlowTemplate,
  type ListColumn,
  type TemplateViewState,
  type AuditEntry,
  type ReviewDecision,
  type ReviewDecisionOption,
} from '../../templates';
// demo.ts 故意不进模板桶导出，本预览页是它唯一合法的消费者，
// 按路径显式引入以保持「假数据只从演示入口注入」的单向依赖。
import {
  demoShipmentRows,
  demoDetailFields,
  demoDetailSections,
  demoDetailAuditTrail,
  demoReviewQueue,
  demoReviewDetailFields,
  demoReviewAuditTrail,
  demoTriageSignalQueue,
  demoTriageDetailFields,
  type DemoShipmentRow,
} from '../../templates/demo';

// 四态切换器的档位。ready 之外四档演示 StateSlot 的各个非就绪态；
// 文案在这里给演示值，真实页面的文案由各自业务场景决定。
type PreviewStateKind = TemplateViewState['kind'];

const STATE_OPTIONS: { kind: PreviewStateKind; label: string }[] = [
  { kind: 'ready', label: '就绪' },
  { kind: 'loading', label: '加载中' },
  { kind: 'empty', label: '空态' },
  { kind: 'error', label: '错误' },
  { kind: 'unconfigured', label: '未配置' },
];

// 演示留痕的时间取本机时刻即可——本页全部数据都是隔离合成 S，不进任何台账。
function formatDemoTimestamp(now: Date): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(
    now.getDate(),
  ).padStart(2, '0')} ${String(now.getHours()).padStart(2, '0')}:${String(
    now.getMinutes(),
  ).padStart(2, '0')}:${String(now.getSeconds()).padStart(2, '0')}`;
}

/** 演示决定 id：形状对齐 ExceptionTriagePage 的分诊四结果，仅供本预览页。 */
type DemoTriageDecisionId =
  | 'link-existing-case'
  | 'auto-establish-case'
  | 'route-manual-review'
  | 'no-case';

// 四结果决定集演示：标签取 visibility-exception CONTEXT「异常分诊」原词。
// 「关联既有案件」演示不可用决定的呈现；依赖说明是真实边界（既有案件查询端点未建），
// 不属合成数据，也不用假案件列表撑起选择交互。
const demoTriageDecisions: ReviewDecisionOption<DemoTriageDecisionId>[] = [
  {
    id: 'link-existing-case',
    label: '关联既有案件',
    variant: 'secondary',
    icon: <Link2 size={13} />,
    disabled: true,
    disabledReason: '目标案件选择依赖既有案件查询端点；端点未建，就绪后随接线一并开放。',
  },
  {
    id: 'auto-establish-case',
    label: '自动建立案件',
    variant: 'default',
    icon: <FolderPlus size={13} />,
  },
  {
    id: 'route-manual-review',
    label: '进入人工复核',
    variant: 'secondary',
    icon: <Search size={13} />,
  },
  {
    id: 'no-case',
    label: '不建案',
    variant: 'danger',
    icon: <Ban size={13} />,
  },
];

// 模板预览页：在同一页内实例化三个页面模板，供视觉与交互验收。
// 本页不发任何请求——所有数据来自 demo.ts 的隔离合成 S 假数据。
export function TemplatePreviewPage() {
  const { addToast } = useToast();
  const [stateKind, setStateKind] = useState<PreviewStateKind>('ready');

  // 列表模板的演示交互状态：真实过滤与分页，让搜索框与页码可操作。
  const [searchTerm, setSearchTerm] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // 复核模板的演示交互状态：决定后把留痕追加进本地审计列表，
  // 演示「决定动作 → 审计留痕」的闭环；不落任何持久化。
  const [selectedReviewId, setSelectedReviewId] = useState<string | null>(
    demoReviewQueue[0]?.id ?? null,
  );
  const [reviewAudit, setReviewAudit] = useState<AuditEntry[]>(demoReviewAuditTrail);

  // 决定集演示档位：binary 走「不传 decisions」的二元回退（既有调用方的兼容路径），
  // configured 走分诊四结果决定集（含不可用决定的依赖说明）。两档各自持有选中与留痕。
  const [reviewDecisionMode, setReviewDecisionMode] = useState<'binary' | 'configured'>('binary');
  const [selectedTriageId, setSelectedTriageId] = useState<string | null>(
    demoTriageSignalQueue[0]?.id ?? null,
  );
  const [triageAudit, setTriageAudit] = useState<AuditEntry[]>([]);

  const viewState: TemplateViewState = useMemo(() => {
    switch (stateKind) {
      case 'ready':
        return { kind: 'ready' };
      case 'loading':
        return { kind: 'loading' };
      case 'empty':
        return { kind: 'empty', description: '演示空态：当前筛选下没有记录。' };
      case 'error':
        return {
          kind: 'error',
          description: '演示错误态：本次未取得结果。',
          onRetry: () => setStateKind('ready'),
        };
      case 'unconfigured':
        // facts 三段式的演示值：让验收者看到结构化事实区的长相；
        // 真实页面的三段取 moduleInfoById 与各自闸门，不抄这里的演示文案。
        return {
          kind: 'unconfigured',
          description: '演示未配置态：接入渠道尚未配置。',
          facts: {
            owner: '演示上下文（synthetic-demo）',
            source: '本页为模板验收页，无业务出处',
            unlock: '真实页面的放行条件由各自准入闸门决定',
          },
        };
    }
  }, [stateKind]);

  const filteredRows = useMemo(() => {
    const term = searchTerm.trim().toLowerCase();
    if (term === '') return demoShipmentRows;
    return demoShipmentRows.filter(
      (row) =>
        row.requestId.toLowerCase().includes(term) ||
        row.shipperClient.toLowerCase().includes(term),
    );
  }, [searchTerm]);

  const pagedRows = useMemo(
    () => filteredRows.slice((page - 1) * pageSize, page * pageSize),
    [filteredRows, page, pageSize],
  );

  const listColumns: ListColumn<DemoShipmentRow>[] = [
    {
      id: 'requestId',
      header: '申报单号',
      render: (row) => <span className="font-mono text-idpxyz-accent">{row.requestId}</span>,
    },
    { id: 'shipperClient', header: '货主客户', render: (row) => row.shipperClient },
    { id: 'destination', header: '目的国/地区', align: 'center', render: (row) => row.destination },
    {
      id: 'status',
      header: '状态',
      align: 'center',
      render: (row) => <StatusBadge status={row.statusKind}>{row.statusLabel}</StatusBadge>,
    },
    {
      id: 'submittedAt',
      header: '提交时间',
      render: (row) => <span className="text-idpxyz-textMuted">{row.submittedAt}</span>,
    },
  ];

  const handleDecide = (decision: ReviewDecision) => {
    const isApprove = decision.decision === 'approve';
    const now = new Date();
    setReviewAudit((prev) => [
      ...prev,
      {
        id: `syn-preview-decision-${now.getTime()}`,
        title: isApprove ? '批准（演示）' : '驳回（演示）',
        description: `对象 ${decision.itemId}；理由：${decision.reason}`,
        timestamp: formatDemoTimestamp(now),
        variant: isApprove ? 'success' : 'error',
      },
    ]);
    addToast({
      type: isApprove ? 'success' : 'warning',
      title: isApprove ? '已批准（演示）' : '已驳回（演示）',
      message: `决定仅追加到本页审计留痕演示区，不产生任何持久化。`,
    });
  };

  const handleTriageDecide = (decision: ReviewDecision<DemoTriageDecisionId>) => {
    const option = demoTriageDecisions.find((item) => item.id === decision.decision);
    const now = new Date();
    setTriageAudit((prev) => [
      ...prev,
      {
        id: `syn-preview-triage-${now.getTime()}`,
        title: `分诊结果：${option?.label ?? decision.decision}（演示）`,
        description: `对象 ${decision.itemId}；理由：${decision.reason}`,
        timestamp: formatDemoTimestamp(now),
        variant: decision.decision === 'no-case' ? 'warning' : 'success',
      },
    ]);
    addToast({
      type: decision.decision === 'no-case' ? 'warning' : 'success',
      title: '已记录分诊结果（演示）',
      message: `决定仅追加到本页审计留痕演示区，不产生任何持久化。`,
    });
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      {/* 醒目横幅：本页所有数据的证据层级是隔离合成 S，先声明再展示。 */}
      <div className="flex items-center gap-2 border-b border-idpxyz-border bg-idpxyz-hover px-4 py-2 shrink-0">
        <FlaskConical className="h-4 w-4 text-idpxyz-accent shrink-0" aria-hidden />
        <span className="text-[12px] font-semibold text-idpxyz-textBright">
          隔离合成 S 演示，非生产数据
        </span>
        <span className="text-[11px] text-idpxyz-textMuted">
          本页仅用于三个页面模板的视觉与交互验收；全部数据来自 demo.ts 合成假数据（SYN-
          前缀），不发请求、不落台账，不得作任何生产默认。
        </span>
      </div>

      {/* 四态切换器：切换所有模板实例共用的 viewState，演示 StateSlot 各态。 */}
      <div className="flex items-center gap-2 border-b border-idpxyz-border px-4 py-1.5 shrink-0">
        <span className="text-[11px] text-idpxyz-textMuted shrink-0">视图状态</span>
        {STATE_OPTIONS.map((option) => (
          <FilterChip
            key={option.kind}
            active={stateKind === option.kind}
            onClick={() => setStateKind(option.kind)}
          >
            {option.label}
          </FilterChip>
        ))}
      </div>

      <Tabs defaultValue="list" className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="list">列表模板</TabsTrigger>
          <TabsTrigger value="detail">详情模板</TabsTrigger>
          <TabsTrigger value="review">复核工作流模板</TabsTrigger>
        </TabsList>

        {/* 每个 Tab 都撑满剩余高度，让模板自己的滚动与吸顶表头生效。 */}
        <TabsContent value="list" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <ListPageTemplate<DemoShipmentRow>
            title="托运申报单（演示）"
            description="隔离合成 S 假数据"
            headerActions={
              <Button
                size="sm"
                onClick={() =>
                  addToast({ type: 'info', title: '演示动作', message: '页头动作区占位，无实际行为。' })
                }
              >
                演示动作
              </Button>
            }
            search={{
              value: searchTerm,
              onChange: (value) => {
                setSearchTerm(value);
                setPage(1);
              },
              placeholder: '搜索申报单号 / 货主客户…',
            }}
            filterSummary={`共 ${filteredRows.length} 条（合成）`}
            columns={listColumns}
            rows={pagedRows}
            rowKey={(row) => row.requestId}
            onRowClick={(row) =>
              addToast({ type: 'info', title: '行点击（演示）', message: row.requestId })
            }
            pagination={{
              page,
              pageSize,
              total: filteredRows.length,
              onPageChange: setPage,
              onPageSizeChange: (size) => {
                setPageSize(size);
                setPage(1);
              },
              pageSizeOptions: [5, 10, 20],
            }}
            viewState={viewState}
          />
        </TabsContent>

        <TabsContent value="detail" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <DetailPageTemplate
            title="托运申报单详情（演示）"
            identifier="SYN-PS-240819-0002"
            status={<StatusBadge status="warning">待人工复核</StatusBadge>}
            headerActions={
              <Button
                size="sm"
                onClick={() =>
                  addToast({ type: 'info', title: '演示动作', message: '详情页动作区占位，无实际行为。' })
                }
              >
                演示动作
              </Button>
            }
            basicFields={demoDetailFields}
            sections={demoDetailSections.map((section) => ({
              id: section.id,
              title: section.title,
              description: section.description,
              content: <p className="text-[12px] leading-6 text-idpxyz-text">{section.body}</p>,
            }))}
            auditTrail={demoDetailAuditTrail}
            viewState={viewState}
          />
        </TabsContent>

        <TabsContent value="review" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {/* 决定集档位：分别验证「不传 decisions 的二元回退」与「可配置决定集」两条渲染路径。 */}
          <div className="flex items-center gap-2 border-b border-idpxyz-border px-4 py-1.5 shrink-0">
            <span className="text-[11px] text-idpxyz-textMuted shrink-0">决定集</span>
            <FilterChip
              active={reviewDecisionMode === 'binary'}
              onClick={() => setReviewDecisionMode('binary')}
            >
              二元回退（默认）
            </FilterChip>
            <FilterChip
              active={reviewDecisionMode === 'configured'}
              onClick={() => setReviewDecisionMode('configured')}
            >
              分诊四结果（可配置）
            </FilterChip>
          </div>
          {reviewDecisionMode === 'binary' ? (
            <ReviewFlowTemplate
              title="接受前人工复核（演示）"
              description="队列 → 详情 → 决定 → 审计留痕"
              queue={demoReviewQueue.map((entry) => ({
                id: entry.id,
                title: entry.title,
                subtitle: entry.subtitle,
                status: <StatusBadge status={entry.statusKind}>{entry.statusLabel}</StatusBadge>,
                meta: entry.meta,
              }))}
              selectedId={selectedReviewId}
              onSelect={setSelectedReviewId}
              detailFields={demoReviewDetailFields}
              onDecide={handleDecide}
              auditTrail={reviewAudit}
              viewState={viewState}
            />
          ) : (
            <ReviewFlowTemplate
              title="异常分诊（演示）"
              description="可配置决定集：队列 → 详情 → 分诊四结果 → 审计留痕"
              queueTitle="进入分诊的信号"
              queue={demoTriageSignalQueue.map((entry) => ({
                id: entry.id,
                title: entry.title,
                subtitle: entry.subtitle,
                status: <StatusBadge status={entry.statusKind}>{entry.statusLabel}</StatusBadge>,
                meta: entry.meta,
              }))}
              selectedId={selectedTriageId}
              onSelect={setSelectedTriageId}
              detailFields={demoTriageDetailFields}
              decisions={demoTriageDecisions}
              onDecide={handleTriageDecide}
              auditTrail={triageAudit}
              viewState={viewState}
            />
          )}
        </TabsContent>
      </Tabs>
    </div>
  );
}
