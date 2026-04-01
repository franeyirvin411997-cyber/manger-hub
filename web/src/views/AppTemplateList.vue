<template>
  <div>
    <h2>应用模板管理</h2>
    <el-button type="primary" @click="dialogVisible = true" style="margin-bottom: 20px;">添加模板</el-button>

    <el-table :data="apps" border style="width: 100%">
      <el-table-column prop="identifier" label="应用标识 (英文)" width="150"></el-table-column>
      <el-table-column prop="display_name" label="显示名称" width="150"></el-table-column>
      <el-table-column prop="default_image" label="默认镜像" width="200"></el-table-column>
      <el-table-column prop="supported_configs" label="需要的配置变量"></el-table-column>
      <el-table-column prop="command_template" label="启动命令模板 (JSON 数组)"></el-table-column>
      <el-table-column label="操作" width="100">
        <template #default="scope">
          <el-button type="danger" size="small" @click="deleteApp(scope.row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="添加应用模板" width="50%">
      <el-form :model="form" label-width="150px">
        <el-form-item label="应用标识 (英文)">
          <el-input v-model="form.identifier" placeholder="例如：repocket"></el-input>
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="form.display_name" placeholder="例如：Repocket"></el-input>
        </el-form-item>
        <el-form-item label="默认 Docker 镜像">
          <el-input v-model="form.default_image" placeholder="例如：repocket/repocket:latest"></el-input>
        </el-form-item>
        <el-form-item label="配置变量 (JSON数组)">
          <el-input v-model="form.supported_configs" placeholder='例如：["email", "api_key"]'></el-input>
        </el-form-item>
        <el-form-item label="命令模板 (JSON数组)">
          <el-input type="textarea" :rows="4" v-model="form.command_template" placeholder='例如：["-e", "RP_EMAIL={{email}}", "repocket/repocket:latest"]'></el-input>
          <div style="font-size: 12px; color: #888; margin-top: 5px;">
            提示: 这里填写除固定网络参数外的实际容器启动参数数组。如果使用变量，请用双大括号包围，如 {{email}}。
          </div>
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
import { ref, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const apps = ref([])
const dialogVisible = ref(false)
const form = ref({
  identifier: '',
  display_name: '',
  default_image: '',
  supported_configs: '[]',
  command_template: '[]',
})

const fetchApps = async () => {
  try {
    const res = await axios.get('/api/v1/apps')
    apps.value = res.data.data || []
  } catch (err) {
    ElMessage.error('获取模板失败')
  }
}

onMounted(() => {
  fetchApps()
})

const submitForm = async () => {
  try {
    // 验证 JSON 格式
    JSON.parse(form.value.supported_configs)
    JSON.parse(form.value.command_template)
  } catch (e) {
    ElMessage.error('配置变量或命令模板必须是合法的 JSON 数组')
    return
  }

  try {
    await axios.post('/api/v1/apps', form.value)
    ElMessage.success('添加成功')
    dialogVisible.value = false
    fetchApps()
  } catch (err) {
    ElMessage.error('添加失败: ' + err.response?.data?.error || err.message)
  }
}

const deleteApp = async (id) => {
  try {
    await ElMessageBox.confirm('确定要删除该应用模板吗？', '提示', { type: 'warning' })
    await axios.delete(`/api/v1/apps/${id}`)
    ElMessage.success('删除成功')
    fetchApps()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}
</script>
