// Node 内建模块的最小环境声明（票 admin-web-audit-followups/01）。
//
// 这是替身不是实现：本机装不了 @types/node（加依赖要重写 pnpm-lock.yaml，共享树上不得单方面跑，
// 见 docs/agents/workflow.md 本机环境），而测试要 import 'node:test' 与 'node:assert/strict'。
// 只声明测试实际用到的签名；@types/node 进来那天整份删掉。每加一格都要有一条测试真的在用它，
// 不为「以后可能用」预声明。
//
// 一律用具名导入（import { test } / import { equal }），不声明 default：tsc 发 CommonJS 时默认导入
// 要经 __importDefault 才对得上 Node 的导出形状，具名导入不经过那一层。

declare module 'node:test' {
  export interface TestContext {
    readonly name: string;
  }

  export type TestFn = (context: TestContext) => void | Promise<void>;

  export function test(name: string, fn: TestFn): Promise<void>;
  export function describe(name: string, fn: () => void): Promise<void>;
  export function it(name: string, fn: TestFn): Promise<void>;
  export function beforeEach(fn: () => void | Promise<void>): void;
  export function afterEach(fn: () => void | Promise<void>): void;
}

declare module 'node:assert/strict' {
  export function ok(value: unknown, message?: string): asserts value;
  export function equal<T>(actual: unknown, expected: T, message?: string): asserts actual is T;
  export function notEqual(actual: unknown, expected: unknown, message?: string): void;
  export function deepEqual<T>(actual: unknown, expected: T, message?: string): asserts actual is T;
  export function match(value: string, pattern: RegExp, message?: string): void;
  export function fail(message?: string): never;
  // 契约错误要证「抛」（templates/inspector.test.ts）：expected 只声明测试用到的两种——Error 子类的构造器、断言函数。
  export function throws(
    fn: () => unknown,
    expected?: (new (...args: never[]) => Error) | ((error: unknown) => boolean),
    message?: string,
  ): void;
}

// 契约测试读后端包 testdata/ 里的夹具（票 admin-web-audit-followups/04）。只声明按 utf8 读成
// 字符串这一种签名——测试只需要 JSON 文本，不需要 Buffer 那一族重载。
declare module 'node:fs' {
  export function readFileSync(path: string, encoding: 'utf8'): string;
}
