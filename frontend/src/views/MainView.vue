<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { GetBaiduFileList } from '../../wailsjs/go/service/App'
import FileList from '../components/FileList.vue'
import Breadcrumb from '../components/Breadcrumb.vue'
import { useI18n } from '../composables/useI18n'
import {
  FolderOpened, Download, Setting, Refresh
} from '@element-plus/icons-vue'

const props = defineProps({
  credential: { type: Object, required: true }
})

const { t } = useI18n()

// 当前页面：files | downloads | settings
const activePage = ref('files')
const files = ref([])
const loading = ref(false)
const currentDir = ref('/')

async function loadFiles(dir = '/') {
  loading.value = true
  try {
    const result = await GetBaiduFileList(dir)
    if (!result?.success) {
      files.value = []
      ElMessage.error(result?.message || '读取文件列表失败')
      return
    }

    files.value = result.list || []
    currentDir.value = result.dir || dir
  } catch (error) {
    files.value = []
    ElMessage.error(error?.message || String(error) || '读取文件列表失败')
  } finally {
    loading.value = false
  }
}

function handleFileAction(payload) {
  // 预留文件操作入口，后续根据 payload.type 实现具体功能
  void payload
}

onMounted(() => loadFiles('/'))

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
      <!-- 账户信息 -->
      <div class="account-card">
        <el-avatar :size="48" :src="credential.photo_url || ''" class="account-avatar">
          {{ credential.username ? credential.username.charAt(0) : '?' }}
        </el-avatar>
        <div class="account-info">
          <div class="account-name">{{ credential.username }}</div>
          <div class="account-tags">
            <el-tag v-if="vipInfo(credential.vip_type)" :type="vipInfo(credential.vip_type).type" size="small" effect="dark">
              {{ vipInfo(credential.vip_type).label }}
            </el-tag>
            <el-tag v-else type="info" size="small">{{ t('no_vip', '无会员') }}</el-tag>
          </div>
        </div>
      </div>

      <!-- 导航菜单 -->
      <el-menu :default-active="activePage" class="sidebar-menu" @select="activePage = $event">
        <el-menu-item v-for="item in menuItems" :key="item.key" :index="item.key">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ t('page_' + item.key, item.key) }}</span>
        </el-menu-item>
      </el-menu>
    </aside>

    <!-- 右侧内容区 -->
    <main class="content-area">
      <section v-if="activePage === 'files'" class="files-page">
        <header class="files-header">
          <Breadcrumb :path="currentDir" @navigate="loadFiles" />
          <el-button class="files-refresh" :icon="Refresh" :loading="loading" @click="loadFiles(currentDir)">
            {{ t('file_refresh', '刷新') }}
          </el-button>
        </header>

        <div class="files-content">
          <FileList
            :files="files"
            :loading="loading"
            @navigate="loadFiles"
            @action="handleFileAction"
          />
        </div>
      </section>
      <div v-else-if="activePage === 'downloads'" class="page-placeholder">
        <el-empty :description="t('page_downloads', '下载管理')" />
      </div>
      <div v-else-if="activePage === 'settings'" class="page-placeholder">
        <el-empty :description="t('page_settings', '设置')" />
      </div>
    </main>
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

.account-card {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  border-bottom: 1px solid #f0f0f0;
}

.account-info {
  flex: 1;
  min-width: 0;
}

.account-name {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.account-tags {
  margin-top: 4px;
}

.sidebar-menu {
  border-right: none;
  flex: 1;
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
  min-height: 56px;
  padding: 10px 16px;
  background: #fff;
  border-bottom: 1px solid #e4e7ed;
  display: flex;
  align-items: center;
  gap: 16px;
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
</style>
