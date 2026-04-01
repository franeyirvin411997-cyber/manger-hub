<template>
  <div>
    <h2>节点列表</h2>
    <el-table :data="nodes" border style="width: 100%">
      <el-table-column prop="id" label="节点ID"></el-table-column>
      <el-table-column prop="display_name" label="显示名"></el-table-column>
      <el-table-column prop="hostname" label="主机名"></el-table-column>
      <el-table-column prop="online_state" label="状态">
        <template #default="scope">
          <el-tag :type="scope.row.online_state === 'online' ? 'success' : 'danger'">
            {{ scope.row.online_state === 'online' ? '在线' : '离线' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="version" label="版本号"></el-table-column>
      <el-table-column prop="last_heartbeat_at" label="最近心跳"></el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import axios from 'axios'

const nodes = ref([])

onMounted(async () => {
  const res = await axios.get('/api/v1/nodes')
  nodes.value = res.data.data
})
</script>
