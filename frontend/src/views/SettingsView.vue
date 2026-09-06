<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Folder, RefreshRight, Link } from '@element-plus/icons-vue'
import {
  GetSettings, SaveSettings, GetALLLang, OpenFolderSelect,
  GetAppVersion, CheckUpdate, GetOShinDVersion, CheckOShinDUpdate
} from '../../wailsjs/go/service/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { useI18n } from '../composables/useI18n'

const { t, loadTextMap } = useI18n()

// 表单数据（从后端加载后双向绑定）
const form = reactive({
  language: 'zh-CN',
  close_action: 'exit',
  download_user_agent: '',
  download_threads: 4,
  download_dir: '',
  download_acc_link: '',
  log_level: 'info'
})
const loading = ref(false)
const saving = ref(false)

// 基线快照：加载/保存成功后的设置值，用于脏检测与恢复
let baseline = ''
// 是否有未保存的修改（供父组件切换页面前查询）
const isDirty = ref(false)

// 将表单转为可比对字符串（键序固定，避免对象键序差异误判）
function serializeForm() {
  return JSON.stringify([
    form.language,
    form.close_action,
    form.download_user_agent,
    form.download_threads,
    form.download_dir,
    form.download_acc_link,
    form.log_level,
  ])
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
// 默认 UA（恢复默认按钮使用，取后端返回的初始值）
const defaultUA = ref('')

// 日志等级与关闭行为选项
const logLevelOptions = [
  { value: 'debug', label: 'DEBUG' },
  { value: 'info', label: 'INFO' },
  { value: 'warn', label: 'WARN' },
  { value: 'error', label: 'ERROR' },
]
const closeActionOptions = computed(() => [
  { value: 'exit', label: t('settings_close_exit', '退出程序') },
  { value: 'tray', label: t('settings_close_tray', '最小化到托盘') },
])

async function loadSettings() {
  loading.value = true
  try {
    const [settings, langs] = await Promise.all([GetSettings(), GetALLLang()])
    Object.assign(form, settings)
    defaultUA.value = settings.download_user_agent
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
    ElMessage.error(t('settings_save_failed', '设置保存失败') + ': ' + String(err))
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
      if (oldBaseline && oldBaseline[0] !== form.language) {
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
  if (defaultUA.value) form.download_user_agent = defaultUA.value
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

// ==================== OShinD 下载组件（预留） ====================
const oshind = ref({ installed: false, version: 'not_installed', repo_url: '' })
const checkingOshind = ref(false)
const oshindResult = ref(null)

async function handleCheckOshind() {
  checkingOshind.value = true
  oshindResult.value = null
  try {
    const result = await CheckOShinDUpdate()
    oshindResult.value = result
    if (!result.success) {
      ElMessage.error(t('update_check_failed', '检查更新失败') + ': ' + result.message)
    } else {
      ElMessage.success(`${t('component_update_found', '组件有新版本')}: ${result.latest_version}`)
    }
  } catch (err) {
    ElMessage.error(t('update_check_failed', '检查更新失败') + ': ' + String(err))
  } finally {
    checkingOshind.value = false
  }
}

function openOshindPage() {
  if (oshindResult.value?.page_url) {
    BrowserOpenURL(oshindResult.value.page_url)
  }
}

onMounted(() => {
  loadSettings()
  GetAppVersion().then(v => { appVersion.value = v }).catch(() => { appVersion.value = '' })
  GetOShinDVersion().then(info => { oshind.value = info }).catch(() => {})
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

        <div class="setting-row">
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

        <!-- 下载设置：目录 / UA，控件均为「无图标输入框 + 带图标按钮」统一结构 -->
        <div class="setting-row">
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

        <!-- 远程解析：加速链接服务地址，留空仅本地解析，填写后文件列表可用远程解析 -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_remote_resolve', '远程解析') }}</div>
            <div class="setting-desc">{{ t('settings_acclink', '加速链接服务地址') }}</div>
          </div>
          <div class="setting-inline">
            <el-input
              v-model="form.download_acc_link"
              :placeholder="t('settings_acclink_placeholder', '留空禁用')"
              clearable
            />
          </div>
        </div>

        <!-- 下载组件（OShinD）：版本显示 + 检查更新 -->
        <div class="setting-row">
          <div class="setting-info">
            <div class="setting-label">{{ t('settings_component', '下载组件') }}</div>
            <div class="setting-desc">{{ t('settings_component_engine', 'OShinD 下载引擎') }} · {{ oshind.installed ? oshind.version : t('settings_component_not_installed', '未安装') }}</div>
          </div>
          <div class="setting-control about-control">
            <el-button :loading="checkingOshind" @click="handleCheckOshind">
              {{ checkingOshind ? t('settings_checking', '检查中...') : t('settings_check_update', '检查更新') }}
            </el-button>
            <el-button
              v-if="oshindResult?.success && oshindResult.page_url"
              :icon="Link"
              @click="openOshindPage"
            >
              {{ t('update_view_page', '前往查看') }}
            </el-button>
          </div>
        </div>

        <!-- 组件更新提示：仅展示最新版本信息 -->
        <div v-if="oshindResult?.success" class="update-panel">
          <div class="update-title">
            {{ t('component_update_found', '组件有新版本') }}:
            <span class="update-versions">{{ oshindResult.latest_version }}</span>
          </div>
          <pre v-if="oshindResult.changelog" class="update-changelog">{{ oshindResult.changelog }}</pre>
        </div>
        <div v-else-if="oshindResult && !oshindResult.success" class="update-panel update-ok">
          {{ oshindResult.message }}
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
            <div class="setting-desc">KinhDesktop {{ appVersion }}</div>
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
