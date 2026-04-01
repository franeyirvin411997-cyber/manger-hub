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
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const groups = ref([])
const nodes = ref([])
const dialogVisible = ref(false)
const form = ref({
  node_id: '',
  tunnel_type: 'gost',
  apps: '[]'
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
</script>
