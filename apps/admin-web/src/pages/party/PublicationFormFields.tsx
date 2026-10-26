import { useEffect, useState, type ReactNode } from 'react';
import { Button, Input } from '@idpxyz/ui-primitives';
import {
  accessRefusalHeadline,
  accessRefusalStatus,
  accessRefusalTitle,
  isOperatorRefusal,
} from '../../auth/access-refusal';
import type { ApiResult } from '../catalogue-api';
import { fieldProblemText, type FieldProblemText } from './field-problem-text';
import {
  vocabularyOptions,
  type CommercialObjectKindName,
  type PublicationVocabularyResponseBody,
} from './publication-draft-api';

/**
 * 发布表单组件件的共享层（票 admin-write-faces/22）：各册逐字段表单此前各自留一份的 Field / Problems / RowFrame /
 * useLoaded / ReferencePicker / VocabularySelect / vocabularyPlaceholder，这里各只有一份，各册表单全从这里导入。
 *
 * **为什么现在才抬**：伞票 07 让各册表单并行写，纪律是「不跨文件借私有件」——那时抬共享层会让并行的会话撞同一个文件；
 * 代价是每张表单各留一份同形副本，各票的非作者评审都点了同一条 Duplicated Code。表单落齐之后一次抬齐，就是本文件。
 *
 * **一条显示规则只在这里定**：一格 = 标签 + 控件 + 服务端点名到这条路径的问题（构造门原话，逐条列出，不改写）。标签上
 * 不挂 JSON 路径（票 group-legal-entities-uiux/02 推翻了票 22 定的「抬齐后一律显」）：票 22 要的是让操作者把公共半边
 * 列出的「未认领路径」与眼前的格对上，代价是每格都挂一串实现名；而未认领的路径按定义没有哪一格认领它，对上它靠的是
 * 子表行头的「第 n 行」与格名，不靠格旁的路径原词。路径只在未认领区随问题一起显。
 * **凡认领了的路径都要有一处渲染**（票 22 判据 3）：Field 渲染自己那条（及别名路径），RowFrame 渲染行本身
 * 那条，节根那条由表单用 Problems 直接挂在节标题下——认领了却没渲染，服务端点到那一格就被静默吞掉。
 *
 * **本文件不算摘要、不裁任何门、不内置任何一格枚举**（伞票 07 硬句）：VocabularySelect 的选项只由服务端词表的码生成，
 * 中文词表只作装饰；ReferencePicker 的候选只由读面生成，读不到退回手填。不预选：「未选」是一个空值状态，不是默认值。
 */

export const fieldLabel = 'block text-[12px] text-idpxyz-breadcrumb mb-1';
export const selectClass =
  'w-full rounded border border-idpxyz-border bg-idpxyz-inputBg px-2 py-1.5 text-[13px] ' +
  'text-idpxyz-text focus:outline-none focus:border-idpxyz-accent disabled:opacity-60';

/**
 * 一条问题：先给配置员读得懂的中文，服务端原话收进可展开的「原话」，排查时还对得上日志（译法见 field-problem-text.ts）。
 * 原话本就是中文白话时不再显第二遍。
 */
export function ProblemText({ problem }: { problem: FieldProblemText }) {
  return (
    <>
      {problem.text}
      {problem.original ? (
        <details className="inline-block ml-1 align-top text-idpxyz-breadcrumb">
          <summary className="cursor-pointer select-none">原话</summary>
          <span className="font-mono break-all">{problem.original}</span>
        </details>
      ) : null}
    </>
  );
}

/** 服务端（或本地编不进类型）点名的那一格的问题，逐条挂在格下；没有问题就不占位。 */
export function Problems({ lines }: { lines?: readonly string[] }) {
  if (!lines || lines.length === 0) return null;
  return (
    <ul className="text-[12px] text-idpxyz-danger list-disc ml-4 mt-1">
      {lines.map((line) => (
        <li key={line}>
          <ProblemText problem={fieldProblemText(line)} />
        </li>
      ))}
    </ul>
  );
}

/**
 * 几条路径的问题合到一处显：一个控件对应服务端几个键（对象 + 版本一起选那种）时用。不再在每条前挂路径——控件只有
 * 一个，路径对配置员没有信息；两个键答同一句时只显一条。原话里仍看得到是哪个键。
 */
export function PathProblems({ problems, paths }: { problems: Record<string, string[]>; paths: readonly string[] }) {
  const lines = [...new Set(paths.flatMap((path) => problems[path] ?? []))];
  return <Problems lines={lines} />;
}

/**
 * 一格：标签、控件、服务端点名到这条路径（及 `alsoPaths` 里各别名路径）的问题。
 *
 * 路径只用来认领问题，不显在标签上：读表单的是运营配置员，`legalEntities[0].revision` 一类实现名对他没有信息。
 * 服务端点名了一条没有任何一格认领的路径时，路径才是定位信息，那时由各表单的未认领问题区连路径一起显。
 *
 * `as="div"`：几格的控件是一组按钮时用 div 不用 label——label 会把点标题读成点第一个按钮。`silent`：给「格就是行」的那种，
 * 问题已由外层 RowFrame 显在行上，这里不再显一遍；认领仍在行那条路径上，不因此少一处渲染。
 */
