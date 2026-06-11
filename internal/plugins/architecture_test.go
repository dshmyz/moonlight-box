package plugins

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/dshmyz/moonlight-box/internal/plugins/apt"
	gomod "github.com/dshmyz/moonlight-box/internal/plugins/go"
	"github.com/dshmyz/moonlight-box/internal/plugins/maven"
	"github.com/dshmyz/moonlight-box/internal/plugins/npm"
	"github.com/dshmyz/moonlight-box/internal/plugins/pypi"
	"github.com/dshmyz/moonlight-box/internal/plugins/raw"
	"github.com/dshmyz/moonlight-box/internal/plugins/yum"
)

func TestPluginHandleMethodsRespectArchitectureBoundaries(t *testing.T) {
	pluginFiles, err := filepath.Glob(filepath.Join("*", "plugin.go"))
	if err != nil {
		t.Fatalf("glob plugin files: %v", err)
	}
	if len(pluginFiles) == 0 {
		t.Fatal("expected protocol plugin files")
	}

	for _, file := range pluginFiles {
		t.Run(file, func(t *testing.T) {
			body, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read plugin file: %v", err)
			}
			for _, handle := range extractHandleBodies(string(body)) {
				for _, forbidden := range []string{
					"http.Get(",
					"http.Post(",
					"http.DefaultClient",
					"Repository.Type",
					"*runtime.ProxyRuntime",
					"*runtime.GroupRuntime",
					"*ProxyRuntime",
					"*GroupRuntime",
				} {
					if strings.Contains(handle, forbidden) {
						t.Fatalf("Handle method contains forbidden architecture pattern %q", forbidden)
					}
				}
			}
		})
	}
}

func TestPluginConstructorsRequireHTTPClient(t *testing.T) {
	tests := []struct {
		name    string
		create  func(*http.Client) any
		message string
	}{
		{"apt", func(c *http.Client) any { return apt.NewAptPlugin(c) }, "apt: httpClient is required"},
		{"go", func(c *http.Client) any { return gomod.NewGoPlugin(c) }, "go: httpClient is required"},
		{"maven", func(c *http.Client) any { return maven.NewMavenPlugin(c) }, "maven: httpClient is required"},
		{"npm", func(c *http.Client) any { return npm.NewNpmPlugin(c) }, "npm: httpClient is required"},
		{"pypi", func(c *http.Client) any { return pypi.NewPyPIPlugin(c) }, "pypi: httpClient is required"},
		{"raw", func(c *http.Client) any { return raw.NewGenericPlugin(c) }, "generic: httpClient is required"},
		{"yum", func(c *http.Client) any { return yum.NewYumPlugin(c) }, "yum: httpClient is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name+" nil panics", func(t *testing.T) {
			defer func() {
				got := recover()
				if got != tt.message {
					t.Fatalf("panic = %#v, want %q", got, tt.message)
				}
			}()
			tt.create(nil)
		})

		t.Run(tt.name+" injected client used", func(t *testing.T) {
			client := &http.Client{Timeout: 123 * time.Second}
			plugin := tt.create(client)
			if got := httpClientPointer(plugin); got != reflect.ValueOf(client).Pointer() {
				t.Fatalf("constructor did not store injected client")
			}
		})
	}
}

func httpClientPointer(plugin any) uintptr {
	v := reflect.ValueOf(plugin)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	field := v.FieldByName("httpClient")
	if !field.IsValid() || field.IsNil() {
		return 0
	}
	return field.Pointer()
}

func extractHandleBodies(src string) []string {
	re := regexp.MustCompile(`func \(p \*[^)]*\) Handle\([^)]*\) error \{`)
	matches := re.FindAllStringIndex(src, -1)
	bodies := make([]string, 0, len(matches))
	for _, match := range matches {
		openBrace := match[1] - 1
		if body, ok := extractBalancedBraceBody(src, openBrace); ok {
			bodies = append(bodies, body)
		}
	}
	return bodies
}

func extractBalancedBraceBody(src string, openBrace int) (string, bool) {
	if openBrace < 0 || openBrace >= len(src) || src[openBrace] != '{' {
		return "", false
	}
	depth := 0
	for i := openBrace; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[openBrace : i+1], true
			}
		}
	}
	return "", false
}
