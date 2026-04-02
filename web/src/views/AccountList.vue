<template>
  <div>
    <h2>账号管理</h2>
    <div style="margin-bottom: 16px; display: flex; gap: 10px; align-items: center;">
      <el-button type="primary" @click="showAdd">添加账号</el-button>
      <el-select v-model="filterApp" placeholder="筛选应用" clearable style="width: 160px;" @change="fetchData">
        <el-option v-for="app in appTemplates" :key="app.identifier" :label="app.display_name" :value="app.identifier" />
      </el-select>
    </div>
    <el-table :data="accounts" border>
      <el-table-column prop="display_name" label="名称" width="180" />
      <el-table-column prop="app_identifier" label="应用" width="140" />
      <el-table-column label="凭证" min-width="200">
        <template #default="{ row }">
          <span v-for="(v, k) in (row.masked_credentials || {})" :key="k" style="margin-right: 8px;">
            {{ k }}: {{ v }}
          </span>
        </template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'active' ? 'success' : row.status === 'banned' ? 'danger' : 'warning'">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="notes" label="备注" width="150" />
      <el-table-column label="操作" width="160">
        <template #default="{ row }">
          <el-button size="small" @click="editAccount(row)">编辑</el-button>
          <el-button size="small" type="danger" @click="delAccount(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" :title="isEdit ? '编辑账号' : '添加账号'" width="500px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="应用类型" v-if="!isEdit">
          <el-select v-model="form.app_identifier" @change="onAppChange" style="width: 100%;">
            <el-option v-for="app in appTemplates" :key="app.identifier" :label="app.display_name" :value="app.identifier" />
          </el-select>
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="form.display_name" />
        </el-form-item>
        <template v-for="key in configKeys" :key="key">
          <el-form-item :label="key">
            <el-input v-model="form.credentials[key]" :type="key.includes('password') || key.includes('token') || key.includes('key') ? 'password' : 'text'" show-password />
          </el-form-item>
        </template>
        <el-form-item label="状态" v-if="isEdit">
          <el-select v-model="form.status" style="width: 100%;">
            <el-option label="活跃" value="active" />
            <el-option label="暂停" value="suspended" />
            <el-option label="封禁" value="banned" />
          </el-select>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.notes" type="textarea" />
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
import { ref, onMounted, computed } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const accounts = ref([])
const appTemplates = ref([])
const filterApp = ref('')
const dialogVisible = ref(false)
const isEdit = ref(false)
const editId = ref('')
const form = ref({ app_identifier: '', display_name: '', credentials: {}, status: 'active', notes: '' })

const configKeys = computed(() => {
  const tmpl = appTemplates.value.find(t => t.identifier === form.value.app_identifier)
  if (!tmpl) return []
  try { return JSON.parse(tmpl.supported_configs) } catch { return [] }
})

const fetchData = async () => {
  try {
    const url = filterApp.value ? `/api/v1/accounts?app_identifier=${filterApp.value}` : '/api/v1/accounts'
    accounts.value = (await axios.get(url)).data.data || []
    appTemplates.value = (await axios.get('/api/v1/apps')).data.data || []
  } catch {}
}
onMounted(fetchData)

const showAdd = () => {
  isEdit.value = false
  form.value = { app_identifier: '', display_name: '', credentials: {}, status: 'active', notes: '' }
  dialogVisible.value = true
}

const onAppChange = () => { form.value.credentials = {} }

const editAccount = async (row) => {
  isEdit.value = true
  editId.value = row.id
  try {
    const res = await axios.get(`/api/v1/accounts/${row.id}`)
    const d = res.data.data
    form.value = { app_identifier: d.app_identifier, display_name: d.display_name, credentials: d.credentials || {}, status: d.status, notes: d.notes || '' }
  } catch {}
  dialogVisible.value = true
}

const submitForm = async () => {
  try {
    if (isEdit.value) {
      await axios.put(`/api/v1/accounts/${editId.value}`, form.value)
    } else {
      await axios.post('/api/v1/accounts', form.value)
    }
    ElMessage.success('操作成功')
    dialogVisible.value = false
    fetchData()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '操作失败')
  }
}

const delAccount = async (id) => {
  try {
    await ElMessageBox.confirm('确定删除该账号？')
    await axios.delete(`/api/v1/accounts/${id}`)
    ElMessage.success('已删除')
    fetchData()
  } catch (e) { if (e !== 'cancel') ElMessage.error(e.response?.data?.error || '删除失败') }
}
</script>
