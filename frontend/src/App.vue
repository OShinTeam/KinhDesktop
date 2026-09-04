<script setup>
import { ref } from 'vue'
import HeaderBar from './components/HeaderBar.vue'
import LoginView from './views/LoginView.vue'
import MainView from './views/MainView.vue'

// 登录成功后保存凭证，切换到主界面
const credential = ref(null)

function handleLoginSuccess(result) {
  credential.value = result
}
</script>


<template>
  <div class="app-layout">
    <HeaderBar />
    <div class="app-content">
      <MainView v-if="credential" :credential="credential" />
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
</style>
