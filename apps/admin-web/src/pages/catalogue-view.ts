import type { ModuleInfo } from '../navigation';
import type { TemplateViewState } from '../templates/state-slot';
import type { ApiResult } from './catalogue-api';

interface CatalogueViewOptions {
  module: ModuleInfo;
  endpoint: string;
  emptyTitle: string;
  emptyDescription: string;
}

export function catalogueViewState<Body>(
  answer: ApiResult<Body> | null,
  recordCount: number,
  retry: () => void,
  options: CatalogueViewOptions,
): TemplateViewState {
  if (!answer) {
    return { kind: 'loading' };
  }
  if (answer.kind === 'outcome') {
    if (recordCount === 0) {
      return {
        kind: 'empty',
        title: options.emptyTitle,
        description: options.emptyDescription,
      };
    }
    return { kind: 'ready' };
  }
  if (answer.kind === 'unconfigured') {
    return {
      kind: 'unconfigured',
      title: '访问通道尚未配置',
      description: `${options.module.owner}的目录读取入口（${options.endpoint}）当前不可用；这不是“目录为空”。`,
      facts: {
        owner: options.module.owner,
        source: options.module.source,
        unlock: '配置该上下文的访问通道后重新查询；页面不会用演示数据代替。',
      },
    };
  }
  if (answer.kind === 'callerProblem') {
    return {
      kind: 'error',
      title: `调用方式问题（HTTP ${answer.status}）`,
      description: problemNote(answer.code),
    };
  }
  if (answer.kind === 'noAnswer') {
    return {
      kind: 'error',
      title: `服务端未形成答案（HTTP ${answer.status}）`,
      description: problemNote(answer.code),
      onRetry: retry,
    };
  }
  return {
    kind: 'error',
    title: '无法连接主数据读取服务',
    description: answer.message,
    onRetry: retry,
  };
}

function problemNote(code: string): string {
  switch (code) {
    case 'METHOD_NOT_ALLOWED':
      return '端点不接受当前 HTTP 方法；请检查前端与服务端版本是否一致。';
    case 'MALFORMED_REQUEST':
      return '查询参数不在服务端接受的封闭集合内；请检查前端与服务端版本是否一致。';
    case 'INTAKE_FAILED':
      return '接入面未能形成授权查询作用域；服务端未读取目录。';
    case 'NO_ANSWER_FORMED':
      return '目录读取依赖未能形成答案，可稍后重试。';
    default:
      return `服务端问题码：${code}`;
  }
}

export function formatInstant(value: string): string {
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toISOString().replace('T', ' ').replace('.000Z', ' UTC');
}

export function formatRange(from: string, to?: string): string {
  return `${formatInstant(from)} → ${to ? formatInstant(to) : '持续有效'}`;
}
