<script setup>
import { ref, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { RestoreLogin } from '../wailsjs/go/service/App'
import HeaderBar from './components/HeaderBar.vue'
import LoginView from './views/LoginView.vue'
import MainView from './views/MainView.vue'

// 登录成功后保存凭证，切换到主界面
const credential = ref(null)
// 启动阶段：先检测本地登录记录，期间不显示登录界面
const checking = ref(true)

function handleLoginSuccess(result) {
  credential.value = result
}

// 启动时尝试用本地保存的登录信息自动登录，无记录才进入登录页
onMounted(async () => {
  try {
    const restored = await RestoreLogin()
    if (restored && restored.success) {
      credential.value = restored
    }
  } catch (err) {
    console.warn('自动登录失败:', err)
  } finally {
    checking.value = false
  }
})
</script>


<template>
  <div class="app-layout">
    <HeaderBar />
    <div class="app-content">
      <!-- 启动检测中 -->
      <div v-if="checking" class="boot-loading">
        <el-icon class="boot-spinner is-loading"><Loading /></el-icon>
        <span>正在检测登录状态...</span>
      </div>
      <!-- 主界面 -->
      <MainView v-else-if="credential" :credential="credential" @logout="credential = null" />
      <!-- 登录界面 -->
      <LoginView v-else @login-success="handleLoginSuccess" />
    </div>
  </div>
</template>

<style scoped>
.app-layout {
  width: 100vw;
  height: 100vh;
  display: flex;
  flex-direction: column;
  background-color: #f5f7fa;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  user-select: none;
}

.app-content {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  display: flex;
  flex-direction: column;
}

.boot-loading {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: #909399;
  font-size: 14px;
}

.boot-spinner {
  font-size: 28px;
  color: #409eff;
}
</style>
