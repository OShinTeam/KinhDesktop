<script setup>
import { onMounted, onUnmounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { GetDownloadTasks, CancelDownloadTask, GetSettings, GetOShinDVersion, SubmitDownloadWithOptions } from '../../wailsjs/go/service/App'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import { formatBytes } from '../utils/format'
import { useI18n } from '../composables/useI18n'
import { Plus, CircleClose } from '@element-plus/icons-vue'

const { t } = useI18n()

// OShinD 组件是否可用：不存在时显示安装引导，存在时显示任务列表 + 新建任务
const oshindInstalled = ref(false)
const oshindVersion = ref('')
const oshindRepoURL = ref('')

async function loadOshindState() {
  try {
    const info = await GetOShinDVersion()
    oshindInstalled.value = !!info?.installed
    oshindVersion.value = info?.installed ? info.version : ''
    oshindRepoURL.value = info?.repo_url || ''
  } catch {
    oshindInstalled.value = false
  }
}

function openRepoPage() {
  if (oshindRepoURL.value) {
    BrowserOpenURL(oshindRepoURL.value)
  }
}

// 任务列表轮询（组件状态由组件侧维护，前端 1s 拉取一次）
const tasks = ref([])
const taskTimer = ref(null)

async function refreshTasks() {
  if (!oshindInstalled.value) return
  try {
    tasks.value = await GetDownloadTasks() || []
  } catch {
    // 轮询失败静默，下轮重试
  }
}

function startPolling() {
  stopPolling()
  taskTimer.value = setInterval(refreshTasks, 1000)
}

function stopPolling() {
  if (taskTimer.value) {
    clearInterval(taskTimer.value)
    taskTimer.value = null
  }
}

// 状态展示映射
function statusLabel(status) {
  const map = {
    PENDING: t('task_status_pending', '等待中'),
    PROBING: t('task_status_probing', '探测中'),
    DOWNLOADING: t('task_status_downloading', '下载中'),
    RESUMING: t('task_status_resuming', '恢复中'),
    VERIFYING: t('task_status_verifying', '校验中'),
    COMPLETED: t('task_status_completed', '已完成'),
    FAILED: t('task_status_failed', '失败'),
    PAUSED: t('task_status_paused', '已暂停'),
  }
  return map[status] || status || '-'
}

function statusType(status) {
  switch (status) {
    case 'DOWNLOADING': return 'primary'
    case 'COMPLETED': return 'success'
    case 'FAILED': return 'danger'
    case 'PAUSED': return 'info'
    default: return 'warning'
  }
}

function progressOf(task) {
  const p = Number(task.progress)
  return Number.isFinite(p) ? Math.min(Math.round(p * 10) / 10, 100) : 0
}

function speedOf(task) {
  const s = Number(task.speed)
  if (!Number.isFinite(s) || s <= 0) return ''
  return formatBytes(s) + '/s'
}

function sizeOf(task) {
  const d = Number(task.downloaded) || 0
  const total = Number(task.total) || 0
  if (!total) return formatBytes(d)
  return formatBytes(d) + ' / ' + formatBytes(total)
}

const runningStatuses = ['PENDING', 'PROBING', 'DOWNLOADING', 'RESUMING', 'VERIFYING']

async function handleCancel(task) {
  try {
    const ok = await CancelDownloadTask(task.task_id)
    if (ok) {
      ElMessage.success(t('task_cancel_success', '任务已取消'))
      refreshTasks()
    } else {
      ElMessage.error(t('task_cancel_failed', '取消任务失败'))
    }
  } catch (err) {
    ElMessage.error(t('task_cancel_failed', '取消任务失败') + ': ' + String(err))
  }
}

// ==================== 新建任务（带高级选项） ====================
const showDialog = ref(false)
const submitting = ref(false)

const defaultDir = ref('')
const defaultThreads = ref(4)
const defaultProxy = ref('')

const newTask = ref({
  url: '',
  file_name: '',
  output_dir: '',
  connections: 4,
  user_agent: '',
  proxy: '',
  headers_text: '',
  checksum_type: '',
  checksum_value: '',
  skip_tls_verify: false,
})

async function loadDefaults() {
  try {
    const settings = await GetSettings()
    defaultDir.value = settings?.download_dir || ''
    defaultThreads.value = settings?.download_threads || 4
    defaultProxy.value = settings?.download_proxy || ''
    // 表单默认值：目录/线程/代理取设置，UA 留空由后端回退默认值（避免设置改动后表单残留旧值）
    newTask.value.output_dir = defaultDir.value
    newTask.value.connections = defaultThreads.value
    newTask.value.proxy = defaultProxy.value
  } catch {
    // 设置读取失败时使用空默认
  }
}

function openNewTaskDialog() {
  // 每次打开重置全部字段，目录/线程/代理恢复设置默认值，UA 留空走后端默认
  newTask.value.url = ''
  newTask.value.file_name = ''
  newTask.value.user_agent = ''
  newTask.value.headers_text = ''
  newTask.value.checksum_type = ''
  newTask.value.checksum_value = ''
  newTask.value.skip_tls_verify = false
  newTask.value.output_dir = defaultDir.value
  newTask.value.connections = defaultThreads.value
  newTask.value.proxy = defaultProxy.value
  showDialog.value = true
}

// 解析自定义 headers 文本（每行一个 "Key: Value"）
function parseHeaders(text) {
  const headers = {}
  for (const line of (text || '').split('\n')) {
    const idx = line.indexOf(':')
    if (idx <= 0) continue
    const key = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (key && value) headers[key] = value
  }
  return headers
}

async function submitNewTask() {
  if (!newTask.value.url) {
    ElMessage.warning(t('task_url_required', '请填写下载地址'))
    return
  }
  submitting.value = true
  try {
    const result = await SubmitDownloadWithOptions({
      url: newTask.value.url,
      file_name: newTask.value.file_name,
      output_dir: newTask.value.output_dir,
      connections: newTask.value.connections,
      user_agent: newTask.value.user_agent,
      proxy: newTask.value.proxy,
      headers: parseHeaders(newTask.value.headers_text),
      checksum_type: newTask.value.checksum_type || '',
      checksum_value: newTask.value.checksum_value || '',
      skip_tls_verify: newTask.value.skip_tls_verify,
    })
    if (!result?.success) {
      ElMessage.error(result?.message || t('download_submit_failed', '添加下载任务失败'))
      return
    }
    ElMessage.success(t('download_submit_success', '已添加到下载管理'))
    showDialog.value = false
    refreshTasks()
  } catch (err) {
    ElMessage.error(t('download_submit_failed', '添加下载任务失败') + ': ' + String(err))
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  loadOshindState().then(() => {
    if (oshindInstalled.value) {
      loadDefaults()
      refreshTasks()
      startPolling()
    }
  })
})

onUnmounted(stopPolling)
</script>

<template>
  <section class="downloads-page">
    <header class="downloads-header">
      <span class="page-title">{{ t('page_downloads', '下载管理') }}</span>
      <el-button
        v-if="oshindInstalled"
        type="primary"
        :icon="Plus"
        @click="openNewTaskDialog"
      >
        {{ t('task_new', '新建任务') }}
      </el-button>
    </header>

    <!-- 组件未安装：安装引导 -->
    <div v-if="!oshindInstalled" class="install-guide">
      <el-empty :description="t('oshind_not_installed', '下载组件未安装')">
        <div class="install-steps">
          <p class="install-intro">{{ t('oshind_install_intro', 'OShinD 是跨平台多线程下载引擎，安装后即可在应用内管理下载任务。') }}</p>
          <ol class="install-list">
            <li>{{ t('oshind_install_step1', '访问 OShinD 仓库 Releases 页面') }}</li>
            <li>{{ t('oshind_install_step2', '下载对应平台组件（Windows: oshind.dll）') }}</li>
            <li>{{ t('oshind_install_step3', '将组件放入程序 data 目录后重启应用') }}</li>
          </ol>
          <el-button type="primary" @click="openRepoPage">{{ t('update_view_page', '前往查看') }}</el-button>
        </div>
      </el-empty>
    </div>

    <!-- 组件已安装：任务列表 -->
    <template v-else>
      <div v-if="tasks.length === 0" class="page-placeholder">
        <el-empty :description="t('task_empty', '暂无下载任务')" />
      </div>
      <ul v-else class="task-list">
        <li v-for="task in tasks" :key="task.task_id" class="task-row">
          <div class="task-main">
            <div class="task-name" :title="task.file_name || task.url">{{ task.file_name || task.url }}</div>
            <div class="task-meta">
              <el-tag :type="statusType(task.status)" size="small">{{ statusLabel(task.status) }}</el-tag>
              <span class="task-size">{{ sizeOf(task) }}</span>
              <span v-if="speedOf(task)" class="task-speed">{{ speedOf(task) }}</span>
            </div>
            <el-progress
              :percentage="progressOf(task)"
              :show-text="false"
              :stroke-width="6"
              :status="task.status === 'FAILED' ? 'exception' : task.status === 'COMPLETED' ? 'success' : undefined"
            />
          </div>
          <div class="task-ops">
            <el-button
              v-if="runningStatuses.includes(task.status)"
              :icon="CircleClose"
              circle
              :title="t('task_cancel', '取消')"
              @click="handleCancel(task)"
            />
          </div>
        </li>
      </ul>
    </template>

    <!-- 新建任务弹窗（带高级选项） -->
    <el-dialog
      v-model="showDialog"
      :title="t('task_new', '新建任务')"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form label-position="top" class="task-form">
        <el-form-item :label="t('task_url', '下载地址')" required>
          <el-input v-model="newTask.url" placeholder="https://" clearable />
        </el-form-item>
        <el-form-item :label="t('task_file_name', '文件名')">
          <el-input v-model="newTask.file_name" :placeholder="t('task_file_name_hint', '留空自动识别')" clearable />
        </el-form-item>

        <el-collapse class="task-advanced">
          <el-collapse-item :title="t('task_advanced', '高级选项')">
            <el-form-item :label="t('settings_dir', '下载目录')">
              <el-input v-model="newTask.output_dir" :placeholder="t('task_use_default_dir', '留空使用默认目录')" clearable />
            </el-form-item>
            <el-form-item :label="t('settings_threads', '下载线程数')">
              <el-input-number v-model="newTask.connections" :min="1" :max="64" :step="1" step-strictly />
            </el-form-item>
            <el-form-item :label="t('settings_ua', '默认 User-Agent')">
              <el-input v-model="newTask.user_agent" :placeholder="t('task_use_default_ua', '留空使用默认 UA')" clearable />
            </el-form-item>
            <el-form-item :label="t('settings_proxy', '下载代理')">
              <el-input v-model="newTask.proxy" :placeholder="t('settings_proxy_placeholder', '留空不启用')" clearable />
            </el-form-item>
            <el-form-item :label="t('task_custom_headers', '自定义请求头')">
              <el-input
                v-model="newTask.headers_text"
                type="textarea"
                :rows="3"
                :placeholder="t('task_custom_headers_hint', '每行一个，格式：Key: Value')"
              />
            </el-form-item>
            <el-form-item :label="t('task_checksum', '文件校验')">
              <div class="task-checksum">
                <el-select v-model="newTask.checksum_type" class="checksum-type" clearable :placeholder="t('task_checksum_type', '类型')">
                  <el-option label="MD5" value="md5" />
                  <el-option label="SHA256" value="sha256" />
                </el-select>
                <el-input v-model="newTask.checksum_value" :placeholder="t('task_checksum_value', '校验值（留空跳过校验）')" clearable />
              </div>
            </el-form-item>
            <el-form-item>
              <el-checkbox v-model="newTask.skip_tls_verify">
                {{ t('task_skip_tls', '跳过 TLS 证书验证') }}
              </el-checkbox>
            </el-form-item>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">{{ t('logout_confirm_cancel', '取消') }}</el-button>
        <el-button type="primary" :loading="submitting" @click="submitNewTask">
          {{ t('task_submit', '开始下载') }}
        </el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style scoped>
.downloads-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}

