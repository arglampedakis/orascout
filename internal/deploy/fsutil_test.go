package deploy

import (
	"reflect"
	"testing"
)

func TestParseShellCommand(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`java -jar foo.jar`, []string{"java", "-jar", "foo.jar"}},
		{`java -jar -Dfile.encoding=UTF-8 {file}`, []string{"java", "-jar", "-Dfile.encoding=UTF-8", "{file}"}},
		{`/bin/sh -c "echo hello world"`, []string{"/bin/sh", "-c", "echo hello world"}},
		{`/bin/sh -c 'echo "quoted"'`, []string{"/bin/sh", "-c", `echo "quoted"`}},
		{`a   b  c`, []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got, err := parseShellCommand(c.in)
		if err != nil {
			t.Errorf("parseShellCommand(%q) err: %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseShellCommand(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseShellCommand_Unterminated(t *testing.T) {
	if _, err := parseShellCommand(`foo "bar`); err == nil {
		t.Errorf("want error for unterminated double quote")
	}
	if _, err := parseShellCommand(`foo 'bar`); err == nil {
		t.Errorf("want error for unterminated single quote")
	}
}

func TestApplyTemplate(t *testing.T) {
	got := applyTemplate([]string{"java", "-jar", "{file}", "--flag"}, "/tmp/foo.jar")
	want := []string{"java", "-jar", "/tmp/foo.jar", "--flag"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("applyTemplate = %v, want %v", got, want)
	}
}
