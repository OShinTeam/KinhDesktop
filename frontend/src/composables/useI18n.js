import { ref, onMounted } from 'vue'
import { App } from '../../bindings/kinh-desktop/service'

// v3 的绑定按服务（命名空间）导出，这里解构回扁平函数，沿用原有的调用写法
const { GetLangTextMap } = App

// 全局单例状态
const textMap = ref({})
const isLoaded = ref(false)

export function useI18n() {
  const loadTextMap = async () => {
    try {
      const map = await GetLangTextMap()
      textMap.value = map || {}
      isLoaded.value = true
    } catch (error) {
      console.error('加载语言包失败:', error)
      textMap.value = {}
      isLoaded.value = true
    }
  }

  const t = (key, defaultValue = '') => {
    return textMap.value[key] || defaultValue || key
  }

  onMounted(() => {
    if (!isLoaded.value) {
      loadTextMap()
    }
  })

  return {
    t,
    loadTextMap
  }
}