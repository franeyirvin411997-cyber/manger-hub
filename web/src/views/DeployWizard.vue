<template>
  <div>
    <h2>部署编排</h2>
    <el-steps :active="step" align-center style="margin-bottom: 30px;">
      <el-step title="选择节点和隧道" />
      <el-step title="选择应用和账号" />
      <el-step title="确认部署" />
    </el-steps>

    <!-- Step 1 -->
    <el-card v-if="step === 0">
      <el-form :model="form" label-width="120px">
        <el-form-item label="显示名称"><el-input v-model="form.display_name" placeholder="如: 美国-住宅-01" /></el-form-item>
        <el-form-item label="部署节点">
          <el-select v-model="form.node_id" style="width: 100%;">
            <el-option v-for="n in onlineNodes" :key="n.id" :label="`${n.display_name} (${n.online_state})`" :value="n.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="隧道类型">
          <el-radio-group v-model="form.tunnel_type">
            <el-radio value="tun2socks">tun2socks</el-radio>
            <el-radio value="tun2proxy">tun2proxy</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="代理分配">
          <el-radio-group v-model="proxyMode">
            <el-radio value="auto">自动分配</el-radio>
            <el-radio value="manual">手动选择</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="选择代理" v-if="proxyMode === 'manual'">
          <el-select v-model="form.proxy_id" style="width: 100%;">
            <el-option v-for="p in availableProxies" :key="p.id" :label="`${p.protocol}://${p.host}:${p.port} [${p.status}]`" :value="p.id" />
          </el-select>
        </el-form-item>
      </el-form>
      <div style="text-align: right; margin-top: 16px;">
        <el-button type="primary" @click="step = 1" :disabled="!form.node_id">下一步</el-button>
      </div>
    </el-card>

    <!-- Step 2 -->
    <el-card v-if="step === 1">
      <el-form label-width="120px">
        <el-form-item label="选择应用">
          <el-checkbox-group v-model="selectedApps">
            <el-checkbox v-for="app in appTemplates" :key="app.identifier" :value="app.identifier" :label="app.display_name" />
          </el-checkbox-group>
        </el-form-item>
        <div v-for="appId in selectedApps" :key="appId" style="margin: 10px 0 20px 20px; padding-left: 12px; border-left: 3px solid #409EFF;">
          <h4>{{ getAppName(appId) }} — 选择账号</h4>
          <el-select v-model="accountBindings[appId]" placeholder="选择账号" style="width: 300px;" clearable>
            <el-option v-for="acc in getAccountsForApp(appId)" :key="acc.id" :label="acc.display_name" :value="acc.id" />
          </el-select>
          <el-button size="small" style="margin-left: 8px;" @click="quickAddAccount(appId)">快速添加</el-button>
        </div>
        <el-form-item label="创建浏览器">
          <el-switch v-model="createBrowser" />
        </el-form-item>
      </el-form>
      <div style="text-align: right; margin-top: 16px;">
        <el-button @click="step = 0">上一步</el-button>
        <el-button type="primary" @click="step = 2" :disabled="selectedApps.length === 0">下一步</el-button>
      </div>
    </el-card>

    <!-- Step 3 -->
    <el-card v-if="step === 2">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="显示名称">{{ form.display_name || '(未命名)' }}</el-descriptions-item>
        <el-descriptions-item label="节点">{{ getNodeName(form.node_id) }}</el-descriptions-item>
        <el-descriptions-item label="隧道类型">{{ form.tunnel_type }}</el-descriptions-item>
        <el-descriptions-item label="代理">{{ proxyMode === 'auto' ? '自动分配' : form.proxy_id }}</el-descriptions-item>
        <el-descriptions-item label="应用">{{ selectedApps.join(', ') }}</el-descriptions-item>
        <el-descriptions-item v-for="appId in selectedApps" :key="appId" :label="getAppName(appId) + ' 账号'">
          {{ getAccountName(accountBindings[appId]) || '未绑定' }}
        </el-descriptions-item>
        <el-descriptions-item label="创建浏览器">{{ createBrowser ? '是' : '否' }}</el-descriptions-item>
      </el-descriptions>
      <div style="text-align: right; margin-top: 20px;">
        <el-button @click="step = 1">上一步</el-button>
        <el-button type="primary" @click="doDeploy" :loading="deploying">确认部署</el-button>
      </div>
    </el-card>
  </div>
</template>
<script setup>
import { ref, computed, onMounted, reactive } from 'vue'
import axios from 'axios'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'

const router = useRouter()
const step = ref(0)
const form = ref({ display_name: '', node_id: '', tunnel_type: 'tun2socks', proxy_id: '' })
const proxyMode = ref('auto')
const selectedApps = ref([])
const accountBindings = reactive({})
const createBrowser = ref(false)
const deploying = ref(false)

const nodes = ref([])
const proxies = ref([])
const appTemplates = ref([])
const accounts = ref([])

const onlineNodes = computed(() => nodes.value.filter(n => n.online_state === 'online'))
const availableProxies = computed(() => proxies.value.filter(p => p.status === 'online' && p.pool_type === 'formal'))

onMounted(async () => {
  try {
    nodes.value = (await axios.get('/api/v1/nodes')).data.data || []
    proxies.value = (await axios.get('/api/v1/proxies')).data.data || []
    appTemplates.value = (await axios.get('/api/v1/apps')).data.data || []
    accounts.value = (await axios.get('/api/v1/accounts')).data.data || []
  } catch {}
})

const getAppName = id => appTemplates.value.find(t => t.identifier === id)?.display_name || id
const getNodeName = id => nodes.value.find(n => n.id === id)?.display_name || id
const getAccountName = id => accounts.value.find(a => a.id === id)?.display_name || ''
const getAccountsForApp = appId => accounts.value.filter(a => a.app_identifier === appId)

const quickAddAccount = (appId) => { router.push('/accounts') }

const doDeploy = async () => {
  deploying.value = true
  try {
    const payload = {
      display_name: form.value.display_name,
      node_id: form.value.node_id,
      tunnel_type: form.value.tunnel_type,
      proxy_id: proxyMode.value === 'manual' ? form.value.proxy_id : '',
      apps: JSON.stringify(selectedApps.value),
      account_bindings: accountBindings,
    }
    const res = await axios.post('/api/v1/groups', payload)
    ElMessage.success('部署指令已下发')

    // 如果需要创建浏览器
    if (createBrowser.value && res.data.id) {
      await axios.post('/api/v1/browsers', {
        display_name: (form.value.display_name || '浏览器') + '-brw',
        node_id: form.value.node_id,
        proxy_id: proxyMode.value === 'manual' ? form.value.proxy_id : '',
      })
      ElMessage.success('浏览器创建指令已下发')
    }
    router.push('/groups')
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '部署失败')
  } finally { deploying.value = false }
}
</script>
