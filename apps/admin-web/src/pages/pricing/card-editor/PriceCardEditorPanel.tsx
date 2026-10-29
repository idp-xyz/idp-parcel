import { useCallback, useEffect, useMemo, useRef, useState, type ReactElement } from 'react';
import { Button, ConfirmDialog } from '@idpxyz/ui-primitives';
import { AlertTriangle, Check } from 'lucide-react';
import { confirmDialogFooter } from '../../../components/confirm-dialog-footer';
import { primaryActionClass } from '../../../components/primary-action';
import type { ApiResult } from '../../catalogue-api';
import {
  previewPriceCardWorkbook,
  submitPriceCardWorkbookDraft,
  type PriceCardDraftSubmissionBody,
  type PriceCardImportReading,
  type PriceCardTemplateSpec,
} from '../api';
import { stepOfField } from '../card-form/fields';
import { emptyState, stepOrder, stepTitles, type CardFormState, type StepId } from '../card-form/model';
import { mapProblems, problemCountsByStep, type MappedProblems } from '../card-form/problems';
import { sampleNote, sampleState } from '../card-form/sample';
import { validateAll, validateStep, type Issue } from '../card-form/validate';
import { formToWorkbook, type GeneratedWorkbook } from '../card-form/workbook';
import { sortPriceCardProblems } from '../price-card-import';
import { ChannelProblem } from './ChannelProblem';
import { EditorContext, type FieldError } from './EditorContext';
import { StepBase } from './StepBase';
import { StepBasics } from './StepBasics';
import { StepBounds } from './StepBounds';
import { StepFees } from './StepFees';
import { StepPreview, type PreviewState } from './StepPreview';
import { StepRefs } from './StepRefs';
import { StepWeight } from './StepWeight';
import { useEditorEnv } from './use-editor-env';

/** 本页按这几版模板设计。规格口报出别的版本说明模板进位了，页面不照旧提交（ADR-0349 决定七）。 */
const knownTemplateVersions: readonly string[] = ['PPT-2'];

type Phase = 'idle' | 'previewing' | 'saving';
type Pending = null | 'sample' | 'clear';

const stepIntro: Readonly<Record<StepId, string>> = {
  basics: '先说清这是哪张价卡：谁的、什么方向、用什么币种和单位、从什么时候起生效。',
  base: '选一种价表类型，把每个分区的运费填进表里。',
  weight: '运费按什么重量算：只看实重，还是把体积折算成重量后取较大者，以及重量怎么进位。',
  fees: '基础运费之外的收费，一项一项加：固定金额、满足条件才收的附加费、按比例收的燃油费等。',
  bounds: '给整票合计设最低 / 最高收费，以及金额取整。都不需要就留空。',
  refs: '价卡引用的外部清单：燃油费率序列、分区目录、偏远档位目录。',
  preview: '检查并预览这张价卡，通过后存为草稿。',
};

function signatureOf(generated: GeneratedWorkbook | null): string {
  return generated === null ? '' : JSON.stringify(generated.workbook);
}

/**
 * 新建价卡：分步向导。运营在表单里逐条加费用项、给每条附加费配「什么时候收」，预览通过后存为草稿，再走现有的批准与发布。
 *
 * 表单提交的是一份结构化的价卡内容，服务端对它与对上传的 Excel 模板读法相同（ADR-0349）；表与列、可选值都取自模板规格口，
 * 页面不抄一份。预览返回的问题按「行号 → 表单控件」的映射标回格子里，映射不到的放总清单。
 *
 * 离开保护：页面切到别的签时表单内容留着（签的内容不卸载）；载入示例或清空重填会覆盖已填的内容，先确认；刷新与关页
 * 由浏览器自己拦。没有「打开已有价卡改一版」——读口只交规范化内容，反推不回表单；要改已有价卡请新建一版，或导入 Excel。
 */
export function PriceCardEditorPanel({ onOpenDrafts, onDraftSaved }: { onOpenDrafts: () => void; onDraftSaved: () => void }) {
  const { spec: specAnswer, env, reload } = useEditorEnv();
  if (specAnswer === null) {
    return <div className="flex-1 p-6 text-[13px] text-idpxyz-breadcrumb">正在读取价卡填写规则…</div>;
  }
  if (specAnswer.kind !== 'outcome') {
    return (
      <div className="flex-1 overflow-auto p-6">
        <ChannelProblem answer={specAnswer} what="读取价卡填写规则" onRetry={reload} />
      </div>
    );
  }
  if (!knownTemplateVersions.includes(specAnswer.body.templateVersion)) {
    return (
      <div className="flex-1 overflow-auto p-6 text-[13px] text-idpxyz-text">
        <p className="font-medium text-idpxyz-textBright">价卡填写规则已经升级到 {specAnswer.body.templateVersion}</p>
        <p className="mt-1 text-[12px] text-idpxyz-breadcrumb">
          这个页面按旧版规则设计，继续提交可能漏掉新增的内容。请刷新页面；仍然看到这条提示，请先用「导入价卡」，并联系管理员升级页面。
        </p>
      </div>
    );
  }
  return <Editor spec={specAnswer.body} env={env} onOpenDrafts={onOpenDrafts} onDraftSaved={onDraftSaved} />;
}

