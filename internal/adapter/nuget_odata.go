package adapter

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ODataFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Xmlns   string       `xml:"xmlns,attr"`
	Base    string       `xml:"xml:base,attr"`
	Entries []ODataEntry `xml:"entry"`
}

type ODataEntry struct {
	XMLName    xml.Name        `xml:"entry"`
	ID         string          `xml:"id"`
	Title      string          `xml:"title"`
	Updated    string          `xml:"updated"`
	Author     ODataAuthor     `xml:"author"`
	Content    ODataContent    `xml:"content"`
	Properties ODataProperties `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices/metadata properties"`
}

type ODataAuthor struct {
	Name string `xml:"name"`
}

type ODataContent struct {
	Type string `xml:"type,attr"`
	Src  string `xml:"src,attr"`
}

type ODataProperties struct {
	ID                   string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices Id"`
	Version              string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices Version"`
	Title                string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices Title"`
	Description          string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices Description"`
	Authors              string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices Authors"`
	IconUrl              string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices IconUrl"`
	LicenseUrl           string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices LicenseUrl"`
	ProjectUrl           string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices ProjectUrl"`
	DownloadCount        int    `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices DownloadCount"`
	VersionDownloadCount int    `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices VersionDownloadCount"`
	Published            string `xml:"http://schemas.microsoft.com/ado/2007/08/dataservices Published"`
}

func (a *NuGetAdapter) ODataQueryPackages(c *gin.Context) {
	filter := c.Query("$filter")
	orderBy := c.Query("$orderby")
	top, _ := strconv.Atoi(c.DefaultQuery("$top", "30"))
	skip, _ := strconv.Atoi(c.DefaultQuery("$skip", "0"))

	query := a.buildODataQuery(filter, orderBy)

	var packages []model.Package
	var total int64

	if err := query.Model(&model.Package{}).Count(&total).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if err := query.Preload("Versions").
		Offset(skip).
		Limit(top).
		Find(&packages).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	feed := a.buildODataFeed(c, packages)

	c.XML(200, feed)
}

func (a *NuGetAdapter) ODataGetPackage(c *gin.Context) {
	id := c.Param("id")
	version := c.Param("version")

	var pkg model.Package
	if err := a.pkgRepo.DB().Preload("Versions").
		Where("name = ? AND type = ?", id, model.PackageTypeNuGet).
		First(&pkg).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			response.NotFound(c, "package not found")
			return
		}
		response.InternalError(c, err.Error())
		return
	}

	var targetVersion *model.PackageVersion
	for _, v := range pkg.Versions {
		if v.Version == version {
			targetVersion = &v
			break
		}
	}

	if targetVersion == nil {
		response.NotFound(c, "version not found")
		return
	}

	entry := a.buildODataEntry(c, pkg, *targetVersion)

	c.XML(200, entry)
}

func (a *NuGetAdapter) ODataSearch(c *gin.Context) {
	searchTerm := c.Query("searchTerm")
	top, _ := strconv.Atoi(c.DefaultQuery("$top", "30"))
	skip, _ := strconv.Atoi(c.DefaultQuery("$skip", "0"))

	query := a.pkgRepo.DB().Model(&model.Package{}).
		Where("type = ?", model.PackageTypeNuGet)

	if searchTerm != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?",
			"%"+searchTerm+"%", "%"+searchTerm+"%")
	}

	var packages []model.Package
	var total int64

	if err := query.Count(&total).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	if err := query.Preload("Versions").
		Offset(skip).
		Limit(top).
		Find(&packages).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	feed := a.buildODataFeed(c, packages)

	c.XML(200, feed)
}

func (a *NuGetAdapter) ODataSearchCount(c *gin.Context) {
	searchTerm := c.Query("searchTerm")

	query := a.pkgRepo.DB().Model(&model.Package{}).
		Where("type = ?", model.PackageTypeNuGet)

	if searchTerm != "" {
		query = query.Where("name ILIKE ? OR description ILIKE ?",
			"%"+searchTerm+"%", "%"+searchTerm+"%")
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		response.InternalError(c, err.Error())
		return
	}

	c.String(200, strconv.FormatInt(count, 10))
}

