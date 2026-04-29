package waltid

// Integration tests that require Docker. Gated behind WALTID_INTEGRATION=1
// so `go test ./...` stays fast for unit-test-only flows. Run with:
//
//   WALTID_INTEGRATION=1 go test -count=1 -v -run Integration ./internal/adapters/waltid/...
//
// Tests use waltid/issuer-api:0.18.2 directly (the image we ship) so any
// breaking change in walt.id's HOCON parser surfaces in CI rather than
// during a /issuer/schema/build save in production.

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/verifiably/verifiably-go/vctypes"
)

func skipUnlessIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("WALTID_INTEGRATION") != "1" {
		t.Skip("set WALTID_INTEGRATION=1 to run live walt.id container tests")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
}

// runDocker is a thin wrapper that returns combined output so failures show
// the actual error from Docker. Timeout caps the wait so a hung Engine API
// doesn't make the test suite hang.
func runDocker(t *testing.T, timeout time.Duration, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	return string(out), err
}

// TestIntegration_WaltidParsesAppendedCatalog is the high-value test: it
// proves walt.id 0.18.2's HOCON parser ACCEPTS an entry produced by
// appendCredentialType. Unit tests can only verify substrings; only the
// real parser can confirm the entry is structurally valid HOCON walt.id
// will deserialise into CredentialTypeConfig.
//
// Strategy:
//  1. Copy the seeded baseline catalog to a temp dir.
//  2. Call appendCredentialType for a custom schema.
//  3. Boot waltid/issuer-api:0.18.2 with the temp dir mounted in.
//  4. Poll /draft13/.well-known/openid-credential-issuer for our configID.
//
// If walt.id reaches "Application started" AND advertises our configID,
// we know parse + load + serve are all working.
func TestIntegration_WaltidParsesAppendedCatalog(t *testing.T) {
	skipUnlessIntegration(t)

	repoRoot, err := repoRoot()
	if err != nil {
		t.Fatalf("locate repo root: %v", err)
	}
	baselinePath := filepath.Join(repoRoot, "deploy/compose/stack/issuer-api/config/credential-issuer-metadata.conf")
	if _, err := os.Stat(baselinePath); err != nil {
		t.Fatalf("seeded baseline missing at %s: %v", baselinePath, err)
	}
	baseline, err := os.ReadFile(baselinePath)
	if err != nil {
		t.Fatalf("read baseline: %v", err)
	}

	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Walt.id needs all three configs to boot. issuer-service.conf and
	// web.conf use the same env-var substitutions as the live deploy.
	if err := os.WriteFile(filepath.Join(configDir, "credential-issuer-metadata.conf"), baseline, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "issuer-service.conf"),
		[]byte(`baseUrl = "http://${SERVICE_HOST}:${ISSUER_API_PORT}"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "web.conf"),
		[]byte(`webHost = "0.0.0.0"`+"\n"+`webPort = "${ISSUER_API_PORT}"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Append a custom entry — this is the unit under test.
	schema := vctypes.Schema{
		ID:     "custom-int1",
		Name:   "Integration Cred",
		Desc:   "validates HOCON appended entry",
		Std:    "w3c_vcdm_2",
		Custom: true,
	}
	configID, changed, err := appendCredentialType(filepath.Join(configDir, "credential-issuer-metadata.conf"), schema)
	if err != nil {
		t.Fatalf("appendCredentialType: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true on first append")
	}

	// Pick a free port to avoid collision with whatever stack the dev has up.
	hostPort := pickFreePort(t)
	containerName := fmt.Sprintf("waltid-integration-%d", time.Now().UnixNano())

	// Make sure we always tear the test container down.
	t.Cleanup(func() {
		_, _ = runDocker(t, 30*time.Second, "rm", "-f", containerName)
	})

	// Start walt.id with our mutated config mounted in.
	out, err := runDocker(t, 30*time.Second, "run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:7002", hostPort),
		"-e", "ISSUER_API_PORT=7002",
		"-e", "SERVICE_HOST=localhost",
		"-v", configDir+":/waltid-issuer-api/config:ro",
		"waltid/issuer-api:0.18.2",
	)
	if err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}

	// Poll the metadata endpoint. ~10s for boot is comfortable on CI; the
	// one-shot Phase-0 test booted in 0.3s on a dev laptop.
	deadline := time.Now().Add(30 * time.Second)
	url := fmt.Sprintf("http://localhost:%d/draft13/.well-known/openid-credential-issuer", hostPort)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var meta credentialIssuerMetadata
		dec := json.NewDecoder(resp.Body)
		decErr := dec.Decode(&meta)
		resp.Body.Close()
		if decErr != nil {
			lastErr = decErr
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if _, ok := meta.CredentialConfigurationsSupported[configID]; ok {
			return // success — walt.id parsed our entry and exposes it
		}
		// Walt.id is up but the configID isn't there. Dump what IS there to
		// make the failure debuggable.
		keys := make([]string, 0, len(meta.CredentialConfigurationsSupported))
		for k := range meta.CredentialConfigurationsSupported {
			keys = append(keys, k)
		}
		t.Fatalf("walt.id booted but configID %q not in credential_configurations_supported. configs: %v",
			configID, keys)
	}
	logs, _ := runDocker(t, 5*time.Second, "logs", "--tail", "60", containerName)
	t.Fatalf("walt.id never exposed metadata at %s within 30s: %v\n--container logs--\n%s", url, lastErr, logs)
}

