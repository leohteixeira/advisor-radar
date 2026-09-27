package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSetsMissingKeyAndKeepsExisting(t *testing.T) {
	t.Setenv("AI_GATEWAY_API_KEY", "from-process")
	t.Setenv("RADAR_TRIAGE_OTHER", "")
	os.Unsetenv("RADAR_TRIAGE_OTHER")

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := "" +
		"# comment\n" +
		"\n" +
		"export AI_GATEWAY_API_KEY=\"from-file\"\n" +
		"RADAR_TRIAGE_OTHER='kept'\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("AI_GATEWAY_API_KEY"); got != "from-process" {
		t.Fatalf("existing key = %q", got)
	}
	if got := os.Getenv("RADAR_TRIAGE_OTHER"); got != "kept" {
		t.Fatalf("file key = %q", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatal(err)
	}
}

func TestParseLine(t *testing.T) {
	key, val, ok, err := parseLine(`AI_GATEWAY_API_KEY="a\"b"`)
	if err != nil || !ok || key != "AI_GATEWAY_API_KEY" || val != `a"b` {
		t.Fatalf("got %q %q ok=%v err=%v", key, val, ok, err)
	}
	if _, _, ok, err := parseLine("not a pair"); err == nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
