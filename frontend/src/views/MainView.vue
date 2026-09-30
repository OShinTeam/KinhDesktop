<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Clipboard, Events } from '@wailsio/runtime'
import { useViewport } from '../composables/useViewport'
import FileList from '../components/FileList.vue'
import Breadcrumb from '../components/Breadcrumb.vue'
import SettingsView from './SettingsView.vue'
import DownloadsView from './DownloadsView.vue'
import { formatBytes } from '../utils/format'
import { useI18n } from '../composables/useI18n'
import {
  FolderOpened, Download, Setting, Refresh, SwitchButton
} from '@element-plus/icons-vue'
import { App } from '../../bindings/kinh-desktop/service'

// v3 的绑定按服务（命名空间）导出，这里解构回扁平函数，沿用原有的调用写法
const {
  GetBaiduFileList, GetBaiduQuota, BaiduLogout, GetBaiduDownloadLink,
  GetBaiduDownloadLinkRemote, GetSettings, SubmitDownload,
  ResolveMultiLink, SubmitDownloadWithOptions,
} = App

// 竖屏（高 > 宽）：导航条移到窗口底部、内容区在上，避免侧栏挤占横向空间；
// 横屏与桌面端保持左侧栏布局（详见 useViewport）
const { isPortrait } = useViewport()

// v2 的 ClipboardSetText 返回布尔，v3 的 Clipboard.SetText 失败时抛错，这里还原成原来的语义
async function ClipboardSetText(text) {
  try {
    await Clipboard.SetText(text)
    return true
  } catch {
    return false
  }
}

const props = defineProps({
  credential: { type: Object, required: true }
})

const emit = defineEmits(['logout'])

const { t } = useI18n()

// 当前页面：files | downloads | settings
const activePage = ref('files')
const files = ref([])
const loading = ref(false)
const currentDir = ref('/')

// 远程解析是否可用：设置中配置了加速链接时文件列表才显示远程解析按钮
const remoteEnabled = ref(false)

// 实验性多地址下载是否可用：设置开关（默认关）
const multiLinkEnabled = ref(false)

async function loadRemoteEnabled() {
  try {
    const settings = await GetSettings()
    remoteEnabled.value = !!settings?.download_acc_link
    multiLinkEnabled.value = !!settings?.experimental_multi_link
    resolveUA.value = settings?.download_user_agent || ''
  } catch {
    remoteEnabled.value = false
    multiLinkEnabled.value = false
    resolveUA.value = ''
  }
}

// 设置页组件引用：脏检测（未保存修改时切换页面需拦截确认）
const settingsRef = ref(null)

// 侧边栏菜单选择：设置页有未保存修改时弹窗确认（保存并离开 / 放弃修改 / 留在本页）
async function handleMenuSelect(key) {
  if (activePage.value === key) return
  if (activePage.value === 'settings' && settingsRef.value?.checkDirty?.()) {
    let action = 'stay'
    try {
      await ElMessageBox.confirm(
        t('settings_unsaved_message', '当前有未保存的设置修改，离开将丢失这些更改。'),
        t('settings_unsaved_title', '未保存的修改'),
        {
          confirmButtonText: t('settings_unsaved_save', '保存并离开'),
          cancelButtonText: t('settings_unsaved_discard', '放弃修改'),
          distinguishCancelAndClose: true,
          type: 'warning',
        }
      )
      action = 'save'
    } catch (e) {
      // cancel = 放弃修改并离开；close/ESC = 留在本页
      action = (e === 'cancel') ? 'discard' : 'stay'
    }

    if (action === 'save') {
      await settingsRef.value.saveAndReturn() // 保存完成后再切页
    } else if (action === 'discard') {
      settingsRef.value.discardChanges() // 恢复到已保存状态后离开
    } else {
      return // 留在本页，不切换
    }
  }
  // 离开设置页时刷新远程解析状态（设置中的加速链接可能已被修改）
  if (activePage.value === 'settings') {
    loadRemoteEnabled()
  }
  activePage.value = key
}

