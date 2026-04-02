<template>
  <el-container class="layout-container" v-if="$route.name !== 'Login'">
    <el-aside width="200px">
      <el-menu :default-active="$route.path" router class="el-menu-vertical">
        <el-menu-item index="/dashboard">
          <el-icon><DataAnalysis /></el-icon>
          <span>仪表盘</span>
        </el-menu-item>
        <el-menu-item index="/nodes">
          <el-icon><Monitor /></el-icon>
          <span>节点管理</span>
        </el-menu-item>
        <el-menu-item index="/proxies">
          <el-icon><Connection /></el-icon>
          <span>代理资源池</span>
        </el-menu-item>
        <el-menu-item index="/deploy">
          <el-icon><Promotion /></el-icon>
          <span>部署编排</span>
        </el-menu-item>
        <el-menu-item index="/groups">
          <el-icon><SetUp /></el-icon>
          <span>代理组管理</span>
        </el-menu-item>
        <el-menu-item index="/apps">
          <el-icon><Menu /></el-icon>
          <span>应用模板</span>
        </el-menu-item>
        <el-menu-item index="/accounts">
          <el-icon><User /></el-icon>
          <span>账号管理</span>
        </el-menu-item>
        <el-menu-item index="/browsers">
          <el-icon><Platform /></el-icon>
          <span>浏览器管理</span>
        </el-menu-item>
        <el-menu-item index="/rules">
          <el-icon><Setting /></el-icon>
          <span>规则引擎</span>
        </el-menu-item>
        <el-menu-item index="/config">
          <el-icon><Tools /></el-icon>
          <span>系统配置</span>
        </el-menu-item>
        <el-menu-item index="/ai">
          <el-icon><ChatDotRound /></el-icon>
          <span>AI 助手</span>
        </el-menu-item>
        <el-menu-item index="/operations">
          <el-icon><Document /></el-icon>
          <span>操作审计</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header>
        多节点流量运营平台
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
import { Monitor, Connection, SetUp, Setting, Document, Menu, User, Platform, Tools, ChatDotRound, DataAnalysis, Promotion } from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const logout = () => {
  localStorage.removeItem('auth_token')
  router.push('/login')
}
</script>

<style>
.layout-container { height: 100vh; }
.el-header { background-color: #409EFF; color: white; line-height: 60px; font-size: 20px; font-weight: bold; }
.el-aside { background-color: #f4f4f5; }
.el-menu-vertical { height: 100%; }
</style>
