import { ref } from 'vue'

// 竖屏判定：视口高度 > 宽度。
//
// 布局策略（见 MainView.vue）：
//   - 竖屏（高 > 宽）→ 底部导航：导航条置底、内容在上，避免侧栏挤占本就有限的横向空间
//   - 横屏 / 桌面（宽 >= 高）→ 普通布局：保留左侧栏
//
// 为什么不用 CSS 媒体查询：布局切换需要同时改变 el-menu 的 mode（横向/纵向），
// 那是组件属性而非样式，CSS 改不了，必须有 JS 状态参与。
//
// 应用级单例，监听常驻（窗口尺寸变化、屏幕旋转都要跟随）。
export const isPortrait = ref(false)

// 小屏判定：视口短边 < 600px。
//
// 用短边而非宽度：手机横屏时宽度会到 800+，按宽度判会把横屏手机误当成平板。
// 手机短边普遍在 360~430，平板的短边通常在 600 以上，600 这条线能把两者分开。
// 用途见 LoginView：移动端小屏隐藏扫码登录（扫码需要另一台设备配合，在手机上没意义）。
export const isSmallScreen = ref(false)

function syncViewport() {
  const { innerWidth, innerHeight } = window
  isPortrait.value = innerHeight > innerWidth
  isSmallScreen.value = Math.min(innerWidth, innerHeight) < 600
}

let bound = false

export function initViewport() {
  if (bound) {
    return
  }
  bound = true
  syncViewport()
  window.addEventListener('resize', syncViewport)
  window.addEventListener('orientationchange', syncViewport)
}

export function useViewport() {
  return { isPortrait, isSmallScreen }
}
