package proc_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

var serviceCommands = []string{
	"account-sim",
	"advisory",
	"triage",
	"cases",
	"timeline-indexer",
	"bff",
}

// safeBuffer is a bytes.Buffer guarded for concurrent writers and readers.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, b.buf.Len())
	copy(out, b.buf.Bytes())
	return out
}

func (b *safeBuffer) String() string {
	return string(b.Bytes())
}

func TestCommands_SignalStop(t *testing.T) {
	moduleRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("module root: %v", err)
	}

	signals := []struct {
		name string
		sig  syscall.Signal
	}{
		{name: "SIGTERM", sig: syscall.SIGTERM},
		{name: "SIGINT", sig: syscall.SIGINT},
	}

	for _, name := range serviceCommands {
		bin := filepath.Join(t.TempDir(), name)
		build := exec.Command("go", "build", "-o", bin, "./cmd/"+name)
		build.Dir = moduleRoot
		build.Env = os.Environ()
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build ./cmd/%s: %v\n%s", name, err, out)
		}

		for _, signal := range signals {
			t.Run(name+"/"+signal.name, func(t *testing.T) {
				cmd := exec.Command(bin)
				var stdout safeBuffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stdout
				if err := cmd.Start(); err != nil {
					t.Fatalf("start %s: %v", name, err)
				}

				deadline := time.Now().Add(2 * time.Second)
				for {
					if bytes.Contains(stdout.Bytes(), []byte(name)) {
						break
					}
					if time.Now().After(deadline) {
						_ = cmd.Process.Kill()
						t.Fatalf("%s did not log service name before signal; output=%q", name, stdout.String())
					}
					time.Sleep(10 * time.Millisecond)
				}

				if err := cmd.Process.Signal(signal.sig); err != nil {
					t.Fatalf("%s %s: %v", signal.name, name, err)
				}

				done := make(chan error, 1)
				go func() { done <- cmd.Wait() }()

				select {
				case err := <-done:
					if err != nil {
						t.Fatalf("%s exit error = %v after %s, want 0; output=%q", name, err, signal.name, stdout.String())
					}
				case <-time.After(5 * time.Second):
					_ = cmd.Process.Kill()
					t.Fatalf("%s did not exit after %s; output=%q", name, signal.name, stdout.String())
				}

				var line map[string]any
				found := false
				for _, raw := range bytes.Split(stdout.Bytes(), []byte("\n")) {
					raw = bytes.TrimSpace(raw)
					if len(raw) == 0 {
						continue
					}
					if err := json.Unmarshal(raw, &line); err != nil {
						continue
					}
					if svc, _ := line["service"].(string); svc == name {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("%s output missing JSON log with service=%q; output=%q", name, name, stdout.String())
				}
			})
		}
	}
}
