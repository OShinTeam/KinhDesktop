<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { GetBaiduFileList, GetBaiduQuota, BaiduLogout, GetBaiduDownloadLink, GetBaiduDownloadLinkRemote, GetSettings } from '../../wailsjs/go/service/App'
import { BrowserOpenURL, ClipboardSetText } from '../../wailsjs/runtime/runtime'
import FileList from '../components/FileList.vue'
import Breadcrumb from '../components/Breadcrumb.vue'
import SettingsView from './SettingsView.vue'
import { formatBytes } from '../utils/format'
import { useI18n } from '../composables/useI18n'
import {
  FolderOpened, Download, Setting, Refresh, SwitchButton
} from '@element-plus/icons-vue'

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

async function loadRemoteEnabled() {
  try {
    const settings = await GetSettings()
    remoteEnabled.value = !!settings?.download_acc_link
  } catch {
    remoteEnabled.value = false
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
  // 离开设置页时刷新远程解析可用状态（设置中加速链接可能已被修改/保存）
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

// 文件操作：download 本地解析 / download_remote 远程解析（需在设置中配置加速链接）
async function handleFileAction(payload) {
  if (payload?.type !== 'download' && payload?.type !== 'download_remote') return
  const item = payload.item
  if (!item?.fs_id) return

  const resolve = payload.type === 'download_remote' ? GetBaiduDownloadLinkRemote : GetBaiduDownloadLink
  try {
    const result = await resolve(item.fs_id)
    if (!result?.success) {
      ElMessage.error(result?.message || t('download_link_failed', '获取下载地址失败'))
      return
    }
    ElMessage.success(t('download_link_success', '已获取下载直链'))
    downloadLink.value = { name: result.filename || item.server_filename || item.filename, url: result.dlink }
  } catch (err) {
    ElMessage.error(t('download_link_failed', '获取下载地址失败') + ': ' + String(err))
  }
}

// 下载直链弹窗：解析成功后展示，提供复制链接 / 浏览器下载两个动作
const downloadLink = ref(null)

function closeDownloadLink() {
  downloadLink.value = null
}

async function copyDownloadLink() {
  if (!downloadLink.value?.url) return
  const ok = await ClipboardSetText(downloadLink.value.url)
  if (ok) {
    ElMessage.success(t('download_link_copied', '下载直链已复制到剪贴板'))
    closeDownloadLink()
  } else {
    ElMessage.error(t('download_link_failed', '获取下载地址失败'))
  }
}

function openDownloadInBrowser() {
  if (!downloadLink.value?.url) return
  BrowserOpenURL(downloadLink.value.url)
  closeDownloadLink()
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
  <div class="main-layout">
    <!-- 左侧边栏 -->
    <aside class="sidebar">
      <!-- 顶部：网盘容量卡片 -->
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

      <!-- 导航菜单 -->
      <el-menu :default-active="activePage" class="sidebar-menu" @select="handleMenuSelect">
        <el-menu-item v-for="item in menuItems" :key="item.key" :index="item.key">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ t('page_' + item.key, item.key) }}</span>
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
            @click="loadFiles(currentDir)"
          >
            {{ t('file_refresh', '刷新') }}
          </el-button>
        </header>

        <div class="files-content">
          <FileList
            :files="files"
            :loading="loading"
            :remote-enabled="remoteEnabled"
            @navigate="loadFiles"
            @action="handleFileAction"
          />
        </div>
      </section>
      <div v-else-if="activePage === 'downloads'" class="page-placeholder">
        <el-empty :description="t('page_downloads', '下载管理')" />
      </div>
      <SettingsView
        v-else-if="activePage === 'settings'"
        ref="settingsRef"
      />
    </main>

    <!-- 下载直链弹窗：解析成功后展示文件名与直链，提供复制 / 浏览器下载 -->
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
      </div>
      <template #footer>
        <el-button @click="closeDownloadLink">{{ t('logout_confirm_cancel', '取消') }}</el-button>
        <el-button @click="copyDownloadLink">{{ t('download_copy_link', '复制链接') }}</el-button>
        <el-button type="primary" @click="openDownloadInBrowser">
          {{ t('download_open_page', '浏览器下载') }}
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

.sidebar {
  width: 220px;
  flex-shrink: 0;
  background: #fff;
  border-right: 1px solid #e4e7ed;
  display: flex;
  flex-direction: column;
}

.sidebar-menu {
  border-right: none;
  flex: 1;
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
</style>
