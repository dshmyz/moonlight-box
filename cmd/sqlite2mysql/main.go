// sqlite2mysql 一次性数据搬运工具：把 SQLite 数据库中的业务数据迁到 MySQL。
//
// 用法：
//
//	go run ./cmd/sqlite2mysql \
//	  -sqlite /path/to/moonlight.db \
//	  -mysql "user:pass@tcp(host:3306)/moonlight?charset=utf8mb4&parseTime=True&loc=Local"
//
// 行为：
//   - 默认在目标库执行 AutoMigrate 建表（与主程序完全同一套模型清单），-no-schema 可跳过
//   - 按依赖顺序逐表复制，保留全部主键 ID（外键关系不丢）
//   - 主键表用 keyset 分页（WHERE (pk) > last），无主键表（artifact_blobs）退化为 offset 分页
//   - 目标表非空时拒绝执行，-truncate 清空目标表后再搬
//   - -skip-logs 跳过 download_logs / download_daily_stats / audit_logs（日志类大表，可后续再搬）
//   - 结束后逐表对账（源行数 vs 目标行数），不一致则退出码 1
//
// 注意：只搬 DB 数据。文件存储（本地磁盘 / S3 中的 blob 内容）不在本工具范围内。
package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/database"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/model"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// tableModels 与 database.AutoMigrate 的模型清单保持同一顺序（依赖序）。
var tableModels = []interface{}{
	&model.User{},
	&model.Role{},
	&model.Permission{},
	&model.UserRole{},
	&model.RolePermission{},
	&model.Blob{},
	&model.Artifact{},
	&model.ArtifactBlob{},
	&model.AuditLog{},
	&model.CacheEntry{},
	&model.SystemConfig{},
	&model.Repository{},
	&model.RepositoryMember{},
	&model.BlockRule{},
	&model.StorageBackend{},
	&model.ScanResult{},
	&model.Vulnerability{},
	&model.VulnRule{},
	&model.VulnDataSource{},
	&model.RiskAssessment{},
	&model.RiskAssessmentItem{},
	&model.Webhook{},
	&model.WebhookDelivery{},
	&model.Backup{},
	&model.DownloadLog{},
	&model.DownloadDailyStats{},
	&model.Package{},
	&model.PackageVersion{},
	&model.AIPromptTemplate{},
	&domain.MigrationPlan{},
	&domain.MigrationJob{},
	&domain.MigrationItem{},
	&domain.MigrationConflict{},
	&domain.MigrationEvent{},
	&model.APIToken{},
}

// logTables -skip-logs 时跳过的日志类大表
var logTables = map[string]bool{
	"download_logs":        true,
	"download_daily_stats": true,
	"audit_logs":           true,
}

type tableSpec struct {
	name         string
	model        interface{}
	modelType    reflect.Type // struct type（非指针）
	pkCols       []string     // 主键列名；无主键模型（artifact_blobs）为空 → offset 分页
	pkIndexes    [][]int      // 主键字段在 struct 中的 Index 路径
	allCols      []string     // 全部列名
	fieldIndexes [][]int      // 与 allCols 一一对应的字段 Index 路径（map 插入时按列取值）
	fieldNotNull []bool       // 与 allCols 一一对应，目标列是否 NOT NULL
}

func parseSpec(db *gorm.DB, m interface{}) (*tableSpec, error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return nil, fmt.Errorf("parse schema: %w", err)
	}
	schema := stmt.Schema
	spec := &tableSpec{
		name:      schema.Table,
		model:     m,
		modelType: reflect.TypeOf(m).Elem(),
		allCols:   schema.DBNames,
	}
	for _, name := range schema.DBNames {
		if field := schema.LookUpField(name); field != nil {
			spec.fieldIndexes = append(spec.fieldIndexes, field.StructField.Index)
			spec.fieldNotNull = append(spec.fieldNotNull, field.NotNull)
		} else {
			return nil, fmt.Errorf("表 %s 找不到列 %s 对应字段", schema.Table, name)
		}
	}
	for _, f := range schema.PrimaryFields {
		spec.pkCols = append(spec.pkCols, f.DBName)
		spec.pkIndexes = append(spec.pkIndexes, f.StructField.Index)
	}
	return spec, nil
}

