<template>
  <div>
    <h2>代理资源池</h2>
    <div style="margin-bottom: 20px; display: flex; gap: 10px;">
      <el-button type="primary" @click="dialogVisible = true">添加代理</el-button>
      <el-upload
        action="/api/v1/proxies/csv"
        :headers="uploadHeaders"
        :show-file-list="false"
        :on-success="handleUploadSuccess"
        :on-error="handleUploadError"
        accept=".csv"
      >
        <el-button type="success">导入 CSV</el-button>
      </el-upload>
    </div>
    <el-table :data="proxies" border style="width: 100%">
      <el-table-column prop="id" label="代理ID" width="300"></el-table-column>
      <el-table-column prop="protocol" label="协议" width="100"></el-table-column>
      <el-table-column prop="host" label="主机地址" width="180"></el-table-column>
      <el-table-column prop="port" label="端口" width="100"></el-table-column>
      <el-table-column prop="pool_type" label="池类型" width="120"></el-table-column>
      <el-table-column prop="status" label="状态" width="120">
        <template #default="scope">
          <el-tag :type="scope.row.status === 'online' ? 'success' : scope.row.status === 'in_use' ? 'warning' : 'info'">
            {{ scope.row.status }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="220">
        <template #default="{ row }">
          <el-button size="small" type="success" v-if="row.pool_type === 'observer'" @click="promote(row.id)">晋升</el-button>
          <el-button size="small" type="warning" v-if="row.pool_type === 'formal' && row.status !== 'in_use'" @click="demote(row.id)">降级</el-button>
          <el-button size="small" type="danger" v-if="row.status !== 'in_use'" @click="delProxy(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="添加代理" width="30%">
      <el-form :model="form" label-width="100px">
        <el-form-item label="协议">
          <el-select v-model="form.protocol">
            <el-option label="socks5" value="socks5" />
            <el-option label="http" value="http" />
          </el-select>
        </el-form-item>
        <el-form-item label="主机地址">
          <el-input v-model="form.host"></el-input>
        </el-form-item>
        <el-form-item label="端口">
          <el-input-number v-model="form.port" :min="1" :max="65535"></el-input-number>
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="form.username"></el-input>
        </el-form-item>
        <el-form-item label="密码">
          <el-input type="password" v-model="form.password" show-password></el-input>
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="submitForm">确定</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const uploadHeaders = computed(() => {
  const token = localStorage.getItem('auth_token')
  return token ? { Authorization: `Basic ${token}` } : {}
})

const proxies = ref([])
const dialogVisible = ref(false)
const form = ref({
  protocol: 'socks5',
  host: '',
  port: 1080,
  username: '',
  password: ''
})

const fetchProxies = async () => {
  const res = await axios.get('/api/v1/proxies')
  proxies.value = res.data.data || []
}

onMounted(() => {
  fetchProxies()
})

const submitForm = async () => {
  try {
    await axios.post('/api/v1/proxies', form.value)
    ElMessage.success('添加成功')
    dialogVisible.value = false
    fetchProxies()
  } catch (err) {
    ElMessage.error('添加失败: ' + err.message)
  }
}

const handleUploadSuccess = (response) => {
  ElMessage.success(response.message || '导入成功')
  fetchProxies()
}

const handleUploadError = (err) => {
  ElMessage.error('导入失败')
}

const promote = async (id) => {
  await axios.post(`/api/v1/proxies/${id}/promote`)
  ElMessage.success('已晋升到正式池')
  fetchProxies()
}

const demote = async (id) => {
  await axios.post(`/api/v1/proxies/${id}/demote`)
  ElMessage.success('已降级到观察池')
  fetchProxies()
}

const delProxy = async (id) => {
  try {
    await ElMessageBox.confirm('确定删除？')
    await axios.delete(`/api/v1/proxies/${id}`)
    ElMessage.success('已删除')
    fetchProxies()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.error || '删除失败') }
}
</script>
