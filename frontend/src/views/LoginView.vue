<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { RefreshRight } from '@element-plus/icons-vue'
import { useI18n } from '../composables/useI18n'
import { usePlatform } from '../composables/usePlatform'
import { useViewport } from '../composables/useViewport'
import { App } from '../../bindings/kinh-desktop/service'

// v3 的绑定按服务（命名空间）导出，这里解构回扁平函数，沿用原有的调用写法
const {
  GetBaiduQR,
  PollBaiduQR,
  BaiduQRLogin,
  LoginWithCookie,
  SendBaiduSMSCode,
  LoginWithBaiduSMS,
} = App

const { t } = useI18n()

const { isMobile } = usePlatform()
const { isSmallScreen } = useViewport()

// 手机端隐藏扫码登录：扫码需要另一台设备配合，在手机上没意义。
// 判定条件是「移动平台 + 小屏」——平板等大屏移动设备短边 >= 600，扫码照常显示。
const showQR = computed(() => !(isMobile.value && isSmallScreen.value))

// 登录成功后向父组件抛出凭证
const emit = defineEmits(['login-success'])

// 当前视图：qr（扫码）| sms（手机号）| cookie
// 默认扫码；手机端扫码被隐藏时会由 watch 切到手机号登录
const activeTab = ref('qr')

// 记住登录：登录成功后保存登录信息到本地（默认勾选）
const rememberLogin = ref(true)

// ==================== 扫码登录 ====================
const qrImage = ref('')
const qrSign = ref('')
const qrLoading = ref(false)      // 获取二维码中
const qrPolling = ref(false)      // 轮询进行中
const loginLoading = ref(false)   // 凭证换取中
const loginResult = ref(null)     // 登录成功后的结果

const pollTimer = ref(null)
// 连续网络错误计数，超过上限停止轮询（防止静默死循环）
const MAX_NETWORK_ERRORS = 3
const networkErrorCount = ref(0)

// 扫码状态文案
const qrStatusText = ref('loading')
// loading | waiting | scanned | success | error

async function loadQR() {
  stopPolling()
  qrImage.value = ''
  qrSign.value = ''
  loginResult.value = null
  networkErrorCount.value = 0
  qrStatusText.value = 'loading'
  qrLoading.value = true
  try {
    const qr = await GetBaiduQR()
    qrImage.value = qr.qr_base64
    qrSign.value = qr.sign
    qrStatusText.value = 'waiting'
    startPolling()
  } catch (err) {
    qrStatusText.value = 'error'
    ElMessage.error(t('qr_load_failed', '二维码获取失败') + ': ' + String(err))
  } finally {
    qrLoading.value = false
  }
}

function startPolling() {
  stopPolling()
  qrPolling.value = true
  pollTimer.value = setInterval(async () => {
    if (!qrSign.value || loginResult.value) {
      stopPolling()
      return
    }
    try {
      const res = await PollBaiduQR(qrSign.value)
      if (res.status === 'scanned') {
        networkErrorCount.value = 0
        qrStatusText.value = 'scanned'
      } else if (res.status === 'success') {
        stopPolling()
        await doQRLogin(res.v)
      } else if (res.status === 'error') {
        stopPolling()
        qrStatusText.value = 'error'
      } else if (res.status === 'network_error') {
        networkErrorCount.value += 1
        if (networkErrorCount.value >= MAX_NETWORK_ERRORS) {
          stopPolling()
          qrStatusText.value = 'error'
          ElMessage.error(t('qr_network_error', '网络异常，扫码轮询已停止，请刷新二维码重试'))
        }
      }
    } catch (err) {
      // 单次轮询异常不中断，连续失败由 network_error 分支兜底
      console.warn('轮询失败:', err)
    }
  }, 2000)
}

function stopPolling() {
  if (pollTimer.value) {
    clearInterval(pollTimer.value)
    pollTimer.value = null
  }
  qrPolling.value = false
}

async function doQRLogin(v) {
  qrStatusText.value = 'success'
  loginLoading.value = true
  try {
    const result = await BaiduQRLogin(v, rememberLogin.value)
    if (result && result.success) {
      loginResult.value = result
      ElMessage.success(t('login_success', '登录成功'))
      emit('login-success', result)
    } else {
      qrStatusText.value = 'error'
      ElMessage.error((result && result.message) || t('login_failed', '登录失败'))
    }
  } catch (err) {
    qrStatusText.value = 'error'
    ElMessage.error(String(err))
  } finally {
    loginLoading.value = false
  }
}

