<script setup>
import { ref, onMounted } from 'vue'
import { Minus, FullScreen, Close } from '@element-plus/icons-vue'
import { App } from '../../bindings/kinh-desktop/service'
import { usePlatform } from '../composables/usePlatform'
import appIcon from '../assets/appicon.png'

// v3 的绑定按服务（命名空间）导出，这里解构回扁平函数，沿用原有的调用写法
const { WindowMinimise, WindowToggleMaximise, WindowClose, GetAppVersion } = App

// 移动端（Android / iOS）没有窗口边框的概念：
// 窗口控制按钮整块不渲染，标题栏拖拽与双击最大化也一并禁用。
// 平台信息由 App.vue 启动时异步初始化，此处只读状态（详见 usePlatform）
const { isMobile } = usePlatform()

// 版本号唯一来源是后端：编译期由 `-ldflags -X .../service.AppVersion` 注入 tag 版本
// （见 service/update.go）。前端不要硬编码，否则发版后会与实际版本脱节。
const appVersion = ref('')

onMounted(() => {
  GetAppVersion()
    .then(v => { appVersion.value = v ? 'v' + v : '' })
    .catch(() => { appVersion.value = '' })
})

// 双击标题栏最大化：移动端无此交互，避免双击缩放时误触发
function onTitleBarDblClick() {
  // script 中访问 ref 需 .value（模板里则由 Vue 自动解包）
  if (!isMobile.value) {
    WindowToggleMaximise()
  }
}
</script>

<template>
  <!-- 上边栏：图标 + 应用信息（名称/版本两行），右侧窗口控制按钮（仅桌面端） -->
  <div class="top-bar" :class="{ 'is-mobile': isMobile }" @dblclick="onTitleBarDblClick">
    <div class="left-panel">
      <img :src="appIcon" class="app-icon" alt="logo" draggable="false" />
      <div class="app-info">
        <span class="app-title">KinhDesktop</span>
        <span v-if="appVersion" class="app-version">{{ appVersion }}</span>
      </div>
    </div>

    <!-- 右侧窗口控制按钮：移动端没有窗口概念，最小化/最大化/关闭均无对应系统行为 -->
    <div v-if="!isMobile" class="right-panel">
      <button class="window-btn" @click="WindowMinimise">
        <el-icon><Minus /></el-icon>
      </button>
      <button class="window-btn" @click="WindowToggleMaximise">
        <el-icon><FullScreen /></el-icon>
      </button>
      <button class="window-btn close-btn" @click="WindowClose">
        <el-icon><Close /></el-icon>
      </button>
    </div>
  </div>
</template>

<style scoped>
.top-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px 0 10px;
  height: 44px;
  background-color: #fff;
  border-bottom: 1px solid #e4e7ed;
  --wails-draggable: drag;
  user-select: none;
}

/* 移动端：拖拽窗口无意义，改为 no-drag 避免手势被拦截；
   顶部补足安全区内边距，防止内容被状态栏/刘海遮挡 */
.top-bar.is-mobile {
  --wails-draggable: no-drag;
  padding-top: env(safe-area-inset-top, 0px);
  height: calc(44px + env(safe-area-inset-top, 0px));
  /* 移动端左右留白收紧，把宽度让给内容 */
  padding-left: 12px;
  padding-right: 12px;
}

.left-panel {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow: hidden;
}

.app-icon {
  width: 28px;
  height: 28px;
  object-fit: contain;
  flex-shrink: 0;
}

.app-info {
  display: flex;
  flex-direction: column;
  justify-content: center;
  line-height: 1.2;
  overflow: hidden;
}

.app-title {
  font-weight: 600;
  font-size: 12px;
  color: #303133;
  white-space: nowrap;
}

.app-version {
  font-size: 10px;
  color: #909399;
  white-space: nowrap;
}

.right-panel {
  display: flex;
  align-items: center;
  gap: 10px;
  /* Wails 拖拽区域排除标记（-webkit-app-region 是 Electron 规范，Wails 不识别） */
  --wails-draggable: no-drag;
}

.window-btn {
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  background-color: transparent;
  color: #606266;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 4px;
  transition: all 0.2s ease;
}

/* 悬停效果仅在支持悬停的设备生效。
   触摸设备（含带触屏的 Windows 设备）上 :hover 会粘住，点过的按钮保持高亮不恢复。 */
@media (hover: hover) {
  .window-btn:hover {
    background-color: rgba(0, 0, 0, 0.08);
  }

  .close-btn:hover {
    background-color: #f56c6c;
    color: #fff;
  }
}

/* 按下反馈：触摸设备靠 :active，抬手即解除 */
.window-btn:active {
  background-color: rgba(0, 0, 0, 0.12);
}

.close-btn:active {
  background-color: #e64242;
}
</style>
