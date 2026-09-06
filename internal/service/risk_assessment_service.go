package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/database/dialect"
	"github.com/dshmyz/moonlight-box/internal/model"
	ver "github.com/dshmyz/moonlight-box/internal/version"
	"github.com/sirupsen/logrus"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

// RiskComponentInput Excel 解析出的单个风险组件。
type RiskComponentInput struct {
	Name     string
	Version  string
	Format   string
	Severity string
	Reason   string
	CVE      string
}

// AICompleter 提供单次 LLM 补全能力，由 *ai.AIService 实现（避免 service→ai 循环依赖）。
type AICompleter interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// ArtifactRemover 研判快速处置所需的制品删除能力，由 *ArtifactService 实现。
type ArtifactRemover interface {
	DeletePackageVersionByCoordinates(ctx context.Context, repoID uint, format, name, version string) error
}

// RiskAssessmentService 风险组件研判：上传风险清单 → 匹配仓库制品与依赖 → 落库。
type RiskAssessmentService struct {
	db                *gorm.DB
	logger            *logrus.Logger
	blockRuleSvc      *BlockRuleService
	artifactSvc       ArtifactRemover
	dependencyResolvers map[string]runtime.DependencyResolver
	aiSvc             AICompleter
}

func NewRiskAssessmentService(db *gorm.DB) *RiskAssessmentService {
	return &RiskAssessmentService{
		db:     db,
		logger: logrus.New(),
	}
}

// SetBlockRuleService 注入阻断规则服务（用于一键生成阻断规则）。
func (s *RiskAssessmentService) SetBlockRuleService(svc *BlockRuleService) {
	s.blockRuleSvc = svc
}

// SetArtifactService 注入制品服务（用于快速处置：移除命中制品）。
func (s *RiskAssessmentService) SetArtifactService(svc ArtifactRemover) {
	s.artifactSvc = svc
}

// SetDependencyResolvers 注入各包格式的依赖反查能力（由协议插件实现），
// 协议语义（依赖键格式、版本约束）由插件负责，服务层只按格式分发。
func (s *RiskAssessmentService) SetDependencyResolvers(resolvers map[string]runtime.DependencyResolver) {
	s.dependencyResolvers = resolvers
}

// SetAIService 注入 AI 服务（用于非标准文档解析与研判报告），未启用时传 nil。
func (s *RiskAssessmentService) SetAIService(svc AICompleter) {
	s.aiSvc = svc
}

var (
	excludedAssessmentKinds = []string{runtime.KindMetadata, runtime.KindChecksum, runtime.KindDirectory}
	artifactHitMax          = 50
	dependencyHitMax        = 200
)

// ParseComponents 解析上传的 Excel/CSV 文件，返回风险组件列表。
func (s *RiskAssessmentService) ParseComponents(r io.Reader, filename string) ([]RiskComponentInput, error) {
	lower := strings.ToLower(filename)
	if strings.HasSuffix(lower, ".csv") {
		return s.parseCSV(r)
	}
	return s.parseExcel(r)
}

func (s *RiskAssessmentService) parseExcel(r io.Reader) ([]RiskComponentInput, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("解析 Excel 失败: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(f.GetActiveSheetIndex())
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("读取工作表失败: %w", err)
	}
	return parseRows(rows)
}

func (s *RiskAssessmentService) parseCSV(r io.Reader) ([]RiskComponentInput, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // 允许行字段数不一致（尾部空列可省略）
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("解析 CSV 失败: %w", err)
	}
	return parseRows(rows)
}