// ==================== Cookie 登录 ====================
const cookieForm = ref({ bduss: '', ptoken: '' })
const cookieLoading = ref(false)

async function handleCookieLogin() {
  if (!cookieForm.value.bduss.trim() || !cookieForm.value.ptoken.trim()) {
    ElMessage.warning(t('cookie_required', '请填写完整的 BDUSS 和 PTOKEN'))
    return
  }
  cookieLoading.value = true
  try {
    const result = await LoginWithCookie(cookieForm.value.bduss.trim(), cookieForm.value.ptoken.trim(), rememberLogin.value)
    if (result && result.success) {
      loginResult.value = result
      ElMessage.success(t('login_success', '登录成功'))
      emit('login-success', result)
    } else {
      ElMessage.error((result && result.message) || t('login_failed', '登录失败'))
    }
  } catch (err) {
    ElMessage.error(String(err))
  } finally {
    cookieLoading.value = false
  }
}

// ==================== 手机号登录 ====================
const smsForm = ref({ phone: '', code: '' })
const smsSending = ref(false)
const smsLoading = ref(false)
const smsCountdown = ref(0)
// 图形验证码：仅在百度风控要求时出现，由发送接口回传
const vcode = ref({ need: false, image: '', input: '', str: '', sign: '' })

let countdownTimer = null

function startCountdown(seconds) {
  clearInterval(countdownTimer)
  smsCountdown.value = seconds
  countdownTimer = setInterval(() => {
    smsCountdown.value -= 1
    if (smsCountdown.value <= 0) {
      clearInterval(countdownTimer)
      countdownTimer = null
    }
  }, 1000)
}

async function handleSendSMS() {
  const phone = smsForm.value.phone.trim()
  if (!/^1\d{10}$/.test(phone)) {
    ElMessage.warning(t('sms_phone_invalid', '请输入正确的手机号'))
    return
  }
  if (vcode.value.need && !vcode.value.input.trim()) {
    ElMessage.warning(t('sms_vcode_required', '请输入图片验证码'))
    return
  }

  smsSending.value = true
  try {
    const res = await SendBaiduSMSCode(
      phone,
      vcode.value.input.trim(),
      vcode.value.str,
      vcode.value.sign
    )
    if (!res) return

    if (res.need_vcode) {
      // 风控要求图形验证码：展示图片并保留本轮凭证，用户填完再点一次
      vcode.value.need = true
      vcode.value.image = res.vcode_image
      vcode.value.str = res.vcode_str
      vcode.value.sign = res.vcode_sign
      vcode.value.input = ''
      ElMessage.warning(res.message || t('sms_need_vcode', '需要图片验证码，请填写后重试'))
      return
    }

    if (res.success) {
      vcode.value.need = false
      ElMessage.success(t('sms_sent', '验证码已发送'))
      startCountdown(60)
    } else {
      ElMessage.error(res.message || t('sms_send_failed', '验证码发送失败'))
    }
  } catch (err) {
    ElMessage.error(String(err))
  } finally {
    smsSending.value = false
  }
}

async function handleSMSLogin() {
  const phone = smsForm.value.phone.trim()
  const code = smsForm.value.code.trim()
  if (!/^1\d{10}$/.test(phone)) {
    ElMessage.warning(t('sms_phone_invalid', '请输入正确的手机号'))
    return
  }
  if (!code) {
    ElMessage.warning(t('sms_code_required', '请填写短信验证码'))
    return
  }

  smsLoading.value = true
  try {
    const result = await LoginWithBaiduSMS(phone, code, rememberLogin.value)
    if (result && result.success) {
      loginResult.value = result
      ElMessage.success(t('login_success', '登录成功'))
      emit('login-success', result)
    } else {
      ElMessage.error((result && result.message) || t('login_failed', '登录失败'))
    }
  } catch (err) {
    ElMessage.error(String(err))
  } finally {
    smsLoading.value = false
  }
}

// 扫码被隐藏后（平台/尺寸判定异步就绪），停在扫码页时切到手机号登录并停掉轮询
watch(showQR, (visible) => {
  if (visible) return
  stopPolling()
  if (activeTab.value === 'qr') {
    activeTab.value = 'sms'
  }
})

onMounted(() => {
  // 扫码不可用时不必发二维码请求，省掉一次网络与后续轮询
  if (showQR.value) {
    loadQR()
  } else if (activeTab.value === 'qr') {
    activeTab.value = 'sms'
  }
})

