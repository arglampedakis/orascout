// Package annotations defines the orascout v1 manifest annotation schema and a
// parser that turns a raw annotation map into a typed Spec. This package is
// importable by push-side tools (CI pipelines) so they can construct valid
// annotation sets without duplicating string constants.
package annotations

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Namespace is the reverse-DNS prefix for all orascout v1 annotations.
const Namespace = "dev.orascout.v1"

// Annotation keys. Keep these grouped by purpose; mirrors SPEC.md §2–§5.
const (
	KeyType = Namespace + ".type"

	KeySourceFile = Namespace + ".source.file"
	KeySourceDir  = Namespace + ".source.dir"

	KeyTargetPath        = Namespace + ".target.path"
	KeyTargetMode        = Namespace + ".target.mode"
	KeyTargetOwner       = Namespace + ".target.owner"
	KeyTargetClear       = Namespace + ".target.clear"
	KeyTargetClearParent = Namespace + ".target.clear-parent"

	KeyServiceName    = Namespace + ".service.name"
	KeyServiceManager = Namespace + ".service.manager"

	KeyRunonceCommand = Namespace + ".runonce.command"

	KeyHookPre  = Namespace + ".hook.pre"
	KeyHookPost = Namespace + ".hook.post"

	KeyHealthCmd      = Namespace + ".healthcheck.cmd"
	KeyHealthTimeout  = Namespace + ".healthcheck.timeout"
	KeyHealthInterval = Namespace + ".healthcheck.interval"
)

// Standard OCI annotations (informational only).
const (
	OCIKeyTitle       = "org.opencontainers.image.title"
	OCIKeyDescription = "org.opencontainers.image.description"
	OCIKeyVersion     = "org.opencontainers.image.version"
	OCIKeyCreated     = "org.opencontainers.image.created"
	OCIKeySource      = "org.opencontainers.image.source"
	OCIKeyRevision    = "org.opencontainers.image.revision"
)

// Type is the deploy strategy selector.
type Type string

const (
	TypeJar      Type = "jar"
	TypeWar      Type = "war"
	TypeStatic   Type = "static"
	TypeRunOnce  Type = "run-once"
	TypeHookOnly Type = "hook-only"
)

// ServiceManager selects which service manager hosts the unit.
type ServiceManager string

const (
	ManagerSystemdUser ServiceManager = "systemd-user"
	ManagerSystemd     ServiceManager = "systemd"
	ManagerNone        ServiceManager = "none"
)

// Spec is the parsed, validated form of the annotation set on a manifest.
type Spec struct {
	Type Type

	SourceFile string
	SourceDir  string

	TargetPath        string
	TargetMode        *uint32 // nil = leave alone
	TargetOwner       string  // "user:group" or "" to skip
	TargetClear       *bool   // nil = strategy default
	TargetClearParent *bool   // nil = strategy default

	ServiceName    string
	ServiceManager ServiceManager // empty = default (systemd-user)

	RunonceCommand string

	HookPre  string
	HookPost string

	HealthCmd      string
	HealthTimeout  time.Duration
	HealthInterval time.Duration

	// Informational
	Title       string
	Description string
	Version     string
}