// 网盘容量信息
const quota = ref({ total: 0, used: 0 })
const quotaPercent = computed(() => {
  if (!quota.value.total) return 0
  const percent = (quota.value.used / quota.value.total) * 100
  return Math.min(Math.round(percent * 10) / 10, 100)
})

function formatQuotaSize(bytes) {
  return formatBytes(bytes, '0 B')
}

async function loadQuota() {
  try {
    const result = await GetBaiduQuota()
    if (result?.success) {
      quota.value = { total: result.total, used: result.used }
    }
  } catch (err) {
    // 容量信息获取失败不影响主流程
    console.warn('获取容量信息失败:', err)
  }
}

async function loadFiles(dir = '/') {
  loading.value = true
  try {
    const result = await GetBaiduFileList(dir)
    if (!result?.success) {
      files.value = []
      ElMessage.error(result?.message || t('file_list_load_failed', '读取文件列表失败'))
      return
    }

    files.value = result.list || []
    currentDir.value = result.dir || dir
  } catch (error) {
    files.value = []
    ElMessage.error(error?.message || String(error) || t('file_list_load_failed', '读取文件列表失败'))
  } finally {
    loading.value = false
  }
}

// 主动刷新：文件列表与网盘容量一并更新。
// quota 是账户级数据，与目录导航无关，故不挂在 loadFiles 上，
// 否则每次切换目录都会产生一次无效的容量请求。
async function refreshAll() {
  await Promise.all([loadFiles(currentDir.value), loadQuota()])
}

// 解析状态：全局同时仅允许一个解析请求。
// FileList 据此禁用其他按钮并转圈当前按钮；此处兜底拦截，双保险防重入
const resolving = ref({ active: false, fs_id: 0, type: '', name: '' })

// 文件操作：download 本地解析 / download_remote 远程解析（需在设置中配置加速链接）
// download_multi 实验性多地址下载（设置开关开启后显示按钮）
async function handleFileAction(payload) {
  if (payload?.type === 'download_multi') {
    await handleMultiLinkDownload(payload.item)
    return
  }
  if (payload?.type !== 'download' && payload?.type !== 'download_remote') return
  const item = payload.item
  if (!item?.fs_id) return
  if (resolving.value.active) return

  resolving.value = {
    active: true,
    fs_id: item.fs_id,
    type: payload.type,
    name: item.server_filename || item.filename || '',
  }
  const resolve = payload.type === 'download_remote' ? GetBaiduDownloadLinkRemote : GetBaiduDownloadLink
  try {
    const result = await resolve(item.fs_id)
    if (!result?.success) {
      ElMessage.error(result?.message || t('download_link_failed', '获取下载地址失败'))
      return
    }
    ElMessage.success(t('download_link_success', '已获取下载直链'))
    downloadLink.value = {
      name: result.filename || item.server_filename || item.filename,
      url: result.dlink,
      ua: resolveUA.value,
    }
  } catch (err) {
    ElMessage.error(t('download_link_failed', '获取下载地址失败') + ': ' + String(err))
  } finally {
    resolving.value = { active: false, fs_id: 0, type: '', name: '' }
  }
}

// 下载 UA：与设置中的默认 UA 一致（弹窗展示/复制用）
const resolveUA = ref('')

// 下载直链弹窗：组件不存在时展示链接与 UA（复制链接/复制UA）；
// 组件存在时提供「添加到下载管理」推送任务
const downloadLink = ref(null)

// ==================== 实验性下载 ====================
// 流程：点击按钮 → 弹窗显示「将获取 N 个地址」（N 来自设置，弹窗内不可改，
// 避免与设置页重复收集同一信息）→ 点「添加到下载管理」→ 开始获取（进度经
// 事件推送）→ 成功后自动提交任务并关闭。失败则留在弹窗可重试。

const multiLink = ref(null) // { name, fs_id, phase: 'idle'|'fetching', count }

const multiLinkProgress = ref({ done: 0, total: 0, links: 0 })

