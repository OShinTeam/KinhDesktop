<script setup>
import { computed } from 'vue'

const props = defineProps({
  path: { type: String, default: '/' }
})

const emit = defineEmits(['navigate'])

const parts = computed(() => props.path.split('/').filter(Boolean))
const isRoot = computed(() => parts.value.length === 0)

// 单段名称最大显示宽度（按视觉宽度计：全角=2、半角=1）
const MAX_NAME_WIDTH = 36
// 中间层级最多保留的段数（首尾之外）
const MAX_VISIBLE_MIDDLE = 3

// 计算字符串视觉宽度
function charWidth(ch) {
  return ch.charCodeAt(0) > 255 ? 2 : 1
}

function displayWidth(str) {
  let width = 0
  for (const ch of str) width += charWidth(ch)
  return width
}

// 截断超长名称：保留扩展名，中段以 … 代替
function truncate(name) {
  if (!name) return name
  if (displayWidth(name) <= MAX_NAME_WIDTH) return name

  const dotIndex = name.lastIndexOf('.')
  const ext = dotIndex > 0 ? name.slice(dotIndex) : ''
  const stem = ext ? name.slice(0, dotIndex) : name
  const extWidth = displayWidth(ext)
  const ellipsis = '…'
  const budget = MAX_NAME_WIDTH - extWidth - displayWidth(ellipsis)
  if (budget <= 0) return ellipsis + ext

  let width = 0
  let cut = 0
  for (let i = 0; i < stem.length; i += 1) {
    width += charWidth(stem[i])
    if (width > budget) break
    cut = i + 1
  }
  return stem.slice(0, cut) + ellipsis + ext
}

// 路径过深时屏蔽中间层级：保留前 MAX_VISIBLE_MIDDLE 段与最后一段，其余显示为省略号
const visibleParts = computed(() => {
  const list = parts.value
  if (list.length <= MAX_VISIBLE_MIDDLE + 2) return list.map((name, index) => ({ name, index }))
  const kept = []
  for (let i = 0; i < MAX_VISIBLE_MIDDLE; i += 1) kept.push({ name: list[i], index: i })
  kept.push(null) // null 代表省略号（不可点击）
  kept.push({ name: list[list.length - 1], index: list.length - 1 })
  return kept
})

function pathAt(index) {
  return '/' + parts.value.slice(0, index + 1).join('/') + '/'
}

function goBack() {
  const list = parts.value
  if (list.length <= 1) {
    emit('navigate', '/')
    return
  }
  emit('navigate', pathAt(list.length - 2))
}
</script>

<template>
  <div class="breadcrumb">
    <template v-if="isRoot">
      <span class="crumb current">全部文件</span>
    </template>

    <template v-else>
      <button class="crumb link" type="button" @click="goBack">返回上一级</button>
      <span class="divider">|</span>
      <button class="crumb link" type="button" @click="emit('navigate', '/')">全部文件</button>
      <span class="divider">&gt;</span>

      <template v-for="(part, position) in visibleParts" :key="position">
        <span v-if="part === null" class="crumb ellipsis">…</span>
        <button
          v-else-if="part.index === parts.length - 1"
          class="crumb current"
          type="button"
          :title="part.name"
        >
          {{ truncate(part.name) }}
        </button>
        <button
          v-else
          class="crumb link"
          type="button"
          :title="part.name"
          @click="emit('navigate', pathAt(part.index))"
        >
          {{ truncate(part.name) }}
        </button>

        <span v-if="part !== null && part.index !== parts.length - 1" class="divider">&gt;</span>
      </template>
    </template>
  </div>
</template>

<style scoped>
.breadcrumb {
  display: flex;
  align-items: center;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  font-size: 13px;
  color: #606266;
}

.crumb {
  flex-shrink: 0;
  max-width: 280px;
  padding: 0;
  border: 0;
  background: transparent;
  font-size: 13px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.crumb.link {
  color: #409eff;
  cursor: pointer;
}

.crumb.link:hover {
  text-decoration: underline;
}

.crumb.current {
  color: #303133;
  cursor: default;
}

.crumb.ellipsis {
  cursor: default;
  color: #909399;
}

.divider {
  flex-shrink: 0;
  margin: 0 6px;
  color: #c0c4cc;
}
</style>
