package deploy

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/arglampedakis/orascout/pkg/annotations"
)

// Deploy targets are attacker-influenced input: anyone with push access to a
// watched repo controls the manifest annotations. The guards in this file
// exist so a malicious or fat-fingered target.path ("/", "/etc", "/var", a
// symlinked webroot) cannot make orascout delete or overwrite the host.
//
// Two layers, both enforced BEFORE any side effect (before services are
// stopped, before hooks run):
//
//  1. A built-in denylist that is always on and cannot be configured away:
//     the filesystem root, top-level system directories, and subtrees no
//     artifact deploy has any business touching.
//  2. An optional operator allowlist (config `allowed_target_roots`): when
//     set, every target path must additionally fall under one of the roots.
//
// Honest scope note: the denylist prevents catastrophic accidents and the
// obvious escalation paths, but a denylist can never enumerate every
// dangerous file on a host. The allowlist is the real security boundary —
// without it, push access to a watched repo is close to host access.
// All annotation paths are POSIX paths per SPEC.md, so the string checks
// here use the `path` package (slash semantics), not `filepath`.

// deniedExact are paths that may never themselves be a deploy target or be
// cleared, even when an allowlist would permit them. Writing to a path
// *under* them can still be legal (e.g. /var/www/html) — blocking whole
// subtrees is deniedSubtrees' job.
var deniedExact = map[string]bool{
	"/":          true,
	"/bin":       true,
	"/boot":      true,
	"/dev":       true,
	"/etc":       true,
	"/home":      true,
	"/lib":       true,
	"/lib64":     true,
	"/media":     true,
	"/mnt":       true,
	"/opt":       true,
	"/proc":      true,
	"/root":      true,
	"/run":       true,
	"/sbin":      true,
	"/srv":       true,
	"/sys":       true,
	"/tmp":       true,
	"/usr":       true,
	"/usr/local": true,
	"/var":       true,
	"/var/lib":   true,
	"/var/log":   true,
	"/var/tmp":   true,
	"/var/www":   true,
}

// deniedSubtrees are trees where nothing may be deployed at any depth.
// These are either kernel/boot surfaces or places where a file write is
// equivalent to arbitrary code execution as root (cron, unit files, PAM,
// system binaries) — or orascout's own state, which a deploy must never
// be able to tamper with.
var deniedSubtrees = []string{
	"/bin",
	"/boot",
	"/dev",
	"/etc",
	"/lib",
	"/lib32",
	"/lib64",
	"/libx32",
	"/proc",
	"/root",
	"/run",
	"/sbin",
	"/sys",
	"/usr/bin",
	"/usr/lib",
	"/usr/lib32",
	"/usr/lib64",
	"/usr/libexec",
	"/usr/sbin",
	"/usr/share",
	"/var/lib/orascout",
	"/var/log/orascout",
	"/var/run",
	"/var/spool/cron",
}

// validateTargetPath checks a write target (the file or directory an
// artifact deploys to) against the built-in denylist and, when non-empty,
// the operator allowlist.
func validateTargetPath(raw string, allowedRoots []string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("target path is empty")
	}
	if strings.Contains(raw, "..") {
		return fmt.Errorf("target path %q contains '..' (path traversal is not allowed)", raw)
	}
	if !strings.HasPrefix(raw, "/") {
		return fmt.Errorf("target path %q must be absolute", raw)
	}
	p := path.Clean(raw)

	if deniedExact[p] {
		return fmt.Errorf("target path %q is a protected system path; deploy into a subdirectory instead", p)
	}
	for _, s := range deniedSubtrees {
		if p == s || strings.HasPrefix(p, s+"/") {
			return fmt.Errorf("target path %q is inside protected subtree %q", p, s)
		}
	}

	if len(allowedRoots) > 0 {
		for _, r := range allowedRoots {
			if p == r || strings.HasPrefix(p, r+"/") {
				return nil
			}
		}
		return fmt.Errorf("target path %q is not under any allowed_target_roots entry %v", p, allowedRoots)
	}
	return nil
}

// validateClearPath checks a directory that a strategy intends to CLEAR
// (rm -rf of its contents). Everything validateTargetPath enforces, plus a
// minimum depth: clearing a top-level directory like /data is refused even
// when it isn't on the denylist — clear targets must be at least two levels
// deep (/data/site is fine).
func validateClearPath(raw string, allowedRoots []string) error {
	if err := validateTargetPath(raw, allowedRoots); err != nil {
		return err
	}
	p := path.Clean(raw)
	depth := len(strings.Split(strings.Trim(p, "/"), "/"))
	if depth < 2 {
		return fmt.Errorf("refusing to clear %q: clear targets must be at least 2 directories deep", p)
	}
	return nil
}

// guardClearDir is the filesystem-level check run by clearDirContents just
// before deletion — defense in depth behind validateClearPath. It resolves
// symlinks so a target directory that is secretly a symlink to / (or any
// protected path) is caught even though the string form looked safe.
func guardClearDir(dir string) error {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing exists, nothing to clear
		}
		return fmt.Errorf("resolve %q before clearing: %w", dir, err)
	}
	p := filepath.ToSlash(resolved)
	// Normalise Windows drive prefixes ("C:/x" -> "/x") so the POSIX string
	// checks work in tests on any OS. On the Linux hosts orascout targets,
	// this is a no-op.
	if len(p) >= 2 && p[1] == ':' {
		p = p[2:]
		if p == "" {
			p = "/"
		}
	}
	if err := validateClearPath(p, nil); err != nil {
		return fmt.Errorf("clear target resolves to %q: %w", resolved, err)
	}
	return nil
}

// securePathJoin joins a manifest-supplied relative path onto a base
// directory and guarantees the result stays inside the base. Used for
// source.file / source.dir / hook paths, which must never escape the
// pulled artifact directory.
func securePathJoin(baseDir, rel string) (string, error) {
	if strings.TrimSpace(rel) == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path.IsAbs(rel) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path %q must be relative to the artifact root", rel)
	}
	base := filepath.Clean(baseDir)
	joined := filepath.Join(base, rel)
	if joined != base && !strings.HasPrefix(joined, base+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes the artifact directory", rel)
	}
	return joined, nil
}

// validateRequest runs every path check relevant to the request's strategy.
// Called by Run() before any side effect (service stop, hook, copy, clear).
func validateRequest(req Request) error {
	spec := req.Spec
	switch spec.Type {
	case annotations.TypeJar:
		return validateTargetPath(spec.TargetPath, req.AllowedRoots)

	case annotations.TypeWar:
		if err := validateTargetPath(spec.TargetPath, req.AllowedRoots); err != nil {
			return err
		}
		clearParent := true // default per SPEC §3.2
		if spec.TargetClearParent != nil {
			clearParent = *spec.TargetClearParent
		}
		if clearParent {
			return validateClearPath(path.Dir(path.Clean(spec.TargetPath)), req.AllowedRoots)
		}
		return nil

	case annotations.TypeStatic:
		if err := validateTargetPath(spec.TargetPath, req.AllowedRoots); err != nil {
			return err
		}
		clear := true // default per SPEC §3.3
		if spec.TargetClear != nil {
			clear = *spec.TargetClear
		}
		if clear {
			return validateClearPath(path.Clean(spec.TargetPath), req.AllowedRoots)
		}
		return nil
	}
	// run-once and hook-only write no host target paths; their source/hook
	// paths are contained by securePathJoin at execution time.
	return nil
}
