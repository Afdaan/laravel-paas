package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProjectExposesDetectedFrameworkSeparately(t *testing.T) {
	project := Project{Framework: "Node.js", DetectedFramework: "Laravel"}

	payload, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}

	serialized := string(payload)
	for _, expected := range []string{`"framework":"Node.js"`, `"detected_framework":"Laravel"`} {
		if !strings.Contains(serialized, expected) {
			t.Fatalf("project JSON missing %s: %s", expected, serialized)
		}
	}
}
