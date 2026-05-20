package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/moonlight-box/registry/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: create ContentResult from string content
func htmlResult(content string) *types.ContentResult {
	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/html",
		Content:     io.NopCloser(strings.NewReader(content)),
	}
}

func jsonResult(content string) *types.ContentResult {
	return &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/vnd.pypi.simple.v1+json",
		Content:     io.NopCloser(strings.NewReader(content)),
	}
}

// ===== PyPI MergeMetadata Tests =====

func TestPyPI_MergeMetadata_Empty(t *testing.T) {
	a := NewPyPIAdapter()
	_, err := a.MergeMetadata(context.Background(), nil, &types.RequestIntent{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no results to merge")
}

func TestPyPI_MergeMetadata_SingleResult(t *testing.T) {
	a := NewPyPIAdapter()
	res := htmlResult("<html><body><a href=\"/simple/requests/\">requests</a></body></html>")
	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res}, &types.RequestIntent{Type: types.RequestList})
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)
}

func TestPyPI_MergeSimpleIndexHTML(t *testing.T) {
	a := NewPyPIAdapter()

	html1 := `<!DOCTYPE html>
<html><head><title>Simple Index</title></head><body>
<a href="/repository/proxy1/pypi/simple/requests/">requests</a><br>
<a href="/repository/proxy1/pypi/simple/numpy/">numpy</a><br>
<a href="/repository/proxy1/pypi/simple/flask/">flask</a><br>
</body></html>`

	html2 := `<!DOCTYPE html>
<html><head><title>Simple Index</title></head><body>
<a href="/repository/proxy2/pypi/simple/requests/">requests</a><br>
<a href="/repository/proxy2/pypi/simple/pandas/">pandas</a><br>
<a href="/repository/proxy2/pypi/simple/flask/">flask</a><br>
</body></html>`

	results := []*types.ContentResult{
		htmlResult(html1),
		htmlResult(html2),
	}

	intent := &types.RequestIntent{Type: types.RequestList}
	result, err := a.MergeMetadata(context.Background(), results, intent)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "text/html", result.ContentType)

	body, err := io.ReadAll(result.Content)
	require.NoError(t, err)
	bodyStr := string(body)

	// Should contain all 4 unique packages: requests, numpy, flask, pandas
	assert.Contains(t, bodyStr, "requests")
	assert.Contains(t, bodyStr, "numpy")
	assert.Contains(t, bodyStr, "flask")
	assert.Contains(t, bodyStr, "pandas")

	// Count how many links (should be exactly 4)
	linkCount := strings.Count(bodyStr, `<a href="`)
	assert.Equal(t, 4, linkCount, "should have exactly 4 unique links")
}

func TestPyPI_MergeSimpleIndexJSON(t *testing.T) {
	a := NewPyPIAdapter()

	json1 := `{"projects":[{"name":"requests","url":"/pypi/simple/requests/"},{"name":"numpy","url":"/pypi/simple/numpy/"}]}`
	json2 := `{"projects":[{"name":"requests","url":"/pypi/simple/requests/"},{"name":"pandas","url":"/pypi/simple/pandas/"}]}`

	results := []*types.ContentResult{
		jsonResult(json1),
		jsonResult(json2),
	}

	intent := &types.RequestIntent{Type: types.RequestList}
	result, err := a.MergeMetadata(context.Background(), results, intent)
	require.NoError(t, err)
	require.NotNil(t, result)

	body, err := io.ReadAll(result.Content)
	require.NoError(t, err)

	var parsed struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Len(t, parsed.Projects, 3)

	names := make(map[string]bool)
	for _, p := range parsed.Projects {
		names[p.Name] = true
	}
	assert.True(t, names["requests"])
	assert.True(t, names["numpy"])
	assert.True(t, names["pandas"])
}