function Editor({
  spec,
  env,
  onOpenDrafts,
  onDraftSaved,
}: {
  spec: PriceCardTemplateSpec;
  env: ReturnType<typeof useEditorEnv>['env'];
  onOpenDrafts: () => void;
  onDraftSaved: () => void;
}) {
  // 干净的初始状态只造一次：行与费用项的键每次现造都不同，拿两份各造的比较会一开始就「有改动」。
  const [initial] = useState<CardFormState>(() => emptyState());
  const [state, setState] = useState<CardFormState>(initial);
  const [step, setStep] = useState<StepId>('basics');
  const [reached, setReached] = useState(0);
  const [attempted, setAttempted] = useState<ReadonlySet<StepId>>(() => new Set());
  const [preview, setPreview] = useState<PreviewState | null>(null);
  const [phase, setPhase] = useState<Phase>('idle');
  const [draft, setDraft] = useState<ApiResult<PriceCardDraftSubmissionBody> | null>(null);
  const [savedSignature, setSavedSignature] = useState<string | null>(null);
  const [cleanState, setCleanState] = useState(() => JSON.stringify(initial));
  const [sampleLoaded, setSampleLoaded] = useState(false);
  const [pending, setPending] = useState<Pending>(null);
  const [focusRequest, setFocusRequest] = useState<{ field: string; nonce: number } | null>(null);
  const contentRef = useRef<HTMLDivElement>(null);

  const update = useCallback((change: (draftState: CardFormState) => CardFormState) => setState(change), []);

  const generated = useMemo(() => formToWorkbook(state, spec), [state, spec]);
  const signature = useMemo(() => signatureOf(generated), [generated]);
  const stateText = useMemo(() => JSON.stringify(state), [state]);
  const dirty = stateText !== cleanState;

  // 刷新与关页时浏览器自己弹确认；只在有没存下的内容时挂。
  useEffect(() => {
    if (!dirty || savedSignature === signature) return;
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', onBeforeUnload);
    return () => window.removeEventListener('beforeunload', onBeforeUnload);
  }, [dirty, savedSignature, signature]);

  // 本地校验只对「点过下一步 / 预览」的步骤亮红，没碰过的步骤不提前吓人。
  const allIssues = useMemo(() => validateAll(state, spec), [state, spec]);
  const issuesByStep = useMemo(() => {
    const byStep: Partial<Record<StepId, Issue[]>> = {};
    for (const issue of allIssues) (byStep[stepOfField(issue.field)] ??= []).push(issue);
    return byStep;
  }, [allIssues]);

  const errors = useMemo(() => {
    const map = new Map<string, FieldError[]>();
    const push = (id: string, error: FieldError) => map.set(id, [...(map.get(id) ?? []), error]);
    for (const issue of allIssues) {
      if (attempted.has(stepOfField(issue.field))) push(issue.field, { where: issue.where, text: issue.text });
    }
    for (const entry of preview?.mapped.located ?? []) push(entry.field, { where: entry.where, text: entry.text, original: entry.original });
    return map;
  }, [allIssues, attempted, preview]);

  const backendCounts = useMemo(() => problemCountsByStep(preview?.mapped.located ?? []), [preview]);

  // 点问题 / 下一步被拦：切到那一步，等它渲染出来再滚到并聚焦那个控件。
  useEffect(() => {
    if (focusRequest === null) return;
    const target = contentRef.current?.querySelector<HTMLElement>(`[data-field="${focusRequest.field}"]`);
    if (target) {
      target.scrollIntoView({ block: 'center', behavior: 'smooth' });
      target.focus({ preventScroll: true });
    } else {
      contentRef.current?.scrollTo({ top: 0 });
    }
  }, [focusRequest]);

  // 存草稿的结果出现在预览之后，页面可能已经滚到下面：把结果带进视野。
  useEffect(() => {
    if (draft === null) return;
    contentRef.current?.querySelector('[aria-label="存草稿的结果"]')?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  }, [draft]);

  const jump = (fieldId: string) => {
    const target = stepOfField(fieldId);
    setStep(target);
    setReached((value) => Math.max(value, stepOrder.indexOf(target)));
    setFocusRequest((previous) => ({ field: fieldId, nonce: (previous?.nonce ?? 0) + 1 }));
  };

  const go = (target: StepId) => {
    setStep(target);
    contentRef.current?.scrollTo({ top: 0 });
  };

  const index = stepOrder.indexOf(step);
  const next = () => {
    const issues = validateStep(step, state, spec);
    if (issues.length > 0) {
      setAttempted((previous) => new Set(previous).add(step));
      jump(issues[0].field);
      return;
    }
    const target = stepOrder[index + 1];
    setReached((value) => Math.max(value, index + 1));
    go(target);
  };

  const applyReading = (answer: ApiResult<PriceCardImportReading>, source: GeneratedWorkbook): PreviewState => {
    const problems = answer.kind === 'outcome' ? sortPriceCardProblems(answer.body.problems ?? []) : [];
    const mapped: MappedProblems = mapProblems(problems, source.origins);
    return { signature: JSON.stringify(source.workbook), answer, mapped, workbook: source.workbook };
  };

  const runPreview = async () => {
    if (allIssues.length > 0) {
      setAttempted(new Set(stepOrder));
      jump(allIssues[0].field);
      return;
    }
    const sent = generated;
    setPhase('previewing');
    setDraft(null);
    const answer = await previewPriceCardWorkbook(sent.workbook);
    const result = applyReading(answer, sent);
    setPreview(result);
    setPhase('idle');
    // 预览发现问题：标红已经随结果出现，直接带到第一个有问题的格子。
    if (answer.kind === 'outcome' && answer.body.outcome !== 'VALIDATED' && result.mapped.located.length > 0) {
      setAttempted(new Set(stepOrder));
      jump(result.mapped.located[0].field);
    }
  };

  const save = async () => {
    if (preview === null || preview.signature !== signature) return;
    setPhase('saving');
    const answer = await submitPriceCardWorkbookDraft(preview.workbook);
    setDraft(answer);
    setPhase('idle');
    if (answer.kind === 'outcome') {
      if (answer.body.draft) {
        setSavedSignature(preview.signature);
        setCleanState(stateText);
        onDraftSaved();
      }
      if (answer.body.reading.outcome !== 'VALIDATED') setPreview(applyReading({ kind: 'outcome', status: answer.status, body: answer.body.reading }, generated));
    }
  };

  const load = (next: CardFormState, loadedSample: boolean) => {
    setState(next);
    setStep('basics');
    setReached(loadedSample ? stepOrder.length - 1 : 0);
    setAttempted(new Set());
    setPreview(null);
    setDraft(null);
    setSavedSignature(null);
    setCleanState(JSON.stringify(loadedSample ? emptyState() : next));
    setSampleLoaded(loadedSample);
  };

  const confirmPending = () => {
    if (pending === 'sample') load(sampleState(), true);
    if (pending === 'clear') load(emptyState(), false);
    setPending(null);
  };
  const ask = (kind: Exclude<Pending, null>) => (dirty && savedSignature !== signature ? setPending(kind) : load(kind === 'sample' ? sampleState() : emptyState(), kind === 'sample'));

  const fresh = preview !== null && preview.signature === signature;
  const draftSaved = savedSignature !== null && savedSignature === signature;
  const ctx = useMemo(() => ({ state, update, spec, env, errors }), [state, update, spec, env, errors]);

  const stepBody: Record<StepId, ReactElement> = {
    basics: <StepBasics />,
    base: <StepBase />,
    weight: <StepWeight />,
    fees: <StepFees />,
    bounds: <StepBounds />,
    refs: <StepRefs />,
    preview: (
      <StepPreview
        localIssues={allIssues}
        preview={preview}
        fresh={fresh}
        workbook={generated.workbook}
        phase={phase}
        draft={draft}
        draftSaved={draftSaved}
        onPreview={() => void runPreview()}
        onSave={() => void save()}
        onJump={jump}
        onOpenDrafts={onOpenDrafts}
      />
    ),
  };

  return (
    <EditorContext.Provider value={ctx}>
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-3 border-b border-idpxyz-border px-4 py-2.5">
          <div className="min-w-0">
            <h2 className="text-[15px] font-semibold text-idpxyz-textBright">新建价卡</h2>
            <p className="text-[12px] text-idpxyz-breadcrumb">
              按步骤填写，预览通过后存为草稿。要改已有的价卡，请新建一个版本，或用「导入价卡」上传 Excel。
            </p>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => ask('sample')}>
              从示例开始
            </Button>
            <Button variant="outline" size="sm" onClick={() => ask('clear')} disabled={!dirty && draft === null && preview === null}>
              清空重填
            </Button>
          </div>
        </div>

        {sampleLoaded ? (
          <p role="note" className="shrink-0 border-b border-idpxyz-border bg-idpxyz-sidebar px-4 py-1.5 text-[12px] text-idpxyz-text">
            {sampleNote}逐步看一遍就知道每一格怎么填；要存成自己的价卡，请把标识和数值都改成真实的。
          </p>
        ) : null}

        <div className="flex min-h-0 flex-1 flex-col md:flex-row">
          <nav aria-label="填写步骤" className="shrink-0 overflow-x-auto border-b border-idpxyz-border md:w-56 md:overflow-y-auto md:border-b-0 md:border-r">
            <ol className="flex gap-1 p-2 md:block md:space-y-1">
              {stepOrder.map((id, position) => {
                const current = id === step;
                const backend = backendCounts[id] ?? 0;
                const issues = (attempted.has(id) ? (issuesByStep[id]?.length ?? 0) : 0) + backend;
                const reachable = position <= reached;
                const done = position < reached && issues === 0 && (issuesByStep[id]?.length ?? 0) === 0;
                return (
                  <li key={id} className="shrink-0">
                    <button
                      type="button"
                      aria-current={current ? 'step' : undefined}
                      disabled={!reachable}
                      onClick={() => go(id)}
                      className={`flex w-full items-center gap-2 rounded px-2.5 py-2 text-left text-[13px] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-idpxyz-accent disabled:cursor-not-allowed disabled:opacity-50 ${
                        current ? 'bg-idpxyz-hover font-medium text-idpxyz-textBright' : 'text-idpxyz-text hover:bg-idpxyz-hover'
                      }`}
                    >
                      <span
                        className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] ${
                          issues > 0
                            ? 'bg-[color:var(--ds-status-danger-fg)] text-white'
                            : current
                              ? 'bg-idpxyz-accent text-white'
                              : done
                                ? 'bg-[color:var(--ds-status-success-fg)] text-white'
                                : 'border border-idpxyz-border text-idpxyz-breadcrumb'
                        }`}
                        aria-hidden="true"
                      >
                        {issues > 0 ? <AlertTriangle className="h-3 w-3" /> : done && !current ? <Check className="h-3 w-3" /> : position + 1}
                      </span>
                      <span className="whitespace-nowrap">{stepTitles[id]}</span>
                      {issues > 0 ? <span className="sr-only">，有 {issues} 处要改</span> : null}
                    </button>
                  </li>
                );
              })}
            </ol>
          </nav>

          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
            <div ref={contentRef} className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
              <div className="mb-4">
                <h3 className="text-[15px] font-semibold text-idpxyz-textBright">
                  {index + 1}. {stepTitles[step]}
                </h3>
                <p className="mt-0.5 text-[12px] text-idpxyz-breadcrumb">{stepIntro[step]}</p>
                {(backendCounts[step] ?? 0) > 0 ? (
                  <p role="alert" className="mt-2 text-[12px] text-[color:var(--ds-status-danger-fg)]">
                    上次预览在这一步发现 {backendCounts[step]} 处要改（已标红）。改完回到最后一步重新预览。
                  </p>
                ) : null}
              </div>
              <div className="max-w-4xl">{stepBody[step]}</div>
            </div>
            <div className="flex shrink-0 items-center justify-between gap-3 border-t border-idpxyz-border px-5 py-2.5">
              <Button variant="outline" onClick={() => go(stepOrder[index - 1])} disabled={index === 0}>
                上一步
              </Button>
              {step === 'preview' ? (
                <span className="text-[12px] text-idpxyz-breadcrumb">
                  {draftSaved ? '草稿已保存' : fresh && preview?.answer.kind === 'outcome' && preview.answer.body.outcome === 'VALIDATED' ? '预览通过，可以存为草稿' : ''}
                </span>
              ) : (
                <Button variant="default" className={primaryActionClass} onClick={next}>
                  下一步
                </Button>
              )}
            </div>
          </div>
        </div>

        <ConfirmDialog
          renderFooter={confirmDialogFooter}
          open={pending !== null}
          tone="warning"
          title={pending === 'sample' ? '载入示例？' : '清空已填的内容？'}
          message={pending === 'sample' ? '载入示例会覆盖现在已填的内容，且不能恢复。' : '清空后已填的内容不会保留。'}
          confirmLabel={pending === 'sample' ? '载入示例' : '清空'}
          cancelLabel="继续填写"
          onConfirm={confirmPending}
          onCancel={() => setPending(null)}
        />
      </div>
    </EditorContext.Provider>
  );
}