/* 顶部工具条：与文件页/设置页同构 */
.downloads-header {
  min-height: 44px;
  padding: 6px 16px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
}

.page-title {
  font-size: 14px;
  color: #303133;
}

.page-placeholder {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: center;
}

/* 安装引导 */
.install-guide {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: center;
}

.install-steps {
  max-width: 420px;
  text-align: center;
  display: flex;
  flex-direction: column;
  gap: 10px;
  align-items: center;
}

.install-intro {
  margin: 0;
  font-size: 13px;
  color: #606266;
  line-height: 1.6;
}

.install-list {
  margin: 0;
  padding-left: 20px;
  text-align: left;
  font-size: 13px;
  color: #909399;
  line-height: 1.8;
}

/* 任务列表 */
.task-list {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  margin: 0;
  padding: 16px 20px;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-row {
  background: #fff;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  padding: 14px 16px;
  display: flex;
  align-items: center;
  gap: 16px;
}

.task-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.task-name {
  font-size: 14px;
  color: #303133;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.task-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 12px;
  color: #909399;
}

.task-speed {
  color: #409eff;
}

.task-ops {
  flex-shrink: 0;
  display: flex;
  gap: 8px;
}

/* 新建任务表单 */
.task-form :deep(.el-form-item) {
  margin-bottom: 14px;
}

.task-advanced {
  border: none;
  margin-bottom: 8px;
}

.task-checksum {
  display: flex;
  gap: 8px;
  width: 100%;
}

.checksum-type {
  width: 120px;
  flex-shrink: 0;
}
</style>
