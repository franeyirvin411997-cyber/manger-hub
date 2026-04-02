<template>
  <div>
    <h2>系统概览</h2>
    <el-row :gutter="16" style="margin-bottom: 20px;">
      <el-col :span="4" v-for="card in cards" :key="card.label">
        <el-card shadow="hover">
          <div style="text-align: center;">
            <div style="font-size: 28px; font-weight: bold; color: #409EFF;">{{ card.value }}</div>
            <div style="font-size: 13px; color: #999; margin-top: 4px;">{{ card.label }}</div>
            <div v-if="card.sub" style="font-size: 12px; color: #ccc;">{{ card.sub }}</div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>
<script setup>
import { ref, onMounted, computed } from 'vue'
import axios from 'axios'
const info = ref({})
onMounted(async () => {
  try {
    const res = await axios.get('/api/v1/info')
    info.value = res.data
  } catch (e) {}
})
const cards = computed(() => [
  { label: '节点在线', value: `${info.value.nodes_online || 0}/${info.value.nodes_total || 0}` },
  { label: '代理使用中', value: info.value.proxies_in_use || 0, sub: `正式池 ${info.value.proxies_formal || 0}` },
  { label: '观察池', value: info.value.proxies_observer || 0 },
  { label: '代理组运行', value: info.value.groups_running || 0, sub: `异常 ${info.value.groups_error || 0}` },
  { label: '账号活跃', value: info.value.accounts_active || 0, sub: `封禁 ${info.value.accounts_banned || 0}` },
  { label: '浏览器', value: info.value.browsers_running || 0 },
])
</script>