func TestPyPI_MergeSimpleIndexJSON_ViaExtraData(t *testing.T) {
	a := NewPyPIAdapter()

	// Simulate the ExtraData format returned by listPackagesJSON
	res1 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		ExtraData: map[string]interface{}{
			"projects": []interface{}{
				map[string]interface{}{"name": "requests", "url": "/pypi/simple/requests/"},
				map[string]interface{}{"name": "numpy", "url": "/pypi/simple/numpy/"},
			},
		},
	}

	res2 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		ExtraData: map[string]interface{}{
			"projects": []interface{}{
				map[string]interface{}{"name": "flask", "url": "/pypi/simple/flask/"},
			},
		},
	}

	intent := &types.RequestIntent{Type: types.RequestList}
	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res1, res2}, intent)
	require.NoError(t, err)
	require.NotNil(t, result)

	body, err := io.ReadAll(result.Content)
	require.NoError(t, err)

	var parsed struct {
		Projects []struct {
			Name string `json:"name"`
		} `json:"projects"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Len(t, parsed.Projects, 3)
}

func TestPyPI_MergePackageFilesHTML(t *testing.T) {
	a := NewPyPIAdapter()

	html1 := `<!DOCTYPE html>
<html><head><title>Links for requests</title></head><body>
<h1>Links for requests</h1>
<a href="/repository/proxy1/packages/requests-2.28.0-py3-none-any.whl">requests-2.28.0-py3-none-any.whl</a><br>
<a href="/repository/proxy1/packages/requests-2.28.0.tar.gz">requests-2.28.0.tar.gz</a><br>
</body></html>`

	html2 := `<!DOCTYPE html>
<html><head><title>Links for requests</title></head><body>
<h1>Links for requests</h1>
<a href="/repository/proxy2/packages/requests-2.25.1-py3-none-any.whl">requests-2.25.1-py3-none-any.whl</a><br>
<a href="/repository/proxy2/packages/requests-2.28.0-py3-none-any.whl">requests-2.28.0-py3-none-any.whl</a><br>
</body></html>`

	results := []*types.ContentResult{
		htmlResult(html1),
		htmlResult(html2),
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Path: "simple/requests/", Name: "requests"}
	result, err := a.MergeMetadata(context.Background(), results, intent)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "text/html", result.ContentType)

	body, err := io.ReadAll(result.Content)
	require.NoError(t, err)
	bodyStr := string(body)

	// Should have 3 unique files: 2.28.0.whl, 2.28.0.tar.gz, 2.25.1.whl
	assert.Contains(t, bodyStr, "requests-2.28.0-py3-none-any.whl")
	assert.Contains(t, bodyStr, "requests-2.28.0.tar.gz")
	assert.Contains(t, bodyStr, "requests-2.25.1-py3-none-any.whl")

	linkCount := strings.Count(bodyStr, `<a href="`)
	assert.Equal(t, 3, linkCount, "should have exactly 3 unique file links (2.28.0.whl deduplicated)")
}

func TestPyPI_MergePackageFilesJSON(t *testing.T) {
	a := NewPyPIAdapter()

	json1 := `{"files":[{"filename":"requests-2.28.0-py3-none-any.whl","url":"/packages/requests-2.28.0-py3-none-any.whl","size":100,"hashes":{"sha256":"abc123"}},{"filename":"requests-2.28.0.tar.gz","url":"/packages/requests-2.28.0.tar.gz","size":200,"hashes":{"sha256":"def456"}}]}`

	json2 := `{"files":[{"filename":"requests-2.25.1-py3-none-any.whl","url":"/packages/requests-2.25.1-py3-none-any.whl","size":90,"hashes":{"sha256":"ghi789"}},{"filename":"requests-2.28.0-py3-none-any.whl","url":"/packages/requests-2.28.0-py3-none-any.whl","size":100,"hashes":{"sha256":"abc123"}}]}`

	results := []*types.ContentResult{
		jsonResult(json1),
		jsonResult(json2),
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata, Path: "simple/requests/", Name: "requests"}
	result, err := a.MergeMetadata(context.Background(), results, intent)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "application/vnd.pypi.simple.v1+json", result.ContentType)

	body, err := io.ReadAll(result.Content)
	require.NoError(t, err)

	var parsed struct {
		Files []struct {
			Filename string `json:"filename"`
			Size     int64  `json:"size"`
		} `json:"files"`
	}
	require.NoError(t, json.Unmarshal(body, &parsed))
	assert.Len(t, parsed.Files, 3, "should have 3 unique files")

	filenames := make(map[string]bool)
	for _, f := range parsed.Files {
		filenames[f.Filename] = true
	}
	assert.True(t, filenames["requests-2.28.0-py3-none-any.whl"])
	assert.True(t, filenames["requests-2.28.0.tar.gz"])
	assert.True(t, filenames["requests-2.25.1-py3-none-any.whl"])
}

