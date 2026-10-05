<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Folder, RefreshRight, Link } from '@element-plus/icons-vue'
import { Browser } from '@wailsio/runtime'
import { useI18n } from '../composables/useI18n'
import { usePlatform } from '../composables/usePlatform'
import { App } from '../../bindings/kinh-desktop/service'

// v3 的绑定按服务（命名空间）导出，这里解构回扁平函数，沿用原有的调用写法
const {
  GetSettings, SaveSettings, GetALLLang, OpenFolderSelect,
  GetAppVersion, CheckUpdate, GetOShinDVersion,
} = App

// 移动端（Android / iOS）需隐藏若干桌面专属设置项：窗口关闭行为、目录选择等
const { isMobile } = usePlatform()

// 退出登录统一交给 MainView 处理（含确认弹窗与凭证清理），此处只对外转发。
// 之所以在设置页也提供入口：竖屏布局下侧栏会转为底部导航，其中的账户卡片被隐藏，
// 若只保留侧栏那个按钮，竖屏用户将无法退出登录
const emit = defineEmits(['logout'])

// v2 的 runtime.BrowserOpenURL 在 v3 中对应 @wailsio/runtime 的 Browser.OpenURL
const BrowserOpenURL = Browser.OpenURL

const { t, loadTextMap } = useI18n()

// 表单数据（从后端加载后双向绑定）
const form = reactive({
  language: 'zh-CN',
  close_action: 'exit',
  download_proxy: '',
  download_user_agent: '',
  download_threads: 4,
  download_chunk_kb: 500,
  download_dir: '',
  download_acc_link: '',
  download_force_tls: true,
  download_max_retries: 3,
  download_max_active: 2,
  experimental_multi_link: false,
  experimental_channel: 'auto',
  experimental_count: 4,
  log_level: 'info'
})

// 内置默认 UA（与后端 DefaultUserAgent 一致），恢复默认按钮使用
const builtInDefaultUA = 'netdisk;DL'
const loading = ref(false)
const saving = ref(false)

// 加速链接是否已配置：未配置时多地址下载禁用「加速链接」渠道选项，
// 且默认渠道强制回落本地解析
const accLinkConfigured = computed(() => !!form.download_acc_link)

// 基线快照：加载/保存成功后的设置值，用于脏检测与恢复
let baseline = ''
// 是否有未保存的修改（供父组件切换页面前查询）
const isDirty = ref(false)

// 将表单转为可比对字符串。
// 用对象字面量而非数组：数组形式下 JSON.parse 得到的是 Array，
// Object.assign(form, array) 会把下标 "0".."14" 赋给 form 而非真实字段名，
// 导致 discardChanges 恢复失败、checkDirty 永远为 true。
// 键序由字面量书写顺序固定，JSON.stringify 保证输出稳定。
function serializeForm() {
  return JSON.stringify({
    language: form.language,
    close_action: form.close_action,
    download_proxy: form.download_proxy,
    download_user_agent: form.download_user_agent,
    download_threads: form.download_threads,
    download_chunk_kb: form.download_chunk_kb,
    download_dir: form.download_dir,
    download_acc_link: form.download_acc_link,
    download_force_tls: form.download_force_tls,
    download_max_retries: form.download_max_retries,
    download_max_active: form.download_max_active,
    experimental_multi_link: form.experimental_multi_link,
    experimental_channel: form.experimental_channel,
    experimental_count: form.experimental_count,
    log_level: form.log_level,
  })
}

// 表单与基线比对，刷新脏标记
function refreshDirty() {
  isDirty.value = serializeForm() !== baseline
}

// 供父组件读取：是否有未保存修改
function checkDirty() {
  return isDirty.value
}

// 供父组件「不保存离开」时恢复表单：未变更时静默返回
function discardChanges() {
  if (baseline) {
    Object.assign(form, JSON.parse(baseline))
  }
  refreshDirty()
}

// 供父组件「保存并离开」时调用：返回保存是否成功
async function saveAndReturn() {
  await handleSave()
  return !isDirty.value
}

// 任意控件改动实时刷新脏标记（加载/保存时的程序性赋值也会触发，但结果一致无副作用）
watch(form, refreshDirty, { deep: true })

// 语言选项（后端扫描 lang 目录得到）
const langOptions = ref([])

