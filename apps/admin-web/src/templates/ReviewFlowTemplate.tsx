import { useState, type ReactNode } from 'react';
import { CheckCircle2, XCircle } from 'lucide-react';
import {
  PageHeader,
  PageHeaderContent,
  PageHeaderTitle,
  PageHeaderDescription,
  PageHeaderActions,
} from '@idpxyz/ui-patterns';
import {
  Button,
  Card,
  CardHeader,
  CardTitle,
  CardContent,
  FormField,
  Label,
  FormError,
  Textarea,
  Timeline,
} from '@idpxyz/ui-primitives';
import { StateSlot, type TemplateViewState, type StateSlotProps } from './state-slot';
import type { DetailField, AuditEntry } from './types';

/** 复核队列中的一项。status 收 ReactNode：徽章语义（哪个状态算警示）归调用方。 */
export interface ReviewQueueItem {
  id: string;
  /** 主标识（如申报单号）。 */
  title: string;
  /** 次要信息（如货主客户、目的国）。 */
  subtitle?: string;
  status?: ReactNode;
  /** 已格式化的辅助信息（如进入队列时间）。 */
  meta?: string;
}

export type ReviewDecisionKind = 'approve' | 'reject';

export interface ReviewDecision {
  itemId: string;
  decision: ReviewDecisionKind;
  /** 复核理由。模板保证非空白——理由必填是复核留痕的门槛，不是可选校验。 */
  reason: string;
}

export interface ReviewFlowTemplateProps {
  title: string;
  description?: string;
  headerActions?: ReactNode;
  /** 待复核队列。选中态受控于调用方，便于路由或跨页保持。 */
  queue: ReviewQueueItem[];
  selectedId: string | null;
  onSelect: (id: string) => void;
  /** 队列区标题，默认「待复核队列」。 */
  queueTitle?: string;
  /** 当前选中项的结构化字段。 */
  detailFields?: DetailField[];
  /** 当前选中项的自由内容区（结构化字段盛不下的，如原始报文对照）。 */
  detailExtra?: ReactNode;
  /** 详情区标题，默认「复核详情」。 */
  detailTitle?: string;
  /**
   * 提交决定。批准与驳回都必须带非空白理由后才会触发；
   * 模板不区分两者的理由门槛——单边放宽是业务决定，出现时再开 prop。
   */
  onDecide: (decision: ReviewDecision) => void;
  /** 提交中置 true：禁用动作按钮，防止重复提交同一决定。 */
  decisionPending?: boolean;
  approveLabel?: string;
  rejectLabel?: string;
  /** 当前选中项的审计留痕；不传则不渲染该区。 */
  auditTrail?: AuditEntry[];
  auditTitle?: string;
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
}