func TestPyPI_MergeMetadata_DefaultFallback(t *testing.T) {
	a := NewPyPIAdapter()
	// RequestDownload type should fall through to first result
	res := htmlResult("<html><body>some content</body></html>")
	intent := &types.RequestIntent{Type: types.RequestDownload}
	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res}, intent)
	assert.NoError(t, err)
	assert.Same(t, res, result)
}

func TestPyPI_MergeMetadata_NilContent_Skipped(t *testing.T) {
	a := NewPyPIAdapter()

	res1 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "text/html",
		Content:     nil, // nil content
	}
	res2 := htmlResult(`<!DOCTYPE html><html><body><a href="/repository/p2/pypi/simple/lodash/">lodash</a><br></body></html>`)

	intent := &types.RequestIntent{Type: types.RequestList}
	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res1, res2}, intent)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 200, result.StatusCode)

	body, _ := io.ReadAll(result.Content)
	assert.Contains(t, string(body), "lodash")
}

// ===== NPM MergeMetadata Tests =====

func TestNpm_MergeMetadata_Empty(t *testing.T) {
	a := NewNpmAdapter(nil, nil)
	_, err := a.MergeMetadata(context.Background(), nil, &types.RequestIntent{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no results to merge")
}

func TestNpm_MergeMetadata_SingleResult(t *testing.T) {
	a := NewNpmAdapter(nil, nil)

	meta := map[string]interface{}{
		"name":    "lodash",
		"versions": map[string]interface{}{
			"4.17.21": map[string]interface{}{
				"version": "4.17.21",
				"dist":    map[string]interface{}{"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"},
			},
		},
		"dist-tags": map[string]interface{}{"latest": "4.17.21"},
	}

	res := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(meta)),
	}

	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res}, &types.RequestIntent{Type: types.RequestMetadata})
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestNpm_MergeMetadata_VersionMapMerge(t *testing.T) {
	a := NewNpmAdapter(nil, nil)

	// Member 1 has versions 4.17.20, 4.17.21
	meta1 := map[string]interface{}{
		"name": "lodash",
		"versions": map[string]interface{}{
			"4.17.20": map[string]interface{}{
				"version": "4.17.20",
				"dist":    map[string]interface{}{"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"},
			},
			"4.17.21": map[string]interface{}{
				"version": "4.17.21",
				"dist":    map[string]interface{}{"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"},
			},
		},
		"dist-tags": map[string]interface{}{"latest": "4.17.21"},
	}

	// Member 2 has versions 4.17.21 (duplicate), 4.18.0
	meta2 := map[string]interface{}{
		"name": "lodash",
		"versions": map[string]interface{}{
			"4.17.21": map[string]interface{}{
				"version": "4.17.21",
				"dist":    map[string]interface{}{"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"},
			},
			"4.18.0": map[string]interface{}{
				"version": "4.18.0",
				"dist":    map[string]interface{}{"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.18.0.tgz"},
			},
		},
		"dist-tags": map[string]interface{}{"latest": "4.18.0"},
	}

	res1 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(meta1)),
	}
	res2 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(meta2)),
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata}
	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res1, res2}, intent)
	require.NoError(t, err)
	require.NotNil(t, result)

	body, err := io.ReadAll(result.Content)
	require.NoError(t, err)

	var merged map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &merged))

	versions := merged["versions"].(map[string]interface{})
	assert.Len(t, versions, 3, "should have 3 unique versions: 4.17.20, 4.17.21, 4.18.0")
	assert.Contains(t, versions, "4.17.20")
	assert.Contains(t, versions, "4.17.21")
	assert.Contains(t, versions, "4.18.0")

	// dist-tags should be merged (last wins for each tag)
	distTags := merged["dist-tags"].(map[string]interface{})
	assert.Equal(t, "4.18.0", distTags["latest"], "last member's latest should win")
}

