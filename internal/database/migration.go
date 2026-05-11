package database

import (
	"fmt"

	"github.com/moonlight-box/registry/internal/config"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/util"
)

func AutoMigrate() error {
	return DB.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.UserRole{},
		&model.RolePermission{},
		&model.Package{},
		&model.PackageVersion{},
		&model.PackageFile{},
		&model.PackageDependency{},
		&model.AuditLog{},
		&model.CacheEntry{},
		&model.SystemConfig{},
		&model.Repository{},
		&model.RepositoryGroup{},
		&model.BlockRule{},
		&model.StorageBackend{},
		&model.ScanResult{},
		&model.Vulnerability{},
		&model.VulnRule{},
		&model.VulnDataSource{},
		&model.Webhook{},
		&model.WebhookDelivery{},
		&model.Backup{},
		&model.MigrationTask{},
		&model.MigrationItem{},
		&model.ProxyDownloadLog{},
	)
}

func SeedData() error {
	// 清理旧角色数据（除 admin 外）
	oldRoles := []string{"maintainer", "developer", "readonly"}
	for _, roleName := range oldRoles {
		var oldRole model.Role
		if err := DB.Where("name = ?", roleName).First(&oldRole).Error; err == nil {
			// 删除角色权限关联
			DB.Where("role_id = ?", oldRole.ID).Delete(&model.RolePermission{})
			// 删除用户角色关联
			DB.Where("role_id = ?", oldRole.ID).Delete(&model.UserRole{})
			// 删除角色
			DB.Delete(&oldRole)
		}
	}

	// 创建新的四角色体系
	roles := []model.Role{
		{
			Name:         "admin",
			Description:  "系统管理员，拥有所有权限",
			IsSystemRole: true,
		},
		{
			Name:         "operations",
			Description:  "运维人员，管理基础设施和仓库",
			IsSystemRole: true,
		},
		{
			Name:         "developer",
			Description:  "普通开发人员，使用包管理服务",
			IsSystemRole: true,
		},
		{
			Name:         "security-auditor",
			Description:  "安全员/审计，安全审计和合规检查",
			IsSystemRole: true,
		},
	}

	for _, role := range roles {
		result := DB.Where("name = ?", role.Name).FirstOrCreate(&role)
		if result.Error != nil {
			return result.Error
		}
	}

	// 创建预置权限
	permissions := []model.Permission{
		// 系统管理
		{Resource: "system", Action: "admin"},
		{Resource: "system", Action: "read"},

		// 用户与权限
		{Resource: "users", Action: "read"},
		{Resource: "users", Action: "write"},

		// 审计日志
		{Resource: "audit", Action: "read"},

		// 仓库管理
		{Resource: "repositories", Action: "read"},
		{Resource: "repositories", Action: "write"},
		{Resource: "repositories", Action: "delete"},

		// 缓存管理
		{Resource: "cache", Action: "read"},
		{Resource: "cache", Action: "write"},

		// 阻断规则
		{Resource: "block-rules", Action: "read"},
		{Resource: "block-rules", Action: "write"},
		{Resource: "block-rules", Action: "delete"},

		// 存储后端
		{Resource: "storage-backends", Action: "read"},
		{Resource: "storage-backends", Action: "write"},

		// 安全扫描
		{Resource: "security", Action: "read"},
		{Resource: "security", Action: "write"},

		// Webhook
		{Resource: "webhooks", Action: "read"},
		{Resource: "webhooks", Action: "write"},

		// NPM 包管理
		{Resource: "npm", Action: "read"},
		{Resource: "npm", Action: "write"},
		{Resource: "npm", Action: "delete"},
		{Resource: "npm", Action: "admin"},

		// Maven 包管理
		{Resource: "maven", Action: "read"},
		{Resource: "maven", Action: "write"},
		{Resource: "maven", Action: "delete"},
		{Resource: "maven", Action: "admin"},

		// 包管理（通用）
		{Resource: "package", Action: "read"},
		{Resource: "package", Action: "write"},
		{Resource: "package", Action: "delete"},
		{Resource: "package", Action: "delete_own"},
	}

	for _, perm := range permissions {
		result := DB.Where("resource = ? AND action = ?", perm.Resource, perm.Action).FirstOrCreate(&perm)
		if result.Error != nil {
			return result.Error
		}
	}

	// 为 admin 角色分配所有权限
	var adminRole model.Role
	if err := DB.Where("name = ?", "admin").First(&adminRole).Error; err != nil {
		return err
	}

	var allPermissions []model.Permission
	DB.Find(&allPermissions)

	for _, perm := range allPermissions {
		rp := model.RolePermission{
			RoleID:       adminRole.ID,
			PermissionID: perm.ID,
		}
		DB.Where(rp).FirstOrCreate(&rp)
	}

	// 为 operations 角色分配运维权限
	var operationsRole model.Role
	if err := DB.Where("name = ?", "operations").First(&operationsRole).Error; err == nil {
		opsPermissions := []struct {
			resource string
			action   string
		}{
			{"system", "read"},
			{"repositories", "read"},
			{"repositories", "write"},
			{"repositories", "delete"},
			{"cache", "read"},
			{"cache", "write"},
			{"storage-backends", "read"},
			{"storage-backends", "write"},
			{"webhooks", "read"},
			{"webhooks", "write"},
			{"npm", "read"},
			{"npm", "write"},
			{"npm", "delete"},
			{"maven", "read"},
			{"maven", "write"},
			{"maven", "delete"},
			{"package", "read"},
			{"package", "write"},
			{"package", "delete"},
			{"audit", "read"},
		}

		for _, permDef := range opsPermissions {
			var perm model.Permission
			if err := DB.Where("resource = ? AND action = ?", permDef.resource, permDef.action).First(&perm).Error; err == nil {
				rp := model.RolePermission{
					RoleID:       operationsRole.ID,
					PermissionID: perm.ID,
				}
				DB.Where(rp).FirstOrCreate(&rp)
			}
		}
	}

	// 为 developer 角色分配开发权限
	var developerRole model.Role
	if err := DB.Where("name = ?", "developer").First(&developerRole).Error; err == nil {
		devPermissions := []struct {
			resource string
			action   string
		}{
			{"npm", "read"},
			{"npm", "write"},
			{"maven", "read"},
			{"maven", "write"},
			{"package", "read"},
			{"package", "write"},
			{"package", "delete_own"},
			{"repositories", "read"},
		}

		for _, permDef := range devPermissions {
			var perm model.Permission
			if err := DB.Where("resource = ? AND action = ?", permDef.resource, permDef.action).First(&perm).Error; err == nil {
				rp := model.RolePermission{
					RoleID:       developerRole.ID,
					PermissionID: perm.ID,
				}
				DB.Where(rp).FirstOrCreate(&rp)
			}
		}
	}

	// 为 security-auditor 角色分配安全审计权限
	var securityAuditorRole model.Role
	if err := DB.Where("name = ?", "security-auditor").First(&securityAuditorRole).Error; err == nil {
		secPermissions := []struct {
			resource string
			action   string
		}{
			{"audit", "read"},
			{"security", "read"},
			{"security", "write"},
			{"block-rules", "read"},
			{"block-rules", "write"},
			{"block-rules", "delete"},
			{"system", "read"},
			{"repositories", "read"},
			{"users", "read"},
			{"npm", "read"},
			{"maven", "read"},
			{"package", "read"},
		}

		for _, permDef := range secPermissions {
			var perm model.Permission
			if err := DB.Where("resource = ? AND action = ?", permDef.resource, permDef.action).First(&perm).Error; err == nil {
				rp := model.RolePermission{
					RoleID:       securityAuditorRole.ID,
					PermissionID: perm.ID,
				}
				DB.Where(rp).FirstOrCreate(&rp)
			}
		}
	}

	// 创建默认管理员账号
	var adminUser model.User
	result := DB.Where("username = ?", "admin").First(&adminUser)
	if result.Error != nil {
		hashedPassword, err := util.HashPassword("admin123")
		if err != nil {
			return err
		}

		adminUser = model.User{
			Username:     "admin",
			PasswordHash: hashedPassword,
			Email:        "admin@moonlight.local",
			DisplayName:  "系统管理员",
			IsActive:     true,
		}

		if err := DB.Create(&adminUser).Error; err != nil {
			return err
		}

		// 为 admin 用户分配 admin 角色
		userRole := model.UserRole{
			UserID:     adminUser.ID,
			RoleID:     adminRole.ID,
			AssignedBy: adminUser.ID,
		}
		if err := DB.Create(&userRole).Error; err != nil {
			return err
		}
	}

	// 创建默认仓库
	if err := seedDefaultRepositories(); err != nil {
		return err
	}

	// 根据配置决定是否加载测试包数据
	cfg := config.Get()
	if cfg != nil && cfg.Seed.LoadTestData {
		if err := seedTestPackages(); err != nil {
			return err
		}
	}

	return nil
}