export function Field({
  label,
  path,
  problems,
  alsoPaths = [],
  as = 'label',
  silent = false,
  children,
}: {
  label: string;
  path: string;
  problems: Record<string, string[]>;
  alsoPaths?: readonly string[];
  as?: 'label' | 'div';
  silent?: boolean;
  children: ReactNode;
}) {
  const lines = silent ? [] : [path, ...alsoPaths].flatMap((candidate) => problems[candidate] ?? []);
  const Wrapper = as;
  return (
    <Wrapper className="block">
      <span className={fieldLabel}>{label}</span>
      {children}
      <Problems lines={lines} />
    </Wrapper>
  );
}

/**
 * 子表的一行：几格并排 + 删行，行本身那条路径（`rows[i]`）的问题显在行下——各格立得住而行拼不成时服务端点名整行，
 * 那条要显在行下不是显成别处的问题。
 */
export function RowFrame({
  path,
  problems,
  locked,
  columns = 'grid-cols-[1fr_2fr_auto]',
  onRemove,
  children,
}: {
  path: string;
  problems: Record<string, string[]>;
  locked: boolean;
  columns?: string;
  onRemove: () => void;
  children: ReactNode;
}) {
  return (
    <div className="rounded border border-idpxyz-border p-2 flex flex-col gap-2">
      <div className={`grid ${columns} gap-2 items-start`}>
        {children}
        <div className="pt-5">
          <Button variant="outline" disabled={locked} onClick={onRemove}>
            删
          </Button>
        </div>
      </div>
      <Problems lines={problems[path]} />
    </div>
  );
}

/**
 * 读一次、只读一次：读面与词表都是目录，表单打开时取一份，不随每次击键重取。`load` 要是稳定引用（模块级函数），写成
 * 内联箭头会每次渲染重取。
 */
export function useLoaded<Body>(load: () => Promise<ApiResult<Body>>): ApiResult<Body> | null {
  const [answer, setAnswer] = useState<ApiResult<Body> | null>(null);
  useEffect(() => {
    let cancelled = false;
    void load().then((next) => {
      if (!cancelled) setAnswer(next);
    });
    return () => {
      cancelled = true;
    };
  }, [load]);
  return answer;
}

export interface PickerOption {
  value: string;
  label: string;
}

/** ReferencePicker 两种取数方式共有的那半：格、候选转写与各句措辞。 */
export interface ReferencePickerFaceProps<Body> {
  label: string;
  path: string;
  problems: Record<string, string[]>;
  value: string;
  locked: boolean;
  onChange: (value: string) => void;
  optionsOf: (body: Body) => PickerOption[];
  emptyNote: string;
  readFace: string;
  /** 读面不可用时手填框的占位。 */
  manualPlaceholder?: string;
  /** 候选非空时挂在选单下的一句；不给就不占位。 */
  optionsNote?: string;
  /** 当前值不在候选里时那一项的括注。 */
  unknownNote?: string;
}

/**
 * 从读面选一条引用。读面答了业务答案就给选单（不按状态过滤——表单不裁，状态显在选项里由人看）；读面在
 * 未配置那堵墙前或读不到时退回手填并说明原因。选出来的只是引用串，在不在册、立不立得住仍由服务端判。
 * 手填时若当前值不在候选里，选单照样保留它作一项，免得读面刷新把人填好的东西静默清掉。
 *
 * 三处可选的整句顶替（`manualPlaceholder` / `optionsNote` / `unknownNote`）只为价卡目录那一格：它的引用串有固定形状
 * （`planId@planVersion`）、候选非空时还要提醒「照目录行上的方向填」、把读面叫「目录」——都是措辞不是形状，抬共享层
 * 不改一字显示文案，所以给句子留口而不另留一份 Picker。
 *
 * 这一份自己读（`load`，读一次只读一次）；调用方手上已经有那份读面答案时用 ReferencePickerFor 把答案交进来，
 * 免得同一册在一张表单里被读两三遍（票 admin-web-group-legal-entities/13 第 5 条）。
 */
export function ReferencePicker<Body>({
  load,
  ...face
}: ReferencePickerFaceProps<Body> & { load: () => Promise<ApiResult<Body>> }) {
  const answer = useLoaded(load);
  return <ReferencePickerFor answer={answer} {...face} />;
}

/**
 * 收一份已取回的读面答案的 ReferencePicker：候选、手填退路与各句措辞同上，只是不自己读。`answer` 为 null 即
 * 「还在读」——页面持有的列表答案首取回来之前就是它，措辞与自己读那份同一句。
 */
