package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseRemoteDirectoryHTML_ApacheStyle(t *testing.T) {
	html := []byte(`<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 3.2 Final//EN">
<html><head><title>Index of /repo/subdir</title></head><body>
<h1>Index of /repo/subdir</h1>
<table><tr><th>Name</th><th>Last modified</th><th>Size</th></tr>
<tr><td><a href="../">Parent Directory</a></td></tr>
<tr><td><a href="builds/">builds/</a></td><td>2024-01-15 10:30</td><td>-</td></tr>
<tr><td><a href="config.yaml">config.yaml</a></td><td>2024-01-14 08:20</td><td>1.2K</td></tr>
<tr><td><a href="release-v1.0.tar.gz">release-v1.0.tar.gz</a></td><td>2024-01-15 10:30</td><td>5.6M</td></tr>
</table></body></html>`)

	entries := parseRemoteDirectoryHTML(html)
	assert.Len(t, entries, 3)

	// Directory entry
	dirEntry := entries[0]
	assert.Equal(t, "builds", dirEntry.Name)
	assert.True(t, dirEntry.IsDir)

	// File entries
	assert.Equal(t, "config.yaml", entries[1].Name)
	assert.False(t, entries[1].IsDir)
	assert.Equal(t, "release-v1.0.tar.gz", entries[2].Name)
	assert.False(t, entries[2].IsDir)
}

func TestParseRemoteDirectoryHTML_NginxStyle(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><head><title>Index of /packages/</title></head><body>
<h1>Index of /packages/</h1><hr>
<pre><a href="../">../</a>
<a href="lib/">lib/</a>                                     15-Jan-2024 10:30                   -
<a href="tools/">tools/</a>                                   14-Jan-2024 08:20                   -
<a href="data.json">data.json</a>                                15-Jan-2024 10:30               12345
<a href="README.md">README.md</a>                                14-Jan-2024 08:20                 890
</pre><hr></body></html>`)

	entries := parseRemoteDirectoryHTML(html)
	assert.Len(t, entries, 4)

	assert.Equal(t, "lib", entries[0].Name)
	assert.True(t, entries[0].IsDir)
	assert.Equal(t, "tools", entries[1].Name)
	assert.True(t, entries[1].IsDir)
	assert.Equal(t, "data.json", entries[2].Name)
	assert.False(t, entries[2].IsDir)
	assert.Equal(t, "README.md", entries[3].Name)
	assert.False(t, entries[3].IsDir)
}

func TestParseRemoteDirectoryHTML_NotDirectory(t *testing.T) {
	// Non-directory HTML: all links are absolute paths (/...) which don't represent
	// directory entries in a repo context. They should NOT be parsed as entries
	// since they start with "/" (absolute paths to other routes, not relative paths).
	html := []byte(`<!DOCTYPE html><html><head><title>My App</title></head><body>
<h1>Welcome</h1>
<a href="/login">Login</a>
<a href="/about">About</a>
<a href="/docs">Docs</a>
</body></html>`)

	entries := parseRemoteDirectoryHTML(html)
	// Absolute path links (starting with /) should not be treated as directory entries
	assert.Empty(t, entries)
}

func TestParseRemoteDirectoryHTML_FiltersParentDir(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><body>
<pre>
<a href="../">Parent Directory</a>
<a href="../../">Parent of Parent</a>
<a href="?C=N;O=D">Name</a>
<a href="?M=A">Date</a>
<a href="?S=S">Size</a>
<a href="?D=A">Type</a>
<a href="real-file.txt">real-file.txt</a>
<a href="subdir/">subdir/</a>
</pre></body></html>`)

	entries := parseRemoteDirectoryHTML(html)
	assert.Len(t, entries, 2)

	assert.Equal(t, "real-file.txt", entries[0].Name)
	assert.False(t, entries[0].IsDir)
	assert.Equal(t, "subdir", entries[1].Name)
	assert.True(t, entries[1].IsDir)
}

func TestParseRemoteDirectoryHTML_EmptyBody(t *testing.T) {
	entries := parseRemoteDirectoryHTML([]byte{})
	assert.Empty(t, entries)
}

func TestParseRemoteDirectoryHTML_NoLinks(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><body><h1>Hello</h1></body></html>`)
	entries := parseRemoteDirectoryHTML(html)
	assert.Empty(t, entries)
}

func TestParseRemoteDirectoryHTML_Duplicates(t *testing.T) {
	// Duplicate links should be deduplicated
	html := []byte(`<pre>
<a href="file.txt">file.txt</a>
<a href="file.txt">file.txt</a>
<a href="dir/">dir/</a>
<a href="dir/">dir/</a>
</pre>`)

	entries := parseRemoteDirectoryHTML(html)
	assert.Len(t, entries, 2)
}

func TestParseRemoteDirectoryHTML_HrefOnlyNoText(t *testing.T) {
	// Links with no text content should use href as name
	html := []byte(`<pre>
<a href="solo-file.bin"></a>
<a href="nested-dir/"></a>
</pre>`)

	entries := parseRemoteDirectoryHTML(html)
	assert.Len(t, entries, 2)
	assert.Equal(t, "solo-file.bin", entries[0].Name)
	assert.False(t, entries[0].IsDir)
	assert.Equal(t, "nested-dir", entries[1].Name)
	assert.True(t, entries[1].IsDir)
}