// 日志等级与关闭行为选项
const logLevelOptions = [
  { value: 'debug', label: 'DEBUG' },
  { value: 'info', label: 'INFO' },
  { value: 'warn', label: 'WARN' },
  { value: 'error', label: 'ERROR' },
]
const closeActionOptions = computed(() => [
  { value: 'ask', label: t('settings_close_ask', '每次询问') },
  { value: 'exit', label: t('settings_close_exit', '退出程序') },
  { value: 'tray', label: t('settings_close_tray', '最小化到托盘') },
])

async function loadSettings() {
  loading.value = true
  try {
    const [settings, langs] = await Promise.all([GetSettings(), GetALLLang()])
    Object.assign(form, settings)
    // 旧设置文件无分片字段（0/undefined）时回退默认值
    if (!form.download_chunk_kb) form.download_chunk_kb = 500
    // 旧设置文件无重试字段时回退默认值（后端已落默认 3，此处兜底 undefined）
    if (form.download_max_retries === undefined || form.download_max_retries === null) {
      form.download_max_retries = 3
    }
    // 旧设置文件无并发上限字段时回退默认值
    if (form.download_max_active === undefined || form.download_max_active === null) {
      form.download_max_active = 2
    }
    // 旧设置文件无实验性字段时回退默认值（默认关、auto 渠道、4 个地址）
    if (form.experimental_multi_link === undefined || form.experimental_multi_link === null) {
      form.experimental_multi_link = false
    }
    if (form.experimental_channel === undefined || form.experimental_channel === null) {
      form.experimental_channel = 'auto'
    }
    if (form.experimental_count === undefined || form.experimental_count === null) {
      form.experimental_count = 4
    }
    // 旧设置文件无强制 TLS 字段时回退默认值（默认开）
    if (form.download_force_tls === undefined || form.download_force_tls === null) {
      form.download_force_tls = true
    }
    langOptions.value = (langs || []).map(l => ({
      value: l.language_code,
      label: `${l.language_name} (${l.language_code})`
    }))
    if (langOptions.value.length === 0) {
      langOptions.value = [{ value: 'zh-CN', label: '简体中文 (zh-CN)' }]
    }
    baseline = serializeForm()
    refreshDirty()
  } catch (err) {
    ElMessage.error(t('settings_load_failed', '读取设置失败') + ': ' + String(err))
  } finally {
    loading.value = false
  }
}

async function handleSave() {
  saving.value = true
  try {
    const err = await SaveSettings({ ...form })
    if (err) {
      ElMessage.error(t('settings_save_failed', '设置保存失败') + ': ' + err)
    } else {
      // 语言变化时重载语言包，全页面文案热切换（无需重启）
      const oldBaseline = baseline ? JSON.parse(baseline) : null
      if (oldBaseline && oldBaseline.language !== form.language) {
        await loadTextMap()
      }
      baseline = serializeForm()
      refreshDirty()
      ElMessage.success(t('settings_save_success', '设置已保存'))
    }
  } catch (err) {
    ElMessage.error(t('settings_save_failed', '设置保存失败') + ': ' + String(err))
  } finally {
    saving.value = false
  }
}

function handleChooseDir() {
  OpenFolderSelect().then(dir => {
    if (dir) form.download_dir = dir
  })
}

function handleResetUA() {
  form.download_user_agent = builtInDefaultUA
}

// ==================== 版本与检查更新 ====================
const appVersion = ref('')
const checking = ref(false)
const updateResult = ref(null)

async function handleCheckUpdate() {
  checking.value = true
  updateResult.value = null
  try {
    const result = await CheckUpdate()
    updateResult.value = result
    if (!result.success) {
      ElMessage.error(t('update_check_failed', '检查更新失败') + ': ' + result.message)
    } else if (result.has_update) {
      ElMessage.success(`${t('update_found', '发现新版本')}: ${result.latest_version}`)
    } else {
      ElMessage.info(t('update_latest', '当前已是最新版本'))
    }
  } catch (err) {
    ElMessage.error(t('update_check_failed', '检查更新失败') + ': ' + String(err))
  } finally {
    checking.value = false
  }
}

function openReleasePage() {
  if (updateResult.value?.page_url) {
    BrowserOpenURL(updateResult.value.page_url)
  }
}

// ==================== OShinD 下载引擎 ====================
// 引擎已随主程序编译，不再有「未安装 / 单独更新」的概念，这里仅展示版本
const oshindVersion = ref('')

