package deploy

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// copyFile copies src to dst, creating parent dirs as needed. mode, if non-nil,
// is applied to dst after the copy. owner, if non-empty ("user:group"), is
// applied via /bin/chown (so we get the same parsing rules sysadmins expect).
// src must be a regular file: symlinks are refused so an artifact cannot
// smuggle host files (e.g. a link to /etc/shadow) into a deploy target.
func copyFile(src, dst string, mode *uint32, owner string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return fmt.Errorf("stat src: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source %q is not a regular file (mode %s) — symlinks and special files are not deployable", src, info.Mode())
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer in.Close()

	tmp := dst + ".orascout.tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open dst tmp: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close dst tmp: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	if mode != nil {
		if err := os.Chmod(dst, os.FileMode(*mode)); err != nil {
			return fmt.Errorf("chmod: %w", err)
		}
	}
	if owner != "" {
		if err := chown(dst, owner); err != nil {
			return fmt.Errorf("chown: %w", err)
		}
	}
	return nil
}

// copyDir recursively copies the contents of srcDir into dstDir. dstDir is
// created if absent. If clear is true, dstDir contents are removed first.
func copyDir(srcDir, dstDir string, clear bool, mode *uint32, owner string) error {
	if clear {
		if err := clearDirContents(dstDir); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	walkErr := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact contains non-regular file %q (mode %s) — symlinks and special files are not deployable", rel, info.Mode())
		}
		return copyFile(path, target, nil, "")
	})
	if walkErr != nil {
		return walkErr
	}

	if mode != nil || owner != "" {
		if err := applyTreePerms(dstDir, mode, owner); err != nil {
			return err
		}
	}
	return nil
}

// clearDirContents removes everything under dir but leaves dir itself.
// guardClearDir (safety.go) runs first as defense in depth behind the
// request-level validation: it resolves symlinks and refuses protected or
// suspiciously shallow paths, so this primitive can never rm -rf the host
// even if a caller forgets to validate.
func clearDirContents(dir string) error {
	if err := guardClearDir(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func applyTreePerms(root string, mode *uint32, owner string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if mode != nil {
			if err := os.Chmod(path, os.FileMode(*mode)); err != nil {
				return err
			}
		}
		if owner != "" {
			if err := chown(path, owner); err != nil {
				return err
			}
		}
		return nil
	})
}

// chown shells out to /bin/chown so we get the standard user:group parsing
// without having to look up uid/gid manually.
func chown(path, owner string) error {
	out, err := exec.Command("chown", owner, path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("chown %s %s: %w (%s)", owner, path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parseShellCommand is a permissive shell-style splitter for runonce.command.
// It handles single/double quotes; anything fancier should use a hook script.
func parseShellCommand(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == ' ' && !inSingle && !inDouble:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(c)
		}
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quote in %q", s)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

// applyTemplate replaces {file} placeholders in tokens with file.
func applyTemplate(tokens []string, file string) []string {
	out := make([]string, len(tokens))
	for i, t := range tokens {
		out[i] = strings.ReplaceAll(t, "{file}", file)
	}
	return out
}
