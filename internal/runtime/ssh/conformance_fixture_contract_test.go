//go:build integration

package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	cryptossh "golang.org/x/crypto/ssh"
)

func TestSSHFixtureRefusesUnboundedCommands(t *testing.T) {
	f := sshConformance
	for _, command := range []string{
		"tmux list-sessions", "'tmux' 'list-sessions'; 'touch' '/tmp/not-allowed'",
		shellQuote([]string{"sh", "-c", "true"}), shellQuote([]string{"tmux", "kill-server"}),
		shellQuote([]string{"tmux", "-S", "/tmp/other", "list-sessions"}),
		shellQuote([]string{"tmux", "new-session", "-d", "-s", "s", "-c", "/", "/bin/sh"}),
		shellQuote([]string{"tmux", "new-session", "-d", "-s", "s", "-c", f.home, "arbitrary-command"}),
		shellQuote([]string{"tmux", "send-keys", "-t", "s", "-l", "arbitrary-command"}),
	} {
		if _, err := f.command(command); err == nil {
			t.Errorf("accepted %q", command)
		}
	}
	if _, err := f.command(shellQuote([]string{"tmux", "new-session", "-d", "-s", "s", "-c", f.home, "/bin/sh"})); err != nil {
		t.Fatal(err)
	}
}

func TestSSHConformanceRejectsChangedHostKey(t *testing.T) {
	signer, err := cryptossh.NewSignerFromKey(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{43}, ed25519.SeedSize)))
	if err != nil {
		t.Fatal(err)
	}
	ep := sshConformance.ep
	ep.KnownHostsPath = filepath.Join(t.TempDir(), "known_hosts")
	before := sshFixtureHostLine("[127.0.0.1]:"+strconv.Itoa(ep.Port), signer.PublicKey())
	if err := os.WriteFile(ep.KnownHostsPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, _, err = New(ep).Exec(ctx, "", []string{"tmux", "list-sessions", "-F", "#{session_name}"})
	if err == nil {
		t.Fatal("changed host key accepted")
	}
	after, readErr := os.ReadFile(ep.KnownHostsPath)
	if readErr != nil || !bytes.Equal(before, after) {
		t.Fatal("refusal changed known_hosts")
	}
}

func TestSSHConformanceRejectsOtherUser(t *testing.T) {
	ep := sshConformance.ep
	ep.User = "not-the-fixture-user"
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, _, err := New(ep).Exec(ctx, "", []string{"tmux", "list-sessions", "-F", "#{session_name}"}); err == nil {
		t.Fatal("unexpected user accepted")
	}
}
