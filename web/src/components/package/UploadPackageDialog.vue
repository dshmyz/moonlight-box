<template>
  <el-dialog
    :model-value="modelValue"
    @update:model-value="$emit('update:modelValue', $event)"
    title="上传包"
    width="600px"
    @close="handleClose"
  >
    <el-form :model="form" :rules="rules" ref="formRef" label-width="100px">
      <el-form-item label="目标仓库" prop="repositoryId">
        <el-select v-model="form.repositoryId" placeholder="选择仓库" @change="handleRepositoryChange" filterable>
          <el-option 
            v-for="repo in availableRepositories" 
            :key="repo.id" 
            :label="repo.display_name || repo.name" 
            :value="repo.id"
          >
            <span>{{ repo.display_name || repo.name }}</span>
            <span style="color: #8492a6; font-size: 13px; margin-left: 8px">
              ({{ repo.type }} / {{ repo.package_type }})
            </span>
          </el-option>
        </el-select>
      </el-form-item>

      <el-form-item label="包类型">
        <el-tag type="info">{{ form.packageType || '未选择' }}</el-tag>
      </el-form-item>

      <template v-if="form.packageType === 'pypi'">
        <el-form-item label="包名称" prop="pypiName">
          <el-input v-model="form.pypiName" placeholder="例如: requests" />
        </el-form-item>
        <el-form-item label="版本号" prop="pypiVersion">
          <el-input v-model="form.pypiVersion" placeholder="例如: 2.28.0" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="form.pypiSummary" placeholder="可选" />
        </el-form-item>
      </template>

      <template v-if="form.packageType === 'npm'">
        <el-form-item label="包名称" prop="npmName">
          <el-input v-model="form.npmName" placeholder="例如: my-pkg（scope 包如 @scope/pkg）" />
        </el-form-item>
        <el-form-item label="版本号">
          <el-tag type="info">以 tarball 内 package.json 为准</el-tag>
        </el-form-item>
      </template>

      <template v-if="form.packageType === 'maven'">
        <el-alert
          v-if="mavenAutoDetected"
          type="success"
          :closable="false"
          show-icon
          style="margin-bottom: 16px"
        >
          已从文件自动解析 Maven 坐标
        </el-alert>
        <el-form-item label="Group ID" prop="mavenGroupId">
          <el-input v-model="form.mavenGroupId" placeholder="例如: com.example" />
        </el-form-item>
        <el-form-item label="Artifact ID" prop="mavenArtifactId">
          <el-input v-model="form.mavenArtifactId" placeholder="例如: my-app" />
        </el-form-item>
        <el-form-item label="版本号" prop="mavenVersion">
          <el-input v-model="form.mavenVersion" placeholder="例如: 1.0.0" />
        </el-form-item>
      </template>

      <template v-if="form.packageType === 'yum'">
        <el-form-item label="仓库名称" prop="yumRepo">
          <el-input v-model="form.yumRepo" placeholder="例如: centos7" />
        </el-form-item>
      </template>

      <template v-if="form.packageType === 'generic'">
        <el-form-item label="目标路径">
          <el-input v-model="form.genericPath" placeholder="可选，留空则上传到根目录" />
        </el-form-item>
      </template>

      <el-form-item label="文件" prop="file">
        <el-upload
          ref="uploadRef"
          class="upload-area"
          drag
          :auto-upload="false"
          :on-change="handleFileChange"
          :on-remove="handleFileRemove"
          :file-list="fileList"
          :limit="1"
          :accept="fileAccept"
        >
          <el-icon class="el-icon--upload"><upload-filled /></el-icon>
          <div class="el-upload__text">拖拽文件到此处，或 <em>点击上传</em></div>
          <template #tip>
            <div class="el-upload__tip">{{ uploadTip }}</div>
          </template>
        </el-upload>
      </el-form-item>

      <el-form-item v-if="uploading">
        <el-progress :percentage="uploadProgress" :status="uploadStatus" />
      </el-form-item>
    </el-form>

    <template #footer>
      <el-button @click="$emit('update:modelValue', false)">取消</el-button>
      <el-button
        type="primary"
        @click="handleUpload"
        :loading="uploading"
        :disabled="fileList.length === 0"
      >
        上传
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { UploadFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import type { UploadFile, UploadInstance, FormInstance, FormRules } from 'element-plus'
import { repositoryApi, type Repository } from '@/api/repository'
import axios from 'axios'
import JSZip from 'jszip'

defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'uploaded'): void
}>()

