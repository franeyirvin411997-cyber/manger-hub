<template>
  <div>
    <h2>浏览器管理</h2>
    <el-button type="primary" @click="showCreate" style="margin-bottom: 16px;">创建浏览器</el-button>

    <el-table :data="browsers" border>
      <el-table-column prop="display_name" label="名称" width="150" />
      <el-table-column prop="node_id" label="节点" width="160">
        <template #default="{ row }">{{ row.node_id?.substring(0,12) }}</template>
      </el-table-column>
      <el-table-column label="代理" width="200">
        <template #default="{ row }">
          <span v-if="row.proxy_host">{{ row.proxy_protocol }}://{{ row.proxy_host }}:{{ row.proxy_port }}</span>
          <span v-else style="color: #999;">无代理</span>
        </template>
      </el-table-column>
      <el-table-column prop="account_id" label="绑定账号" width="120">
        <template #default="{ row }">{{ row.account_id ? row.account_id.substring(0,8) : '-' }}</template>
      </el-table-column>
      <el-table-column prop="status" label="状态" width="100">
        <template #default="{ row }">
          <el-tag :type="row.status === 'running' ? 'success' : row.status === 'error' ? 'danger' : 'info'">{{ row.status }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="vnc_port" label="VNC端口" width="90" />
      <el-table-column label="操作" width="280">
        <template #default="{ row }">
          <el-button size="small" type="primary" @click="openViewer(row)" :disabled="row.status !== 'running'">查看</el-button>
          <el-button size="small" @click="bindAccount(row)">绑定账号</el-button>
          <el-button size="small" type="warning" @click="stopBrw(row.id)">停止</el-button>
          <el-button size="small" type="danger" @click="delBrw(row.id)">删除</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 创建对话框 -->
    <el-dialog v-model="createVisible" title="创建浏览器" width="450px">
      <el-form :model="createForm" label-width="100px">
        <el-form-item label="名称"><el-input v-model="createForm.display_name" /></el-form-item>
        <el-form-item label="节点">
          <el-select v-model="createForm.node_id" style="width: 100%;">
            <el-option v-for="n in nodes" :key="n.id" :label="n.display_name" :value="n.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="代理">
          <el-select v-model="createForm.proxy_id" clearable placeholder="不选则无代理" style="width: 100%;">
            <el-option v-for="p in availableProxies" :key="p.id" :label="`${p.protocol}://${p.host}:${p.port} [${p.status}]`" :value="p.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="绑定账号">
          <el-select v-model="createForm.account_id" clearable style="width: 100%;">
            <el-option v-for="a in allAccounts" :key="a.id" :label="a.display_name" :value="a.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">取消</el-button>
        <el-button type="primary" @click="doCreate">创建</el-button>
      </template>
    </el-dialog>

    <!-- 绑定账号对话框 -->
    <el-dialog v-model="bindVisible" title="绑定账号" width="400px">
      <el-select v-model="bindAccountId" style="width: 100%;">
        <el-option v-for="a in allAccounts" :key="a.id" :label="a.display_name + ' (' + a.app_identifier + ')'" :value="a.id" />
      </el-select>
      <template #footer>
        <el-button @click="bindVisible = false">取消</el-button>
        <el-button type="primary" @click="doBind">确认绑定</el-button>
      </template>
    </el-dialog>

    <!-- VNC 查看器 -->
    <el-dialog v-model="viewerVisible" :title="'浏览器 - ' + viewerName" width="90%" top="2vh" :close-on-click-modal="false">
      <div v-if="vncUrl" style="text-align: center;">
        <iframe :src="vncUrl" style="width: 100%; height: 75vh; border: 1px solid #ddd; border-radius: 4px;" allow="clipboard-read; clipboard-write" />
      </div>
      <div v-else style="text-align: center; padding: 40px; color: #999;">VNC 连接信息获取中...</div>
    </el-dialog>
  </div>
</template>
<script setup>
import { ref, computed, onMounted } from 'vue'
import axios from 'axios'
import { ElMessage, ElMessageBox } from 'element-plus'

const browsers = ref([])
const nodes = ref([])
const proxies = ref([])
const allAccounts = ref([])
const createVisible = ref(false)
const createForm = ref({ display_name: '', node_id: '', proxy_id: '', account_id: '' })
const bindVisible = ref(false)
const bindBrowserId = ref('')
const bindAccountId = ref('')
const viewerVisible = ref(false)
const vncUrl = ref('')
const viewerName = ref('')

const availableProxies = computed(() => proxies.value.filter(p => p.pool_type === 'formal'))

const fetchData = async () => {
  try {
    browsers.value = (await axios.get('/api/v1/browsers')).data.data || []
    nodes.value = (await axios.get('/api/v1/nodes')).data.data || []
    proxies.value = (await axios.get('/api/v1/proxies')).data.data || []
    allAccounts.value = (await axios.get('/api/v1/accounts')).data.data || []
  } catch {}
}
onMounted(fetchData)

const showCreate = () => {
  createForm.value = { display_name: '', node_id: '', proxy_id: '', account_id: '' }
  createVisible.value = true
}

const doCreate = async () => {
  try {
    await axios.post('/api/v1/browsers', createForm.value)
    ElMessage.success('创建指令已下发')
    createVisible.value = false
    fetchData()
  } catch (e) { ElMessage.error(e.response?.data?.error || '创建失败') }
}

const stopBrw = async (id) => {
  await axios.post(`/api/v1/browsers/${id}/stop`)
  ElMessage.success('停止指令已下发')
  fetchData()
}

const delBrw = async (id) => {
  try {
    await ElMessageBox.confirm('确定删除？')
    await axios.delete(`/api/v1/browsers/${id}`)
    ElMessage.success('已删除')
    fetchData()
  } catch (e) { if (e !== 'cancel') ElMessage.error('删除失败') }
}

const bindAccount = (row) => {
  bindBrowserId.value = row.id
  bindAccountId.value = row.account_id || ''
  bindVisible.value = true
}

const doBind = async () => {
  try {
    await axios.post(`/api/v1/browsers/${bindBrowserId.value}/bind`, { account_id: bindAccountId.value })
    ElMessage.success('绑定成功')
    bindVisible.value = false
    fetchData()
  } catch (e) { ElMessage.error('绑定失败') }
}

const openViewer = async (row) => {
  viewerName.value = row.display_name || row.id.substring(0, 8)
  try {
    const res = await axios.get(`/api/v1/browsers/${row.id}/vnc`)
    if (res.data.vnc_url) {
      const url = new URL(res.data.vnc_url, window.location.origin)
      if (res.data.vnc_password) {
        url.searchParams.set('password', res.data.vnc_password)
      }
      url.searchParams.set('autoconnect', '1')
      vncUrl.value = url.pathname + url.search
    } else {
      vncUrl.value = ''
    }
  } catch { vncUrl.value = '' }
  viewerVisible.value = true
}
</script>
