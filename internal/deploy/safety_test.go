package deploy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateTargetPath_Denylist(t *testing.T) {
	denied := []string{
		"/",
		"/etc",
		"/etc/cron.d/evil",      // subtree: root code exec via cron
		"/etc/systemd/system/x", // subtree: unit file injection
		"/usr",
		"/usr/bin/sudo",                // subtree: system binaries
		"/usr/share/polkit-1/x",        // subtree: polkit rules
		"/var",                         // exact
		"/var/www",                     // exact: clearing it nukes all sites
		"/var/lib/orascout/state.json", // subtree: own state tamper
		"/var/spool/cron/root",         // subtree: crontab injection
		"/proc/sys/kernel/x",
		"/boot/vmlinuz",
		"/root/.bashrc",
		"/run/systemd/x",
		"relative/path",             // not absolute
		"/opt/app/../../etc/passwd", // traversal
		"",
	}
	for _, p := range denied {
		if err := validateTargetPath(p, nil); err == nil {
			t.Errorf("validateTargetPath(%q) = nil, want error", p)
		}
	}

	allowed := []string{
		"/var/www/html", // classic webroot
		"/var/www/html/site",
		"/opt/tomcat/instances/Bar/webapps/ROOT.war",
		"/home/tomcat/jar-services/foo.jar",
		"/usr/local/bin/mytool", // /usr/local subdirs are legitimate
		"/srv/app/data",
		"/data/deployments", // non-standard root, still fine
	}
	for _, p := range allowed {
		if err := validateTargetPath(p, nil); err != nil {
			t.Errorf("validateTargetPath(%q) = %v, want nil", p, err)
		}
	}
}

func TestValidateTargetPath_Allowlist(t *testing.T) {
	roots := []string{"/var/www/html", "/opt/tomcat/instances"}

	ok := []string{
		"/var/www/html",
		"/var/www/html/Bar-UI/index.html",
		"/opt/tomcat/instances/Bar/webapps/ROOT.war",
	}
	for _, p := range ok {
		if err := validateTargetPath(p, roots); err != nil {
			t.Errorf("validateTargetPath(%q, roots) = %v, want nil", p, err)
		}
	}

	rejected := []string{
		"/var/www/htmlevil",  // prefix trick: not actually under the root
		"/home/tomcat/x.jar", // outside every root
		"/opt/tomcat/instancesX",
	}
	for _, p := range rejected {
		if err := validateTargetPath(p, roots); err == nil {
			t.Errorf("validateTargetPath(%q, roots) = nil, want error", p)
		}
	}

	// Denylist still wins over the allowlist.
	if err := validateTargetPath("/etc/passwd", []string{"/etc"}); err == nil {
		t.Error("allowlisting /etc must not override the built-in denylist")
	}
}

func TestValidateClearPath_MinDepth(t *testing.T) {
	if err := validateClearPath("/data", nil); err == nil {
		t.Error("clearing a depth-1 directory must be refused")
	}
	if err := validateClearPath("/data/site", nil); err != nil {
		t.Errorf("clearing /data/site should be allowed, got %v", err)
	}
	if err := validateClearPath("/var/www/html", nil); err != nil {
		t.Errorf("clearing /var/www/html should be allowed, got %v", err)
	}
}

func TestGuardClearDir_SymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "webroot")
	// A "webroot" that is secretly a symlink to /etc.
	if err := os.Symlink("/etc", link); err != nil {
		t.Fatal(err)
	}
	if err := guardClearDir(link); err == nil {
		t.Error("guardClearDir must refuse a symlink resolving to a protected path")
	}
}

func TestGuardClearDir_MissingDirIsFine(t *testing.T) {
	if err := guardClearDir(filepath.Join(t.TempDir(), "does-not-exist")); err != nil {
		t.Errorf("missing dir should be a no-op, got %v", err)
	}
}

func TestSecurePathJoin(t *testing.T) {
	base := t.TempDir()

	if _, err := securePathJoin(base, "../outside"); err == nil {
		t.Error("traversal out of the artifact dir must be refused")
	}
	if _, err := securePathJoin(base, "a/../../outside"); err == nil {
		t.Error("nested traversal must be refused")
	}
	if _, err := securePathJoin(base, "/etc/passwd"); err == nil {
		t.Error("absolute source paths must be refused")
	}
	if _, err := securePathJoin(base, ""); err == nil {
		t.Error("empty source path must be refused")
	}

	got, err := securePathJoin(base, "dist/index.html")
	if err != nil {
		t.Fatalf("legit relative path rejected: %v", err)
	}
	want := filepath.Join(base, "dist", "index.html")
	if got != want {
		t.Errorf("securePathJoin = %q, want %q", got, want)
	}
}

func TestCopyFile_RefusesSymlinkSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	if err := os.WriteFile(secret, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "innocent.jar")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(link, filepath.Join(dir, "out.jar"), nil, ""); err == nil {
		t.Error("copyFile must refuse a symlink source")
	}
}
