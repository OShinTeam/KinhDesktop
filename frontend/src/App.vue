<script setup>
import { ref, onMounted } from 'vue'
import { Loading } from '@element-plus/icons-vue'
import { ElMessageBox } from 'element-plus'
import { Events } from '@wailsio/runtime'
import { App } from '../bindings/kinh-desktop/service'
import HeaderBar from './components/HeaderBar.vue'
import LoginView from './views/LoginView.vue'
import MainView from './views/MainView.vue'
import { useI18n } from './composables/useI18n'

// v3 的绑定按服务（命名空间）导出，这里解构回扁平函数，沿用原有的调用写法
const { RestoreLogin, ResolveCloseAction } = App

const { t } = useI18n()

// 登录成功后保存凭证，切换到主界面
const credential = ref(null)
// 启动阶段：先检测本地登录记录，期间不显示登录界面
const checking = ref(true)

function handleLoginSuccess(result) {
  credential.value = result
}

// ==================== 全局键盘滚动 ====================
// 滚动条已全局隐藏，滚轮不受影响；键盘上下键默认只作用于焦点所在容器，
// 点击列表项后焦点不在滚动容器上会失效，这里转发到目标所在的最近可滚动容器
const scrollKeys = new Set(['ArrowUp', 'ArrowDown', 'PageUp', 'PageDown'])

// 判断元素是否为实际可滚动的纵向容器
function isScrollable(el) {
  if (!(el instanceof HTMLElement)) return false
  const style = getComputedStyle(el)
  const canScroll = style.overflowY === 'auto' || style.overflowY === 'scroll'
  return canScroll && el.scrollHeight > el.clientHeight
}

// 目标元素向上找最近的可滚动容器；找不到则从候选主容器中挑第一个可滚动的
function findScrollTarget(from) {
  for (let node = from; node && node !== document.body; node = node.parentElement) {
    if (isScrollable(node)) return node
  }
  return [...document.querySelectorAll('.app-content, .file-list, .settings-body, .task-list')]
    .find(isScrollable) || null
}

function handleKeyScroll(e) {
  if (!scrollKeys.has(e.key)) return
  const target = e.target
  const tag = (target?.tagName || '').toLowerCase()
  // 输入类控件内的方向键交给默认行为（光标移动）
  if (tag === 'input' || tag === 'textarea' || tag === 'select' || target?.isContentEditable) return

  const from = (target instanceof HTMLElement && target !== document.body) ? target : document.activeElement
  const el = findScrollTarget(from instanceof HTMLElement ? from : document.body)
  if (!el) return
  e.preventDefault()
  const page = el.clientHeight * 0.85
  const delta = e.key === 'ArrowUp' ? -80 : e.key === 'ArrowDown' ? 80 : e.key === 'PageUp' ? -page : page
  el.scrollBy({ top: delta, behavior: 'smooth' })
}

// ==================== 关闭行为询问 ====================
// close_action 未设置时后端会拦截关闭并发出该事件，这里弹窗收集选择后回传。
// 选项与设置页保持一致：确认=最小化到托盘，取消=退出程序，
// 右上角关闭或 Esc 视为取消本次关闭（distinguishCancelAndClose 用于区分后两者）
let closing = false

function setupCloseConfirm() {
  Events.On('close-action-request', async () => {
    // 弹窗未关闭时忽略重复请求，避免连点关闭叠出多层弹窗
    if (closing) return
    closing = true
    try {
      await ElMessageBox.confirm(
        t('close_confirm_message', '请选择点击关闭按钮后的行为，可在设置中固定为默认。'),
        t('close_confirm_title', '关闭窗口'),
        {
          confirmButtonText: t('close_confirm_tray', '最小化到托盘'),
          cancelButtonText: t('close_confirm_exit', '退出程序'),
          distinguishCancelAndClose: true,
          type: 'info',
        }
      )
      await ResolveCloseAction('tray', false)
    } catch (action) {
      if (action === 'cancel') {
        await ResolveCloseAction('exit', false)
      }
    } finally {
      closing = false
    }
  })
}

// 启动时尝试用本地保存的登录信息自动登录，无记录才进入登录页
onMounted(async () => {
  window.addEventListener('keydown', handleKeyScroll)
  setupCloseConfirm()
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
        <span>{{ t('boot_checking', '正在检测登录状态...') }}</span>
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
  /* 100% 而非 100vw：移动端 100vw 会把滚动条宽度算进去，导致横向溢出 */
  width: 100%;
  /* 100dvh：移动端地址栏收缩/展开时不跳动；不支持 dvh 的内核回落到 100vh */
  height: 100vh;
  height: 100dvh;
  display: flex;
  flex-direction: column;
  background-color: #f5f7fa;
  /* 字体走全局 --app-font-family（style.css 定义，body 已继承），不再单独声明 */
  user-select: none;
}

.app-content {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  display: flex;
  flex-direction: column;
  /* 移动端底部补足安全区，避免内容被手势条遮挡（桌面端 env 为 0，无影响） */
  padding-bottom: env(safe-area-inset-bottom, 0px);
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
