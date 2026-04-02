<template>
  <div>
    <h2>应用模板管理</h2>
    <el-button type="primary" @click="showAdd" style="margin-bottom: 20px;">添加模板</el-button>

    <el-table :data="apps" border style="width: 100%">
      <el-table-column prop="identifier" label="标识" width="140" />
      <el-table-column prop="display_name" label="名称" width="140" />
      <el-table-column prop="default_image" label="默认镜像" width="220" />
      <el-table-column prop="supported_configs" label="配置变量" min-width="160" />
      <el-table-column prop="command_template" label="命令模板" min-width="240">
        <template #default="{ row }">
          <span style="font-size: 12px; word-break: break-all;">{{ row.command_template }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="140">
        <template #default="{ row }">
          <el-button size="small" @click="showEdit(row)">编辑</el-button>
          <el-button type="danger" size="small" @click="deleteApp(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑模板' : '添加模板'" width="55%">
      <el-form :model="form" label-width="160px">
        <el-form-item label="应用标识 (英文)">
          <el-input v-model="form.identifier" :disabled="isEdit" placeholder="例如：repocket" />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="form.display_name" placeholder="例如：Repocket" />
        </el-form-item>
        <el-form-item label="默认 Docker 镜像">
          <el-input v-model="form.default_image" placeholder="例如：repocket/repocket:latest" />
        </el-form-item>
        <el-form-item label="配置变量 (JSON数组)">
          <el-input v-model="form.supported_configs" placeholder='例如：["email", "api_key"]' />
        </el-form-item>
        <el-form-item label="命令模板 (JSON数组)">
          <el-input type="textarea" :rows="4" v-model="form.command_template" placeholder='例如：["-e", "RP_EMAIL={{email}}", "repocket/repocket:latest"]' />
          <div style="font-size: 12px; color: #888; margin-top: 5px;">
            变量用双大括号包围，如 {{email}}。数组元素对应 docker run 参数。
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitForm">确定</el-button>
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
const isEdit = ref(false)
const editId = ref('')
const form = ref({
  identifier: '', display_name: '', default_image: '',
  supported_configs: '[]', command_template: '[]',
})

const fetchApps = async () => {
  try {
    apps.value = (await axios.get('/api/v1/apps')).data.data || []
  } catch { ElMessage.error('获取模板失败') }
}
onMounted(fetchApps)

const showAdd = () => {
  isEdit.value = false
  form.value = { identifier: '', display_name: '', default_image: '', supported_configs: '[]', command_template: '[]' }
  dialogVisible.value = true
}

const showEdit = (row) => {
  isEdit.value = true
  editId.value = row.id
  form.value = {
    identifier: row.identifier,
    display_name: row.display_name,
    default_image: row.default_image,
    supported_configs: row.supported_configs,
    command_template: row.command_template,
  }
  dialogVisible.value = true
}

const submitForm = async () => {
  try {
    JSON.parse(form.value.supported_configs)
    JSON.parse(form.value.command_template)
  } catch {
    ElMessage.error('配置变量或命令模板必须是合法的 JSON 数组')
    return
  }
  try {
    if (isEdit.value) {
      await axios.put(`/api/v1/apps/${editId.value}`, form.value)
      ElMessage.success('更新成功')
    } else {
      await axios.post('/api/v1/apps', form.value)
      ElMessage.success('添加成功')
    }
    dialogVisible.value = false
    fetchApps()
  } catch (err) {
    ElMessage.error(err.response?.data?.error || '操作失败')
  }
}

const deleteApp = async (id) => {
  try {
    await ElMessageBox.confirm('确定删除该模板？', '提示', { type: 'warning' })
    await axios.delete(`/api/v1/apps/${id}`)
    ElMessage.success('删除成功')
    fetchApps()
  } catch (e) { if (e !== 'cancel') ElMessage.error('删除失败') }
}
</script>