// 复核工作流模板：左侧队列 → 右侧详情 → 决定动作（批准/驳回，理由必填）→ 审计留痕。
// 理由校验放模板内：空白理由直接拦下并提示，不把半成品决定交给 onDecide——
// 复核动作一旦提交就进审计，输入门槛必须在提交前站住。
export function ReviewFlowTemplate({
  title,
  description,
  headerActions,
  queue,
  selectedId,
  onSelect,
  queueTitle = '待复核队列',
  detailFields,
  detailExtra,
  detailTitle = '复核详情',
  onDecide,
  decisionPending = false,
  approveLabel = '批准',
  rejectLabel = '驳回',
  auditTrail,
  auditTitle = '审计留痕',
  viewState,
  stateOverride,
}: ReviewFlowTemplateProps) {
  const [reason, setReason] = useState('');
  const [reasonError, setReasonError] = useState<string | null>(null);

  const selected = queue.find((item) => item.id === selectedId) ?? null;

  const handleSelect = (id: string) => {
    // 切换对象即作废已输入理由：理由是针对单个对象的判断，不该跨对象残留。
    if (id !== selectedId) {
      setReason('');
      setReasonError(null);
    }
    onSelect(id);
  };

  const submit = (decision: ReviewDecisionKind) => {
    if (!selected) return;
    const trimmed = reason.trim();
    if (trimmed === '') {
      setReasonError('必须填写复核理由后才能提交决定。');
      return;
    }
    setReasonError(null);
    onDecide({ itemId: selected.id, decision, reason: trimmed });
    setReason('');
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <PageHeader>
        <PageHeaderContent>
          <PageHeaderTitle>{title}</PageHeaderTitle>
          {description && <PageHeaderDescription>{description}</PageHeaderDescription>}
        </PageHeaderContent>
        {headerActions && <PageHeaderActions>{headerActions}</PageHeaderActions>}
      </PageHeader>

      {viewState.kind === 'ready' ? (
        <div className="flex flex-1 overflow-hidden">
          {/* 队列区：固定宽度，独立滚动，选中项高亮。 */}
          <div className="flex w-[300px] shrink-0 flex-col border-r border-idpxyz-border">
            <div className="flex h-8 shrink-0 items-center justify-between border-b border-idpxyz-border px-3">
              <span className="text-[11px] font-medium text-idpxyz-textMuted">{queueTitle}</span>
              <span className="text-[11px] text-idpxyz-textMuted">{queue.length}</span>
            </div>
            <div className="flex-1 overflow-auto">
              {queue.length === 0 ? (
                <p className="px-3 py-4 text-[12px] text-idpxyz-textMuted">队列为空。</p>
              ) : (
                queue.map((item) => (
                  <button
                    key={item.id}
                    type="button"
                    onClick={() => handleSelect(item.id)}
                    className={`block w-full border-b border-idpxyz-border px-3 py-2.5 text-left transition-colors ${
                      item.id === selectedId
                        ? 'bg-idpxyz-activeItem/10 border-l-2 border-l-idpxyz-accent'
                        : 'hover:bg-idpxyz-hover border-l-2 border-l-transparent'
                    }`}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate font-mono text-[12px] text-idpxyz-textBright">
                        {item.title}
                      </span>
                      {item.status}
                    </div>
                    {item.subtitle && (
                      <div className="mt-0.5 truncate text-[11px] text-idpxyz-text">
                        {item.subtitle}
                      </div>
                    )}
                    {item.meta && (
                      <div className="mt-0.5 text-[11px] text-idpxyz-textMuted">{item.meta}</div>
                    )}
                  </button>
                ))
              )}
            </div>
          </div>

          {/* 详情 + 决定 + 审计：随选中项切换。 */}
          <div className="flex-1 overflow-auto">
            {selected ? (
              <div className="mx-auto flex max-w-[820px] flex-col gap-4 p-4">
                <Card>
                  <CardHeader>
                    <CardTitle>{detailTitle}</CardTitle>
                  </CardHeader>
                  <CardContent>
                    {detailFields && detailFields.length > 0 && (
                      <dl className="grid grid-cols-1 gap-x-8 gap-y-2.5 sm:grid-cols-2">
                        {detailFields.map((field) => (
                          <div key={field.label} className="flex gap-3 text-[12px] leading-5">
                            <dt className="w-[96px] shrink-0 text-idpxyz-textMuted">
                              {field.label}
                            </dt>
                            <dd className="min-w-0 flex-1 break-words text-idpxyz-text">
                              {field.value}
                            </dd>
                          </div>
                        ))}
                      </dl>
                    )}
                    {detailExtra && <div className="mt-3">{detailExtra}</div>}
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>复核决定</CardTitle>
                  </CardHeader>
                  <CardContent>
                    <FormField>
                      <Label htmlFor="review-reason" required>
                        复核理由
                      </Label>
                      <Textarea
                        id="review-reason"
                        value={reason}
                        placeholder="写明本次判断的依据；该理由随决定进入审计留痕。"
                        onChange={(e) => {
                          setReason(e.target.value);
                          if (reasonError) setReasonError(null);
                        }}
                        disabled={decisionPending}
                      />
                      {reasonError && <FormError>{reasonError}</FormError>}
                    </FormField>
                    <div className="mt-3 flex items-center gap-2">
                      <Button
                        variant="default"
                        disabled={decisionPending}
                        onClick={() => submit('approve')}
                      >
                        <CheckCircle2 size={13} />
                        {approveLabel}
                      </Button>
                      <Button
                        variant="danger"
                        disabled={decisionPending}
                        onClick={() => submit('reject')}
                      >
                        <XCircle size={13} />
                        {rejectLabel}
                      </Button>
                      {decisionPending && (
                        <span className="text-[11px] text-idpxyz-textMuted">决定提交中…</span>
                      )}
                    </div>
                  </CardContent>
                </Card>

                {auditTrail && (
                  <Card>
                    <CardHeader>
                      <CardTitle>{auditTitle}</CardTitle>
                    </CardHeader>
                    <CardContent>
                      {auditTrail.length === 0 ? (
                        <p className="text-[12px] text-idpxyz-textMuted">暂无审计记录。</p>
                      ) : (
                        <Timeline items={auditTrail} />
                      )}
                    </CardContent>
                  </Card>
                )}
              </div>
            ) : (
              <div className="flex h-full items-center justify-center">
                <p className="text-[12px] text-idpxyz-textMuted">从左侧队列选择一项开始复核。</p>
              </div>
            )}
          </div>
        </div>
      ) : (
        <div className="flex-1 flex items-center justify-center overflow-auto">
          <StateSlot state={viewState} override={stateOverride} />
        </div>
      )}
    </div>
  );
}