export function ReferencePickerFor<Body>({
  answer,
  label,
  path,
  problems,
  value,
  locked,
  onChange,
  optionsOf,
  emptyNote,
  readFace,
  manualPlaceholder = '引用串（读面不可用时手填）',
  optionsNote,
  unknownNote = '手填，不在读面上',
}: ReferencePickerFaceProps<Body> & { answer: ApiResult<Body> | null }) {
  if (answer?.kind === 'outcome') {
    const options = optionsOf(answer.body);
    const known = options.some((option) => option.value === value);
    const note = options.length === 0 ? emptyNote : optionsNote;
    return (
      <Field label={label} path={path} problems={problems}>
        <select className={selectClass} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
          <option value="">未选</option>
          {!known && value !== '' ? (
            <option value={value}>
              {value}（{unknownNote}）
            </option>
          ) : null}
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
        {note !== undefined ? <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">{note}</span> : null}
      </Field>
    );
  }

  return (
    <Field label={label} path={path} problems={problems}>
      <Input
        value={value}
        disabled={locked}
        className="font-mono text-[13px]"
        placeholder={manualPlaceholder}
        onChange={(event) => onChange(event.target.value)}
      />
      <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
        {answer === null
          ? `正在读${readFace}…`
          : answer.kind === 'unconfigured'
            ? `暂时读不到${readFace}（接入渠道未配置），先手填标识；标识对不对由服务端在发布时检查。`
            : isOperatorRefusal(answer)
              ? `暂时读不到${readFace}（${accessRefusalTitle(answer.kind)}），先手填标识；标识对不对由服务端在发布时检查。`
              : `${readFace}读不到，先手填；引用在不在册由服务端发布时判。`}
      </span>
    </Field>
  );
}

/**
 * 封闭集下拉：选项 = 服务端词表（票 20）那一集的码 × 本页中文词表；词表没收录的码原样示出。词表在未配置那堵墙前
 * （403）、调用方问题、未形成答案、没到达、或答复里没有这一集时都**显占位不显码**——表单不内置任何一格，内置一份
 * 就是同一封闭集的第二份写法。不预选：「未选」是一个空值状态，不是默认值。
 *
 * `codes` 为 null 即「词表里没有这一集」（缺席与空数组分开：空数组是有这一集但今天没给码）。`kind` 只用来把读口的
 * 查询串写进占位说明；某册那句说明要换措辞的，给 `unavailableNote` 整句顶替。
 */
export function VocabularySelect({
  label,
  path,
  setName,
  kind,
  codes,
  vocabulary,
  labels,
  problems,
  value,
  locked,
  onChange,
  unavailableNote,
}: {
  label: string;
  path: string;
  setName: string;
  kind: CommercialObjectKindName;
  codes: string[] | null;
  vocabulary: ApiResult<PublicationVocabularyResponseBody> | null;
  labels: Record<string, string>;
  problems: Record<string, string[]>;
  value: string;
  locked: boolean;
  onChange: (value: string) => void;
  unavailableNote?: string;
}) {
  if (codes !== null) {
    const options = vocabularyOptions(codes, labels);
    const known = options.some((option) => option.value === value);
    return (
      <Field label={label} path={path} problems={problems}>
        <select className={selectClass} value={value} disabled={locked} onChange={(event) => onChange(event.target.value)}>
          <option value="">未选</option>
          {!known && value !== '' ? <option value={value}>{value}（不在词表上）</option> : null}
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label} · {option.value}
            </option>
          ))}
        </select>
        {options.length === 0 ? (
          <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">服务端暂时没有提供这一格的可选项。</span>
        ) : null}
      </Field>
    );
  }

  return (
    <Field label={label} path={path} problems={problems}>
      <select className={selectClass} value="" disabled>
        <option value="">{vocabularyPlaceholder(vocabulary, setName)}</option>
      </select>
      <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
        {/* 可选项只从服务端词表读口取，表单不内置枚举、不自造码；读到之前这一格选不了，留空送预览由服务端点名。 */}
        {unavailableNote ?? '可选项由服务端提供，还没读到时这一格暂时选不了。'}
      </span>
    </Field>
  );
}

/** 词表就绪前下拉里那一句占位：按五格结果代数各写一句，答了业务答案却没有这一集也算未就绪。 */
export function vocabularyPlaceholder(answer: ApiResult<PublicationVocabularyResponseBody> | null, setName: string): string {
  if (answer === null) return '正在读取可选项…';
  switch (answer.kind) {
    case 'outcome':
      return '暂无可选项';
    case 'unconfigured':
      return '暂时读不到可选项（接入渠道未配置）';
    case 'loginRequired':
    case 'notGranted':
      return `暂时读不到可选项（${accessRefusalTitle(answer.kind)}）`;
    case 'callerProblem':
      return `暂时读不到可选项（请求有误，HTTP ${answer.status}）`;
    case 'noAnswer':
      return `暂时读不到可选项（服务端出错，HTTP ${answer.status}）`;
    case 'transport':
      return '暂时读不到可选项（连不上服务）';
  }
}
