//go:build integration

package ssh

import (
	"bytes"
	"crypto/ed25519"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	cryptossh "golang.org/x/crypto/ssh"
)

var sshConformance *sshConformanceFixture

func TestMain(m *testing.M) {
	os.Exit(runSSHConformanceMain(m))
}

func runSSHConformanceMain(m *testing.M) int {
	// Resolve dependencies before clearing PATH. Never silently skip this proof.
	sshBinary, err := exec.LookPath("ssh")
	if err != nil {
		fmt.Fprintln(os.Stderr, "SSH conformance requires OpenSSH:", err)
		return 1
	}
	tmuxBinary, err := exec.LookPath("tmux")
	if err != nil {
		fmt.Fprintln(os.Stderr, "SSH conformance requires tmux:", err)
		return 1
	}
	sshBinary, err = filepath.Abs(sshBinary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	tmuxBinary, err = filepath.Abs(tmuxBinary)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	home, err := os.MkdirTemp("/tmp", "gc-ssh-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f := &sshConformanceFixture{
		home: home, socket: filepath.Join(home, "tmux.sock"), tmux: tmuxBinary,
		accepted: make(chan struct{}), connections: make(map[net.Conn]bool),
	}
	sshConformance = f
	defer fmt.Fprintln(os.Stderr, "preserved SSH fixture:", home)
	// A public, deterministic test-only host identity. No operator key, agent or
	// authentication credential is read or created. The server accepts only this
	// synthetic username on an ephemeral IPv4 loopback listener.
	signer, err := cryptossh.NewSignerFromKey(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{42}, ed25519.SeedSize)))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f.config = &cryptossh.ServerConfig{NoClientAuth: true, NoClientAuthCallback: func(c cryptossh.ConnMetadata) (*cryptossh.Permissions, error) {
		if c.User() != "gc-conformance" {
			return nil, fmt.Errorf("fixture user required")
		}
		return nil, nil
	}}
	f.config.AddHostKey(signer)
	f.listener, err = net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	go f.serve()
	defer f.close()
	port := f.listener.Addr().(*net.TCPAddr).Port
	known := filepath.Join(home, "known_hosts")
	if err := writeFixtureFile(known, sshFixtureHostLine("[127.0.0.1]:"+strconv.Itoa(port), signer.PublicKey()), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	f.ep = Endpoint{User: "gc-conformance", Host: "127.0.0.1", Port: port, KnownHostsPath: known}
	// OpenSSH uses passwd-home for ~/.ssh/config, so HOME alone is insufficient.
	// This wrapper only isolates config/identity discovery; it does not replace
	// transport, remote argv, exit status or host-key verification.
	wrapper := "#!/bin/sh\nexec " + shellQuote([]string{
		sshBinary, "-F", "/dev/null", "-o", "IdentityAgent=none", "-o", "IdentityFile=none",
		"-o", "IdentitiesOnly=yes", "-o", "GlobalKnownHostsFile=/dev/null", "-o", "UpdateHostKeys=no", "-o", "ConnectTimeout=10",
	}) + " \"$@\"\n"
	bin := filepath.Join(home, "bin")
	if err := writeFixtureFile(filepath.Join(bin, "ssh"), []byte(wrapper), 0o700); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	os.Clearenv()
	for key, value := range map[string]string{"HOME": home, "PATH": bin + ":/usr/bin:/bin", "SHELL": "/bin/sh", "TERM": "xterm-256color", "LANG": "C.UTF-8"} {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	code := m.Run()
	out, stderr, status := f.runTmux("list-sessions", "-F", "#{session_name}")
	if status != 1 || len(out) != 0 {
		fmt.Fprintf(os.Stderr, "SSH fixture leaked sessions: status=%d out=%q stderr=%q\n", status, out, stderr)
		code = 1
		// Only this test's private socket; never a default/city server.
		_, _, _ = f.runTmux("kill-server")
	}
	if err := f.close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	f.mu.Lock()
	fmt.Fprintf(os.Stderr, "SSH conformance fixture: commands=%d connections=%d unexpected=%d home=%s\n", f.commands, len(f.connections), len(f.unexpected), home)
	f.mu.Unlock()
	return code
}

func TestSSHConformanceEnvironmentIsIsolated(t *testing.T) {
	if os.Getenv("HOME") != sshConformance.home {
		t.Fatal("not in fixture HOME")
	}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		for _, prefix := range []string{"GC_", "SSH_", "ANTHROPIC_", "OPENAI_", "CLAUDE_", "BEADS_", "DOLT_"} {
			if strings.HasPrefix(key, prefix) {
				t.Fatalf("inherited context remains: %s", key)
			}
		}
	}
}
