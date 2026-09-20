import { AlertTriangle, RotateCw } from 'lucide-react';

// 区块级错误（票 admin-web-ux-alignment/06 第 2 条；手册「Error State」的 section / widget 层）：一段区读失败只红那一段，
// 页面其余部分照常——与页级 ErrorState（整个内容区换掉）是两层，不混用。供抽屉里一段历史、表单里一册候选这类地方用。
//
// 为什么不是包 ui-primitives 的 SectionErrorState：它的标题句「Section failed to load」与按钮字「Retry」写死在原语里、只开放
// message 一格，包一层就会把英文缺省句漏到页面上——文案归本仓（spec 红线：内容规范赢形态规范，与 FilteredEmptyState 不换同一
// 条理由）。这里照它的形做（窄边框区块、小警示图标 + 加粗标题、说明一行、小号重试按钮），字全由调用方给；原语哪天开放标题与
// 按钮文案，换成包它即可，调用方不动。色只用 idpxyz token：危险色做左侧色条与标题，不引 Tailwind 调色板。
//
// 自成一个文件而不并进 index.tsx：index.tsx 引 @idpxyz/ui-primitives（其 exports 无 require 条件，CJS 测试产物 require 不到），
// 分开放才能在 node:test 里用 renderToStaticMarkup 钉「渲染出重试按钮」。

export interface SectionErrorProps {
  /** 一句话说这一段怎么了；调用方给，不留缺省。 */
  title: string;
  /** 续办或原因；可省。 */
  description?: string;
  /** 给了才出重试按钮——重试不会改变结果的（未配置）不要给。 */
  onRetry?: () => void;
}

export function SectionError({ title, description, onRetry }: SectionErrorProps) {
  return (
    <div
      role="alert"
      className="mt-1 rounded border border-idpxyz-border border-l-2 border-l-idpxyz-danger bg-idpxyz-panelBg px-3 py-2 text-[12px] text-idpxyz-text"
    >
      <div className="flex items-center gap-2 text-idpxyz-danger">
        <AlertTriangle className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span className="font-medium">{title}</span>
      </div>
      {description ? <p className="mt-1 leading-5 text-idpxyz-textMuted">{description}</p> : null}
      {onRetry ? (
        <button
          type="button"
          onClick={onRetry}
          className="mt-2 inline-flex items-center gap-1.5 px-2.5 py-1 text-[12px] rounded border border-idpxyz-border text-idpxyz-text hover:bg-idpxyz-hover"
        >
          <RotateCw className="h-3.5 w-3.5" aria-hidden />
          重试
        </button>
      ) : null}
    </div>
  );
}
