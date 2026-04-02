<template>
  <div class="node-page">
    <div class="page-header">
      <h2>节点管理</h2>
      <el-button type="primary" data-testid="open-node-enrollment" @click="openEnrollmentDialog">
        添加节点
      </el-button>
    </div>

    <el-table :data="nodes" border style="width: 100%">
      <el-table-column prop="id" label="节点ID" width="200">
        <template #default="{ row }">{{ row.id.substring(0, 16) }}...</template>
      </el-table-column>
      <el-table-column prop="display_name" label="显示名" width="140" />
      <el-table-column prop="hostname" label="主机名" width="140" />
      <el-table-column prop="ip" label="IP" width="130" />
      <el-table-column prop="online_state" label="状态" width="90">
        <template #default="{ row }">
          <el-tag :type="row.online_state === 'online' ? 'success' : 'danger'">
            {{ row.online_state === 'online' ? '在线' : '离线' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="version" label="版本" width="80" />
      <el-table-column prop="max_groups" label="最大组数" width="90" />
      <el-table-column prop="last_heartbeat_at" label="最近心跳" />
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button size="small" :data-testid="`open-node-logs-${row.id}`" @click="openLogDialog(row)">日志</el-button>
          <el-button size="small" @click="editNode(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="delNode(row)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <h3 class="section-title">接入记录</h3>
    <el-table :data="enrollments" border style="width: 100%">
      <el-table-column prop="display_name" label="备注名" min-width="160" />
      <el-table-column prop="manager_http_addr" label="HTTP 地址" min-width="220" />
      <el-table-column prop="manager_grpc_addr" label="gRPC 地址" min-width="180" />
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'used' ? 'success' : row.status === 'expired' ? 'danger' : 'warning'">
            {{ row.status === 'used' ? '已使用' : row.status === 'expired' ? '已过期' : '待使用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" min-width="180" />
      <el-table-column prop="expires_at" label="过期时间" min-width="180" />
      <el-table-column prop="used_at" label="使用时间" min-width="180" />
      <el-table-column prop="used_by_node_id" label="已绑定节点" min-width="180" />
    </el-table>

    <el-dialog v-model="enrollmentVisible" title="添加节点" width="680px">
      <template v-if="!enrollmentResult">
        <el-form :model="enrollmentForm" label-width="140px">
          <el-form-item label="节点备注">
            <div data-testid="node-enrollment-display-name" class="test-hook">
              <el-input v-model="enrollmentForm.display_name" />
            </div>
          </el-form-item>
          <el-form-item label="Manager HTTP 地址">
            <div data-testid="node-enrollment-http" class="test-hook">
              <el-input v-model="enrollmentForm.manager_http_addr" />
            </div>
          </el-form-item>
          <el-form-item label="Manager gRPC 地址">
            <div data-testid="node-enrollment-grpc" class="test-hook">
              <el-input v-model="enrollmentForm.manager_grpc_addr" />
            </div>
          </el-form-item>
          <el-alert
            type="info"
            :closable="false"
            title="安装命令仅展示一次，授权 24 小时有效且只能使用一次，目标服务器需支持 systemd。"
          />
        </el-form>
      </template>

      <template v-else>
        <el-alert
          type="success"
          :closable="false"
          title="安装命令已生成，请尽快复制到目标 VPS 执行。"
        />
        <p class="result-line">过期时间：{{ enrollmentResult.expires_at }}</p>
        <pre data-testid="node-enrollment-command" class="command-box">{{ enrollmentResult.install_command }}</pre>
      </template>

      <template #footer>
        <el-button @click="enrollmentVisible = false">关闭</el-button>
        <el-button
          v-if="!enrollmentResult"
          type="primary"
          data-testid="submit-node-enrollment"
          @click="createEnrollment"
        >
          生成安装命令
        </el-button>
        <el-button v-else type="primary" @click="copyInstallCommand">
          复制命令
        </el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="logVisible" title="节点日志" width="80%" @closed="stopLogs">
      <el-form :inline="true" class="log-toolbar">
        <el-form-item label="日志源">
          <el-select v-model="logForm.sourceType">
            <el-option label="节点服务日志" value="node_service" />
            <el-option label="容器日志" value="container" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="logForm.sourceType === 'container'" label="容器名">
          <el-input v-model="logForm.containerName" placeholder="例如 app-demo / tunnel-group-1" />
        </el-form-item>
      </el-form>

      <el-alert :title="logStatus" :closable="false" data-testid="node-logs-status" />

      <div class="log-grid">
        <section class="log-section">
          <h3>实时日志</h3>
          <pre data-testid="node-logs-live" class="log-pane">{{ liveLogs.join('\n') }}</pre>
        </section>
        <section class="log-section">
          <h3>最近 200 行</h3>
          <pre data-testid="node-logs-history" class="log-pane">{{ historyLogs.join('\n') }}</pre>
        </section>
      </div>

      <template #footer>
        <el-button @click="clearLogs">清空</el-button>
        <el-button @click="stopLogs">停止</el-button>
        <el-button data-testid="start-node-logs" type="primary" @click="startLogs">开始</el-button>
        <el-button @click="restartLogs">重新连接</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="editVisible" title="编辑节点" width="400px">
      <el-form :model="editForm" label-width="100px">
        <el-form-item label="显示名称"><el-input v-model="editForm.display_name" /></el-form-item>
        <el-form-item label="最大组数"><el-input-number v-model="editForm.max_groups" :min="1" :max="500" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" @click="saveNode">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const nodes = ref([])
const editVisible = ref(false)
const editId = ref('')
const editForm = ref({ display_name: '', max_groups: 50 })
const enrollments = ref([])
const enrollmentVisible = ref(false)
const enrollmentResult = ref(null)
const logVisible = ref(false)
const logNode = ref(null)
const logStatus = ref('未连接')
const liveLogs = ref([])
const historyLogs = ref([])
const logForm = ref({ sourceType: 'node_service', containerName: '' })
const enrollmentForm = ref({
  display_name: '',
  manager_http_addr: window.location.origin,
  manager_grpc_addr: `${window.location.hostname}:50051`,
})
let logAbortController = null

const fetchNodes = async () => {
  const res = await axios.get('/api/v1/nodes')
  nodes.value = res.data.data || []
}

const fetchEnrollments = async () => {
  const res = await axios.get('/api/v1/node-enrollments')
  enrollments.value = res.data.data || []
}

const openEnrollmentDialog = () => {
  enrollmentResult.value = null
  enrollmentForm.value = {
    display_name: '',
    manager_http_addr: window.location.origin,
    manager_grpc_addr: `${window.location.hostname}:50051`,
  }
  enrollmentVisible.value = true
}

const createEnrollment = async () => {
  try {
    const res = await axios.post('/api/v1/node-enrollments', enrollmentForm.value)
    enrollmentResult.value = res.data.data
    ElMessage.success('安装命令已生成')
    await fetchEnrollments()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '生成失败')
  }
}

const copyInstallCommand = async () => {
  try {
    await navigator.clipboard.writeText(enrollmentResult.value.install_command)
    ElMessage.success('安装命令已复制')
  } catch (e) {
    ElMessage.error('复制失败')
  }
}

const buildLogUrl = () => {
  const params = new URLSearchParams({ source_type: logForm.value.sourceType })
  if (logForm.value.sourceType === 'container') {
    params.set('container_name', logForm.value.containerName)
  }
  return `/api/v1/nodes/${logNode.value.id}/logs/stream?${params.toString()}`
}

const openLogDialog = (row) => {
  logNode.value = row
  logStatus.value = '未连接'
  liveLogs.value = []
  historyLogs.value = []
  logForm.value = { sourceType: 'node_service', containerName: '' }
  logVisible.value = true
}

const parseEventStream = async (response) => {
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })

    let splitIndex = buffer.indexOf('\n\n')
    while (splitIndex !== -1) {
      const frame = buffer.slice(0, splitIndex)
      buffer = buffer.slice(splitIndex + 2)
      const lines = frame.split('\n')
      const event = lines.find(line => line.startsWith('event:'))?.slice(6).trim()
      const dataLine = lines.find(line => line.startsWith('data:'))?.slice(5).trim()
      if (event && dataLine) {
        const data = JSON.parse(dataLine)
        if (event === 'status') {
          logStatus.value = data.content === 'live_connected' ? '实时流已连接' : data.content
        } else if (event === 'history') {
          historyLogs.value.push(data.content)
        } else if (event === 'live') {
          liveLogs.value.push(data.content)
        } else if (event === 'error') {
          logStatus.value = data.content
        }
      }
      splitIndex = buffer.indexOf('\n\n')
    }
  }
}

