<template>
  <div class="login-container">
    <el-card class="box-card">
      <template #header>
        <div class="card-header">
          <span>多节点流量变现运营平台</span>
        </div>
      </template>
      <el-form :model="form" @submit.prevent="handleLogin">
        <el-form-item>
          <el-input v-model="form.username" placeholder="Username"></el-input>
        </el-form-item>
        <el-form-item>
          <el-input type="password" v-model="form.password" placeholder="Password"></el-input>
        </el-form-item>
        <el-button type="primary" native-type="submit" style="width: 100%;">登录</el-button>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import axios from 'axios'
import { ElMessage } from 'element-plus'

const router = useRouter()
const form = ref({ username: '', password: '' })

const handleLogin = async () => {
  try {
    const authString = btoa(`${form.value.username}:${form.value.password}`)
    await axios.get('/api/v1/auth', {
      headers: { Authorization: `Basic ${authString}` }
    })

    // Save token logic (for basic auth demo we just use localStorage)
    localStorage.setItem('auth_token', authString)

    // Set default auth for future requests
    axios.defaults.headers.common['Authorization'] = `Basic ${authString}`

    ElMessage.success('登录成功')
    router.push('/nodes')
  } catch (e) {
    ElMessage.error('用户名或密码错误')
  }
}
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background-color: #f4f4f5;
}
.box-card {
  width: 400px;
}
.card-header {
  text-align: center;
  font-weight: bold;
}
</style>
