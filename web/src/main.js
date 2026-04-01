import { createApp } from 'vue'
import './style.css'
import App from './App.vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import router from './router'
import axios from 'axios'

// Set default auth token if available
const token = localStorage.getItem('auth_token')
if (token) {
  axios.defaults.headers.common['Authorization'] = `Basic ${token}`
}

const app = createApp(App)

app.use(ElementPlus)
app.use(router)

app.mount('#app')