func gormConfig() *gorm.Config {
	return &gorm.Config{
		Logger:  gormlogger.Default.LogMode(gormlogger.Error),
		NowFunc: func() time.Time { return time.Now().UTC() },
	}
}

func main() {
	sqlitePath := flag.String("sqlite", "", "源 SQLite 数据库文件路径（必填）")
	mysqlDSN := flag.String("mysql", "", "目标 MySQL DSN，必须含 parseTime=True&charset=utf8mb4&loc=Local（必填）")
	batch := flag.Int("batch", 500, "每批读写的行数")
	skipLogs := flag.Bool("skip-logs", false, "跳过 download_logs/download_daily_stats/audit_logs")
	truncate := flag.Bool("truncate", false, "目标表非空时先 TRUNCATE 再搬（危险：清空目标库数据）")
	noSchema := flag.Bool("no-schema", false, "跳过目标库 AutoMigrate 建表")
	flag.Parse()

	if *sqlitePath == "" || *mysqlDSN == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *batch <= 0 {
		*batch = 500
	}
	if !strings.Contains(*mysqlDSN, "parseTime") {
		fmt.Fprintln(os.Stderr, "警告: MySQL DSN 未包含 parseTime=True，时间字段可能扫描失败")
	}

	// ---- 打开源（SQLite）与目标（MySQL）----
	sqliteDSN := *sqlitePath
	if !strings.Contains(sqliteDSN, "?") {
		sqliteDSN += "?_busy_timeout=30000"
	}
	src, err := gorm.Open(sqlite.Open(sqliteDSN), gormConfig())
	if err != nil {
		fatal("打开 SQLite 源库失败: %v", err)
	}
	dst, err := gorm.Open(mysql.Open(*mysqlDSN), gormConfig())
	if err != nil {
		fatal("连接 MySQL 目标库失败: %v", err)
	}
	if sqlDB, pingErr := dst.DB(); pingErr == nil {
		// 单连接串行写入：SET FOREIGN_KEY_CHECKS 等 SET 是会话级的，
		// 连接池多连接时后续语句可能换连接执行、设置丢失。本工具是纯串行
		// 搬运，单连接无吞吐损失，还保证 TRUNCATE/INSERT 与设置同会话生效。
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		if pingErr = sqlDB.Ping(); pingErr != nil {
			fatal("连接 MySQL 目标库失败: %v", pingErr)
		}
		// AutoMigrate 会为模型上的 relation 字段建真实外键；被外键引用的父表
		// 即使子表已空，TRUNCATE 也会报 1701。搬运全程（TRUNCATE/INSERT）关闭
		// 外键检查，结束后恢复。依赖序的插入正确性由"保留主键 + 依赖序表清单"保证。
		if err := dst.Exec("SET FOREIGN_KEY_CHECKS=0").Error; err != nil {
			fatal("设置 FOREIGN_KEY_CHECKS=0 失败: %v", err)
		}
		defer func() {
			_ = dst.Exec("SET FOREIGN_KEY_CHECKS=1").Error
		}()
	} else {
		fatal("获取 MySQL 连接失败: %v", pingErr)
	}

	// ---- 目标库建表（与主程序同一套 AutoMigrate）----
	if !*noSchema {
		database.DB = dst
		if err := database.AutoMigrate(); err != nil {
			fatal("目标库 AutoMigrate 失败: %v", err)
		}
		fmt.Println("目标库 schema 已就绪 (AutoMigrate)")
	}

	// ---- 解析各表规格 ----
	specs := make([]*tableSpec, 0, len(tableModels))
	for _, m := range tableModels {
		spec, err := parseSpec(src, m)
		if err != nil {
			fatal("%v", err)
		}
		specs = append(specs, spec)
	}

	// ---- 目标表非空守卫 ----
	var nonEmpty []string
	for _, spec := range specs {
		n, err := countRows(dst, spec.name)
		if err != nil {
			fatal("检查目标表 %s 失败: %v", spec.name, err)
		}
		if n > 0 {
			nonEmpty = append(nonEmpty, fmt.Sprintf("%s(%d 行)", spec.name, n))
		}
	}
	if len(nonEmpty) > 0 {
		if !*truncate {
			fatal("目标库以下表非空，拒绝覆盖: %s\n  确认清空目标库请加 -truncate", strings.Join(nonEmpty, ", "))
		}
		// 只清空本次会复制的表。-skip-logs 想保留的日志表不动，
		// 避免"先全量迁移、再 -truncate -skip-logs 刷新业务数据"时把已迁日志清掉。
		for i := len(specs) - 1; i >= 0; i-- {
			if *skipLogs && logTables[specs[i].name] {
				continue
			}
			if err := dst.Exec(fmt.Sprintf("TRUNCATE TABLE `%s`", specs[i].name)).Error; err != nil {
				fatal("清空目标表 %s 失败: %v", specs[i].name, err)
			}
		}
		fmt.Println("目标表已清空 (-truncate)")
	}

	// ---- 逐表复制 ----
	copied := make(map[string]int64)
	for _, spec := range specs {
		if *skipLogs && logTables[spec.name] {
			fmt.Printf("[跳过] %-24s (-skip-logs)\n", spec.name)
			copied[spec.name] = -1
			continue
		}
		start := time.Now()
		fixed, err := copyTable(src, dst, spec, *batch, copied)
		if err != nil {
			fatal("复制表 %s 失败（已复制 %d 行，可 -truncate 后重跑）: %v", spec.name, copied[spec.name], err)
		}
		fmt.Printf("[完成] %-24s %8d 行  %s\n", spec.name, copied[spec.name], time.Since(start).Round(time.Millisecond))
		if fixed > 0 {
			fmt.Printf("[回填] %-24s %d 处空值：NOT NULL 列回填 1970-01-01/零值（SQLite 历史脏数据），可空列置 NULL\n", spec.name, fixed)
		}
	}

	// ---- 对账 ----
	fmt.Println("\n===== 对账（源 vs 目标）=====")
	ok := true
	for _, spec := range specs {
		srcN, err := countRows(src, spec.name)
		if err != nil {
			fatal("统计源表 %s 失败: %v", spec.name, err)
		}
		dstN, err := countRows(dst, spec.name)
		if err != nil {
			fatal("统计目标表 %s 失败: %v", spec.name, err)
		}
		switch {
		case copied[spec.name] < 0:
			fmt.Printf("%-24s src=%-10d dst=%-10d 已跳过(-skip-logs)\n", spec.name, srcN, dstN)
		case srcN == dstN:
			fmt.Printf("%-24s src=%-10d dst=%-10d OK\n", spec.name, srcN, dstN)
		default:
			ok = false
			fmt.Printf("%-24s src=%-10d dst=%-10d ❌ 不一致\n", spec.name, srcN, dstN)
		}
	}
	if !ok {
		fmt.Println("\n存在行数不一致的表，迁移未通过对账。")
		os.Exit(1)
	}
	fmt.Println("\n迁移完成，对账全部一致。")
	fmt.Println("下一步：把主程序配置指向 MySQL（database.driver: mysql + dsn）并启动验证；blob 文件内容需按原存储配置另行就位。")
}

