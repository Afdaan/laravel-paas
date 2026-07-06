package models

import "testing"

func TestResolveRuntimeExposurePrecedence(t *testing.T) {
	manualPort := 4000
	project := Project{Framework: "Laravel", Port: &manualPort}

	exposure := project.ResolveRuntimeExposure(3000)

	if !exposure.WebFacing || exposure.Port != manualPort || exposure.Reason != RuntimeExposureReasonManualPort {
		t.Fatalf("expected manual port, got %+v", exposure)
	}
}

func TestResolveRuntimeExposureDisabledPort(t *testing.T) {
	disabledPort := 0
	project := Project{Framework: "Laravel", Port: &disabledPort}

	exposure := project.ResolveRuntimeExposure(3000)

	if exposure.WebFacing || exposure.Port != 0 || exposure.Reason != RuntimeExposureReasonDisabledPort {
		t.Fatalf("expected disabled port, got %+v", exposure)
	}
}

func TestResolveRuntimeExposureImageExposeBeatsFrameworkDefault(t *testing.T) {
	project := Project{Framework: "Node.js"}

	exposure := project.ResolveRuntimeExposure(8080)

	if !exposure.WebFacing || exposure.Port != 8080 || exposure.Reason != RuntimeExposureReasonImageExpose {
		t.Fatalf("expected image exposed port, got %+v", exposure)
	}
}

func TestResolveRuntimeExposureFrameworkDefaults(t *testing.T) {
	cases := map[string]int{
		"Laravel":    80,
		"Node.js":    3000,
		"Next.js":    3000,
		"Nuxt.js":    3000,
		"React":      3000,
		"Vue":        3000,
		"Svelte":     3000,
		"Angular":    3000,
		"TypeScript": 3000,
		"Vite":       3000,
	}

	for framework, expectedPort := range cases {
		t.Run(framework, func(t *testing.T) {
			exposure := (&Project{Framework: framework}).ResolveRuntimeExposure(0)
			if !exposure.WebFacing || exposure.Port != expectedPort || exposure.Reason != RuntimeExposureReasonFrameworkDefault {
				t.Fatalf("expected framework default %d, got %+v", expectedPort, exposure)
			}
		})
	}
}

func TestResolveRuntimeExposureUnknownFrameworkIsNotWebFacing(t *testing.T) {
	exposure := (&Project{Framework: "Custom"}).ResolveRuntimeExposure(0)

	if exposure.WebFacing || exposure.Port != 0 || exposure.Reason != RuntimeExposureReasonUnknownFramework {
		t.Fatalf("expected unknown framework non-web, got %+v", exposure)
	}
}