// Parse turns a raw OCI annotations map into a validated Spec. Returns an
// error if a required annotation is missing or malformed.
func Parse(raw map[string]string) (*Spec, error) {
	s := &Spec{
		HealthTimeout:  30 * time.Second,
		HealthInterval: 2 * time.Second,
	}

	t := strings.TrimSpace(raw[KeyType])
	if t == "" {
		return nil, fmt.Errorf("missing required annotation %s", KeyType)
	}
	switch Type(t) {
	case TypeJar, TypeWar, TypeStatic, TypeRunOnce, TypeHookOnly:
		s.Type = Type(t)
	default:
		return nil, fmt.Errorf("unknown %s value %q (supported: jar|war|static|run-once|hook-only)", KeyType, t)
	}

	s.SourceFile = strings.TrimSpace(raw[KeySourceFile])
	s.SourceDir = strings.TrimSpace(raw[KeySourceDir])
	s.TargetPath = strings.TrimSpace(raw[KeyTargetPath])
	s.TargetOwner = strings.TrimSpace(raw[KeyTargetOwner])
	s.ServiceName = strings.TrimSpace(raw[KeyServiceName])
	s.RunonceCommand = strings.TrimSpace(raw[KeyRunonceCommand])
	s.HookPre = strings.TrimSpace(raw[KeyHookPre])
	s.HookPost = strings.TrimSpace(raw[KeyHookPost])
	s.HealthCmd = strings.TrimSpace(raw[KeyHealthCmd])

	if v := strings.TrimSpace(raw[KeyTargetMode]); v != "" {
		m, err := parseFileMode(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", KeyTargetMode, err)
		}
		s.TargetMode = &m
	}
	if v := strings.TrimSpace(raw[KeyTargetClear]); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", KeyTargetClear, err)
		}
		s.TargetClear = &b
	}
	if v := strings.TrimSpace(raw[KeyTargetClearParent]); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", KeyTargetClearParent, err)
		}
		s.TargetClearParent = &b
	}

	if v := strings.TrimSpace(raw[KeyServiceManager]); v != "" {
		switch ServiceManager(v) {
		case ManagerSystemd, ManagerSystemdUser, ManagerNone:
			s.ServiceManager = ServiceManager(v)
		default:
			return nil, fmt.Errorf("%s: unknown value %q", KeyServiceManager, v)
		}
	}

	if v := strings.TrimSpace(raw[KeyHealthTimeout]); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", KeyHealthTimeout, err)
		}
		s.HealthTimeout = d
	}
	if v := strings.TrimSpace(raw[KeyHealthInterval]); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", KeyHealthInterval, err)
		}
		s.HealthInterval = d
	}

	s.Title = raw[OCIKeyTitle]
	s.Description = raw[OCIKeyDescription]
	s.Version = raw[OCIKeyVersion]

	if err := s.validate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Spec) validate() error {
	switch s.Type {
	case TypeJar:
		if s.SourceFile == "" {
			return fmt.Errorf("type=jar requires %s", KeySourceFile)
		}
		if s.TargetPath == "" {
			return fmt.Errorf("type=jar requires %s", KeyTargetPath)
		}
		if s.ServiceName == "" {
			return fmt.Errorf("type=jar requires %s", KeyServiceName)
		}
	case TypeWar:
		if s.SourceFile == "" {
			return fmt.Errorf("type=war requires %s", KeySourceFile)
		}
		if s.TargetPath == "" {
			return fmt.Errorf("type=war requires %s", KeyTargetPath)
		}
		if s.ServiceName == "" {
			return fmt.Errorf("type=war requires %s", KeyServiceName)
		}
	case TypeStatic:
		if s.SourceDir == "" {
			return fmt.Errorf("type=static requires %s", KeySourceDir)
		}
		if s.TargetPath == "" {
			return fmt.Errorf("type=static requires %s", KeyTargetPath)
		}
	case TypeRunOnce:
		if s.SourceFile == "" {
			return fmt.Errorf("type=run-once requires %s", KeySourceFile)
		}
		if s.RunonceCommand == "" {
			return fmt.Errorf("type=run-once requires %s", KeyRunonceCommand)
		}
	case TypeHookOnly:
		if s.HookPre == "" && s.HookPost == "" {
			return fmt.Errorf("type=hook-only requires at least one of %s or %s", KeyHookPre, KeyHookPost)
		}
	}
	return nil
}

// EffectiveServiceManager returns the manager to use, defaulting to systemd-user
// when the spec leaves it unset.
func (s *Spec) EffectiveServiceManager() ServiceManager {
	if s.ServiceManager == "" {
		return ManagerSystemdUser
	}
	return s.ServiceManager
}

// parseFileMode parses an octal string like "0644" or "644" into a uint32.
func parseFileMode(s string) (uint32, error) {
	v, err := strconv.ParseUint(s, 8, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid octal file mode %q: %w", s, err)
	}
	return uint32(v), nil
}
