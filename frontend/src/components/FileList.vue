<script setup>
import {
  Folder, Document, Picture, VideoPlay, Headset, Tickets, Box, Files, Download, Connection, MagicStick
} from '@element-plus/icons-vue'
import { formatBytes, formatTimestamp } from '../utils/format'
import { useI18n } from '../composables/useI18n'

const { t } = useI18n()

const props = defineProps({
  files: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false },
  remoteEnabled: { type: Boolean, default: false }, // 远程解析是否可用（已配置加速链接）
  multiLinkEnabled: { type: Boolean, default: false }, // 实验性多地址下载是否可用（设置开关）
  // 解析状态：全局同时仅允许一个解析请求（active 时其余按钮禁用，目标按钮转圈）
  resolving: { type: Object, default: () => ({ active: false, fs_id: 0, type: '', name: '' }) }
})

const emit = defineEmits(['navigate', 'action'])

// 按扩展名判断文件类型（参考 KinhWebEO utils.ts getFileCategoryByFilename）
const IMAGE_EXTS = ['jpg', 'jpeg', 'png', 'gif', 'bmp', 'webp', 'svg', 'ico', 'tiff', 'tif']
const VIDEO_EXTS = ['mp4', 'avi', 'mkv', 'mov', 'wmv', 'flv', 'webm', 'mpg', 'mpeg', 'm4v', '3gp', 'ts', 'vob']
const AUDIO_EXTS = ['mp3', 'wav', 'flac', 'aac', 'ogg', 'm4a', 'wma', 'opus', 'ape']
const DOC_EXTS = ['pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx']
const TEXT_EXTS = ['txt', 'md', 'json', 'xml', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'conf', 'log', 'csv', 'tsv', 'html', 'htm', 'css', 'js', 'ts', 'jsx', 'tsx', 'py', 'go', 'java', 'c', 'cpp', 'h', 'rs', 'sh', 'bat', 'ps1', 'sql', 'php', 'rb', 'swift', 'kt', 'dart', 'lua', 'r', 'perl', 'pl', 'pm']
const ARCHIVE_EXTS = ['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz', 'zst', 'lz4', 'iso', 'img', 'dmg', 'cab']

function fileExt(name) {
  if (!name) return ''
  const index = name.lastIndexOf('.')
  return index === -1 ? '' : name.slice(index + 1).toLowerCase()
}

// 图标统一灰色（由 .file-icon 的 CSS 决定），仅按类型区分形状
function iconFor(item) {
  if (item.isdir === 1) return Folder
  const ext = fileExt(item.server_filename || item.filename)
  if (IMAGE_EXTS.includes(ext)) return Picture
  if (VIDEO_EXTS.includes(ext)) return VideoPlay
  if (AUDIO_EXTS.includes(ext)) return Headset
  if (DOC_EXTS.includes(ext)) return Tickets
  if (TEXT_EXTS.includes(ext)) return Document
  if (ARCHIVE_EXTS.includes(ext)) return Box
  return Files
}

function fileName(item) {
  return item.server_filename || item.filename || item.path || '-'
}

function fileMeta(item) {
  return item.isdir === 1
    ? '文件夹'
    : `${formatTimestamp(item.server_mtime)} · ${formatBytes(item.size)}`
}

function openItem(item) {
  if (item.isdir === 1) emit('navigate', item.path)
}

function triggerAction(item, type) {
  emit('action', { item, type })
}

// isResolving 该文件该类型的按钮是否正在解析中（转圈的是它）
function isResolving(item, type) {
  return props.resolving.active
    && props.resolving.fs_id === item.fs_id
    && props.resolving.type === type
}
</script>

<template>
  <div class="file-list" v-loading="loading">
    <!-- 解析中提示条：显式反馈当前正在获取哪个文件的下载地址 -->
    <div v-if="resolving.active" class="resolving-tip">
      {{ t('download_resolving', '正在获取下载地址') }}: {{ resolving.name }}
    </div>
    <el-empty v-if="!loading && files.length === 0" :description="t('file_list_empty', '暂无文件')" />

    <ul v-else class="file-rows">
      <li
        v-for="item in files"
        :key="item.fs_id"
        class="file-row"
        :class="{ folder: item.isdir === 1 }"
        @click="openItem(item)"
      >
        <el-icon class="file-icon">
          <component :is="iconFor(item)" />
        </el-icon>

        <div class="file-text">
          <div class="file-name" :title="fileName(item)">{{ fileName(item) }}</div>
          <div class="file-meta">{{ fileMeta(item) }}</div>
        </div>

        <div class="file-ops" @click.stop>
          <template v-if="item.isdir !== 1">
            <el-button
              class="file-download-btn"
              circle
              :icon="Download"
              :title="t('download_get_link', '获取下载地址')"
              :loading="isResolving(item, 'download')"
              :disabled="resolving.active && !isResolving(item, 'download')"
              @click="triggerAction(item, 'download')"
            />
            <el-button
              v-if="remoteEnabled"
              class="file-download-btn"
              circle
              :icon="Connection"
              :title="t('download_remote_resolve', '远程解析')"
              :loading="isResolving(item, 'download_remote')"
              :disabled="resolving.active && !isResolving(item, 'download_remote')"
              @click="triggerAction(item, 'download_remote')"
            />
            <el-button
              v-if="multiLinkEnabled"
              class="file-download-btn"
              circle
              :icon="MagicStick"
              :title="t('download_multi_link', '实验性下载')"
              :loading="isResolving(item, 'download_multi')"
              :disabled="resolving.active && !isResolving(item, 'download_multi')"
              @click="triggerAction(item, 'download_multi')"
            />
          </template>
          <span v-else class="file-ops-empty">-</span>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.file-list {
  width: 100%;
  height: 100%;
  min-height: 0;
  background: #fff;
  overflow-y: auto;
}

.resolving-tip {
  padding: 6px 16px;
  font-size: 12px;
  color: #409eff;
  background: #ecf5ff;
  border-bottom: 1px solid #e1eefb;
}

.file-rows {
  margin: 0;
  padding: 0;
  list-style: none;
}

.file-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  border-bottom: 1px solid #f0f0f0;
  cursor: pointer;
}

/* 悬停高亮仅在支持悬停的设备生效。
   触摸设备的 :hover 会「粘住」：点过的行会一直保持高亮，直到点击别处才恢复。 */
@media (hover: hover) {
  .file-row:hover {
    background: #f5f7fa;
  }
}

/* 触摸设备改用 :active 提供按下反馈，抬手即解除 */
@media (hover: none) {
  .file-row:active {
    background: #f5f7fa;
  }
}

.file-icon {
  flex-shrink: 0;
  font-size: 24px;
  /* 图标统一灰色（原先由 JS 的 ICON_COLOR 常量内联绑定，恒为同一值） */
  color: #909399;
}

/* 信息区：占据剩余空间 */
.file-text {
  flex: 1 1 auto;
  min-width: 0;
}

/* 功能区：固定宽度，与信息区分块，按钮/- 居中显示 */
.file-ops {
  flex: 0 0 180px;
  margin-left: 12px;
  padding: 0 12px;
  border-left: 1px solid #f0f0f0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
}

.file-download-btn + .file-download-btn {
  margin-left: 0; /* 覆盖 Element Plus 相邻按钮默认左边距，用 gap 控制间距 */
}

.file-name {
  font-size: 14px;
  color: #303133;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.file-meta {
  margin-top: 2px;
  font-size: 12px;
  color: #909399;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.file-download-btn {
  font-size: 18px;
}

.file-ops-empty {
  color: #c0c4cc;
  line-height: 1;
}
</style>