// 打开弹窗：数量取设置值
async function handleMultiLinkDownload(item) {
  if (!item?.fs_id) return
  if (resolving.value.active) return
  if (multiLink.value?.phase === 'fetching') return

  const name = item.server_filename || item.filename || ''
  let count = 4
  try {
    const settings = await GetSettings()
    if (settings?.experimental_count >= 2) count = settings.experimental_count
    if (count > 6) count = 6
  } catch { /* 取不到设置就用默认 4 */ }

  multiLink.value = { name, fs_id: item.fs_id, phase: 'idle', count }
}

// 点「添加到下载管理」：先并发解析（进度经事件推送），成功后立即提交任务
async function pushMultiLinkToDownload() {
  if (!multiLink.value || multiLink.value.phase === 'fetching') return
  const { fs_id, name, count } = multiLink.value

  multiLink.value.phase = 'fetching'
  multiLinkProgress.value = { done: 0, total: count, links: 0 }
  resolving.value = { active: true, fs_id, type: 'download_multi', name }

  try {
    const result = await ResolveMultiLink(fs_id, name, count)
    if (!result?.success || !result.dlink) {
      ElMessage.error(result?.message || t('download_link_failed', '获取下载地址失败'))
      // 留在弹窗恢复 idle，用户可直接重试
      if (multiLink.value && multiLink.value.fs_id === fs_id) {
        multiLink.value.phase = 'idle'
      }
      return
    }

    const submit = await SubmitDownloadWithOptions({
      url: result.dlink,
      file_name: result.filename || name,
      multi_sources: result.sources || [],
    })
    if (!submit?.success) {
      ElMessage.error(submit?.message || t('download_submit_failed', '添加下载任务失败'))
      if (multiLink.value && multiLink.value.fs_id === fs_id) {
        multiLink.value.phase = 'idle'
      }
      return
    }

    ElMessage.success(submit.queued
      ? t('download_submit_queued', '并发已满，任务已加入排队')
      : t('download_submit_success', '已添加到下载管理'))
    closeMultiLink()
  } catch (err) {
    ElMessage.error(t('download_submit_failed', '添加下载任务失败') + ': ' + String(err))
    if (multiLink.value && multiLink.value.fs_id === fs_id) {
      multiLink.value.phase = 'idle'
    }
  } finally {
    resolving.value = { active: false, fs_id: 0, type: '', name: '' }
  }
}

// 解析进度事件：后端每次解析请求完成时推送一次
Events.On('experimental-link-progress', (ev) => {
  const p = ev?.data
  if (!p || !multiLink.value || multiLink.value.phase !== 'fetching') return
  multiLinkProgress.value = { done: p.done, total: p.total, links: p.links }
})

function closeMultiLink() {
  multiLink.value = null
}

function closeDownloadLink() {
  downloadLink.value = null
}

async function copyText(text, successMsg) {
  const ok = await ClipboardSetText(text)
  if (ok) {
    ElMessage.success(successMsg)
  } else {
    ElMessage.error(t('download_copy_failed', '复制失败'))
  }
  return ok
}

// 复制后不关闭弹窗：用户通常需要同时复制链接与 UA 两个参数，仅在点击取消时关闭
async function copyDownloadLink() {
  if (downloadLink.value?.url) {
    await copyText(downloadLink.value.url, t('download_link_copied', '下载直链已复制到剪贴板'))
  }
}

async function copyDownloadUA() {
  if (downloadLink.value?.ua) {
    await copyText(downloadLink.value.ua, t('download_ua_copied', 'User-Agent 已复制到剪贴板'))
  }
}

// 组件存在时推送任务到下载管理
const submittingTask = ref(false)

async function pushToDownloadManager() {
  if (!downloadLink.value?.url) return
  submittingTask.value = true
  try {
    const result = await SubmitDownload(downloadLink.value.url, downloadLink.value.name, 0)
    if (!result?.success) {
      ElMessage.error(result?.message || t('download_submit_failed', '添加下载任务失败'))
      return
    }
    ElMessage.success(t('download_submit_success', '已添加到下载管理'))
    closeDownloadLink()
  } catch (err) {
    ElMessage.error(t('download_submit_failed', '添加下载任务失败') + ': ' + String(err))
  } finally {
    submittingTask.value = false
  }
}

