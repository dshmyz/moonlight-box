# 配置文件模板

本目录包含各种包管理器的配置文件模板，帮助您快速配置客户端。

## 文件列表

| 文件名 | 说明 | 目标位置 |
|--------|------|----------|
| `.npmrc` | NPM 配置文件 | `~/.npmrc` 或项目根目录 |
| `settings.xml` | Maven 配置文件 | `~/.m2/settings.xml` |
| `pip.conf` | pip 配置文件 | `~/.pip/pip.conf` (Linux/Mac) |
| `.pypirc` | PyPI 上传配置 | `~/.pypirc` |
| `NuGet.Config` | NuGet 配置文件 | 项目根目录或 `%APPDATA%\NuGet\` |
| `moonlight.repo` | Yum 仓库配置 | `/etc/yum.repos.d/moonlight.repo` |
| `moonlight.list` | APT 仓库配置 | `/etc/apt/sources.list.d/moonlight.list` |
| `go-env.sh` | Go 环境变量 | `~/.bashrc` 或 `~/.zshrc` |

## 使用方法

### 方式一：直接下载

```bash
# 下载单个文件
curl -o ~/.npmrc https://your-registry/docs/templates/.npmrc

# 下载所有模板
wget -r -np -nH --cut-dirs=3 -R index.html https://your-registry/docs/templates/
```

### 方式二：复制粘贴

1. 打开对应的模板文件
2. 复制内容
3. 粘贴到目标位置
4. 修改配置参数

### 方式三：使用脚本

```bash
# 自动配置脚本（即将推出）
curl -fsSL https://your-registry/docs/install.sh | bash
```

## 配置步骤

### 1. NPM 配置

```bash
# 下载模板
curl -o ~/.npmrc https://your-registry/docs/templates/.npmrc

# 编辑配置
vi ~/.npmrc

# 替换 YOUR_TOKEN_HERE 为实际令牌
sed -i 's/YOUR_TOKEN_HERE/your-actual-token/g' ~/.npmrc

# 验证配置
npm config list
```

### 2. Maven 配置

```bash
# 创建目录
mkdir -p ~/.m2

# 下载模板
curl -o ~/.m2/settings.xml https://your-registry/docs/templates/settings.xml

# 编辑配置
vi ~/.m2/settings.xml

# 替换认证信息
sed -i 's/YOUR_USERNAME/your-username/g' ~/.m2/settings.xml
sed -i 's/YOUR_PASSWORD/your-password/g' ~/.m2/settings.xml

# 验证配置
mvn help:effective-settings
```

### 3. PyPI 配置

```bash
# 创建目录
mkdir -p ~/.pip

# 下载模板
curl -o ~/.pip/pip.conf https://your-registry/docs/templates/pip.conf

# 验证配置
pip config list
```

### 4. Go 配置

```bash
# 下载环境变量脚本
curl -o ~/moonlight-go-env.sh https://your-registry/docs/templates/go-env.sh

# 添加到 shell 配置
echo "source ~/moonlight-go-env.sh" >> ~/.bashrc

# 重新加载配置
source ~/.bashrc

# 验证配置
go env GOPROXY
```

### 5. NuGet 配置

```powershell
# 下载配置文件
Invoke-WebRequest -Uri "https://your-registry/docs/templates/NuGet.Config" -OutFile "NuGet.Config"

# 编辑配置
notepad NuGet.Config

# 验证配置
nuget sources list
```

### 6. Yum 配置

```bash
# 下载配置文件
sudo curl -o /etc/yum.repos.d/moonlight.repo https://your-registry/docs/templates/moonlight.repo

# 更新缓存
sudo yum clean all
sudo yum makecache

# 验证配置
yum repolist
```

### 7. APT 配置

```bash
# 下载配置文件
sudo curl -o /etc/apt/sources.list.d/moonlight.list https://your-registry/docs/templates/moonlight.list

# 更新索引
sudo apt update

# 验证配置
apt-cache policy
```

## 安全建议

### ⚠️ 重要提示

1. **不要提交敏感信息到版本控制**
   - 包含密码和令牌的文件应添加到 `.gitignore`
   - 使用环境变量存储敏感信息

2. **定期更新凭据**
   - 定期轮换密码
   - 使用短期令牌而非长期令牌

3. **限制文件权限**
   ```bash
   # 设置文件权限
   chmod 600 ~/.npmrc
   chmod 600 ~/.pypirc
   chmod 600 ~/.m2/settings.xml
   ```

4. **使用 HTTPS（生产环境）**
   - 生产环境应使用 HTTPS
   - 将模板中的 `http://` 改为 `https://`

## 自定义配置

### 环境变量替换

模板中的占位符可以使用环境变量替换：

```bash
# 设置环境变量
export MOONLIGHT_TOKEN="your-token"
export MOONLIGHT_USERNAME="your-username"
export MOONLIGHT_PASSWORD="your-password"

# 使用 envsubst 替换
envsubst < .npmrc.template > ~/.npmrc
```

### 多环境配置

为不同环境创建不同的配置文件：

```bash
# 开发环境
.npmrc.dev

# 测试环境
.npmrc.test

# 生产环境
.npmrc.prod

# 切换环境
ln -s .npmrc.dev ~/.npmrc
```

## 故障排查

### 配置不生效

1. 检查文件位置是否正确
2. 检查文件权限
3. 检查配置语法
4. 重启终端或 IDE

### 认证失败

1. 检查用户名和密码是否正确
2. 检查令牌是否过期
3. 检查权限是否足够

### 连接失败

1. 检查网络连通性
2. 检查仓库地址是否正确
3. 检查防火墙设置

## 获取帮助

如果遇到问题：

1. 查看 [客户端配置指南](../client-configuration.md)
2. 查看 [常见问题解答](../faq.md)
3. 联系管理员：admin@company.com

---

**最后更新**：2026-05-03
