import { StatusBar } from '@idpxyz/ui-workspace';
import { useTheme } from '@idpxyz/ui-theme-runtime';
import { themeWord } from './workspace-state';

// 底部状态栏（票 admin-web-workspace-form/01 第 4 条）：包 @idpxyz/ui-workspace 的 StatusBar，只喂它真实的两件事——
// 左侧是活动标签地址的人话（Layout 用 workspaceLocationLabel 算好交进来），右侧是当前主题词。
//
// 密度那一档不另写：StatusBar 自带的密度切换按钮读的就是 DensityProvider（与 TopBar 的 DensityToggle 同源），
// 它既显档位也能切，再摆一份中文档位词是同一事实写两遍。主题词与它并排，说的是「现在是什么」；顶栏那个按钮说的是
// 「切过去会变成什么」（top-bar-model 的 themeToggleLabel），两处分工不同。
//
// commandFeedback 位留给命令面板动作的反馈（票 03 今天没有反馈通道，传 null 不渲染那一格）；showDemoIndicators 关——
// 那组「142 Active / All Carriers Online」是无数据的样板字，spec 红线不允许假计数。

export interface WorkspaceStatusBarProps {
  /** 活动位置的人话，见 shell/workspace-state.ts 的 workspaceLocationLabel。 */
  location: string;
}

export function WorkspaceStatusBar({ location }: WorkspaceStatusBarProps) {
  const { theme } = useTheme();
  return (
    <StatusBar
      commandFeedback={null}
      showDemoIndicators={false}
      leftSlot={
        <span className="max-w-[480px] truncate" title={location} data-workspace-location>
          {location}
        </span>
      }
      rightSlot={<span data-workspace-theme>{themeWord(theme)}</span>}
    />
  );
}