// copyTable 把一张表从源库复制到目标库，保留主键。主键表 keyset 分页，无主键表 offset 分页。
// 返回本表回填的空值数量（见 insertRows）。
func copyTable(src, dst *gorm.DB, spec *tableSpec, batch int, copied map[string]int64) (totalFixed int, err error) {
	if len(spec.pkCols) > 0 {
		cond := fmt.Sprintf("(%s) > (%s)",
			strings.Join(spec.pkCols, ", "),
			strings.TrimSuffix(strings.Repeat("?,", len(spec.pkCols)), ","))
		last := make([]interface{}, len(spec.pkCols))
		first := true
		for {
			slicePtr := reflect.New(reflect.SliceOf(spec.modelType))
			q := src.Unscoped().Model(reflect.New(spec.modelType).Interface()).
				Order(quoteCols(spec.pkCols)).
				Limit(batch)
			if !first {
				q = q.Where(cond, last...)
			}
			if err := q.Find(slicePtr.Interface()).Error; err != nil {
				return totalFixed, err
			}
			rows := slicePtr.Elem()
			n := rows.Len()
			if n == 0 {
				return totalFixed, nil
			}
			fixed, err := insertRows(dst, spec, rows, batch)
			if err != nil {
				return fixed, err
			}
			totalFixed += fixed
			row := rows.Index(n - 1)
			for i, idx := range spec.pkIndexes {
				last[i] = row.FieldByIndex(idx).Interface()
			}
			first = false
			copied[spec.name] += int64(n)
			if n < batch {
				return totalFixed, nil
			}
		}
	}

	// 无主键表（artifact_blobs）：offset 分页，按全部列排序保证确定性
	offset := 0
	for {
		slicePtr := reflect.New(reflect.SliceOf(spec.modelType))
		err := src.Unscoped().Model(reflect.New(spec.modelType).Interface()).
			Order(quoteCols(spec.allCols)).
			Offset(offset).
			Limit(batch).
			Find(slicePtr.Interface()).Error
		if err != nil {
			return totalFixed, err
		}
		rows := slicePtr.Elem()
		n := rows.Len()
		if n == 0 {
			return totalFixed, nil
		}
		fixed, err := insertRows(dst, spec, rows, batch)
		if err != nil {
			return totalFixed + fixed, err
		}
		totalFixed += fixed
		offset += n
		copied[spec.name] += int64(n)
		if n < batch {
			return totalFixed, nil
		}
	}
}

