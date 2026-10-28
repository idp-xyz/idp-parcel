import { Rows3, Rows4 } from 'lucide-react';
import { useDensity, useTheme } from '@idpxyz/ui-theme-runtime';
import { densityToggleLabel } from './top-bar-model';
import { densityWord, themeWord } from './workspace-state';

// 底部状态栏（票 admin-web-workspace-form/01 第 4 条）：左侧是活动标签地址的人话（Layout 用 workspaceLocationLabel 算好交进来），
// 右侧是密度档位与当前主题词。
//
// 改版第一步起不再包 @idpxyz/ui-workspace 的 StatusBar：它自带的密度按钮把档位写成英文（标题「List density: Comfortable」、
// 正文 `capitalize` 的 density 原词），没有改字的口，而界面上不许出现英文档位词。高度、底色与排版沿它的形（22px、statusBar
// 色、12px 白字），只把密度那颗按钮换成中文——它读的仍是 DensityProvider，与顶栏的 DensityToggle 同源，既显档位也能切；
// 按钮的可访问名说「切过去会变成什么」（top-bar-model 的 densityToggleLabel），可见文字说「现在是什么」（densityWord）。
//
// 上游的 commandFeedback 与 showDemoIndicators 两位本仓本来就不用：没有反馈通道；「142 Active / All Carriers Online」是
// 无数据的样板字，spec 红线不允许假计数。

export interface WorkspaceStatusBarProps {
  /** 活动位置的人话，见 shell/workspace-state.ts 的 workspaceLocationLabel。 */
  location: string;
}

export function WorkspaceStatusBar({ location }: WorkspaceStatusBarProps) {
  const { theme } = useTheme();
  const { density, toggleDensity } = useDensity();
  return (
    <div className="flex h-[22px] shrink-0 select-none items-center justify-between bg-idpxyz-statusBar px-2 text-[12px] text-white">
      <div className="flex items-center gap-3">
        <span className="max-w-[480px] truncate" title={location} data-workspace-location>
          {location}
        </span>
      </div>
      <div className="flex items-center gap-3">
        <button
          type="button"
          onClick={toggleDensity}
          title={densityToggleLabel(density)}
          aria-label={densityToggleLabel(density)}
          className="flex h-[18px] items-center gap-1 rounded-sm px-1.5 hover:bg-idpxyz-statusHover"
          data-workspace-density={density}
        >
          {density === 'compact' ? <Rows4 size={12} aria-hidden="true" /> : <Rows3 size={12} aria-hidden="true" />}
          <span>{densityWord(density)}</span>
        </button>
        <span data-workspace-theme>{themeWord(theme)}</span>
      </div>
    </div>
  );
}