// 退出登录：弹窗确认后调用后端清除凭证（含本地密钥与保存的登录信息）
// 后端登出失败时保持当前界面，避免本地已登出而后端凭证仍存活的错位状态
async function handleLogout() {
  try {
    await ElMessageBox.confirm(
      t('logout_confirm_message', '确定要退出当前账号吗？本地保存的登录信息将被清除。'),
      t('logout_confirm_title', '退出登录'),
      {
        confirmButtonText: t('logout_confirm_ok', '退出'),
        cancelButtonText: t('logout_confirm_cancel', '取消'),
        type: 'warning',
        confirmButtonClass: 'el-button--danger',
      }
    )
  } catch {
    return // 用户取消
  }

  try {
    await BaiduLogout()
  } catch (err) {
    ElMessage.error(t('logout_failed', '退出登录失败') + ': ' + String(err))
    return
  }
  ElMessage.success(t('logout_success', '已退出登录'))
  emit('logout')
}

onMounted(() => {
  loadFiles('/')
  loadQuota()
  loadRemoteEnabled()
})

const menuItems = [
  { key: 'files', icon: FolderOpened },
  { key: 'downloads', icon: Download },
  { key: 'settings', icon: Setting },
]

// vip_type: 0 无会员 / 1 VIP / 2 SVIP
function vipInfo(vipType) {
  switch (vipType) {
    case 2: return { label: 'SVIP', type: 'danger' }
    case 1: return { label: 'VIP', type: 'warning' }
    default: return null
  }
}
</script>