const formRef = ref<FormInstance>()
const uploadRef = ref<UploadInstance>()
const fileList = ref<UploadFile[]>([])
const availableRepositories = ref<Repository[]>([])
const form = ref({
  repositoryId: null as number | null,
  packageType: '',
  repositoryName: '',
  pypiName: '',
  pypiVersion: '',
  pypiSummary: '',
  npmName: '',
  mavenGroupId: '',
  mavenArtifactId: '',
  mavenVersion: '',
  yumRepo: '',
  genericPath: ''
})
const uploading = ref(false)
const uploadProgress = ref(0)
const mavenAutoDetected = ref(false)

const rules: FormRules = {
  repositoryId: [{ required: true, message: '请选择目标仓库', trigger: 'change' }],
  pypiName: [{ required: true, message: '请输入包名称', trigger: 'blur' }],
  pypiVersion: [{ required: true, message: '请输入版本号', trigger: 'blur' }],
  npmName: [{ required: true, message: '请输入包名称', trigger: 'blur' }],
  mavenGroupId: [{ required: true, message: '请输入 Group ID', trigger: 'blur' }],
  mavenArtifactId: [{ required: true, message: '请输入 Artifact ID', trigger: 'blur' }],
  mavenVersion: [{ required: true, message: '请输入版本号', trigger: 'blur' }]
}

const uploadStatus = computed(() => {
  if (uploadProgress.value === 100) return 'success'
  return undefined
})

const loadRepositories = async () => {
  try {
    const res = await repositoryApi.list({ type: 'local' })
    const repos: any[] = (res && typeof res === 'object' && 'items' in res)
      ? (res as any).items
      : (res as any[]) || []
    availableRepositories.value = repos.filter((repo: any) => repo.enabled && repo.package_type)
  } catch (error) {
    console.error('Failed to load repositories:', error)
  }
}

const handleRepositoryChange = (repoId: number) => {
  const repo = availableRepositories.value.find(r => r.id === repoId)
  if (!repo) return

  form.value.packageType = repo.package_type
  form.value.repositoryName = repo.name
  fileList.value = []
  mavenAutoDetected.value = false
}

onMounted(() => {
  loadRepositories()
})

const fileAccept = computed(() => {
  switch (form.value.packageType) {
    case 'pypi':
      return '.whl,.tar.gz,.zip'
    case 'maven':
      return '.jar,.war,.ear,.pom,.aar'
    case 'apt':
      return '.deb'
    case 'yum':
      return '.rpm'
    default:
      return ''
  }
})

const uploadTip = computed(() => {
  switch (form.value.packageType) {
    case 'pypi':
      return '支持 .whl, .tar.gz, .zip 格式'
    case 'maven':
      return '支持 .jar, .war, .ear, .pom, .aar 格式（可自动解析坐标）'
    case 'apt':
      return '支持 .deb 格式'
    case 'yum':
      return '支持 .rpm 格式'
    default:
      return '支持所有文件格式'
  }
})

const handleFileChange = async (_file: UploadFile, files: UploadFile[]) => {
  fileList.value = files
  mavenAutoDetected.value = false

  if (form.value.packageType === 'maven' && files.length > 0 && files[0].raw) {
    await parseMavenCoordinates(files[0].raw)
  }
}

const handleFileRemove = (_file: UploadFile, files: UploadFile[]) => {
  fileList.value = files
  mavenAutoDetected.value = false
}

const parseMavenCoordinates = async (file: File) => {
  const filename = file.name.toLowerCase()

  if (filename.endsWith('.pom')) {
    await parsePomFile(file)
  } else if (filename.endsWith('.jar') || filename.endsWith('.war') || filename.endsWith('.ear')) {
    await parseJarFile(file)
  }
}

const parsePomFile = async (file: File) => {
  try {
    const content = await file.text()
    const parser = new DOMParser()
    const doc = parser.parseFromString(content, 'application/xml')

    const groupId = getElementText(doc, 'groupId')
    const artifactId = getElementText(doc, 'artifactId')
    const version = getElementText(doc, 'version')

    if (groupId && artifactId && version) {
      form.value.mavenGroupId = groupId
      form.value.mavenArtifactId = artifactId
      form.value.mavenVersion = version
      mavenAutoDetected.value = true
      ElMessage.success('已从 POM 文件解析 Maven 坐标')
    }
  } catch (error) {
    console.error('Failed to parse POM file:', error)
  }
}