func TestNpm_MergeMetadata_DistTagsMerge(t *testing.T) {
	a := NewNpmAdapter(nil, nil)

	meta1 := map[string]interface{}{
		"name": "express",
		"versions": map[string]interface{}{
			"4.18.0": map[string]interface{}{"version": "4.18.0"},
		},
		"dist-tags": map[string]interface{}{
			"latest": "4.18.0",
			"beta":   "5.0.0-beta.1",
		},
	}

	meta2 := map[string]interface{}{
		"name": "express",
		"versions": map[string]interface{}{
			"4.18.1": map[string]interface{}{"version": "4.18.1"},
		},
		"dist-tags": map[string]interface{}{
			"latest":     "4.18.1",
			"next":       "5.0.0-next.1",
			"release-5": "5.0.0",
		},
	}

	res1 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(meta1)),
	}
	res2 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(meta2)),
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata}
	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res1, res2}, intent)
	require.NoError(t, err)

	body, _ := io.ReadAll(result.Content)
	var merged map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &merged))

	distTags := merged["dist-tags"].(map[string]interface{})
	assert.Len(t, distTags, 4, "should have 4 dist-tags: latest, beta, next, release-5")
	// latest should be overwritten by second member
	assert.Equal(t, "4.18.1", distTags["latest"])
	// beta from first member should remain
	assert.Equal(t, "5.0.0-beta.1", distTags["beta"])
	// next and release-5 from second member
	assert.Equal(t, "5.0.0-next.1", distTags["next"])
	assert.Equal(t, "5.0.0", distTags["release-5"])

	versions := merged["versions"].(map[string]interface{})
	assert.Len(t, versions, 2)
}

func TestNpm_MergeMetadata_TarballURLRewrite(t *testing.T) {
	a := NewNpmAdapter(nil, nil)

	meta1 := map[string]interface{}{
		"name": "lodash",
		"versions": map[string]interface{}{
			"4.17.20": map[string]interface{}{
				"version": "4.17.20",
				"dist":    map[string]interface{}{"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.17.20.tgz"},
			},
		},
	}
	meta2 := map[string]interface{}{
		"name": "lodash",
		"versions": map[string]interface{}{
			"4.17.21": map[string]interface{}{
				"version": "4.17.21",
				"dist":    map[string]interface{}{"tarball": "https://registry.npmjs.org/lodash/-/lodash-4.17.21.tgz"},
			},
		},
	}

	res1 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(meta1)),
	}
	res2 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(meta2)),
	}

	// Set base URL in context to simulate virtual repo
	ctx := context.WithValue(context.Background(), BaseURLCtxKey{}, "http://localhost:9081/repository/npm-virtual")
	intent := &types.RequestIntent{Type: types.RequestMetadata}
	result, err := a.MergeMetadata(ctx, []*types.ContentResult{res1, res2}, intent)
	require.NoError(t, err)

	body, _ := io.ReadAll(result.Content)
	bodyStr := string(body)

	// Tarball URLs should be rewritten to point to virtual repo
	assert.Contains(t, bodyStr, "http://localhost:9081/repository/npm-virtual/lodash/-/lodash-4.17.20.tgz")
	assert.Contains(t, bodyStr, "http://localhost:9081/repository/npm-virtual/lodash/-/lodash-4.17.21.tgz")
	assert.NotContains(t, bodyStr, "https://registry.npmjs.org")
}

func TestNpm_MergeMetadata_NilContentAndExtraData_Skipped(t *testing.T) {
	a := NewNpmAdapter(nil, nil)

	res1 := &types.ContentResult{StatusCode: 200, ContentType: "application/json"} // nil content and nil ExtraData
	res2 := &types.ContentResult{
		StatusCode:  200,
		ContentType: "application/json",
		Content:     io.NopCloser(mustMarshalReader(map[string]interface{}{
			"name":     "test",
			"versions": map[string]interface{}{},
		})),
	}

	intent := &types.RequestIntent{Type: types.RequestMetadata}
	result, err := a.MergeMetadata(context.Background(), []*types.ContentResult{res1, res2}, intent)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func mustMarshalReader(v interface{}) io.Reader {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal JSON: %v", err))
	}
	return bytes.NewReader(data)
}
