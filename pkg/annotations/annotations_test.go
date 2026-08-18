package annotations

import (
	"strings"
	"testing"
	"time"
)

func TestParse_JarHappyPath(t *testing.T) {
	raw := map[string]string{
		KeyType:        "jar",
		KeySourceFile:  "foo.jar",
		KeyTargetPath:  "/opt/jars/foo.jar",
		KeyServiceName: "foo.service",
		OCIKeyTitle:    "Foo",
		OCIKeyVersion:  "1.4.2",
	}
	s, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.Type != TypeJar {
		t.Errorf("Type = %q, want jar", s.Type)
	}
	if s.SourceFile != "foo.jar" {
		t.Errorf("SourceFile = %q", s.SourceFile)
	}
	if s.TargetPath != "/opt/jars/foo.jar" {
		t.Errorf("TargetPath = %q", s.TargetPath)
	}
	if s.ServiceName != "foo.service" {
		t.Errorf("ServiceName = %q", s.ServiceName)
	}
	if s.EffectiveServiceManager() != ManagerSystemdUser {
		t.Errorf("default ServiceManager = %q, want systemd-user", s.EffectiveServiceManager())
	}
	if s.Title != "Foo" {
		t.Errorf("Title = %q", s.Title)
	}
}

func TestParse_MissingType(t *testing.T) {
	_, err := Parse(map[string]string{})
	if err == nil || !strings.Contains(err.Error(), KeyType) {
		t.Errorf("want missing-type error, got %v", err)
	}
}

func TestParse_UnknownType(t *testing.T) {
	_, err := Parse(map[string]string{KeyType: "container"})
	if err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("want unknown-type error, got %v", err)
	}
}

func TestParse_JarMissingRequired(t *testing.T) {
	cases := []struct {
		name string
		raw  map[string]string
	}{
		{"no source.file", map[string]string{
			KeyType:        "jar",
			KeyTargetPath:  "/opt/x.jar",
			KeyServiceName: "x.service",
		}},
		{"no target.path", map[string]string{
			KeyType:        "jar",
			KeySourceFile:  "x.jar",
			KeyServiceName: "x.service",
		}},
		{"no service.name", map[string]string{
			KeyType:       "jar",
			KeySourceFile: "x.jar",
			KeyTargetPath: "/opt/x.jar",
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := Parse(c.raw); err == nil {
				t.Errorf("Parse(%v) succeeded, want error", c.raw)
			}
		})
	}
}

func TestParse_RunOnceRequiresCommand(t *testing.T) {
	_, err := Parse(map[string]string{
		KeyType:       "run-once",
		KeySourceFile: "migrate.jar",
	})
	if err == nil {
		t.Fatal("want error for run-once without command")
	}
	if !strings.Contains(err.Error(), KeyRunonceCommand) {
		t.Errorf("error should mention command key: %v", err)
	}
}

func TestParse_HookOnlyRequiresAHook(t *testing.T) {
	_, err := Parse(map[string]string{KeyType: "hook-only"})
	if err == nil {
		t.Fatal("want error for hook-only without any hook")
	}

	// hook-only with just pre-hook is valid
	if _, err := Parse(map[string]string{
		KeyType:    "hook-only",
		KeyHookPre: "scripts/pre.sh",
	}); err != nil {
		t.Errorf("hook-only with pre-hook should be valid: %v", err)
	}
}

func TestParse_OctalMode(t *testing.T) {
	s, err := Parse(map[string]string{
		KeyType:        "jar",
		KeySourceFile:  "x.jar",
		KeyTargetPath:  "/opt/x.jar",
		KeyServiceName: "x.service",
		KeyTargetMode:  "0644",
	})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if s.TargetMode == nil || *s.TargetMode != 0o644 {
		got := uint32(0)
		if s.TargetMode != nil {
			got = *s.TargetMode
		}
		t.Errorf("TargetMode = %o, want 0644", got)
	}
}

func TestParse_BadOctalMode(t *testing.T) {
	_, err := Parse(map[string]string{
		KeyType:        "jar",
		KeySourceFile:  "x.jar",
		KeyTargetPath:  "/opt/x.jar",
		KeyServiceName: "x.service",
		KeyTargetMode:  "not-a-number",
	})
	if err == nil {
		t.Fatal("want error for invalid octal mode")
	}
}

func TestParse_HealthcheckDefaults(t *testing.T) {
	s, err := Parse(map[string]string{
		KeyType:        "jar",
		KeySourceFile:  "x.jar",
		KeyTargetPath:  "/opt/x.jar",
		KeyServiceName: "x.service",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.HealthTimeout != 30*time.Second {
		t.Errorf("default HealthTimeout = %s, want 30s", s.HealthTimeout)
	}
	if s.HealthInterval != 2*time.Second {
		t.Errorf("default HealthInterval = %s, want 2s", s.HealthInterval)
	}
}

func TestParse_ServiceManagerValidation(t *testing.T) {
	_, err := Parse(map[string]string{
		KeyType:           "jar",
		KeySourceFile:     "x.jar",
		KeyTargetPath:     "/opt/x.jar",
		KeyServiceName:    "x.service",
		KeyServiceManager: "kubernetes",
	})
	if err == nil {
		t.Fatal("want error for unknown service.manager")
	}
}

func TestParse_TargetClearBool(t *testing.T) {
	s, err := Parse(map[string]string{
		KeyType:        "static",
		KeySourceDir:   "dist",
		KeyTargetPath:  "/var/www/x",
		KeyTargetClear: "false",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.TargetClear == nil || *s.TargetClear {
		t.Errorf("TargetClear should be set to false")
	}
}