const parseJarFile = async (file: File) => {
  try {
    const zip = await JSZip.loadAsync(file)

    const pomPropertiesPath = findPomPropertiesPath(zip)
    if (pomPropertiesPath) {
      const content = await zip.file(pomPropertiesPath)?.async('string')
      if (content) {
        const props = parseProperties(content)
        if (props['groupId'] && props['artifactId'] && props['version']) {
          form.value.mavenGroupId = props['groupId']
          form.value.mavenArtifactId = props['artifactId']
          form.value.mavenVersion = props['version']
          mavenAutoDetected.value = true
          ElMessage.success('已从 JAR 包解析 Maven 坐标')
          return
        }
      }
    }

    const pomPath = findPomPath(zip)
    if (pomPath) {
      const content = await zip.file(pomPath)?.async('string')
      if (content) {
        const parser = new DOMParser()
        const doc = parser.parseFromString(content, 'application/xml')

        let groupId = getElementText(doc, 'groupId')
        let artifactId = getElementText(doc, 'artifactId')
        let version = getElementText(doc, 'version')

        if (!groupId) {
          groupId = getElementText(doc, 'parent > groupId')
        }
        if (!version) {
          version = getElementText(doc, 'parent > version')
        }

        if (groupId && artifactId && version) {
          form.value.mavenGroupId = groupId
          form.value.mavenArtifactId = artifactId
          form.value.mavenVersion = version
          mavenAutoDetected.value = true
          ElMessage.success('已从 JAR 包中的 POM 解析 Maven 坐标')
        }
      }
    }
  } catch (error) {
    console.error('Failed to parse JAR file:', error)
  }
}

const findPomPropertiesPath = (zip: JSZip): string | null => {
  for (const path of Object.keys(zip.files)) {
    if (path.includes('META-INF/maven/') && path.endsWith('pom.properties')) {
      return path
    }
  }
  return null
}

const findPomPath = (zip: JSZip): string | null => {
  for (const path of Object.keys(zip.files)) {
    if (path.includes('META-INF/maven/') && path.endsWith('pom.xml')) {
      return path
    }
  }
  return null
}

const parseProperties = (content: string): Record<string, string> => {
  const props: Record<string, string> = {}
  const lines = content.split('\n')
  for (const line of lines) {
    const trimmed = line.trim()
    if (trimmed && !trimmed.startsWith('#')) {
      const idx = trimmed.indexOf('=')
      if (idx > 0) {
        props[trimmed.substring(0, idx).trim()] = trimmed.substring(idx + 1).trim()
      }
    }
  }
  return props
}

const getElementText = (doc: Document, selector: string): string => {
  const el = doc.querySelector(selector)
  return el?.textContent?.trim() || ''
}

const handleUpload = async () => {
  if (!formRef.value) return

  try {
    await formRef.value.validate()
  } catch {
    return
  }

  if (fileList.value.length === 0) {
    ElMessage.warning('请选择要上传的文件')
    return
  }

  const file = fileList.value[0]
  if (!file.raw) {
    ElMessage.error('文件读取失败')
    return
  }

  uploading.value = true
  uploadProgress.value = 0

  try {
    switch (form.value.packageType) {
      case 'generic':
        await uploadGeneric(file.raw)
        break
      case 'pypi':
        await uploadPyPI(file.raw)
        break
      case 'maven':
        await uploadMaven(file.raw)
        break
      case 'apt':
        await uploadApt(file.raw)
        break
      case 'yum':
        await uploadYum(file.raw)
        break
      case 'npm':
        await uploadNpm(file.raw)
        break
      default:
        // 未实现的包类型：明确报错，避免静默 fall-through 造成"假成功"
        throw new Error(`暂不支持上传 ${form.value.packageType || '未知'} 类型`)
    }

    ElMessage.success('上传成功')
    emit('uploaded')
    emit('update:modelValue', false)
    handleClose()
  } catch (error: any) {
    // 优先取后端返回的错误信息；本地 throw 的 Error 则取 message
    ElMessage.error(error?.response?.data?.message || error?.message || '上传失败')
  } finally {
    uploading.value = false
  }
}

const uploadPyPI = async (file: File) => {
  const formData = new FormData()
  formData.append('content', file)
  formData.append('name', form.value.pypiName)
  formData.append('version', form.value.pypiVersion)
  if (form.value.pypiSummary) {
    formData.append('summary', form.value.pypiSummary)
  }
  if (form.value.repositoryName) {
    formData.append('repository', form.value.repositoryName)
  }
  const token = localStorage.getItem('token')

  await axios.post('/pypi/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
      ...(token ? { 'Authorization': `Bearer ${token}` } : {})
    },
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        uploadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
      }
    }
  })
}