onMounted(() => {
  loadSettings()
  GetAppVersion().then(v => { appVersion.value = v }).catch(() => { appVersion.value = '' })
  GetOShinDVersion().then(info => { oshindVersion.value = info?.version || '' }).catch(() => {})
})

// 暴露给父组件：脏检测、放弃修改、保存（切换页面拦截用）
defineExpose({ checkDirty, discardChanges, saveAndReturn })
</script>

<template>
  <section v-loading="loading" class="settings-page">
    <!-- 顶部工具条：与「我的文件」的 files-header 同构（左标题 + 右操作） -->
    <header class="settings-header">
      <span class="page-title">{{ t('page_settings', '设置') }}</span>
      <el-button :loading="saving" @click="handleSave">
        {{ t('settings_save', '保存设置') }}
      </el-button>
    </header>

    <div class="settings-body">
      <!-- 程序设置 -->
      <div class="settings-card">
        <div class="settings-card-header">
          <span class="card-title">{{ t('settings_program', '程序') }}</span>
          <span class="card-desc">{{ t('settings_program_desc', '') }}</span>
        </div>

        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_lang', '语言') }}</div>
            <div class="setting-desc">{{ t('settings_lang_desc', '') }}</div>
          </div>
          <el-select v-model="form.language" class="setting-control" style="width: 200px">
            <el-option v-for="opt in langOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </div>

        <!-- 窗口关闭行为：移动端没有窗口边框，该设置无意义 -->
        <div v-if="!isMobile" class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_close_action', '点击关闭后的操作') }}</div>
            <div class="setting-desc">{{ t('settings_close_action_desc', '') }}</div>
          </div>
          <el-radio-group v-model="form.close_action" class="setting-control">
            <el-radio v-for="opt in closeActionOptions" :key="opt.value" :value="opt.value">
              {{ opt.label }}
            </el-radio>
          </el-radio-group>
        </div>

        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_log_level', '日志等级') }}</div>
            <div class="setting-desc">{{ t('settings_log_level_desc', '') }}</div>
          </div>
          <el-select v-model="form.log_level" class="setting-control" style="width: 140px">
            <el-option v-for="opt in logLevelOptions" :key="opt.value" :label="opt.label" :value="opt.value" />
          </el-select>
        </div>
      </div>

      <!-- 下载设置 -->
      <div class="settings-card">
        <div class="settings-card-header">
          <span class="card-title">{{ t('settings_download', '下载设置') }}</span>
          <span class="card-desc">{{ t('settings_download_desc', '') }}</span>
        </div>

        <!-- 下载目录：移动端没有可用的目录选择器（系统只提供 SAF，尚未接入），隐藏该项 -->
        <div v-if="!isMobile" class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_dir', '下载目录') }}</div>
            <div class="setting-desc">{{ t('settings_dir_desc', '') }}</div>
          </div>
          <div class="setting-inline">
            <el-input v-model="form.download_dir" readonly />
            <el-button :icon="Folder" @click="handleChooseDir">{{ t('settings_dir_choose', '选择目录') }}</el-button>
          </div>
        </div>

        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_ua', '默认 User-Agent') }}</div>
            <div class="setting-desc">{{ t('settings_ua_desc', '') }}</div>
          </div>
          <div class="setting-inline">
            <el-input v-model="form.download_user_agent" />
            <el-button :icon="RefreshRight" @click="handleResetUA">
              {{ t('settings_ua_reset', '恢复默认') }}
            </el-button>
          </div>
        </div>

        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_threads', '下载线程数') }}</div>
            <div class="setting-desc">{{ t('settings_threads_desc', '') }}</div>
          </div>
          <el-input-number
            v-model="form.download_threads"
            class="setting-control"
            :min="1"
            :max="64"
            :step="1"
            step-strictly
          />
        </div>

        <!-- 分片大小：KB 单位，组件侧限制 64KB ~ 1GB -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_chunk', '分片大小') }}</div>
            <div class="setting-desc">{{ t('settings_chunk_desc', '') }}</div>
          </div>
          <el-input-number
            v-model="form.download_chunk_kb"
            class="setting-control"
            :min="64"
            :max="1048576"
            :step="100"
            step-strictly
          />
        </div>

        <!-- 失败自动重试：下载失败时自动恢复任务的次数，0 为不重试 -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_max_retries', '失败自动重试次数') }}</div>
            <div class="setting-desc">{{ t('settings_max_retries_desc', '下载失败后自动恢复任务的次数，0 为不自动重试') }}</div>
          </div>
          <el-input-number
            v-model="form.download_max_retries"
            class="setting-control"
            :min="0"
            :max="10"
            :step="1"
            step-strictly
          />
        </div>

        <!-- 同时下载文件数：超出的任务排队等待，完成一个自动开始下一个 -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_max_active', '同时下载文件数') }}</div>
            <div class="setting-desc">{{ t('settings_max_active_desc', '超出上限的任务会排队等待，有任务完成后自动开始下一个') }}</div>
          </div>
          <el-input-number
            v-model="form.download_max_active"
            class="setting-control"
            :min="1"
            :max="10"
            :step="1"
            step-strictly
          />
        </div>

        <!-- 强制启用 TLS：本地解析出的 http 最终地址升级为 https（不影响手动新建任务） -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_force_tls', '强制启用 TLS') }}</div>
            <div class="setting-desc">{{ t('settings_force_tls_desc', '将本地解析出的非 https 下载地址升级为 https。仅作用于网盘文件解析，不影响手动新建的任务') }}</div>
          </div>
          <el-switch v-model="form.download_force_tls" />
        </div>

        <!-- 实验性下载：文件列表多一个下载按钮，对选定渠道并发发起多次地址解析 -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_multi_link', '实验性下载') }}</div>
            <div class="setting-desc">{{ t('settings_multi_link_desc', '对同一渠道并发发起多次解析以获取多个下载地址，从多个地址同时拉取同一文件。功能尚在实验阶段') }}</div>
          </div>
          <el-switch v-model="form.experimental_multi_link" />
        </div>

        <template v-if="form.experimental_multi_link">
          <!-- 地址渠道：无加速链接时「加速链接」不可选 -->
          <div class="setting-row">
            <div class="setting-info">
              <div class="setting-label">{{ t('settings_multi_channel', '地址渠道') }}</div>
              <div class="setting-desc">{{ accLinkConfigured
                ? t('settings_multi_channel_desc', '并发解析使用的地址来源')
                : t('settings_multi_channel_no_acc', '未配置加速链接，仅可使用本地解析') }}</div>
            </div>
            <el-select
              v-model="form.experimental_channel"
              class="setting-control"
              style="width: 140px"
            >
              <el-option value="auto" :label="t('settings_multi_channel_auto', '程序决定')" />
              <el-option
                value="remote"
                :label="t('settings_multi_channel_remote', '加速链接')"
                :disabled="!accLinkConfigured"
              />
              <el-option value="local" :label="t('settings_multi_channel_local', '本地解析')" />
            </el-select>
          </div>

          <!-- 获取地址数量 -->
          <div class="setting-row">
            <div class="setting-info">
              <div class="setting-label">{{ t('settings_multi_count', '获取地址数量') }}</div>
              <div class="setting-desc">{{ t('settings_multi_count_desc', '并发发起的地址解析次数，地址按渠道实际返回为准') }}</div>
            </div>
            <el-input-number
              v-model="form.experimental_count"
              class="setting-control"
              :min="2"
              :max="6"
              :step="1"
              step-strictly
            />
          </div>
        </template>

        <!-- 下载代理：留空不启用（⚠️ 当前引擎不支持代理配置，设置后不生效） -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_proxy', '下载代理') }}</div>
            <div class="setting-desc">{{ t('settings_proxy_desc', '') }}</div>
          </div>
          <div class="setting-inline">
            <el-input
              v-model="form.download_proxy"
              :placeholder="t('settings_proxy_placeholder', '留空不启用')"
              clearable
            />
          </div>
        </div>

        <!-- 远程解析：加速链接服务地址，留空仅本地解析，填写后文件列表可用远程解析 -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_remote_resolve', '远程解析') }}</div>
            <div class="setting-desc">{{ t('settings_acclink', '加速链接服务地址') }}</div>
          </div>
          <div class="setting-inline">
            <el-input
              v-model="form.download_acc_link"
              :placeholder="t('settings_acclink_placeholder', '留空不启用')"
              clearable
            />
          </div>
        </div>

        <!-- 下载组件（OShinD）：引擎随主程序编译，仅展示版本，没有独立更新入口 -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_component', '下载组件') }}</div>
            <div class="setting-desc">{{ t('settings_component_engine', 'OShinD 下载引擎') }} · {{ oshindVersion || t('settings_component_unknown', '未知') }}</div>
          </div>
        </div>
      </div>

      <!-- 关于 -->
      <div class="settings-card">
        <div class="settings-card-header">
          <span class="card-title">{{ t('settings_about', '关于') }}</span>
        </div>

        <div class="setting-row about-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_version', '当前版本') }}</div>
            <div class="setting-desc">KinhDesktop {{ appVersion ? 'v' + appVersion : '' }}</div>
          </div>
          <div class="setting-control about-control">
            <el-button :loading="checking" @click="handleCheckUpdate">
              {{ checking ? t('settings_checking', '检查中...') : t('settings_check_update', '检查更新') }}
            </el-button>
            <el-button
              v-if="updateResult?.success && updateResult.has_update"
              :icon="Link"
              @click="openReleasePage"
            >
              {{ t('update_view_page', '前往查看') }}
            </el-button>
          </div>
        </div>

        <!-- 更新提示区：有新版本显示版本号与更新说明，无更新显示结论 -->
        <div v-if="updateResult?.success && updateResult.has_update" class="update-panel">
          <div class="update-title">
            {{ t('update_found', '发现新版本') }}:
            <span class="update-versions">{{ updateResult.current_version }} → {{ updateResult.latest_version }}</span>
          </div>
          <pre v-if="updateResult.changelog" class="update-changelog">{{ updateResult.changelog }}</pre>
        </div>
        <div v-else-if="updateResult?.success && !updateResult.has_update" class="update-panel update-ok">
          {{ t('update_latest', '当前已是最新版本') }}
        </div>

        <!-- 退出登录：与侧栏账户卡片同源，全平台保留此入口
             （竖屏下侧栏转为底部导航，账户卡片被隐藏，此处是唯一出口） -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('btn_logout', '退出登录') }}</div>
            <div class="setting-desc">{{ t('settings_logout_desc', '退出当前账号并清除本地保存的登录信息。') }}</div>
          </div>
          <div class="setting-control">
            <el-button type="danger" plain @click="emit('logout')">
              {{ t('btn_logout', '退出登录') }}
            </el-button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.settings-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}