func (a *NuGetAdapter) buildODataQuery(filter, orderBy string) *gorm.DB {
	query := a.pkgRepo.DB().Model(&model.Package{}).
		Where("type = ?", model.PackageTypeNuGet)

	if filter != "" {
		filter = strings.TrimSpace(filter)
		if strings.Contains(filter, "Id eq") {
			id := extractODataFilterValue(filter, "Id")
			if id != "" {
				query = query.Where("name = ?", id)
			}
		} else if strings.Contains(filter, "substringof") {
			searchTerm := extractODataSubstringValue(filter)
			if searchTerm != "" {
				query = query.Where("name ILIKE ? OR description ILIKE ?",
					"%"+searchTerm+"%", "%"+searchTerm+"%")
			}
		}
	}

	if orderBy != "" {
		orderBy = strings.ToLower(orderBy)
		switch {
		case strings.Contains(orderBy, "downloadcount"):
			query = query.Order("download_count DESC")
		case strings.Contains(orderBy, "published"):
			query = query.Order("created_at DESC")
		default:
			query = query.Order("name ASC")
		}
	} else {
		query = query.Order("name ASC")
	}

	return query
}

func extractODataFilterValue(filter, field string) string {
	pattern := field + " eq '"
	start := strings.Index(filter, pattern)
	if start == -1 {
		return ""
	}
	start += len(pattern)
	end := strings.Index(filter[start:], "'")
	if end == -1 {
		return ""
	}
	return filter[start : start+end]
}

func extractODataSubstringValue(filter string) string {
	start := strings.Index(filter, "substringof('")
	if start == -1 {
		return ""
	}
	start += len("substringof('")
	end := strings.Index(filter[start:], "'")
	if end == -1 {
		return ""
	}
	return filter[start : start+end]
}

func (a *NuGetAdapter) buildODataFeed(c *gin.Context, packages []model.Package) ODataFeed {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/nuget", scheme, c.Request.Host)

	feed := ODataFeed{
		Xmlns: "http://www.w3.org/2005/Atom",
		Base:  baseURL,
	}

	for _, pkg := range packages {
		if len(pkg.Versions) > 0 {
			latestVersion := pkg.Versions[len(pkg.Versions)-1]
			entry := a.buildODataEntry(c, pkg, latestVersion)
			feed.Entries = append(feed.Entries, entry)
		}
	}

	return feed
}

func (a *NuGetAdapter) buildODataEntry(c *gin.Context, pkg model.Package, version model.PackageVersion) ODataEntry {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	baseURL := fmt.Sprintf("%s://%s/nuget", scheme, c.Request.Host)
	downloadURL := fmt.Sprintf("%s/v3-flatcontainer/%s/%s/%s.%s.nupkg",
		baseURL, strings.ToLower(pkg.Name), version.Version, pkg.Name, version.Version)

	return ODataEntry{
		ID:      fmt.Sprintf("%s/Packages(Id='%s',Version='%s')", baseURL, pkg.Name, version.Version),
		Title:   pkg.Name,
		Updated: version.PublishedAt.Format("2006-01-02T15:04:05Z"),
		Author: ODataAuthor{
			Name: extractAuthors(version.Metadata),
		},
		Content: ODataContent{
			Type: "application/zip",
			Src:  downloadURL,
		},
		Properties: ODataProperties{
			ID:                   pkg.Name,
			Version:              version.Version,
			Title:                pkg.Name,
			Description:          pkg.Description,
			Authors:              extractAuthors(version.Metadata),
			IconUrl:              extractMetadataField(version.Metadata, "iconUrl"),
			LicenseUrl:           extractMetadataField(version.Metadata, "licenseUrl"),
			ProjectUrl:           extractMetadataField(version.Metadata, "projectUrl"),
			DownloadCount:        int(pkg.DownloadCount),
			VersionDownloadCount: version.DownloadCount,
			Published:            version.PublishedAt.Format("2006-01-02T15:04:05Z"),
		},
	}
}

func extractAuthors(metadata string) string {
	if metadata == "" {
		return ""
	}

	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		return ""
	}

	if authors, ok := meta["authors"].(string); ok {
		return authors
	}
	return ""
}

func extractMetadataField(metadata, field string) string {
	if metadata == "" {
		return ""
	}

	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(metadata), &meta); err != nil {
		return ""
	}

	if value, ok := meta[field].(string); ok {
		return value
	}
	return ""
}