const uploadMaven = async (file: File) => {
  const groupId = form.value.mavenGroupId.replace(/\./g, '/')
  const artifactId = form.value.mavenArtifactId
  const version = form.value.mavenVersion
  const filename = file.name

  let path = `/maven2/${groupId}/${artifactId}/${version}/${filename}`
  if (form.value.repositoryName) {
    path = `/repository/${form.value.repositoryName}${path}`
  }
  const token = localStorage.getItem('token')

  await axios.put(path, file, {
    headers: {
      'Content-Type': 'application/octet-stream',
      ...(token ? { 'Authorization': `Bearer ${token}` } : {})
    },
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        uploadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
      }
    }
  })
}

const uploadNpm = async (file: File) => {
  // npm 采用标准 publish 协议：PUT /:package，body 为 packument（versions + base64 _attachments）。
  // 网页上传只拿得到 .tgz，因此这里只把 tarball 作为 _attachments 的 base64 数据发送，
  // 包名取自表单（UI 输入），版本/其它元数据由后端从 tarball 内 package.json 解析并规范成完整 packument。
  const buf = await file.arrayBuffer()
  const bytes = new Uint8Array(buf)
  let binary = ''
  const chunk = 0x8000
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode.apply(null, bytes.subarray(i, i + chunk) as unknown as number[])
  }
  const b64 = btoa(binary)

  // scoped 包（@scope/pkg）按 `/` 分段编码，保持路径层级；
  // 非 scoped 包名直接编码。
  const pathName = form.value.npmName
    .split('/')
    .map((seg) => encodeURIComponent(seg))
    .join('/')

  const body = {
    _attachments: {
      [file.name]: { content_type: 'application/octet-stream', data: b64 }
    }
  }
  const token = localStorage.getItem('token')

  await axios.put(`/repository/${form.value.repositoryName}/${pathName}`, body, {
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { 'Authorization': `Bearer ${token}` } : {})
    },
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        uploadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
      }
    }
  })
}

const uploadApt = async (file: File) => {
  const formData = new FormData()
  formData.append('file', file)
  if (form.value.repositoryName) {
    formData.append('repository', form.value.repositoryName)
  }
  const token = localStorage.getItem('token')

  await axios.post('/apt/upload', formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
      ...(token ? { 'Authorization': `Bearer ${token}` } : {})
    },
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        uploadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
      }
    }
  })
}

const uploadGeneric = async (file: File) => {
  let path = form.value.genericPath || file.name
  if (form.value.repositoryName) {
    path = `/repository/${form.value.repositoryName}/${path}`
  } else {
    path = `/repository/generic-local/${path}`
  }
  const token = localStorage.getItem('token')

  await axios.put(path, file, {
    headers: {
      'Content-Type': 'application/octet-stream',
      ...(token ? { 'Authorization': `Bearer ${token}` } : {})
    },
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        uploadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
      }
    }
  })
}

const uploadYum = async (file: File) => {
  const formData = new FormData()
  formData.append('file', file)
  const token = localStorage.getItem('token')
  const repo = form.value.repositoryName || form.value.yumRepo

  await axios.post(`/yum/${repo}/upload`, formData, {
    headers: {
      'Content-Type': 'multipart/form-data',
      ...(token ? { 'Authorization': `Bearer ${token}` } : {})
    },
    onUploadProgress: (progressEvent) => {
      if (progressEvent.total) {
        uploadProgress.value = Math.round((progressEvent.loaded * 100) / progressEvent.total)
      }
    }
  })
}

const handleClose = () => {
  fileList.value = []
  form.value = {
    repositoryId: null,
    packageType: '',
    repositoryName: '',
    pypiName: '',
    pypiVersion: '',
    pypiSummary: '',
    npmName: '',
    mavenGroupId: '',
    mavenArtifactId: '',
    mavenVersion: '',
    yumRepo: '',
    genericPath: ''
  }
  uploadProgress.value = 0
  uploading.value = false
  mavenAutoDetected.value = false
  formRef.value?.resetFields()
}
</script>

<style scoped>
.upload-area {
  width: 100%;
}

.el-icon--upload {
  font-size: 48px;
  color: #409eff;
  margin-bottom: 8px;
}

.el-upload__text {
  color: #606266;
}

.el-upload__text em {
  color: #409eff;
  font-style: normal;
}

.el-upload__tip {
  color: #909399;
  font-size: 12px;
  margin-top: 8px;
}
</style>
