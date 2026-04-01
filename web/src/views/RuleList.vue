<template>
  <div>
    <h2>规则管理</h2>
    <el-button type="primary" @click="dialogVisible = true" style="margin-bottom: 20px;">添加规则</el-button>

    <el-table :data="rules" border style="width: 100%">
      <el-table-column prop="name" label="规则名称" width="200"></el-table-column>
      <el-table-column prop="description" label="描述" width="300"></el-table-column>
      <el-table-column prop="condition" label="触发条件 (例如: latency > 500)"></el-table-column>
      <el-table-column prop="action" label="执行动作" width="200"></el-table-column>
      <el-table-column label="操作" width="100">
        <template #default="scope">
          <el-button type="danger" size="small" @click="deleteRule(scope.row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="dialogVisible" title="添加自动化规则" width="30%">
      <el-form :model="form" label-width="100px">
        <el-form-item label="规则名称">
          <el-input v-model="form.name" placeholder="例如：延迟过高下线"></el-input>
        </el-form-item>
        <el-form-item label="规则描述">
          <el-input v-model="form.description"></el-input>
        </el-form-item>
        <el-form-item label="触发条件">
          <el-input v-model="form.condition" placeholder="例如：latency > 500"></el-input>
        </el-form-item>
        <el-form-item label="执行动作">
          <el-select v-model="form.action">
            <el-option label="标记为失效 (mark_offline)" value="mark_offline" />
            <el-option label="尝试重启 (restart_app)" value="restart_app" />
          </el-select>
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

const rules = ref([])
const dialogVisible = ref(false)
const form = ref({
  name: '',
  description: '',
  condition: '',
  action: 'mark_offline'
})

const fetchRules = async () => {
  try {
    const res = await axios.get('/api/v1/rules')
    rules.value = res.data.data || []
  } catch (err) {
    ElMessage.error('获取规则失败')
  }
}

onMounted(() => {
  fetchRules()
})

const submitForm = async () => {
  if (!form.value.name || !form.value.condition) {
    ElMessage.warning('名称和条件不能为空')
    return
  }
  try {
    await axios.post('/api/v1/rules', form.value)
    ElMessage.success('添加成功')
    dialogVisible.value = false
    fetchRules()
  } catch (err) {
    ElMessage.error('添加失败: ' + err.message)
  }
}

const deleteRule = async (id) => {
  try {
    await ElMessageBox.confirm('确定要删除该规则吗？', '提示', { type: 'warning' })
    await axios.delete(`/api/v1/rules/${id}`)
    ElMessage.success('删除成功')
    fetchRules()
  } catch (e) {
    if (e !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}
</script>