onBeforeUnmount(() => {
  stopPolling()
  clearInterval(countdownTimer)
})
</script>

<template>
  <div class="login-wrapper">
    <el-card class="login-card" shadow="always">
      <div class="login-header">
        <h2 class="login-title">{{ t('login_title', '登录百度账号') }}</h2>
        <p class="login-subtitle">{{ t('login_subtitle', '选择一种方式登录你的账号') }}</p>
      </div>

      <el-tabs v-model="activeTab" stretch class="login-tabs">
        <!-- 记住登录（三种登录方式共用） -->
        <div class="remember-row">
          <el-checkbox v-model="rememberLogin">
            {{ t('remember_login', '记住登录') }}
          </el-checkbox>
        </div>
        <!-- 扫码登录（手机端隐藏：需另一台设备配合，详见 showQR 说明） -->
        <el-tab-pane v-if="showQR" :label="t('tab_qr_login', '扫码登录')" name="qr">
          <div class="pane">
            <div class="qr-panel">
              <!-- 二维码区域 -->
              <div class="qr-box" v-loading="qrLoading || loginLoading">
                <img
                  v-if="qrImage && qrStatusText !== 'error'"
                  :src="qrImage"
                  class="qr-image"
                  alt="QR Code"
                />
                <div v-else-if="qrStatusText === 'error'" class="qr-error">
                  <p>{{ t('qr_expired', '二维码已过期或加载失败') }}</p>
                  <el-button type="primary" :icon="RefreshRight" @click="loadQR" round>
                    {{ t('refresh_qr', '刷新二维码') }}
                  </el-button>
                </div>
              </div>

              <!-- 状态提示 -->
              <div class="qr-status">
                <template v-if="qrStatusText === 'waiting'">
                  <span class="status-dot waiting"></span>
                  {{ t('qr_waiting', '请使用百度 APP 扫描二维码登录') }}
                </template>
                <template v-else-if="qrStatusText === 'scanned'">
                  <span class="status-dot scanned"></span>
                  {{ t('qr_scanned', '已扫码，请在手机上确认登录') }}
                </template>
                <template v-else-if="qrStatusText === 'loading'">
                  {{ t('qr_loading', '正在获取二维码...') }}
                </template>
              </div>

              <el-button
                text
                type="primary"
                :icon="RefreshRight"
                size="small"
                @click="loadQR"
              >
                {{ t('refresh_qr', '刷新二维码') }}
              </el-button>
            </div>
          </div>
        </el-tab-pane>

        <!-- 手机号登录 -->
        <el-tab-pane :label="t('tab_sms_login', '手机号登录')" name="sms">
          <div class="pane">
            <el-form class="sms-form" label-position="top" @submit.prevent="handleSMSLogin">
              <el-form-item :label="t('label_phone', '手机号')">
                <el-input
                  v-model="smsForm.phone"
                  type="tel"
                  inputmode="numeric"
                  maxlength="11"
                  :placeholder="t('placeholder_phone', '请输入手机号')"
                  clearable
                />
              </el-form-item>
              <el-form-item :label="t('label_sms_code', '短信验证码')">
                <div class="code-row">
                  <el-input
                    v-model="smsForm.code"
                    inputmode="numeric"
                    maxlength="6"
                    :placeholder="t('placeholder_sms_code', '请输入 6 位验证码')"
                    clearable
                  />
                  <el-button
                    class="code-btn"
                    :disabled="smsCountdown > 0"
                    :loading="smsSending"
                    @click="handleSendSMS"
                  >
                    {{ smsCountdown > 0 ? smsCountdown + 's' : t('btn_send_sms', '获取验证码') }}
                  </el-button>
                </div>
              </el-form-item>
              <!-- 图形验证码：仅在百度风控要求时出现 -->
              <el-form-item v-if="vcode.need" :label="t('label_img_code', '图片验证码')">
                <div class="code-row">
                  <el-input
                    v-model="vcode.input"
                    :placeholder="t('placeholder_img_code', '请输入图片验证码')"
                    clearable
                  />
                  <img
                    v-if="vcode.image"
                    :src="vcode.image"
                    class="vcode-img"
                    :alt="t('label_img_code', '图片验证码')"
                    @click="handleSendSMS"
                  />
                </div>
              </el-form-item>
              <el-button
                type="primary"
                class="login-btn"
                :loading="smsLoading"
                @click="handleSMSLogin"
              >
                {{ t('btn_login', '登 录') }}
              </el-button>
              <p class="cookie-tip">{{ t('sms_tip', '验证码将发送至该手机号') }}</p>
            </el-form>
          </div>
        </el-tab-pane>

        <!-- Cookie 登录 -->
        <el-tab-pane :label="t('tab_cookie_login', 'Cookie 登录')" name="cookie">
          <div class="pane">
            <el-form class="cookie-form" label-position="top" @submit.prevent="handleCookieLogin">
              <el-form-item :label="t('label_bduss', 'BDUSS')">
                <el-input
                  v-model="cookieForm.bduss"
                  type="password"
                  show-password
                  :placeholder="t('placeholder_bduss', '请输入 BDUSS')"
                  clearable
                />
              </el-form-item>
              <el-form-item :label="t('label_ptoken', 'PTOKEN')">
                <el-input
                  v-model="cookieForm.ptoken"
                  type="password"
                  show-password
                  :placeholder="t('placeholder_ptoken', '请输入 PTOKEN')"
                  clearable
                />
              </el-form-item>
              <el-button
                type="primary"
                class="login-btn"
                :loading="cookieLoading"
                @click="handleCookieLogin"
              >
                {{ t('btn_login', '登 录') }}
              </el-button>
              <p class="cookie-tip">{{ t('cookie_tip', '填写 BDUSS 和 PTOKEN 的值即可') }}</p>
            </el-form>
          </div>
        </el-tab-pane>
      </el-tabs>

      <!-- 登录成功 -->
      <div v-if="loginResult" class="login-success">
        <el-result
          icon="success"
          :title="t('login_success', '登录成功')"
          :sub-title="loginResult.username ? (t('welcome', '欢迎回来') + ', ' + loginResult.username) : t('logged_in', '已登录')"
        />
      </div>
    </el-card>
  </div>