<template>
  <div class="main-layout" :class="{ 'is-portrait': isPortrait }">
    <!-- 左侧边栏：竖屏时由 CSS 转成底部导航条 -->
    <aside class="sidebar">
      <!-- 顶部：网盘容量卡片（竖屏的底部导航里放不下，交由 CSS 隐藏） -->
      <div class="quota-card">
        <div class="quota-title">
          <span>{{ t('disk_space', '网盘空间') }}</span>
          <span class="quota-percent">{{ quotaPercent }}%</span>
        </div>
        <el-progress
          :percentage="quotaPercent"
          :show-text="false"
          :stroke-width="8"
          :color="quotaPercent > 90 ? '#f56c6c' : quotaPercent > 70 ? '#e6a23c' : '#409eff'"
        />
        <div class="quota-detail">
          {{ formatQuotaSize(quota.used) }} / {{ formatQuotaSize(quota.total) }}
        </div>
      </div>

      <!-- 导航菜单：竖屏切换为横向排布（mode 是组件属性，必须由 JS 状态驱动，CSS 改不了） -->
      <el-menu
        :default-active="activePage"
        :mode="isPortrait ? 'horizontal' : 'vertical'"
        class="sidebar-menu"
        @select="handleMenuSelect"
      >
        <el-menu-item v-for="item in menuItems" :key="item.key" :index="item.key">
          <el-icon><component :is="item.icon" /></el-icon>
          <template #title>{{ t('page_' + item.key, item.key) }}</template>
        </el-menu-item>
      </el-menu>

      <!-- 底部账户信息 -->
      <div class="account-card">
        <div class="account-top">
          <el-avatar :size="48" :src="credential.photo_url || ''" class="account-avatar">
            {{ credential.username ? credential.username.charAt(0) : '?' }}
          </el-avatar>
          <div class="account-name" :title="credential.username">{{ credential.username }}</div>
        </div>
        <div class="account-bottom">
          <div class="account-tags">
            <el-tag v-if="vipInfo(credential.vip_type)" :type="vipInfo(credential.vip_type).type" size="small" effect="dark">
              {{ vipInfo(credential.vip_type).label }}
            </el-tag>
            <el-tag v-else type="info" size="small">{{ t('no_vip', '无会员') }}</el-tag>
          </div>
          <el-tooltip :content="t('btn_logout', '退出登录')" placement="top">
            <el-button
              class="logout-btn"
              :icon="SwitchButton"
              circle
              text
              type="danger"
              :title="t('btn_logout', '退出登录')"
              @click="handleLogout"
            />
          </el-tooltip>
        </div>
      </div>
    </aside>

    <!-- 右侧内容区 -->
    <main class="content-area">
      <section v-if="activePage === 'files'" class="files-page">
        <header class="files-header">
          <Breadcrumb :path="currentDir" @navigate="loadFiles" />
          <el-button
            class="files-refresh"
            :icon="Refresh"
            :loading="loading"
            @click="refreshAll"
          >
            {{ t('file_refresh', '刷新') }}
          </el-button>
        </header>

        <div class="files-content">
          <FileList
            :files="files"
            :loading="loading"
            :remote-enabled="remoteEnabled"
            :multi-link-enabled="multiLinkEnabled"
            :resolving="resolving"
            @navigate="loadFiles"
            @action="handleFileAction"
          />
        </div>
      </section>
      <DownloadsView v-else-if="activePage === 'downloads'" />
      <SettingsView
        v-else-if="activePage === 'settings'"
        ref="settingsRef"
        @logout="handleLogout"
      />
    </main>

    <!-- 下载直链弹窗：展示文件名/直链/UA；组件存在时可推送下载管理，否则复制链接与 UA -->
    <el-dialog
      :model-value="!!downloadLink"
      :title="t('download_link_success', '已获取下载直链')"
      width="560px"
      :close-on-click-modal="false"
      @close="closeDownloadLink"
    >
      <div v-if="downloadLink" class="dl-link-body">
        <div class="dl-link-name" :title="downloadLink.name">{{ downloadLink.name }}</div>
        <div class="dl-link-url">{{ downloadLink.url }}</div>
        <div class="dl-link-ua">
          <span class="dl-link-ua-label">UA</span>
          <span class="dl-link-ua-text">{{ downloadLink.ua || '-' }}</span>
        </div>
      </div>
      <template #footer>
        <el-button @click="closeDownloadLink">{{ t('logout_confirm_cancel', '取消') }}</el-button>
        <el-button @click="copyDownloadLink">{{ t('download_copy_link', '复制链接') }}</el-button>
        <el-button @click="copyDownloadUA">{{ t('download_copy_ua', '复制UA') }}</el-button>
        <el-button
          type="primary"
          :loading="submittingTask"
          @click="pushToDownloadManager"
        >
          {{ t('download_push_task', '添加到下载管理') }}
        </el-button>
      </template>
    </el-dialog>

    <!-- 实验性下载弹窗：数量来自设置（弹窗内不可改，避免与设置页重复收集）；
         点「添加到下载管理」后才开始解析，进度条实时显示，成功后自动提交并关闭 -->
    <el-dialog
      :model-value="!!multiLink"
      :title="t('download_multi_title', '实验性下载')"
      width="480px"
      :close-on-click-modal="false"
      @close="closeMultiLink"
    >
      <div v-if="multiLink" class="dl-link-body">
        <div class="dl-link-name" :title="multiLink.name">{{ multiLink.name }}</div>

        <!-- 待获取：显示将获取的地址数量（来自设置） -->
        <template v-if="multiLink.phase === 'idle'">
          <el-result
            icon="info"
            :title="`${t('download_multi_will_fetch', '将获取')} ${multiLink.count} ${t('download_multi_links_unit', '个')} ${t('download_multi_addresses', '下载地址')}`"
            :sub-title="t('download_multi_ready', '将从多个地址同时拉取该文件')"
          />
        </template>

        <!-- 获取中：进度条 -->
        <template v-else-if="multiLink.phase === 'fetching'">
          <el-progress
            :percentage="multiLinkProgress.total
              ? Math.round((multiLinkProgress.done / multiLinkProgress.total) * 100)
              : 0"
            :stroke-width="10"
          />
          <div class="multi-progress-text">
            {{ t('download_multi_progress', '正在获取下载地址') }}
            ({{ multiLinkProgress.done }}/{{ multiLinkProgress.total }})
            ·
            {{ t('download_multi_got', '已获取') }} {{ multiLinkProgress.links }}
            {{ t('download_multi_links_unit', '个') }}
          </div>
        </template>
      </div>
      <template #footer>
        <el-button @click="closeMultiLink">{{ t('logout_confirm_cancel', '取消') }}</el-button>
        <el-button
          type="primary"
          :loading="multiLink?.phase === 'fetching'"
          @click="pushMultiLinkToDownload"
        >
          {{ multiLink?.phase === 'fetching'
            ? t('download_multi_fetching', '获取中…')
            : t('download_push_task', '添加到下载管理') }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.main-layout {
  flex: 1;
  display: flex;
  overflow: hidden;
}

/* 竖屏（高 > 宽）：整体改为纵向，导航条落到窗口底部。
   用 column-reverse 而不动 DOM 顺序（侧栏仍写在前面），视觉上即「内容在上、导航在下」 */
.main-layout.is-portrait {
  flex-direction: column-reverse;
}

.sidebar {
  width: 220px;
  flex-shrink: 0;
  background: #fff;
  border-right: 1px solid #e4e7ed;
  display: flex;
  flex-direction: column;
}

/* 竖屏：侧栏化身底部导航条 —— 通栏、横排，只保留菜单 */
.main-layout.is-portrait .sidebar {
  width: 100%;
  height: 56px;
  flex-direction: row;
  align-items: center;
  border-right: none;
  border-top: 1px solid #e4e7ed;
}

/* 容量卡片与账户信息在 56px 高的导航条里排不下，竖屏隐藏 */
.main-layout.is-portrait .quota-card,
.main-layout.is-portrait .account-card {
  display: none;
}

.sidebar-menu {
  border-right: none;
  flex: 1;
}

/* 竖屏：菜单横向铺满导航条 */
.main-layout.is-portrait .sidebar-menu {
  display: flex;
  justify-content: space-around;
  border-top: none;
}

/* 顶部网盘容量卡片 */
.quota-card {
  padding: 16px;
  border-bottom: 1px solid #f0f0f0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.quota-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: #909399;
}

.quota-percent {
  font-size: 11px;
  color: #909399;
}

.quota-detail {
  font-size: 12px;
  color: #606266;
}

/* 底部账户区：头像+用户名与标签/退出按钮分两行，用户名可完整显示 */
.account-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 20px 16px;
  border-top: 1px solid #f0f0f0;
}