// seedTestPackages 创建测试用包数据
func seedTestPackages() error {
	// 获取 admin 用户 ID
	var adminUser model.User
	if err := DB.Where("username = ?", "admin").First(&adminUser).Error; err != nil {
		return err
	}

	// 获取或创建各类型本地仓库
	repoMap := make(map[string]*model.Repository)
	repoDefs := []struct {
		name        string
		displayName string
		pkgType     model.PackageType
	}{
		{"npm-local", "NPM 本地仓库", model.PackageTypeNPM},
		{"maven-local", "Maven 本地仓库", model.PackageTypeMaven},
		{"pypi-local", "PyPI 本地仓库", model.PackageTypePyPI},
		{"go-local", "Go 本地仓库", model.PackageTypeGo},
		{"generic-local", "Generic 本地仓库", model.PackageTypeGeneric},
	}

	for _, rd := range repoDefs {
		var repo model.Repository
		result := DB.Where("name = ?", rd.name).First(&repo)
		if result.Error != nil {
			repo = model.Repository{
				Name:        rd.name,
				DisplayName: rd.displayName,
				Description: fmt.Sprintf("存储内部发布的 %s 包", rd.pkgType),
				Type:        model.RepoTypeLocal,
				PackageType: string(rd.pkgType),
				Enabled:     true,
			}
			if err := DB.Create(&repo).Error; err != nil {
				return err
			}
		}
		repoMap[string(rd.pkgType)] = &repo
	}

	// NPM 测试包
	npmPackages := []struct {
		name        string
		description string
		homepage    string
		license     string
		downloads   int64
		versions    []string
	}{
		{"vue", "Vue.js is a progressive, incrementally-adoptable JavaScript framework for building UI on the web.", "https://vuejs.org", "MIT", 15234567, []string{"3.4.21", "3.4.20", "3.3.13", "3.2.47"}},
		{"react", "React is a JavaScript library for building user interfaces.", "https://reactjs.org", "MIT", 18765432, []string{"18.2.0", "18.1.0", "17.0.2", "16.14.0"}},
		{"typescript", "TypeScript is a language for application scale JavaScript development.", "https://www.typescriptlang.org", "Apache-2.0", 12456789, []string{"5.4.3", "5.3.3", "5.2.2", "4.9.5"}},
		{"element-plus", "A Vue 3 based component library for designers and developers.", "https://element-plus.org", "MIT", 3456789, []string{"2.6.3", "2.6.2", "2.5.6", "2.4.4"}},
		{"pinia", "Intuitive, type safe, light and flexible Store for Vue.", "https://pinia.vuejs.org", "MIT", 4567890, []string{"2.1.7", "2.1.6", "2.0.36"}},
		{"vite", "Native-ESM powered web dev build tool.", "https://vitejs.dev", "MIT", 9876543, []string{"5.2.8", "5.1.6", "4.5.3", "3.2.10"}},
		{"tailwindcss", "A utility-first CSS framework for rapid UI development.", "https://tailwindcss.com", "MIT", 7654321, []string{"3.4.3", "3.4.1", "3.3.6", "3.2.7"}},
		{"axios", "Promise based HTTP client for the browser and node.js.", "https://axios-http.com", "MIT", 11234567, []string{"1.6.8", "1.6.7", "1.5.1", "0.27.2"}},
		{"lodash", "Lodash modular utilities.", "https://lodash.com", "MIT", 23456789, []string{"4.17.21", "4.17.20", "4.17.19"}},
		{"echarts", "A powerful, interactive charting and data visualization library for browser.", "https://echarts.apache.org", "Apache-2.0", 5678901, []string{"5.5.0", "5.4.3", "5.3.2"}},
	}

	for _, np := range npmPackages {
		insertPackage(repoMap[string(model.PackageTypeNPM)], adminUser.ID, model.PackageTypeNPM, np.name, np.description, np.homepage, np.license, np.downloads, np.versions)
	}

	// Maven 测试包
	mavenPackages := []struct {
		name        string
		description string
		homepage    string
		license     string
		downloads   int64
		versions    []string
	}{
		{"org.springframework.boot:spring-boot-starter-web", "Starter for building web, including RESTful, applications using Spring MVC.", "https://mvnrepository.com/artifact/org.springframework.boot/spring-boot-starter-web", "Apache-2.0", 8765432, []string{"3.2.4", "3.2.3", "3.1.8", "2.7.18"}},
		{"com.alibaba:fastjson", "A fast JSON parser/generator for Java.", "https://mvnrepository.com/artifact/com.alibaba/fastjson", "Apache-2.0", 6543210, []string{"2.0.48", "2.0.47", "1.2.83"}},
		{"org.mybatis:mybatis", "The MyBatis SQL mapper framework.", "https://mvnrepository.com/artifact/org.mybatis/mybatis", "Apache-2.0", 5432109, []string{"3.5.16", "3.5.15", "3.5.13"}},
		{"com.google.guava:guava", "Google core libraries for Java.", "https://mvnrepository.com/artifact/com.google.guava/guava", "Apache-2.0", 9876543, []string{"33.1.0-jre", "33.0.0-jre", "32.1.3-jre", "31.1-jre"}},
		{"org.projectlombok:lombok", "Java annotation library which helps to reduce boilerplate code.", "https://mvnrepository.com/artifact/org.projectlombok/lombok", "MIT", 7654321, []string{"1.18.32", "1.18.30", "1.18.28"}},
		{"io.netty:netty-all", "Netty is an asynchronous event-driven network application framework.", "https://mvnrepository.com/artifact/io.netty/netty-all", "Apache-2.0", 6789012, []string{"4.1.108.Final", "4.1.107.Final", "4.1.100.Final"}},
		{"org.apache.commons:commons-lang3", "Apache Commons Lang provides extra utilities for the java.lang API.", "https://mvnrepository.com/artifact/org.apache.commons/commons-lang3", "Apache-2.0", 8901234, []string{"3.14.0", "3.13.0", "3.12.0"}},
		{"com.baomidou:mybatis-plus-boot-starter", "An enhanced toolkit for MyBatis that simplifies development.", "https://mvnrepository.com/artifact/com.baomidou/mybatis-plus-boot-starter", "Apache-2.0", 4321098, []string{"3.5.6", "3.5.5", "3.5.4", "3.5.3.1"}},
	}

	for _, mp := range mavenPackages {
		insertPackage(repoMap[string(model.PackageTypeMaven)], adminUser.ID, model.PackageTypeMaven, mp.name, mp.description, mp.homepage, mp.license, mp.downloads, mp.versions)
	}

	// PyPI 测试包
	pypiPackages := []struct {
		name        string
		description string
		homepage    string
		license     string
		downloads   int64
		versions    []string
	}{
		{"requests", "Python HTTP for Humans.", "https://requests.readthedocs.io", "Apache-2.0", 98765432, []string{"2.31.0", "2.30.0", "2.28.2"}},
		{"django", "A high-level Python web framework.", "https://www.djangoproject.com", "BSD-3-Clause", 56789012, []string{"5.0.4", "5.0.3", "4.2.11", "3.2.25"}},
		{"numpy", "Fundamental package for array computing in Python.", "https://numpy.org", "BSD-3-Clause", 87654321, []string{"1.26.4", "1.26.3", "1.25.2"}},
		{"pandas", "Powerful data structures for data analysis.", "https://pandas.pydata.org", "BSD-3-Clause", 76543210, []string{"2.2.2", "2.2.1", "2.1.4"}},
		{"flask", "A simple framework for building complex web applications.", "https://flask.palletsprojects.com", "BSD-3-Clause", 45678901, []string{"3.0.3", "3.0.2", "2.3.3"}},
	}

	for _, pp := range pypiPackages {
		insertPackage(repoMap[string(model.PackageTypePyPI)], adminUser.ID, model.PackageTypePyPI, pp.name, pp.description, pp.homepage, pp.license, pp.downloads, pp.versions)
	}

	// Go 测试包
	goPackages := []struct {
		name        string
		description string
		homepage    string
		license     string
		downloads   int64
		versions    []string
	}{
		{"github.com/gin-gonic/gin", "Gin is a HTTP web framework written in Go.", "https://gin-gonic.com", "MIT", 12345678, []string{"v1.9.1", "v1.9.0", "v1.8.2"}},
		{"github.com/gorilla/mux", "A powerful URL router and dispatcher for Golang.", "https://github.com/gorilla/mux", "BSD-3-Clause", 8765432, []string{"v1.8.1", "v1.8.0", "v1.7.4"}},
		{"github.com/spf13/cobra", "A Commander for modern Go CLI interactions.", "https://github.com/spf13/cobra", "Apache-2.0", 7654321, []string{"v1.8.0", "v1.7.0", "v1.6.1"}},
		{"github.com/stretchr/testify", "A toolkit with common assertions and mocks for Go.", "https://github.com/stretchr/testify", "MIT", 15678901, []string{"v1.9.0", "v1.8.4", "v1.8.2"}},
		{"gorm.io/gorm", "The fantastic ORM library for Golang.", "https://gorm.io", "MIT", 9876543, []string{"v1.25.9", "v1.25.8", "v1.25.5"}},
	}

	for _, gp := range goPackages {
		insertPackage(repoMap[string(model.PackageTypeGo)], adminUser.ID, model.PackageTypeGo, gp.name, gp.description, gp.homepage, gp.license, gp.downloads, gp.versions)
	}

	// NuGet 测试包
	// Generic 测试包
	genericPackages := []struct {
		name        string
		description string
		homepage    string
		license     string
		downloads   int64
		versions    []string
	}{
		{"ubuntu-server-22.04", "Ubuntu Server 22.04 LTS base image.", "https://ubuntu.com", "GPL-2.0", 3456789, []string{"22.04.4", "22.04.3", "22.04.2"}},
		{"nodejs-binary", "Node.js pre-built binary distribution.", "https://nodejs.org", "MIT", 5678901, []string{"20.12.2", "20.11.1", "18.20.2"}},
		{"terraform-provider-aws", "Terraform AWS Provider binary.", "https://registry.terraform.io/providers/hashicorp/aws", "MPL-2.0", 4567890, []string{"5.45.0", "5.44.0", "5.43.0"}},
	}

	for _, ggp := range genericPackages {
		insertPackage(repoMap[string(model.PackageTypeGeneric)], adminUser.ID, model.PackageTypeGeneric, ggp.name, ggp.description, ggp.homepage, ggp.license, ggp.downloads, ggp.versions)
	}

	return nil
}

