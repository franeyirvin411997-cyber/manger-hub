<template>
  <div>
    <h2>代理组管理</h2>
    <el-button type="primary" @click="$router.push('/deploy')" style="margin-bottom: 20px;">创建代理组</el-button>

    <el-table :data="groups" border style="width: 100%">
      <el-table-column prop="id" label="代理组ID" width="120">
        <template #default="{ row }">{{ row.id?.substring(0,8) }}</template>
      </el-table-column>
      <el-table-column prop="display_name" label="名称" width="150" />
      <el-table-column prop="node_id" label="节点" width="140">
        <template #default="{ row }">{{ row.node_id?.substring(0,12) }}</template>
      </el-table-column>
      <el-table-column prop="tunnel_type" label="隧道" width="100" />
      <el-table-column label="代理" width="160">
        <template #default="{ row }">{{ row.proxy_host ? `${row.proxy_protocol || ''}://${row.proxy_host}:${row.proxy_port}` : '-' }}</template>
      </el-table-column>
      <el-table-column prop="current_state" label="状态" width="110">
        <template #default="{ row }">
          <el-tag :type="row.current_state === 'running' ? 'success' : row.current_state === 'error' ? 'danger' : row.current_state === 'stopped' ? 'info' : 'warning'">
            {{ row.current_state || 'unknown' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="apps" label="应用" width="150" />
      <el-table-column prop="last_error" label="错误" min-width="120">
        <template #default="{ row }"><span style="color: #F56C6C; font-size: 12px;">{{ row.last_error }}</span></template>
      </el-table-column>
      <el-table-column label="操作" width="380">
        <template #default="{ row }">
          <el-button type="success" size="small" @click="handleLifecycle(row.id, 'start')">启动</el-button>
          <el-button type="info" size="small" @click="handleLifecycle(row.id, 'stop')">停止</el-button>
          <el-button type="warning" size="small" @click="handleLifecycle(row.id, 'restart')">重启</el-button>
          <el-button type="warning" size="small" @click="handleReplaceProxy(row)">换代理</el-button>
          <el-button type="primary" size="small" @click="handleMigrateNode(row)">迁移</el-button>
          <el-button type="danger" size="small" @click="handleDelete(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="migrateDialogVisible" title="迁移代理组到新节点" width="30%">
      <el-form :model="migrateForm" label-width="100px">
        <el-form-item label="目标节点">
          <el-select v-model="migrateForm.target_node_id" placeholder="请选择目标节点">
            <el-option v-for="node in nodes" :key="node.id" :label="node.display_name" :value="node.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="migrateDialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submitMigrate">确认迁移</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const groups = ref([])
const nodes = ref([])

const migrateDialogVisible = ref(false)
const currentMigrateGroup = ref(null)
const migrateForm = ref({
  target_node_id: ''
})

const fetchData = async () => {
  try {
    groups.value = (await axios.get('/api/v1/groups')).data.data || []
    nodes.value = (await axios.get('/api/v1/nodes')).data.data || []
  } catch (e) {
    ElMessage.error('数据加载失败')
  }
}

onMounted(() => {
  fetchData()
})

const handleLifecycle = async (id, action) => {
  try {
    await axios.post(`/api/v1/groups/${id}/${action}`)
    ElMessage.success('指令已下发')
    fetchData()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  }
}

const handleDelete = async (id) => {
  try {
    await ElMessageBox.confirm('确定要删除该代理组并释放代理吗？', '提示', { type: 'error' })
    await axios.delete(`/api/v1/groups/${id}`)
    ElMessage.success('代理组已删除')
    fetchData()
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(e.response?.data?.error || '删除失败')
  }
}

const handleReplaceProxy = async (group) => {
  try {
    await ElMessageBox.confirm('确定要为该代理组更换代理资源吗？', '提示', { type: 'warning' })
    const res = await axios.post(`/api/v1/groups/${group.id}/replace_proxy`)
    ElMessage.success(res.data.message || '替换指令已下发')
    fetchData()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error(e.response?.data?.error || '替换失败')
    }
  }
}

const handleMigrateNode = (group) => {
  currentMigrateGroup.value = group
  migrateForm.value.target_node_id = ''
  migrateDialogVisible.value = true
}

const submitMigrate = async () => {
  if (!migrateForm.value.target_node_id) {
    ElMessage.warning('请选择目标节点')
    return
  }
  try {
    await axios.post(`/api/v1/groups/${currentMigrateGroup.value.id}/migrate_node`, migrateForm.value)
    ElMessage.success('迁移指令已下发')
    migrateDialogVisible.value = false
    fetchData()
  } catch (err) {
    ElMessage.error(err.response?.data?.error || '迁移失败')
  }
}
</script>