// parseRows 识别表头列，逐行解析为 RiskComponentInput。
func parseRows(rows [][]string) ([]RiskComponentInput, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("文件为空")
	}

	// 找到第一行含"组件名/版本"语义的表头
	var idx map[string]int
	headerRow := -1
	for i, row := range rows {
		cells := normalizeRow(row)
		if cells["name"] >= 0 && cells["version"] >= 0 {
			idx = cells
			headerRow = i
			break
		}
	}
	if headerRow < 0 {
		return nil, fmt.Errorf("未识别到表头：需要包含“组件名/名称”和“版本”列")
	}

	var out []RiskComponentInput
	for _, row := range rows[headerRow+1:] {
		name := cellAt(row, idx["name"])
		version := cellAt(row, idx["version"])
		if name == "" && version == "" {
			continue
		}
		if name == "" || version == "" {
			return nil, fmt.Errorf("存在缺少组件名或版本的行: %v", row)
		}
		out = append(out, RiskComponentInput{
			Name:     name,
			Version:  version,
			Format:   cellAt(row, idx["format"]),
			Severity: cellAt(row, idx["severity"]),
			Reason:   cellAt(row, idx["reason"]),
			CVE:      cellAt(row, idx["cve"]),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未解析到有效数据")
	}
	return out, nil
}

var headerAliases = map[string][]string{
	"name":     {"name", "component", "component_name", "artifact", "组件名", "组件名称", "包名", "包名称", "依赖名"},
	"version":  {"version", "风险版本", "版本", "版本号", "组件版本"},
	"format":   {"format", "type", "pkg_type", "package_type", "格式", "类型", "包类型", "仓库格式"},
	"severity": {"severity", "风险等级", "风险级别", "等级", "严重程度"},
	"reason":   {"reason", "原因", "描述", "备注", "说明", "风险说明"},
	"cve":      {"cve", "cve_id", "cve编号", "cve编号", "漏洞编号"},
}

// normalizeRow 将表头/行归一化并识别列位置。
func normalizeRow(row []string) map[string]int {
	out := make(map[string]int, len(headerAliases))
	for k := range headerAliases {
		out[k] = -1
	}
	for i, cell := range row {
		c := strings.ToLower(strings.TrimSpace(cell))
		if c == "" {
			continue
		}
		for k, aliases := range headerAliases {
			if out[k] >= 0 {
				continue
			}
			for _, alias := range aliases {
				if c == alias {
					out[k] = i
					break
				}
			}
		}
	}
	return out
}

func cellAt(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// Analyze 对风险组件列表执行研判并落库，返回带明细的研判记录。
func (s *RiskAssessmentService) Analyze(ctx context.Context, fileName string, createdBy uint, components []RiskComponentInput) (*model.RiskAssessment, error) {
	if len(components) == 0 {
		return nil, fmt.Errorf("风险组件列表为空")
	}

	items := make([]model.RiskAssessmentItem, 0, len(components))
	matchedItems, artifactHits, dependencyHits := 0, 0, 0

	matches := s.matchAll(ctx, components)

	for i, c := range components {
		item := model.RiskAssessmentItem{
			Name:     c.Name,
			Version:  c.Version,
			Format:   c.Format,
			Severity: c.Severity,
			Reason:   c.Reason,
			CVE:      c.CVE,
		}

		item.ArtifactHit = matches[i].ArtifactHit
		item.DependencyHit = matches[i].DependencyHit

		if len(item.ArtifactHit) > 0 || len(item.DependencyHit) > 0 {
			item.Matched = true
			matchedItems++
		}
		if len(item.ArtifactHit) > 0 {
			artifactHits++
		}
		if len(item.DependencyHit) > 0 {
			dependencyHits++
		}
		items = append(items, item)
	}

	assessment := &model.RiskAssessment{
		FileName:       fileName,
		TotalItems:     len(components),
		MatchedItems:   matchedItems,
		ArtifactHits:   artifactHits,
		DependencyHits: dependencyHits,
		CreatedBy:      createdBy,
		Items:          items,
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Omit("Items") 避免 GORM 级联创建明细，明细统一在下方显式插入
		if err := tx.Omit("Items").Create(assessment).Error; err != nil {
			return err
		}
		for i := range assessment.Items {
			assessment.Items[i].AssessmentID = assessment.ID
		}
		// 分批插入：单条 INSERT 全量写入在行数多时会超出 SQLite/MySQL 的绑定参数上限
		return tx.CreateInBatches(&assessment.Items, 500).Error
	}); err != nil {
		return nil, fmt.Errorf("保存研判记录失败: %w", err)
	}

	s.logger.Infof("风险研判完成: %d 个组件，命中 %d 个（制品 %d，依赖 %d）",
		len(components), matchedItems, artifactHits, dependencyHits)
	return assessment, nil
}

// artifactHit 单个制品命中。
type artifactHit struct {
	RepoID   uint
	RepoName string
	Format   string
	Version  string
}

// findArtifactHits 全量匹配仓库制品（无命中上限），研判展示与处置移除共用，
// 保证存储命中被截断（artifactHitMax）后移除操作仍能覆盖全部版本。
func (s *RiskAssessmentService) findArtifactHits(ctx context.Context, name, version, format string) []artifactHit {
	var artifacts []struct {
		RepositoryID uint
		Version      string
		Format       string
		RepoName     string
	}
	q := s.db.WithContext(ctx).Table("artifacts AS a").
		Joins("LEFT JOIN repositories AS r ON r.id = a.repository_id").
		Select("a.repository_id AS repository_id, a.version AS version, a.format AS format, r.name AS repo_name").
		Where("a.name = ? AND a.version != ''", name).
		Where("(a.kind IS NULL OR a.kind NOT IN ?)", excludedAssessmentKinds).
		Limit(2000)
	if format != "" {
		q = q.Where("a.format = ?", format)
	}
	if err := q.Scan(&artifacts).Error; err != nil {
		s.logger.Warnf("查询制品 %s 失败: %v", name, err)
		return nil
	}

	var out []artifactHit
	seen := make(map[string]bool)
	for _, a := range artifacts {
		if !ver.Matches(version, a.Version) {
			continue
		}
		key := a.RepoName + "|" + a.Format + "|" + a.Version
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, artifactHit{RepoID: a.RepositoryID, RepoName: a.RepoName, Format: a.Format, Version: a.Version})
	}
	return out
}

// matchAll 批量匹配所有风险组件：制品/依赖元数据/漏洞各一次查询，
// 替代每组件 3 次串行查询（N 行 Excel 从 3N 次降为 3 次）。
func (s *RiskAssessmentService) matchAll(ctx context.Context, components []RiskComponentInput) []componentMatches {
	out := make([]componentMatches, len(components))
	artifactBatch := s.matchArtifactsBatch(ctx, components)
	depsBatch := s.matchMetadataDependentsBatch(ctx, components)
	vulnBatch := s.matchVulnDependentsBatch(ctx, components)

	for i := range components {
		out[i].ArtifactHit = artifactBatch[i]
		deps := depsBatch[i]
		if v := vulnBatch[i]; len(v) > 0 {
			deps = append(deps, v...)
		}
		if len(deps) > dependencyHitMax {
			deps = deps[:dependencyHitMax]
		}
		out[i].DependencyHit = deps
	}
	return out
}

