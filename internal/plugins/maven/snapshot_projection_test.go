package maven

import "testing"

func TestCurrentSnapshotFileDisplaysSelectsCurrentPerExtensionAndClassifier(t *testing.T) {
	displays := CurrentSnapshotFileDisplays("lib", "1.0-SNAPSHOT", []string{
		"lib-1.0-20230101.120000-1.jar",
		"lib-1.0-20230102.120000-2.jar",
		"lib-1.0-20230101.120000-1-sources.jar",
		"lib-1.0-20230101.120000-1.pom",
		"maven-metadata.xml",
	})

	assertDisplay := func(filename string, current bool, group string) {
		t.Helper()
		display, ok := displays[filename]
		if !ok {
			t.Fatalf("missing display for %s", filename)
		}
		if display.Current != current {
			t.Fatalf("%s Current = %v, want %v", filename, display.Current, current)
		}
		if display.DisplayGroup != group {
			t.Fatalf("%s DisplayGroup = %q, want %q", filename, display.DisplayGroup, group)
		}
	}

	assertDisplay("lib-1.0-20230101.120000-1.jar", false, "20230101.120000-1")
	assertDisplay("lib-1.0-20230102.120000-2.jar", true, "20230102.120000-2")
	assertDisplay("lib-1.0-20230101.120000-1-sources.jar", true, "20230101.120000-1")
	assertDisplay("lib-1.0-20230101.120000-1.pom", true, "20230101.120000-1")
	if _, ok := displays["maven-metadata.xml"]; ok {
		t.Fatal("maven-metadata.xml should not have snapshot display info")
	}
}