const stopLogs = () => {
  if (logAbortController) {
    logAbortController.abort()
    logAbortController = null
  }
  logStatus.value = '日志流已停止'
}

const clearLogs = () => {
  liveLogs.value = []
  historyLogs.value = []
}

const startLogs = () => {
  if (logForm.value.sourceType === 'container' && !logForm.value.containerName) {
    ElMessage.error('请输入容器名')
    return
  }

  stopLogs()
  logStatus.value = '正在连接实时日志...'
  logAbortController = new AbortController()

  fetch(buildLogUrl(), {
    headers: {
      Authorization: `Basic ${localStorage.getItem('auth_token')}`,
      Accept: 'text/event-stream',
    },
    signal: logAbortController.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const text = await response.text()
        throw new Error(text || '日志流建立失败')
      }
      await parseEventStream(response)
      logStatus.value = '日志流已断开'
    })
    .catch((error) => {
      if (error.name !== 'AbortError') {
        logStatus.value = error.message || '日志流已断开'
      }
    })
}

const restartLogs = () => {
  startLogs()
}

const editNode = (row) => {
  editId.value = row.id
  editForm.value = { display_name: row.display_name, max_groups: row.max_groups || 50 }
  editVisible.value = true
}

const saveNode = async () => {
  try {
    await axios.put(`/api/v1/nodes/${editId.value}`, editForm.value)
    ElMessage.success('已更新')
    editVisible.value = false
    await fetchNodes()
  } catch (e) {
    ElMessage.error('更新失败')
  }
}

const delNode = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除节点 ${row.display_name}？`)
    await axios.delete(`/api/v1/nodes/${row.id}`)
    ElMessage.success('已删除')
    await fetchNodes()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '删除失败')
    }
  }
}

onMounted(async () => {
  await Promise.all([fetchNodes(), fetchEnrollments()])
})
</script>

<style scoped>
.node-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.page-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.section-title {
  margin: 8px 0 0;
}

.log-toolbar {
  margin-bottom: 16px;
}

.log-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
  margin-top: 16px;
}

.log-section {
  min-width: 0;
}

.log-section h3 {
  margin: 0 0 8px;
}

.log-pane {
  margin: 0;
  min-height: 260px;
  max-height: 420px;
  overflow: auto;
  padding: 16px;
  border-radius: 8px;
  background: #0f172a;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  line-height: 1.5;
}

.result-line {
  margin: 16px 0 12px;
}

.command-box {
  margin: 0;
  padding: 16px;
  border-radius: 8px;
  background: #111827;
  color: #f9fafb;
  white-space: pre-wrap;
  word-break: break-all;
  font-size: 13px;
  line-height: 1.5;
}

.test-hook {
  width: 100%;
}

@media (max-width: 900px) {
  .log-grid {
    grid-template-columns: 1fr;
  }
}
</style>