type componentMatches struct {
	ArtifactHit   model.JSONArray
	DependencyHit model.JSONArray
}

// distinctNames 提取去重后的组件名列表。
func distinctNames(components []RiskComponentInput) []string {
	seen := make(map[string]bool)
	var out []string
	for _, c := range components {
		if c.Name == "" || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		out = append(out, c.Name)
	}
	return out
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// likeEscapeClause 返回 LIKE 的 ESCAPE 子句，声明 escapeLike 使用的转义符。
// MySQL/PostgreSQL 默认以反斜杠转义，但 SQLite 的 LIKE 没有默认转义符
// （不声明 ESCAPE 时反斜杠是普通字符，\_/\% 永远匹配不上），必须显式声明。
func likeEscapeClause(dialectName string) string {
	if strings.EqualFold(dialectName, "mysql") {
		// MySQL 字符串字面量中 '\\' 才是一个反斜杠
		return ` ESCAPE '\\'`
	}
	return ` ESCAPE '\'`
}

// matchArtifactsBatch 批量匹配仓库制品（一次 IN 查询，命中上限按组件截断）。
func (s *RiskAssessmentService) matchArtifactsBatch(ctx context.Context, components []RiskComponentInput) []model.JSONArray {
	out := make([]model.JSONArray, len(components))
	names := distinctNames(components)
	if len(names) == 0 {
		return out
	}
	var rows []struct {
		Name         string
		RepositoryID uint
		Version      string
		Format       string
		RepoName     string
	}
	err := s.db.WithContext(ctx).Table("artifacts AS a").
		Joins("LEFT JOIN repositories AS r ON r.id = a.repository_id").
		Select("a.name AS name, a.repository_id AS repository_id, a.version AS version, a.format AS format, r.name AS repo_name").
		Where("a.name IN ? AND a.version != ''", names).
		Where("(a.kind IS NULL OR a.kind NOT IN ?)", excludedAssessmentKinds).
		Scan(&rows).Error
	if err != nil {
		s.logger.Warnf("批量查询制品失败: %v", err)
		return out
	}

	type artifactBatchRow struct {
		RepositoryID uint
		Version      string
		Format       string
		RepoName     string
	}
	byName := make(map[string][]artifactBatchRow)
	for _, r := range rows {
		byName[r.Name] = append(byName[r.Name], artifactBatchRow{r.RepositoryID, r.Version, r.Format, r.RepoName})
	}

	for i, c := range components {
		var hits model.JSONArray
		seen := make(map[string]bool)
		for _, a := range byName[c.Name] {
			if c.Format != "" && a.Format != "" && a.Format != c.Format {
				continue
			}
			if !ver.Matches(c.Version, a.Version) {
				continue
			}
			key := a.RepoName + "|" + a.Format + "|" + a.Version
			if seen[key] {
				continue
			}
			seen[key] = true
			hits = append(hits, map[string]interface{}{
				"repo": a.RepoName, "repo_id": a.RepositoryID, "format": a.Format, "version": a.Version,
			})
			if len(hits) >= artifactHitMax {
				break
			}
		}
		out[i] = hits
	}
	return out
}

// matchMetadataDependentsBatch 从 attributes.dependencies/devDependencies 批量反查依赖方（一次 OR LIKE 查询）。
func (s *RiskAssessmentService) matchMetadataDependentsBatch(ctx context.Context, components []RiskComponentInput) []model.JSONArray {
	out := make([]model.JSONArray, len(components))
	names := distinctNames(components)
	if len(names) == 0 {
		return out
	}
	depExpr := dialect.JSONTextExpr(s.db.Dialector.Name(), "attributes", "dependencies")
	devExpr := dialect.JSONTextExpr(s.db.Dialector.Name(), "attributes", "devDependencies")
	var conds []string
	var args []interface{}
	for _, n := range names {
		like := "%" + escapeLike(n) + "%"
		conds = append(conds, "(? LIKE ?"+likeEscapeClause(s.db.Dialector.Name())+" OR ? LIKE ?"+likeEscapeClause(s.db.Dialector.Name())+")")
		args = append(args, gorm.Expr(depExpr), like, gorm.Expr(devExpr), like)
	}
	var rows []struct {
		Name       string
		Version    string
		Format     string
		Attributes string
		RepoName   string
	}
	err := s.db.WithContext(ctx).Table("artifacts AS a").
		Joins("LEFT JOIN repositories AS r ON r.id = a.repository_id").
		Select("a.name AS name, a.version AS version, a.format AS format, a.attributes AS attributes, r.name AS repo_name").
		Where(strings.Join(conds, " OR "), args...).
		Limit(10000).
		Scan(&rows).Error
	if err != nil {
		s.logger.Warnf("批量查询元数据依赖失败: %v", err)
		return out
	}

	for i, c := range components {
		var hits model.JSONArray
		seen := make(map[string]bool)
		for _, a := range rows {
			if c.Format != "" && a.Format != "" && a.Format != c.Format {
				continue
			}
			resolver, ok := s.dependencyResolvers[a.Format]
			if !ok {
				continue // 该格式未注册依赖反查能力
			}
			// 快速子串过滤：名字未出现在 attributes 中则不可能命中
			if !strings.Contains(a.Attributes, c.Name) {
				continue
			}
			var attrs map[string]string
			if err := json.Unmarshal([]byte(a.Attributes), &attrs); err != nil {
				continue
			}
			for _, constraint := range resolver.ResolveDependencies(attrs, c.Name, c.Version) {
				key := a.Name + "|" + a.Version + "|" + a.Format + "|" + a.RepoName
				if seen[key] {
					continue
				}
				seen[key] = true
				hits = append(hits, map[string]interface{}{
					"dependent_name": a.Name, "version": a.Version, "format": a.Format, "repo": a.RepoName, "constraint": constraint,
				})
			}
		}
		out[i] = hits
	}
	return out
}

// matchVulnDependentsBatch 通过漏洞记录批量反查依赖方（一次 IN 查询），按风险版本过滤避免安全版本误报。
func (s *RiskAssessmentService) matchVulnDependentsBatch(ctx context.Context, components []RiskComponentInput) []model.JSONArray {
	out := make([]model.JSONArray, len(components))
	names := distinctNames(components)
	if len(names) == 0 {
		return out
	}
	var rows []struct {
		Name           string
		Version        string
		Format         string
		RepoName       string
		DependencyName string
		CurrentVersion string
	}
	err := s.db.WithContext(ctx).Table("artifacts AS a").
		Joins("LEFT JOIN repositories AS r ON r.id = a.repository_id").
		Joins("JOIN scan_results s ON s.component_id = a.id").
		Joins("JOIN vulnerabilities v ON v.scan_result_id = s.id").
		Select("DISTINCT a.name AS name, a.version AS version, a.format AS format, r.name AS repo_name, v.dependency_name AS dependency_name, v.current_version AS current_version").
		Where("v.dependency_name IN ?", names).
		Scan(&rows).Error
	if err != nil {
		s.logger.Warnf("批量漏洞反查依赖失败: %v", err)
		return out
	}

	type vulnDependentRow struct {
		Name, Version, Format, RepoName, CurrentVersion string
	}
	byName := make(map[string][]vulnDependentRow)
	for _, r := range rows {
		byName[r.DependencyName] = append(byName[r.DependencyName], vulnDependentRow{r.Name, r.Version, r.Format, r.RepoName, r.CurrentVersion})
	}

	for i, c := range components {
		var hits model.JSONArray
		seen := make(map[string]bool)
		for _, d := range byName[c.Name] {
			if c.Format != "" && d.Format != "" && d.Format != c.Format {
				continue
			}
			// 项目声明的依赖版本已知且不是风险版本时，判定为安全版本，不命中
			if d.CurrentVersion != "" && !ver.Matches(c.Version, d.CurrentVersion) {
				continue
			}
			key := d.Name + "|" + d.Version + "|" + d.Format + "|" + d.RepoName
			if seen[key] {
				continue
			}
			seen[key] = true
			hits = append(hits, map[string]interface{}{
				"dependent_name": d.Name, "version": d.Version, "format": d.Format, "repo": d.RepoName, "constraint": d.CurrentVersion,
			})
		}
		out[i] = hits
	}
	return out
}

// ListAssessments 分页列出研判记录。
func (s *RiskAssessmentService) ListAssessments(ctx context.Context, page, pageSize int) ([]model.RiskAssessment, int64, error) {
	var total int64
	if err := s.db.WithContext(ctx).Model(&model.RiskAssessment{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []model.RiskAssessment
	err := s.db.WithContext(ctx).Model(&model.RiskAssessment{}).
		Order("created_at DESC").
		Offset((page - 1) * pageSize).Limit(pageSize).
		Find(&list).Error
	return list, total, err
}

// loadAssessment 加载研判记录（不含处置建议计算），供不需要建议的内部路径复用。
func (s *RiskAssessmentService) loadAssessment(ctx context.Context, id uint) (*model.RiskAssessment, error) {
	var assessment model.RiskAssessment
	err := s.db.WithContext(ctx).Preload("Items").First(&assessment, id).Error
	if err != nil {
		return nil, err
	}
	return &assessment, nil
}

// GetAssessment 获取研判记录详情（含明细与处置建议）。
func (s *RiskAssessmentService) GetAssessment(ctx context.Context, id uint) (*model.RiskAssessment, error) {
	assessment, err := s.loadAssessment(ctx, id)
	if err != nil {
		return nil, err
	}
	s.EnrichSuggestions(ctx, assessment)
	return assessment, nil
}

// DeleteAssessment 删除研判记录及其明细。
func (s *RiskAssessmentService) DeleteAssessment(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("assessment_id = ?", id).Delete(&model.RiskAssessmentItem{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.RiskAssessment{}, id).Error
	})
}

// ExportAssessment 将研判结果导出为 xlsx 字节流。
func (s *RiskAssessmentService) ExportAssessment(ctx context.Context, id uint) ([]byte, error) {
	assessment, err := s.loadAssessment(ctx, id)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	sheet := "研判结果"
	index, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(index)

	headers := []string{"组件名", "风险版本", "格式", "风险等级", "原因", "CVE", "是否命中", "制品命中", "依赖命中", "处置状态"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}

	for i, item := range assessment.Items {
		matched := "否"
		if item.Matched {
			matched = "是"
		}
		disposition := ""
		switch item.Disposition {
		case model.DispositionRemoved:
			disposition = "已移除"
		case model.DispositionBlocked:
			disposition = "已阻断"
		case model.DispositionIgnored:
			disposition = "已忽略"
		}
		row := []interface{}{
			item.Name, item.Version, item.Format, item.Severity, item.Reason, item.CVE,
			matched, formatHits(item.ArtifactHit), formatHits(item.DependencyHit), disposition,
		}
		for j, v := range row {
			cell, _ := excelize.CoordinatesToCellName(j+1, i+2)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	var buf strings.Builder
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// formatHits 将命中明细格式化为可读字符串。
func formatHits(hits model.JSONArray) string {
	if len(hits) == 0 {
		return ""
	}
	parts := make([]string, 0, len(hits))
	for _, h := range hits {
		if dep, ok := h["dependent_name"].(string); ok {
			parts = append(parts, fmt.Sprintf("%s@%v(%v:%v)", dep, h["version"], h["format"], h["repo"]))
			continue
		}
		parts = append(parts, fmt.Sprintf("%v@%v(%v)", h["repo"], h["version"], h["format"]))
	}
	return strings.Join(parts, "；")
}

// TemplateBytes 生成研判模板 xlsx（含示例行与说明）。
func (s *RiskAssessmentService) TemplateBytes() ([]byte, error) {
	f := excelize.NewFile()
	sheet := "风险组件清单"
	index, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(index)

	headers := []string{"组件名", "版本", "格式", "风险等级", "原因", "CVE"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	example := []interface{}{"lodash", "4.17.18", "npm", "高危", "原型污染（示例行，研判时请删除）", "CVE-2021-23337"}
	for i, v := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet, cell, v)
	}

	noteSheet := "填写说明"
	noteIdx, _ := f.NewSheet(noteSheet)
	notes := []string{
		"风险组件研判模板使用说明：",
		"1. 必填列：组件名、版本；可选列：格式、风险等级、原因、CVE。",
		"2. 格式列填 npm / maven / pypi / go / yum / apt / generic，留空则按全格式匹配。",
		"3. 版本支持精确（1.2.3）、版本族（1.2.x / 1.2 / 1.2*）、快照（1.2.3-SNAPSHOT）。",
		"4. 每一行代表一个风险组件，将研判仓库中是否存在对应版本的制品及依赖。",
		"5. 表头列名不区分顺序，也兼容英文（name/version/format/severity/reason/cve）。",
		"6. 非标准格式文档可先上传，若标准解析失败可尝试 AI 智能解析。",
	}
	for i, n := range notes {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = f.SetCellValue(noteSheet, cell, n)
	}
	_ = noteIdx

	var buf strings.Builder
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// EnrichSuggestions 为研判记录的每个明细生成确定性处置建议。
func (s *RiskAssessmentService) EnrichSuggestions(ctx context.Context, assessment *model.RiskAssessment) {
	if assessment == nil {
		return
	}
	byName := s.loadVulnRulesByName(ctx)
	for i := range assessment.Items {
		assessment.Items[i].Suggestion = s.suggestForItem(ctx, &assessment.Items[i], byName)
	}
}

// suggestForItem 生成单个明细的处置建议。
func (s *RiskAssessmentService) suggestForItem(ctx context.Context, item *model.RiskAssessmentItem, vulnRules map[string][]model.VulnRule) string {
	if !item.Matched {
		return "未命中：仓库中无该组件对应版本的制品或依赖记录，暂无需处置。"
	}

	var parts []string
	if len(item.ArtifactHit) > 0 {
		locs := make([]string, 0, len(item.ArtifactHit))
		for _, h := range item.ArtifactHit {
			locs = append(locs, fmt.Sprintf("%v@%v(%v)", h["repo"], h["version"], h["format"]))
		}
		parts = append(parts, "制品命中："+strings.Join(locs, "、"))
	}
	if len(item.DependencyHit) > 0 {
		depNames := make([]string, 0, len(item.DependencyHit))
		seen := make(map[string]bool)
		for _, h := range item.DependencyHit {
			if name, ok := h["dependent_name"].(string); ok && !seen[name] {
				seen[name] = true
				depNames = append(depNames, name)
			}
		}
		if len(depNames) > 3 {
			depNames = depNames[:3]
			parts = append(parts, fmt.Sprintf("依赖命中：%d 个包依赖该组件（如 %s 等），需推动相关项目升级依赖",
				len(item.DependencyHit), strings.Join(depNames, "、")))
		} else {
			parts = append(parts, fmt.Sprintf("依赖命中：%s 依赖该组件，需推动其升级", strings.Join(depNames, "、")))
		}
	}

	if fixed := s.findFixedVersion(item, vulnRules); fixed != "" {
		parts = append(parts, "建议升级到修复版本 "+fixed)
	} else {
		parts = append(parts, "建议升级到安全版本或移除/替换该组件，并评估受影响项目")
	}
	return strings.Join(parts, "；")
}

// loadVulnRulesByName 加载启用中的漏洞规则，按包名模式分组。
func (s *RiskAssessmentService) loadVulnRulesByName(ctx context.Context) map[string][]model.VulnRule {
	result := make(map[string][]model.VulnRule)
	var rules []model.VulnRule
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&rules).Error; err != nil {
		s.logger.Warnf("加载漏洞规则失败: %v", err)
		return result
	}
	for _, r := range rules {
		result[r.PackagePattern] = append(result[r.PackagePattern], r)
	}
	return result
}

// findFixedVersion 从漏洞规则中查找命中该组件的修复版本。
func (s *RiskAssessmentService) findFixedVersion(item *model.RiskAssessmentItem, vulnRules map[string][]model.VulnRule) string {
	for pattern, rules := range vulnRules {
		re, err := regexp.Compile(pattern)
		if err != nil || !re.MatchString(item.Name) {
			continue
		}
		for _, r := range rules {
			if r.FixedVersion == "" {
				continue
			}
			if r.MaxVersion == "" || isVersionLessThan(item.Version, r.MaxVersion) {
				return r.FixedVersion
			}
		}
	}
	return ""
}

// CreateBlockRules 为研判记录中的命中项一键生成阻断规则。
// itemIDs 为空时对全部命中项生成；返回成功创建的规则数。
func (s *RiskAssessmentService) CreateBlockRules(ctx context.Context, assessmentID uint, itemIDs []uint) (int, error) {
	if s.blockRuleSvc == nil {
		return 0, fmt.Errorf("阻断规则服务未配置")
	}
	assessment, err := s.loadAssessment(ctx, assessmentID)
	if err != nil {
		return 0, err
	}

	selectID := make(map[uint]bool, len(itemIDs))
	for _, id := range itemIDs {
		selectID[id] = true
	}

	var rules []*model.BlockRule
	for i := range assessment.Items {
		item := &assessment.Items[i]
		if !item.Matched {
			continue
		}
		if len(itemIDs) > 0 && !selectID[item.ID] {
			continue
		}
		pkgType := hitFormat(item.ArtifactHit, item.DependencyHit)
		rule := &model.BlockRule{
			PackageName: item.Name,
			PackageType: pkgType,
			Reason:      blockRuleReason(item),
			Enabled:     true,
		}
		if ver.IsFamily(item.Version) {
			rule.MatchType = model.BlockMatchWildcard
			rule.Version = strings.ReplaceAll(strings.ReplaceAll(item.Version, "x", "*"), "X", "*")
		} else {
			rule.MatchType = model.BlockMatchExact
			rule.Version = item.Version
		}
		rules = append(rules, rule)
	}
	if len(rules) == 0 {
		return 0, nil
	}

	success, failed, err := s.blockRuleSvc.BatchCreate(rules)
	if err != nil {
		return success, err
	}
	if failed > 0 {
		s.logger.Warnf("生成阻断规则: 成功 %d，失败 %d", success, failed)
	}
	return success, nil
}

func hitFormat(artifactHit, depHit model.JSONArray) string {
	for _, h := range artifactHit {
		if f, ok := h["format"].(string); ok && f != "" {
			return f
		}
	}
	for _, h := range depHit {
		if f, ok := h["format"].(string); ok && f != "" {
			return f
		}
	}
	return "generic"
}

func blockRuleReason(item *model.RiskAssessmentItem) string {
	if item.CVE != "" {
		return "风险组件研判：" + item.CVE
	}
	if item.Reason != "" {
		return "风险组件研判：" + item.Reason
	}
	return "风险组件研判自动生成（" + item.Name + "@" + item.Version + "）"
}

// AIParse 用 AI 解析非标准格式的风险组件文档（标准解析失败时兜底）。
func (s *RiskAssessmentService) AIParse(ctx context.Context, r io.Reader, filename string) ([]RiskComponentInput, error) {
	if s.aiSvc == nil {
		return nil, fmt.Errorf("AI 服务未启用，无法智能解析")
	}
	// 限制输入大小，避免过长的文档；LimitReader 在读取阶段就封顶，防止大文件全量进内存
	const maxBytes = 256 * 1024
	raw, err := io.ReadAll(io.LimitReader(r, maxBytes))
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %w", err)
	}

	text, err := excelizeTableToText(raw)
	if err != nil {
		return nil, err
	}

	system := "你是包仓库安全研判助手。用户会上传一个包含风险组件清单的表格（可能是 Excel/CSV/文本，格式可能不规范）。" +
		"请解析出每个风险组件的 组件名(name)、版本(version)、可选格式(format，如 npm/maven/pypi)、风险等级(severity)、原因(reason)、CVE编号(cve)。" +
		"严格只输出一个 JSON 数组，格式为 [{\"name\":\"...\",\"version\":\"...\",\"format\":\"...\",\"severity\":\"...\",\"reason\":\"...\",\"cve\":\"...\"}]，" +
		"不要输出任何其他文字、markdown 代码块或解释。组件名和版本必填，其余字段可省略（省略时用空字符串）。"
	user := "文件名：" + filename + "\n表格内容：\n" + text

	reply, err := s.aiSvc.Complete(ctx, system, user)
	if err != nil {
		return nil, fmt.Errorf("AI 解析失败: %w", err)
	}

	components, err := parseAIComponents(reply)
	if err != nil {
		return nil, err
	}
	if len(components) == 0 {
		return nil, fmt.Errorf("AI 未解析出有效组件")
	}
	return components, nil
}

// excelizeTableToText 将上传文件内容转为纯文本表格（xlsx 用 excelize 读取，否则按原文处理）。
func excelizeTableToText(raw []byte) (string, error) {
	f, err := excelize.OpenReader(strings.NewReader(string(raw)))
	if err != nil {
		return string(raw), nil // 非 xlsx（如 CSV/文本），直接返回原文
	}
	defer f.Close()

	var sb strings.Builder
	sheet := f.GetSheetName(f.GetActiveSheetIndex())
	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", fmt.Errorf("读取工作表失败: %w", err)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(strings.ReplaceAll(cell, ",", " "))
		}
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

// parseAIComponents 从 AI 回复中提取组件 JSON 数组（容忍 markdown 代码块包裹）。
func parseAIComponents(reply string) ([]RiskComponentInput, error) {
	text := strings.TrimSpace(reply)
	// 去掉 ```json ... ``` 包裹
	if strings.HasPrefix(text, "```") {
		start := strings.Index(text, "\n")
		if start >= 0 {
			text = text[start+1:]
		}
		text = strings.TrimSuffix(strings.TrimSpace(text), "```")
	}
	// 定位 JSON 数组
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("AI 返回格式无法解析")
	}
	text = text[start : end+1]

	var raw []map[string]interface{}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return nil, fmt.Errorf("AI 返回 JSON 解析失败: %w", err)
	}
	out := make([]RiskComponentInput, 0, len(raw))
	for _, m := range raw {
		name := strings.TrimSpace(strVal(m["name"]))
		version := strings.TrimSpace(strVal(m["version"]))
		if name == "" || version == "" {
			continue
		}
		out = append(out, RiskComponentInput{
			Name:     name,
			Version:  version,
			Format:   strVal(m["format"]),
			Severity: strVal(m["severity"]),
			Reason:   strVal(m["reason"]),
			CVE:      strVal(m["cve"]),
		})
	}
	return out, nil
}

func strVal(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// AIReport 生成研判结果的 AI 总结报告与处置建议。
func (s *RiskAssessmentService) AIReport(ctx context.Context, assessmentID uint) (string, error) {
	if s.aiSvc == nil {
		return "", fmt.Errorf("AI 服务未启用")
	}
	assessment, err := s.loadAssessment(ctx, assessmentID)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("研判来源：%s（共 %d 个组件，命中 %d 个：制品 %d，依赖 %d）\n",
		assessment.FileName, assessment.TotalItems, assessment.MatchedItems, assessment.ArtifactHits, assessment.DependencyHits))
	for i := range assessment.Items {
		item := &assessment.Items[i]
		status := "未命中"
		if item.Matched {
			status = "命中"
		}
		sb.WriteString(fmt.Sprintf("- %s@%s [%s] %s：制品 %d 处，依赖方 %d 个",
			item.Name, item.Version, item.Severity, status, len(item.ArtifactHit), len(item.DependencyHit)))
		if item.Reason != "" {
			sb.WriteString(fmt.Sprintf("（%s）", item.Reason))
		}
		sb.WriteString("\n")
	}

	system := "你是企业包仓库的安全研判专家。根据给定的风险组件研判结果，输出一份简洁的中文研判报告。" +
		"要求：1) 分三部分：风险概览（命中/未命中统计、受影响最严重组件）、处置建议（按优先级，明确建议升级/移除/阻断/推动依赖方升级，给出依据）、后续行动清单；" +
		"2) 只基于给定数据，不要编造组件版本或 CVE；3) 用 markdown 输出，控制在 500 字以内。"
	user := "研判结果如下：\n" + sb.String()

	return s.aiSvc.Complete(ctx, system, user)
}

// DisposeInput 快速处置请求。
type DisposeInput struct {
	ItemID uint
	Action string // remove | ignore | reset
	Note   string
	UserID uint
}

// DisposeItem 对单个研判明细执行快速处置并留痕。
// remove：删除所有命中仓库中的该组件版本制品，并自动生成阻断规则防止再次引入；
// ignore：标记忽略；reset：清空处置状态。
func (s *RiskAssessmentService) DisposeItem(ctx context.Context, input DisposeInput) (*model.RiskAssessmentItem, error) {
	var item model.RiskAssessmentItem
	if err := s.db.WithContext(ctx).First(&item, input.ItemID).Error; err != nil {
		return nil, err
	}

	var blockCreated int
	var partialErr error
	switch input.Action {
	case "remove":
		if s.artifactSvc == nil {
			return nil, fmt.Errorf("制品服务未配置，无法移除制品")
		}
		if !item.Matched || len(item.ArtifactHit) == 0 {
			return nil, fmt.Errorf("该明细没有制品命中，无需移除")
		}
		removed, err := s.removeArtifactHits(ctx, &item)
		blockCreated = s.autoBlockAfterRemove(&item)
		item.DispositionNote = fmt.Sprintf("移除 %d 个制品版本", removed)
		if err != nil {
			// 部分移除失败仍记录处置状态与失败明细，避免"制品已删但状态未落"的中间态
			partialErr = err
			item.DispositionNote += "；" + err.Error()
		}
		if input.Note != "" {
			item.DispositionNote += "；" + input.Note
		}
	case "block":
		if s.blockRuleSvc == nil {
			return nil, fmt.Errorf("阻断规则服务未配置")
		}
		blockCreated = s.autoBlockItem(&item)
		if blockCreated == 0 {
			return nil, fmt.Errorf("生成阻断规则失败，未创建任何规则")
		}
		item.DispositionNote = input.Note
	case "ignore":
		item.DispositionNote = input.Note
	case "reset":
		item.Disposition = model.DispositionNone
		item.DispositionNote = ""
		item.DisposedBy = 0
		item.DisposedAt = nil
		if err := s.db.WithContext(ctx).Model(&item).Select("disposition", "disposition_note", "disposed_by", "disposed_at").Updates(&item).Error; err != nil {
			return nil, err
		}
		return &item, nil
	default:
		return nil, fmt.Errorf("不支持的处置动作: %s", input.Action)
	}

	now := time.Now()
	item.DisposedBy = input.UserID
	item.DisposedAt = &now
	if input.Action == "remove" {
		item.Disposition = model.DispositionRemoved
	} else if input.Action == "block" {
		item.Disposition = model.DispositionBlocked
	} else {
		item.Disposition = model.DispositionIgnored
	}
	if err := s.db.WithContext(ctx).Model(&item).Select("disposition", "disposition_note", "disposed_by", "disposed_at").Updates(&item).Error; err != nil {
		return nil, err
	}

	s.logger.Infof("风险研判处置: item=%d %s@%s action=%s block_rules=%d by=%d",
		item.ID, item.Name, item.Version, input.Action, blockCreated, input.UserID)
	if partialErr != nil {
		return &item, partialErr
	}
	return &item, nil
}

// removeArtifactHits 重新全量匹配并逐个删除命中制品，返回成功移除数。
// 不依赖存储中被截断（artifactHitMax）的 ArtifactHit，保证移除覆盖全部版本。
func (s *RiskAssessmentService) removeArtifactHits(ctx context.Context, item *model.RiskAssessmentItem) (int, error) {
	hits := s.findArtifactHits(ctx, item.Name, item.Version, "")
	removed := 0
	var errs []string
	for _, h := range hits {
		if h.RepoID == 0 || h.Format == "" || h.Version == "" {
			continue
		}
		if err := s.artifactSvc.DeletePackageVersionByCoordinates(ctx, h.RepoID, h.Format, item.Name, h.Version); err != nil {
			// 已不存在视为移除成功
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "record not found") {
				removed++
				continue
			}
			errs = append(errs, fmt.Sprintf("%s@%s: %v", h.RepoName, h.Version, err))
			continue
		}
		removed++
	}
	if len(errs) > 0 {
		return removed, fmt.Errorf("部分移除失败: %s", strings.Join(errs, "；"))
	}
	return removed, nil
}

