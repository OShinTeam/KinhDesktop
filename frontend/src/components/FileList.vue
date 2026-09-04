<script setup>
import {
  Folder, Document, Picture, VideoPlay, Headset, Tickets, Box, Files, Download
} from '@element-plus/icons-vue'
import { formatBytes, formatTimestamp } from '../utils/format'

const props = defineProps({
  files: { type: Array, default: () => [] },
  loading: { type: Boolean, default: false }
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

// 图标统一灰色，仅按类型区分形状
const ICON_COLOR = '#909399'

function iconFor(item) {
  if (item.isdir === 1) return { component: Folder, color: ICON_COLOR }
  const ext = fileExt(item.server_filename || item.filename)
  if (IMAGE_EXTS.includes(ext)) return { component: Picture, color: ICON_COLOR }
  if (VIDEO_EXTS.includes(ext)) return { component: VideoPlay, color: ICON_COLOR }
  if (AUDIO_EXTS.includes(ext)) return { component: Headset, color: ICON_COLOR }
  if (DOC_EXTS.includes(ext)) return { component: Tickets, color: ICON_COLOR }
  if (TEXT_EXTS.includes(ext)) return { component: Document, color: ICON_COLOR }
  if (ARCHIVE_EXTS.includes(ext)) return { component: Box, color: ICON_COLOR }
  return { component: Files, color: ICON_COLOR }
}

function fileName(item) {
  return item.server_filename || item.filename || item.path || '-'
}

function formatSize(size) {
  return formatBytes(size)
}

function formatTime(timestamp) {
  return formatTimestamp(timestamp)
}

function fileMeta(item) {
  return item.isdir === 1
    ? '文件夹'
    : `${formatTime(item.server_mtime)} · ${formatSize(item.size)}`
}

function openItem(item) {
  if (item.isdir === 1) emit('navigate', item.path)
}

function triggerAction(item, type) {
  emit('action', { item, type })
}
</script>

<template>
  <div class="file-list" v-loading="loading">
    <el-empty v-if="!loading && files.length === 0" description="暂无文件" />

    <ul v-else class="file-rows">
      <li
        v-for="item in files"
        :key="item.fs_id"
        class="file-row"
        :class="{ folder: item.isdir === 1 }"
        @click="openItem(item)"
      >
        <el-icon class="file-icon" :style="{ color: iconFor(item).color }">
          <component :is="iconFor(item).component" />
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
              title="下载"
              @click="triggerAction(item, 'download')"
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

.file-row:hover {
  background: #f5f7fa;
}

.file-icon {
  flex-shrink: 0;
  font-size: 24px;
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
