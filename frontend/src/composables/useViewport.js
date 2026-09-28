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

function syncViewport() {
  isPortrait.value = window.innerHeight > window.innerWidth
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
  return { isPortrait }
}
