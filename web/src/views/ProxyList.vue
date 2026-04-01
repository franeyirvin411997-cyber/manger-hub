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
          <el-tag :type="scope.row.status === 'online' ? 'success' : 'info'">
            {{ scope.row.status }}
          </el-tag>
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
import { ElMessage } from 'element-plus'

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
</script>
