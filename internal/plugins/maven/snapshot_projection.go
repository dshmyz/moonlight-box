package maven

type SnapshotFileDisplay struct {
	Current      bool
	DisplayGroup string
}

// CurrentSnapshotFileDisplays resolves the current Maven SNAPSHOT file for each
// extension+classifier pair, matching the semantics used by maven-metadata.xml.
func CurrentSnapshotFileDisplays(artifact, version string, filenames []string) map[string]SnapshotFileDisplay {
	type candidate struct {
		filename string
		info     snapshotFileInfo
	}

	currentByType := make(map[string]candidate)
	all := make(map[string]snapshotFileInfo)
	for _, filename := range filenames {
		info, ok := parseSnapshotFileInfo(artifact, version, filename)
		if !ok {
			continue
		}
		all[filename] = info
		key := info.ext + "\x00" + info.classifier
		if current, ok := currentByType[key]; !ok || compareMavenSnapshotBuild(info.timestamp, info.buildNum, current.info.timestamp, current.info.buildNum) > 0 {
			currentByType[key] = candidate{filename: filename, info: info}
		}
	}

	displays := make(map[string]SnapshotFileDisplay, len(all))
	for filename, info := range all {
		key := info.ext + "\x00" + info.classifier
		current := currentByType[key]
		displays[filename] = SnapshotFileDisplay{
			Current:      current.filename == filename,
			DisplayGroup: info.timestamp + "-" + info.buildNum,
		}
	}
	return displays
}
