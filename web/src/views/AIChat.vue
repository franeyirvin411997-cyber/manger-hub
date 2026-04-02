<template>
  <div>
    <h2>AI 助手</h2>
    <div style="max-width: 800px;">
      <div ref="chatBox" style="height: 55vh; overflow-y: auto; border: 1px solid #eee; border-radius: 8px; padding: 16px; margin-bottom: 16px; background: #fafafa;">
        <div v-for="(msg, i) in messages" :key="i" :style="{ textAlign: msg.role === 'user' ? 'right' : 'left', marginBottom: '12px' }">
          <el-tag :type="msg.role === 'user' ? '' : 'success'" style="max-width: 85%; white-space: pre-wrap; text-align: left; padding: 8px 12px; line-height: 1.5;">
            {{ msg.role === 'user' ? '👤 ' : '🤖 ' }}{{ msg.content }}
          </el-tag>
        </div>
        <div v-if="loading" style="text-align: left;">
          <el-tag type="info">🤖 思考中...</el-tag>
        </div>
      </div>
      <div style="display: flex; gap: 8px;">
        <el-input v-model="input" placeholder="输入自然语言指令..." @keyup.enter="send" :disabled="loading" />
        <el-button type="primary" @click="send" :loading="loading">发送</el-button>
      </div>
    </div>
  </div>
</template>
<script setup>
import { ref, nextTick } from 'vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const messages = ref([])
const input = ref('')
const loading = ref(false)
const chatBox = ref(null)

const send = async () => {
  if (!input.value.trim() || loading.value) return
  const text = input.value.trim()
  messages.value.push({ role: 'user', content: text })
  input.value = ''
  loading.value = true
  await nextTick()
  chatBox.value?.scrollTo(0, chatBox.value.scrollHeight)

  try {
    const res = await axios.post('/api/v1/ai/chat', { message: text })
    const data = res.data.data
    let reply = data.response || '(无响应)'
    if (data.tools_used?.length) {
      reply += `\n\n[使用工具: ${data.tools_used.join(', ')}]`
    }
    messages.value.push({ role: 'ai', content: reply })
  } catch (e) {
    messages.value.push({ role: 'ai', content: '❌ ' + (e.response?.data?.error || e.message) })
  } finally {
    loading.value = false
    await nextTick()
    chatBox.value?.scrollTo(0, chatBox.value.scrollHeight)
  }
}
</script>
