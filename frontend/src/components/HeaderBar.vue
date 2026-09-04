<script setup>
import { ref } from 'vue'
import { Minus, FullScreen, Close } from '@element-plus/icons-vue'
import { WindowMinimise, WindowToggleMaximise, WindowClose } from '../../wailsjs/go/service/App'
import appIcon from '../assets/appicon.png'

// 版本号集中定义，与应用图标一起在顶栏展示
const appVersion = ref('v0.1.0')
</script>

<template>
  <!-- 上边栏：图标 + 应用信息（名称/版本两行），右侧窗口控制按钮 -->
  <div class="top-bar" @dblclick="WindowToggleMaximise">
    <div class="left-panel">
      <img :src="appIcon" class="app-icon" alt="logo" draggable="false" />
      <div class="app-info">
        <span class="app-title">KinhDesktop</span>
        <span v-if="appVersion" class="app-version">{{ appVersion }}</span>
      </div>
    </div>

    <!-- 右侧窗口控制按钮 -->
    <div class="right-panel">
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

.window-btn:hover {
  background-color: rgba(0, 0, 0, 0.08);
}

.window-btn:active {
  background-color: rgba(0, 0, 0, 0.12);
}

.close-btn:hover {
  background-color: #f56c6c;
  color: #fff;
}

.close-btn:active {
  background-color: #e64242;
}
</style>