// TestIntegration_RestartContainer exercises the docker.go restart path
// against a real (but tiny) sentinel container. We skip the issuer-api
// itself — booting walt.id twice in a test is slow — and instead use a
// busybox sleeper labeled with com.docker.compose.service so
// findContainerByService picks it up exactly the way it would in prod.
func TestIntegration_RestartContainer(t *testing.T) {
	skipUnlessIntegration(t)

	containerName := fmt.Sprintf("waltid-restart-test-%d", time.Now().UnixNano())
	serviceName := containerName // use unique label so other compose services don't collide

	t.Cleanup(func() {
		_, _ = runDocker(t, 30*time.Second, "rm", "-f", containerName)
	})

	out, err := runDocker(t, 30*time.Second, "run", "-d",
		"--name", containerName,
		"--label", "com.docker.compose.service="+serviceName,
		"busybox:latest", "sleep", "120",
	)
	if err != nil {
		t.Fatalf("docker run busybox sentinel: %v\n%s", err, out)
	}

	// Capture the original container's StartedAt so we can compare post-restart.
	initial, err := containerStartedAt(t, containerName)
	if err != nil {
		t.Fatalf("read initial StartedAt: %v", err)
	}

	if err := restartContainer(serviceName); err != nil {
		t.Fatalf("restartContainer: %v", err)
	}

	after, err := containerStartedAt(t, containerName)
	if err != nil {
		t.Fatalf("read post-restart StartedAt: %v", err)
	}
	if !after.After(initial) {
		t.Errorf("StartedAt did not advance after restart: initial=%s after=%s", initial, after)
	}
}

// TestIntegration_FindContainerByServiceMissing surfaces the friendly error
// when no compose container matches. Real Bug-Pre-Phase-1 behaviour: a
// silent nil. Now the operator gets a useful message in the toast.
func TestIntegration_FindContainerByServiceMissing(t *testing.T) {
	skipUnlessIntegration(t)
	_, err := findContainerByService("definitely-not-a-real-compose-service-xyz123")
	if err == nil {
		t.Fatal("expected error for missing service")
	}
	if !strings.Contains(err.Error(), "no container found") {
		t.Errorf("error should mention 'no container found', got %v", err)
	}
}

// repoRoot walks up from the test working dir until it finds a go.mod file.
// Lets the integration test resolve deploy/compose paths regardless of
// where the test runner CDs to.
func repoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for d := wd; d != "/" && d != ""; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
	}
	return "", fmt.Errorf("no go.mod above %s", wd)
}

// pickFreePort asks the kernel for an unused TCP port. Avoids a hard-coded
// port colliding with the dev's own running stack on the same machine.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// containerStartedAt parses Docker's State.StartedAt for a container.
func containerStartedAt(t *testing.T, name string) (time.Time, error) {
	t.Helper()
	out, err := runDocker(t, 5*time.Second, "inspect", "--format", "{{.State.StartedAt}}", name)
	if err != nil {
		return time.Time{}, fmt.Errorf("docker inspect: %w (%s)", err, out)
	}
	return time.Parse(time.RFC3339Nano, strings.TrimSpace(out))
}
