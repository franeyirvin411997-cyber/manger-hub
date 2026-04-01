<template>
  <div>
    <h2>代理组编排</h2>
    <el-button type="primary" @click="dialogVisible = true" style="margin-bottom: 20px;">创建代理组</el-button>

    <el-table :data="groups" border style="width: 100%">
      <el-table-column prop="id" label="代理组ID" width="300"></el-table-column>
      <el-table-column prop="node_id" label="部署节点" width="200"></el-table-column>
      <el-table-column prop="tunnel_type" label="隧道类型" width="120"></el-table-column>
      <el-table-column prop="apps" label="应用清单" width="200"></el-table-column>
      <el-table-column prop="created_at" label="创建时间"></el-table-column>
      <el-table-column label="操作" width="200">
        <template #default="scope">
          <el-button type="warning" size="small" @click="handleReplaceProxy(scope.row)">更换代理</el-button>
          <el-button type="primary" size="small" @click="handleMigrateNode(scope.row)">迁移节点</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="创建代理组" width="30%">
      <el-form :model="form" label-width="100px">
        <el-form-item label="部署节点">
          <el-select v-model="form.node_id" placeholder="请选择节点">
            <el-option v-for="node in nodes" :key="node.id" :label="node.display_name" :value="node.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="隧道类型">
          <el-select v-model="form.tunnel_type">
            <el-option label="gost" value="gost" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submitForm">创建</el-button>
        </span>
      </template>
    </el-dialog>

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
const dialogVisible = ref(false)
const form = ref({
  node_id: '',
  tunnel_type: 'gost',
  apps: '[]'
})

const migrateDialogVisible = ref(false)
const currentMigrateGroup = ref(null)
const migrateForm = ref({
  target_node_id: ''
})

const fetchData = async () => {
  try {
    const resGroups = await axios.get('/api/v1/groups')
    groups.value = resGroups.data.data || []

    const resNodes = await axios.get('/api/v1/nodes')
    nodes.value = resNodes.data.data || []
  } catch (e) {
    ElMessage.error('数据加载失败')
  }
}

onMounted(() => {
  fetchData()
})

const submitForm = async () => {
  try {
    await axios.post('/api/v1/groups', form.value)
    ElMessage.success('创建指令已下发')
    dialogVisible.value = false
    fetchData()
  } catch (err) {
    ElMessage.error('创建失败: ' + err.message)
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
