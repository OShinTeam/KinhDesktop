import { ref } from 'vue'
import { System } from '@wailsio/runtime'

// 是否运行在移动平台（Android / iOS）。
//
// ⚠️ 不要用 System.IsMobile()：它是同步读取 window._wails.environment，
// 而该对象由 runtime 在启动阶段注入。在模块顶层（<script setup> 第一层）调用时
// 它往往还不存在，会读到 undefined 并误判成桌面端 —— 表现为移动端仍然渲染
// 窗口控制按钮。System.Environment() 是异步接口，会等到数据就绪再返回。
//
// 取不到平台信息时按桌面端处理（isMobile = false），保证桌面功能完整；
// 移动端最坏情况只是多显示几个无效按钮，不会影响功能。
export const isMobile = ref(false)

let inited = false

export function initPlatform() {
  if (inited) {
    return Promise.resolve()
  }
  inited = true
  return System.Environment()
    .then((env) => {
      isMobile.value = env?.OS === 'android' || env?.OS === 'ios'
    })
    .catch(() => {
      isMobile.value = false
    })
}

export function usePlatform() {
  return { isMobile }
}