// autoBlockItem 为明细生成阻断规则，返回创建数。
func (s *RiskAssessmentService) autoBlockItem(item *model.RiskAssessmentItem) int {
	rule := &model.BlockRule{
		PackageName: item.Name,
		PackageType: hitFormat(item.ArtifactHit, item.DependencyHit),
		Reason:      blockRuleReason(item),
		Enabled:     true,
	}
	if ver.IsFamily(item.Version) {
		rule.MatchType = model.BlockMatchWildcard
		rule.Version = strings.ReplaceAll(strings.ReplaceAll(item.Version, "x", "*"), "X", "*")
	} else {
		rule.MatchType = model.BlockMatchExact
		rule.Version = item.Version
	}
	success, failed, err := s.blockRuleSvc.BatchCreate([]*model.BlockRule{rule})
	if err != nil || success == 0 {
		s.logger.Warnf("自动阻断失败 item=%d: success=%d failed=%d err=%v", item.ID, success, failed, err)
		return success
	}
	return success
}

// autoBlockAfterRemove 移除后自动生成阻断规则。
func (s *RiskAssessmentService) autoBlockAfterRemove(item *model.RiskAssessmentItem) int {
	if s.blockRuleSvc == nil {
		return 0
	}
	return s.autoBlockItem(item)
}

// DisposeStats 处置进度统计。
type DisposeStats struct {
	Total    int `json:"total"`
	Disposed int `json:"disposed"`
	Removed  int `json:"removed"`
	Blocked  int `json:"blocked"`
	Ignored  int `json:"ignored"`
}

// AssessmentDisposeStats 统计研判记录的处置进度。
func (s *RiskAssessmentService) AssessmentDisposeStats(ctx context.Context, assessmentID uint) (*DisposeStats, error) {
	var items []model.RiskAssessmentItem
	if err := s.db.WithContext(ctx).Where("assessment_id = ?", assessmentID).Find(&items).Error; err != nil {
		return nil, err
	}
	stats := &DisposeStats{Total: len(items)}
	for _, it := range items {
		if it.Disposition != model.DispositionNone {
			stats.Disposed++
		}
		switch it.Disposition {
		case model.DispositionRemoved:
			stats.Removed++
		case model.DispositionBlocked:
			stats.Blocked++
		case model.DispositionIgnored:
			stats.Ignored++
		}
	}
	return stats, nil
}
