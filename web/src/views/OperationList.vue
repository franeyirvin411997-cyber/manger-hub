<template>
  <div>
    <h2>操作审计</h2>

    <el-tabs v-model="activeTab">
      <el-tab-pane label="业务操作记录 (Operations)" name="operations">
        <el-table :data="operations" border style="width: 100%" size="small">
          <el-table-column prop="id" label="操作ID" width="300"></el-table-column>
          <el-table-column prop="type" label="操作类型" width="150"></el-table-column>
          <el-table-column prop="target_id" label="目标组ID" width="300"></el-table-column>
          <el-table-column prop="status" label="状态" width="120">
            <template #default="scope">
              <el-tag :type="getTagType(scope.row.status)">{{ scope.row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" label="创建时间"></el-table-column>
        </el-table>
      </el-tab-pane>

      <el-tab-pane label="远程执行任务 (Tasks)" name="tasks">
        <el-table :data="tasks" border style="width: 100%" size="small">
          <el-table-column prop="id" label="任务ID" width="300"></el-table-column>
          <el-table-column prop="operation_id" label="关联操作" width="300"></el-table-column>
          <el-table-column prop="node_id" label="目标节点" width="150"></el-table-column>
          <el-table-column prop="type" label="指令" width="150"></el-table-column>
          <el-table-column prop="status" label="执行状态" width="120">
            <template #default="scope">
              <el-tag :type="getTagType(scope.row.status)">{{ scope.row.status }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="error_message" label="错误信息"></el-table-column>
        </el-table>
      </el-tab-pane>
    </el-tabs>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const activeTab = ref('operations')
const operations = ref([])
const tasks = ref([])

const getTagType = (status) => {
  if (status === 'success') return 'success'
  if (status === 'failed' || status === 'error') return 'danger'
  if (status === 'running') return 'warning'
  return 'info'
}

const fetchData = async () => {
  try {
    const resOps = await axios.get('/api/v1/operations')
    operations.value = resOps.data.data || []

    const resTasks = await axios.get('/api/v1/tasks')
    tasks.value = resTasks.data.data || []
  } catch (e) {
    ElMessage.error('获取审计日志失败')
  }
}

onMounted(() => {
  fetchData()
})
</script>
