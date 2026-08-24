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

/** 泛型参数 K 是决定 id 的并集；不带参数时即扩展前的二元形状，既有调用方零改动。 */
export interface ReviewDecision<K extends string = ReviewDecisionKind> {
  itemId: string;
  decision: K;
  /** 复核理由。模板保证非空白——理由必填是复核留痕的门槛，不是可选校验。 */
  reason: string;
}

/**
 * 决定集中的一个决定。id 原样进入 ReviewDecision.decision 回传；
 * 决定的业务语义（哪个建案、哪个终止）归调用方，模板只管呈现与理由门槛。
 */
export interface ReviewDecisionOption<K extends string = string> {
  id: K;
  label: string;
  /** 按钮视觉档位；不传按中性 secondary 呈现。哪个决定算否定是业务判断，归调用方。 */
  variant?: 'default' | 'secondary' | 'ghost' | 'outline' | 'danger';
  /** 决定按钮图标；与默认二元决定对齐时建议 size 13。 */
  icon?: ReactNode;
  /**
   * 依赖未就绪时置 true：决定如实呈现为不可用，而不是从决定集里消失——
   * 复核角色需要看到完整决定集，才能分清「不能这样选」是环境所限还是业务如此。
   */
  disabled?: boolean;
  /** 不可用的依赖说明，随不可用决定一起展示；仅 disabled 时生效。 */
  disabledReason?: string;
}

export interface ReviewFlowTemplateProps<K extends string = ReviewDecisionKind> {
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
   * 决定集。不传时回退为二元「批准/驳回」（approveLabel/rejectLabel 生效），
   * 与既有调用方的历史契约一致；传入后 approveLabel/rejectLabel 不参与渲染。
   */
  decisions?: ReviewDecisionOption<K>[];
  /**
   * 提交决定。任何决定都必须带非空白理由后才会触发；
   * 模板不区分各决定的理由门槛——单边放宽是业务决定，出现时再开 prop。
   */
  onDecide: (decision: ReviewDecision<K>) => void;
  /** 提交中置 true：禁用动作按钮，防止重复提交同一决定。 */
  decisionPending?: boolean;
  /** 回退二元决定的标签；仅在未传 decisions 时生效。 */
  approveLabel?: string;
  rejectLabel?: string;
  /** 当前选中项的审计留痕；不传则不渲染该区。 */
  auditTrail?: AuditEntry[];
  auditTitle?: string;
  viewState: TemplateViewState;
  stateOverride?: StateSlotProps['override'];
}

// 复核工作流模板：左侧队列 → 右侧详情 → 决定动作（可配置决定集，理由必填）→ 审计留痕。
// 理由校验放模板内：空白理由直接拦下并提示，不把半成品决定交给 onDecide——
// 复核动作一旦提交就进审计，输入门槛必须在提交前站住。
export function ReviewFlowTemplate<K extends string = ReviewDecisionKind>({
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
  decisions,
  onDecide,
  decisionPending = false,
  approveLabel = '批准',
  rejectLabel = '驳回',
  auditTrail,
  auditTitle = '审计留痕',
  viewState,
  stateOverride,
}: ReviewFlowTemplateProps<K>) {
  const [reason, setReason] = useState('');
  const [reasonError, setReasonError] = useState<string | null>(null);

  const selected = queue.find((item) => item.id === selectedId) ?? null;

  // 未传 decisions 时回退为扩展前硬编码的二元动作区，id 固定 'approve'/'reject'。
  // 内部按 string 处理决定 id：回传边界处收窄回 K——id 只可能出自本决定集，
  // 而决定集要么是调用方传入的 ReviewDecisionOption<K>[]，要么是 K 取默认值时的二元回退。
  const decisionOptions: ReviewDecisionOption[] = decisions ?? [
    { id: 'approve', label: approveLabel, variant: 'default', icon: <CheckCircle2 size={13} /> },
    { id: 'reject', label: rejectLabel, variant: 'danger', icon: <XCircle size={13} /> },
  ];

  // 不可用决定的依赖说明；排除掉没写原因的（没有可展示内容）。
  const unavailableNotes = decisionOptions.filter(
    (option) => option.disabled && option.disabledReason,
  );

  const handleSelect = (id: string) => {
    // 切换对象即作废已输入理由：理由是针对单个对象的判断，不该跨对象残留。
    if (id !== selectedId) {
      setReason('');
      setReasonError(null);
    }
    onSelect(id);
  };

  const submit = (decisionId: string) => {
    if (!selected) return;
    const trimmed = reason.trim();
    if (trimmed === '') {
      setReasonError('必须填写复核理由后才能提交决定。');
      return;
    }
    setReasonError(null);
    onDecide({ itemId: selected.id, decision: decisionId as K, reason: trimmed });
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
                    <div className="mt-3 flex flex-wrap items-center gap-2">
                      {decisionOptions.map((option) => (
                        <Button
                          key={option.id}
                          variant={option.variant ?? 'secondary'}
                          disabled={decisionPending || option.disabled}
                          onClick={() => submit(option.id)}
                        >
                          {option.icon}
                          {option.label}
                        </Button>
                      ))}
                      {decisionPending && (
                        <span className="text-[11px] text-idpxyz-textMuted">决定提交中…</span>
                      )}
                    </div>
                    {unavailableNotes.length > 0 && (
                      <div className="mt-2 flex flex-col gap-0.5">
                        {unavailableNotes.map((option) => (
                          <p key={option.id} className="text-[11px] text-idpxyz-textMuted">
                            「{option.label}」暂不可用：{option.disabledReason}
                          </p>
                        ))}
                      </div>
                    )}
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