// insertPackage 插入包和版本数据
func insertPackage(repo *model.Repository, userID uint, pkgType model.PackageType, name, description, homepage, license string, downloads int64, versions []string) error {
	// 检查是否已存在
	var existing model.Package
	result := DB.Where("name = ? AND type = ?", name, pkgType).First(&existing)
	if result.Error == nil {
		// 已存在，跳过
		return nil
	}

	pkg := model.Package{
		Name:           name,
		Type:           pkgType,
		Description:    description,
		RepositoryID:   repo.ID,
		RepositoryType: model.RepoTypeLocal,
		Homepage:       homepage,
		License:        license,
		DownloadCount:  downloads,
		CreatedBy:      userID,
	}

	if err := DB.Create(&pkg).Error; err != nil {
		return err
	}

	for i, ver := range versions {
		version := model.PackageVersion{
			PackageID:      pkg.ID,
			Version:        ver,
			Status:         model.StatusPublished,
			StoragePath:    fmt.Sprintf("%s/%s/%s", pkgType, name, ver),
			SizeBytes:      int64(100000 + i*50000),
			ChecksumSHA256: fmt.Sprintf("sha256-%s-%d", name, i),
			PublishedBy:    userID,
		}

		if err := DB.Create(&version).Error; err != nil {
			return err
		}
	}

	return nil
}