.account-top {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.account-name {
  flex: 1;
  min-width: 0;
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.account-bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.account-tags {
  min-width: 0;
}

.logout-btn {
  flex-shrink: 0;
}

.content-area {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  background: #f5f7fa;
}

.files-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.files-header {
  min-height: 44px;
  padding: 6px 16px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  gap: 12px;
}

.files-header .breadcrumb {
  flex: 1;
  min-width: 0;
}

.files-refresh {
  flex-shrink: 0;
}

.files-content {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.page-placeholder {
  flex: 1;
  display: flex;
  justify-content: center;
  align-items: center;
}

/* 下载直链弹窗内容 */
.dl-link-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.dl-link-name {
  font-size: 14px;
  color: #303133;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.dl-link-url {
  padding: 8px 10px;
  background: #f5f7fa;
  border-radius: 4px;
  font-size: 12px;
  line-height: 1.5;
  color: #606266;
  word-break: break-all;
  max-height: 120px;
  overflow-y: auto;
  user-select: text;
}

.dl-link-ua {
  display: flex;
  align-items: baseline;
  gap: 8px;
  font-size: 12px;
  color: #909399;
}

.dl-link-ua-label {
  flex-shrink: 0;
  font-weight: 600;
}

.dl-link-ua-text {
  min-width: 0;
  word-break: break-all;
  user-select: text;
}

.multi-progress-text {
  margin-top: 10px;
  font-size: 13px;
  color: var(--el-text-color-secondary);
  text-align: center;
}
</style>