// insertRows 按列名 map 写入目标库，不触发模型钩子（BeforeSave/自动时间戳不重跑，数据保真）。
// 零值/NULL 的处理：
//   - 可空列：零值 time.Time → NULL。go-sql-driver 会把 Go 零值时间写成 MySQL 的
//     '0000-00-00'，严格模式直接拒绝（Error 1292）；SQLite 中它表示"未设置"，NULL 才是等价语义。
//   - NOT NULL 列：SQLite 历史数据里可能存在 NULL/零值（建表早于 not null 约束，SQLite 不追溯），
//     直接插 NULL 会被 MySQL 拒绝（Error 1048）。回填 1970-01-01 / 类型零值，并计数返回供日志提示。
func insertRows(dst *gorm.DB, spec *tableSpec, rows reflect.Value, batch int) (fixed int, err error) {
	n := rows.Len()
	maps := make([]map[string]interface{}, n)
	for i := 0; i < n; i++ {
		row := rows.Index(i)
		m := make(map[string]interface{}, len(spec.allCols))
		for j, col := range spec.allCols {
			fv := row.FieldByIndex(spec.fieldIndexes[j])
			v := fv.Interface()
			if t, ok := v.(time.Time); ok {
				if t.IsZero() {
					if spec.fieldNotNull[j] {
						m[col] = time.Unix(0, 0)
						fixed++
					} else {
						m[col] = nil
					}
				} else {
					m[col] = t
				}
				continue
			}
			if v == nil && spec.fieldNotNull[j] {
				// 指针字段为 nil 但目标列 NOT NULL：回填指向类型的零值
				if fv.Type().Elem().Kind() == reflect.Struct {
					m[col] = time.Unix(0, 0)
				} else {
					m[col] = reflect.Zero(fv.Type().Elem()).Interface()
				}
				fixed++
				continue
			}
			m[col] = v
		}
		maps[i] = m
	}
	return fixed, dst.Table(spec.name).CreateInBatches(&maps, batch).Error
}

func countRows(db *gorm.DB, table string) (int64, error) {
	var n int64
	err := db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdent(table))).Scan(&n).Error
	return n, err
}

// quoteIdent 用反引号包裹 MySQL 标识符：保留字列名/表名不加引号会直接语法错误。
func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func quoteCols(cols []string) string {
	quoted := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = quoteIdent(c)
	}
	return strings.Join(quoted, ", ")
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "错误: "+format+"\n", args...)
	os.Exit(1)
}