// seedDefaultRepositories 创建默认仓库（本地仓、代理仓、虚拟仓）并设置虚拟仓成员关系
func seedDefaultRepositories() error {
	// npm 仓库
	npmRepos := []model.Repository{
		{
			Name:        "npm-local",
			DisplayName: "NPM 本地仓库",
			Description: "存储内部发布的 npm 包",
			Type:        model.RepoTypeLocal,
			PackageType: string(model.PackageTypeNPM),
			Enabled:     true,
		},
		{
			Name:              "npm-proxy-cn",
			DisplayName:       "NPM 国内代理",
			Description:       "国内 npm 镜像源代理",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypeNPM),
			Enabled:           true,
			RemoteURL:         "https://registry.npmmirror.com",
			AuthType:          "none",
			ProxyPriority:     1,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:              "npm-proxy-official",
			DisplayName:       "NPM 官方代理",
			Description:       "npm 官方仓库代理",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypeNPM),
			Enabled:           true,
			RemoteURL:         "https://registry.npmjs.org",
			AuthType:          "none",
			ProxyPriority:     2,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:           "npm-virtual",
			DisplayName:    "NPM 虚拟仓库",
			Description:    "聚合本地仓和代理仓的虚拟仓库",
			Type:           model.RepoTypeVirtual,
			PackageType:    string(model.PackageTypeNPM),
			Enabled:        true,
			AllowOverwrite: false,
			AllowDelete:    false,
		},
	}

	// maven 仓库
	mavenRepos := []model.Repository{
		{
			Name:        "maven-local",
			DisplayName: "Maven 本地仓库",
			Description: "存储内部发布的 Maven 包",
			Type:        model.RepoTypeLocal,
			PackageType: string(model.PackageTypeMaven),
			Enabled:     true,
			AllowDelete: true,
		},
		{
			Name:           "maven-snapshots",
			DisplayName:    "Maven SNAPSHOT 仓库",
			Description:    "存储内部发布的 Maven SNAPSHOT 版本",
			Type:           model.RepoTypeLocal,
			PackageType:    string(model.PackageTypeMaven),
			Enabled:        true,
			AllowDelete:    true,
			AllowOverwrite: true,
		},
		{
			Name:              "maven-proxy-aliyun",
			DisplayName:       "Maven 阿里云代理",
			Description:       "阿里云 Maven 镜像代理",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypeMaven),
			Enabled:           true,
			RemoteURL:         "https://maven.aliyun.com/repository/public",
			AuthType:          "none",
			ProxyPriority:     1,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:              "maven-proxy-central",
			DisplayName:       "Maven Central 代理",
			Description:       "Maven Central 官方仓库代理",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypeMaven),
			Enabled:           true,
			RemoteURL:         "https://repo.maven.apache.org/maven2",
			AuthType:          "none",
			ProxyPriority:     2,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:           "maven-virtual",
			DisplayName:    "Maven 虚拟仓库",
			Description:    "聚合本地仓和代理仓的虚拟仓库",
			Type:           model.RepoTypeVirtual,
			PackageType:    string(model.PackageTypeMaven),
			Enabled:        true,
			AllowOverwrite: false,
			AllowDelete:    false,
		},
	}

	// pypi 仓库
	pypiRepos := []model.Repository{
		{
			Name:        "pypi-local",
			DisplayName: "PyPI 本地仓库",
			Description: "存储内部发布的 Python 包",
			Type:        model.RepoTypeLocal,
			PackageType: string(model.PackageTypePyPI),
			Enabled:     true,
			AllowDelete: true,
		},
		{
			Name:              "pypi-proxy-tuna",
			DisplayName:       "PyPI 清华源代理",
			Description:       "PyPI 清华镜像源代理",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypePyPI),
			Enabled:           true,
			RemoteURL:         "https://mirrors.aliyun.com/pypi",
			AuthType:          "none",
			ProxyPriority:     1,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:              "pypi-proxy-official",
			DisplayName:       "PyPI 官方代理",
			Description:       "PyPI 官方仓库代理",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypePyPI),
			Enabled:           true,
			RemoteURL:         "https://pypi.org",
			AuthType:          "none",
			ProxyPriority:     2,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:           "pypi-virtual",
			DisplayName:    "PyPI 虚拟仓库",
			Description:    "聚合本地仓和代理仓的虚拟仓库",
			Type:           model.RepoTypeVirtual,
			PackageType:    string(model.PackageTypePyPI),
			Enabled:        true,
			AllowOverwrite: false,
			AllowDelete:    false,
		},
	}

	// go 仓库
	goRepos := []model.Repository{
		{
			Name:        "go-local",
			DisplayName: "Go 本地仓库",
			Description: "存储内部发布的 Go 模块",
			Type:        model.RepoTypeLocal,
			PackageType: string(model.PackageTypeGo),
			Enabled:     true,
			AllowDelete: true,
		},
		{
			Name:              "go-proxy-goproxy-cn",
			DisplayName:       "Go goproxy.cn 代理",
			Description:       "goproxy.cn Go 模块代理",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypeGo),
			Enabled:           true,
			RemoteURL:         "https://goproxy.cn",
			AuthType:          "none",
			ProxyPriority:     1,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:              "go-proxy-official",
			DisplayName:       "Go 官方代理",
			Description:       "Go 官方模块代理 proxy.golang.org",
			Type:              model.RepoTypeProxy,
			PackageType:       string(model.PackageTypeGo),
			Enabled:           true,
			RemoteURL:         "https://proxy.golang.org",
			AuthType:          "none",
			ProxyPriority:     2,
			CacheEnabled:      true,
			CacheTTLSeconds:   86400,
			TimeoutSeconds:    30,
			MaxRedirects:      5,
			FailureCacheRules: `[{"status_code": 404, "ttl_seconds": 300}, {"status_code_range": [500, 599], "ttl_seconds": 60}]`,
		},
		{
			Name:           "go-virtual",
			DisplayName:    "Go 虚拟仓库",
			Description:    "聚合本地仓和代理仓的虚拟仓库",
			Type:           model.RepoTypeVirtual,
			PackageType:    string(model.PackageTypeGo),
			Enabled:        true,
			AllowOverwrite: false,
			AllowDelete:    false,
		},
	}

	// 合并所有仓库
	allRepos := append(npmRepos, mavenRepos...)
	allRepos = append(allRepos, pypiRepos...)
	allRepos = append(allRepos, goRepos...)

	// 创建所有仓库并记录ID映射
	repoIDMap := make(map[string]uint)
	for _, repo := range allRepos {
		// 检查是否已存在
		var existing model.Repository
		result := DB.Where("name = ?", repo.Name).First(&existing)
		if result.Error == nil {
			// 已存在，记录ID
			repoIDMap[repo.Name] = existing.ID
			continue
		}

		// 不存在，创建
		if err := DB.Create(&repo).Error; err != nil {
			return err
		}
		repoIDMap[repo.Name] = repo.ID
	}

	// 设置虚拟仓库成员关系
	// npm-virtual: npm-local (priority=0), npm-proxy-cn (priority=1), npm-proxy-official (priority=2)
	npmVirtualID := repoIDMap["npm-virtual"]
	npmLocalID := repoIDMap["npm-local"]
	npmProxyCN := repoIDMap["npm-proxy-cn"]
	npmProxyOfficial := repoIDMap["npm-proxy-official"]

	npmMembers := []struct {
		memberID uint
		priority int
	}{
		{memberID: npmLocalID, priority: 0},
		{memberID: npmProxyCN, priority: 1},
		{memberID: npmProxyOfficial, priority: 2},
	}

	for _, m := range npmMembers {
		group := model.RepositoryGroup{
			VirtualRepoID: npmVirtualID,
			MemberRepoID:  m.memberID,
			Priority:      m.priority,
		}
		// 使用 FirstOrCreate 避免重复
		existingGroup := model.RepositoryGroup{}
		DB.Where("virtual_repo_id = ? AND member_repo_id = ?", npmVirtualID, m.memberID).
			FirstOrCreate(&existingGroup, group)
	}

	// maven-virtual: maven-local (priority=0), maven-proxy-aliyun (priority=1), maven-proxy-central (priority=2)
	mavenVirtualID := repoIDMap["maven-virtual"]
	mavenLocalID := repoIDMap["maven-local"]
	mavenProxyAliyun := repoIDMap["maven-proxy-aliyun"]
	mavenProxyCentral := repoIDMap["maven-proxy-central"]

	mavenMembers := []struct {
		memberID uint
		priority int
	}{
		{memberID: mavenLocalID, priority: 0},
		{memberID: mavenProxyAliyun, priority: 1},
		{memberID: mavenProxyCentral, priority: 2},
	}

	for _, m := range mavenMembers {
		group := model.RepositoryGroup{
			VirtualRepoID: mavenVirtualID,
			MemberRepoID:  m.memberID,
			Priority:      m.priority,
		}
		// 使用 FirstOrCreate 避免重复
		existingGroup := model.RepositoryGroup{}
		DB.Where("virtual_repo_id = ? AND member_repo_id = ?", mavenVirtualID, m.memberID).
			FirstOrCreate(&existingGroup, group)
	}

	return nil
}
