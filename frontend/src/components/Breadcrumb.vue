<script setup>
import { computed } from 'vue'

const props = defineProps({
  path: { type: String, default: '/' }
})

const emit = defineEmits(['navigate'])

// 单段名称最大显示字符数，超出部分省略
const MAX_NAME_LENGTH = 24
// 中间层级最多保留的段数（首尾之外）
const MAX_VISIBLE_MIDDLE = 3

const parts = computed(() => props.path.split('/').filter(Boolean))
const isRoot = computed(() => parts.value.length === 0)

function truncate(name) {
  if (!name || name.length <= MAX_NAME_LENGTH) return name
  return name.slice(0, MAX_NAME_LENGTH - 1) + '…'
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
  font-size: 14px;
  color: #606266;
}

.crumb {
  flex-shrink: 0;
  max-width: 280px;
  padding: 0;
  border: 0;
  background: transparent;
  font-size: 14px;
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
  margin: 0 8px;
  color: #c0c4cc;
}
</style>
