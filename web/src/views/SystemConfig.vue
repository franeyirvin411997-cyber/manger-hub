<template>
  <div>
    <h2>系统配置</h2>
    <el-button type="primary" @click="saveAll" style="margin-bottom: 16px;">保存所有修改</el-button>
    <div v-for="cat in categories" :key="cat" style="margin-bottom: 24px;">
      <h3 style="border-bottom: 1px solid #eee; padding-bottom: 8px;">{{ catLabels[cat] || cat }}</h3>
      <el-form label-width="220px" style="max-width: 700px;">
        <el-form-item v-for="cfg in configsByCategory(cat)" :key="cfg.key" :label="cfg.description || cfg.key">
          <el-input v-model="cfg.value" :type="cfg.key.includes('password') || cfg.key.includes('api_key') || cfg.key.includes('token') ? 'password' : 'text'" show-password />
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>
<script setup>
import { ref, computed, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const configs = ref([])
const catLabels = { general: '通用', proxy: '代理', browser: '浏览器', tgbot: 'Telegram Bot', ai: 'AI 配置' }

const categories = computed(() => {
  const cats = [...new Set(configs.value.map(c => c.category))]
  return ['general', 'proxy', 'browser', 'tgbot', 'ai'].filter(c => cats.includes(c))
})
const configsByCategory = (cat) => configs.value.filter(c => c.category === cat)

onMounted(async () => {
  try { configs.value = (await axios.get('/api/v1/config')).data.data || [] } catch {}
})

const saveAll = async () => {
  const changedConfigs = {}
  configs.value.forEach(c => {
    if (!c.value.includes('****')) { changedConfigs[c.key] = c.value }
  })
  try {
    await axios.put('/api/v1/config/batch', { configs: changedConfigs })
    ElMessage.success('配置已保存')
  } catch (e) { ElMessage.error('保存失败') }
}
</script>
