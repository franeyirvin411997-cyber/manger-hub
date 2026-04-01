<template>
  <el-container class="layout-container" v-if="$route.name !== 'Login'">
    <el-aside width="200px">
      <el-menu :default-active="$route.path" router class="el-menu-vertical">
        <el-menu-item index="/nodes">
          <el-icon><Monitor /></el-icon>
          <span>节点管理</span>
        </el-menu-item>
        <el-menu-item index="/proxies">
          <el-icon><Connection /></el-icon>
          <span>代理资源池</span>
        </el-menu-item>
        <el-menu-item index="/groups">
          <el-icon><SetUp /></el-icon>
          <span>代理组编排</span>
        </el-menu-item>
        <el-menu-item index="/rules">
          <el-icon><Setting /></el-icon>
          <span>规则引擎</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header>
        多节点流量变现运营平台
        <el-button @click="logout" style="float: right; margin-top: 15px;" type="danger" size="small">退出</el-button>
      </el-header>
      <el-main>
        <router-view></router-view>
      </el-main>
    </el-container>
  </el-container>
  <router-view v-else></router-view>
</template>

<script setup>
import { Monitor, Connection, SetUp, Setting } from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const logout = () => {
  localStorage.removeItem('auth_token')
  router.push('/login')
}
</script>

<style>
.layout-container {
  height: 100vh;
}
.el-header {
  background-color: #409EFF;
  color: white;
  line-height: 60px;
  font-size: 20px;
  font-weight: bold;
}
.el-aside {
  background-color: #f4f4f5;
}
.el-menu-vertical {
  height: 100%;
}
</style>