</template>

<style scoped>
.login-wrapper {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 24px;
}

.login-card {
  width: 420px;
  border-radius: 12px;
}

.login-header {
  text-align: center;
  margin-bottom: 12px;
}

.login-title {
  margin: 0;
  font-size: 20px;
  font-weight: 700;
  color: #303133;
}

.login-subtitle {
  margin: 8px 0 0;
  font-size: 13px;
  color: #909399;
}

/* 统一两个 tab 的高度，切换时不引起卡片尺寸变化 */
.login-tabs :deep(.el-tabs__content) {
  overflow: hidden;
}

.remember-row {
  display: flex;
  justify-content: center;
  padding: 4px 0 0;
}

/* 统一各 tab 的最小高度，切换时不引起卡片尺寸变化。
   手机号登录命中风控时会多出「图片验证码」一行，用 min-height 允许临时撑高而不是被裁掉 */
.pane {
  min-height: 300px;
  display: flex;
  flex-direction: column;
  justify-content: flex-start;
}

.qr-panel {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 16px 0 8px;
}

.qr-box {
  width: 200px;
  height: 200px;
  display: flex;
  justify-content: center;
  align-items: center;
  border: 1px solid #ebeef5;
  border-radius: 8px;
  background: #fff;
  overflow: hidden;
}

.qr-image {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.qr-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: #909399;
  padding: 0 12px;
}

.qr-status {
  margin-top: 16px;
  font-size: 13px;
  color: #606266;
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 20px;
}

.status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.status-dot.waiting {
  background-color: #409eff;
  animation: pulse 1.5s infinite;
}

.status-dot.scanned {
  background-color: #67c23a;
  animation: pulse 1s infinite;
}

@keyframes pulse {
  0% { opacity: 1; }
  50% { opacity: 0.3; }
  100% { opacity: 1; }
}

.cookie-form,
.sms-form {
  padding: 8px 8px 0;
}

/* 输入框与附属控件（获取验证码按钮 / 图形验证码图片）同排 */
.code-row {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.code-row .el-input {
  flex: 1;
  /* 不加会在长占位文案下把按钮挤出容器 */
  min-width: 0;
}

.code-btn {
  flex: none;
  width: 116px;
}

.vcode-img {
  flex: none;
  width: 92px;
  height: 32px;
  object-fit: contain;
  background: #fff;
  border: 1px solid #ebeef5;
  border-radius: 4px;
  cursor: pointer;
}

.login-btn {
  width: 100%;
  margin-top: 4px;
}

.cookie-tip {
  margin: 12px 0 0;
  font-size: 12px;
  color: #909399;
  text-align: center;
}

.login-success {
  margin-top: 8px;
}
</style>