/* 顶部工具条：白底，与文件页 files-header 同构 */
.settings-header {
  min-height: 44px;
  padding: 6px 16px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-shrink: 0;
}

.page-title {
  font-size: 14px;
  color: #303133;
}

.settings-body {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 设置卡片 */
.settings-card {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 16px 20px;
  flex-shrink: 0;
}

.settings-card-header {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f0f0f0;
  margin-bottom: 4px;
}

.card-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.card-desc {
  font-size: 12px;
  color: #909399;
}

/* 单行设置项：左信息右控件 */
.setting-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  padding: 14px 0;
}

.setting-row + .setting-row {
  border-top: 1px solid #f5f7fa;
}

.setting-info {
  flex: 1;
  min-width: 0;
}

.setting-label {
  font-size: 13px;
  color: #303133;
}

.setting-desc {
  font-size: 12px;
  color: #909399;
  margin-top: 2px;
}

.setting-control {
  flex-shrink: 0;
}

/* 横向「输入框 + 按钮」组合控件（目录/UA 共用） */
.setting-inline {
  display: flex;
  gap: 8px;
  width: 420px;
  max-width: 55%;
}

/* 关于行 */
.about-row {
  border-top: none;
}

.about-control {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* 更新结果面板 */
.update-panel {
  margin-top: 8px;
  padding: 12px 14px;
  border-radius: 6px;
  background: #f0f9eb;
  border: 1px solid #e1f3d8;
  font-size: 12px;
  color: #303133;
}

.update-panel.update-ok {
  background: #f4f4f5;
  border-color: #e9e9eb;
  color: #606266;
}

.update-title {
  font-size: 13px;
  font-weight: 600;
}

.update-versions {
  color: #409eff;
}

.update-changelog {
  margin: 8px 0 0;
  padding: 8px 10px;
  background: #fff;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  max-height: 180px;
  overflow-y: auto;
  font-family: inherit;
}

/* 底部保存栏已移除，保存按钮置于顶部工具条 */
</style>
