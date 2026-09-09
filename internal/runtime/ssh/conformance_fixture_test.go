//go:build integration

package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	shlex "github.com/flynn-archive/go-shlex"
	cryptossh "golang.org/x/crypto/ssh"
)

// The fixture is a protocol peer, not a substitute for Conn.run: production
// shellRunner invokes real OpenSSH. Only the fixed shared-suite tmux grammar
// can reach exec; no remote shell, arbitrary command or user tmux socket exists.
type sshConformanceFixture struct {
	home, socket, tmux string
	ep                 Endpoint
	listener           net.Listener
	config             *cryptossh.ServerConfig
	accepted           chan struct{}
	workers            sync.WaitGroup
	mu                 sync.Mutex
	connections        map[net.Conn]bool
	commands           int
	unexpected         []string
}

func (f *sshConformanceFixture) serve() {
	defer close(f.accepted)
	for {
		conn, err := f.listener.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.connections[conn] = true
		f.mu.Unlock()
		f.workers.Add(1)
		go func() {
			defer f.workers.Done()
			defer func() {
				conn.Close()
				f.mu.Lock()
				delete(f.connections, conn)
				f.mu.Unlock()
			}()
			_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
			server, channels, requests, err := cryptossh.NewServerConn(conn, f.config)
			if err != nil {
				return
			} // Host-key/authentication negatives close here.
			defer server.Close()
			go cryptossh.DiscardRequests(requests)
			for newChannel := range channels {
				if newChannel.ChannelType() != "session" {
					_ = newChannel.Reject(cryptossh.Prohibited, "only fixture exec is supported")
					continue
				}
				channel, requests, err := newChannel.Accept()
				if err != nil {
					continue
				}
				// OpenSSH sends one exec per connection; process requests synchronously
				// so connection completion also proves all command handlers are gone.
				f.handle(channel, requests)
			}
		}()
	}
}

func (f *sshConformanceFixture) handle(channel cryptossh.Channel, requests <-chan *cryptossh.Request) {
	defer channel.Close()
	for request := range requests {
		if request.Type != "exec" {
			if request.WantReply {
				_ = request.Reply(false, nil)
			}
			continue
		}
		var payload struct{ Command string }
		err := cryptossh.Unmarshal(request.Payload, &payload)
		var argv []string
		if err == nil {
			argv, err = f.command(payload.Command)
		}
		if err != nil {
			f.mu.Lock()
			f.unexpected = append(f.unexpected, err.Error())
			f.mu.Unlock()
			_ = request.Reply(false, nil)
			return
		}
		_ = request.Reply(true, nil)
		f.mu.Lock()
		f.commands++
		f.mu.Unlock()
		out, stderr, code := f.runTmux(argv...)
		if code < 0 || code == 255 {
			f.mu.Lock()
			f.unexpected = append(f.unexpected, fmt.Sprintf("fixture execution failure: code=%d stderr=%q", code, stderr))
			f.mu.Unlock()
		}
		_, _ = channel.Write(out)
		_, _ = channel.Stderr().Write(stderr)
		_, _ = channel.SendRequest("exit-status", false, cryptossh.Marshal(struct{ Code uint32 }{uint32(code)}))
		_ = channel.CloseWrite()
		return
	}
}

func (f *sshConformanceFixture) runTmux(args ...string) ([]byte, []byte, int) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.tmux, append([]string{"-S", f.socket, "-f", "/dev/null"}, args...)...)
	cmd.Env = []string{"HOME=" + f.home, "PATH=/usr/bin:/bin", "SHELL=/bin/sh", "TERM=xterm-256color"}
	cmd.Dir = f.home
	cmd.WaitDelay = 10 * time.Second
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err == nil {
		return out, []byte(stderr.String()), 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return out, []byte(stderr.String()), exit.ExitCode()
	}
	return out, []byte(fmt.Sprintf("%s: %v", stderr.String(), err)), 255
}

// command decodes the exact quoted argv form without evaluating a shell.
// Strict operation/argument matching confines the real tmux process to the
// synthetic contract. Rejected grammar is also regression-tested directly.
func (f *sshConformanceFixture) command(command string) ([]string, error) {
	argv, err := shlex.Split(command)
	if err != nil || len(argv) < 2 || shellQuote(argv) != command || argv[0] != "tmux" {
		return nil, fmt.Errorf("invalid fixture command grammar")
	}
	a := argv[1:]
	target := func(index int) bool { return index < len(a) && validTmuxName.MatchString(a[index]) }
	valid := false
	switch a[0] {
	case "new-session":
		valid = len(a) == 7 && a[1] == "-d" && a[2] == "-s" && target(3) &&
			a[4] == "-c" && a[5] == f.home && a[6] == "/bin/sh"
	case "has-session", "kill-session", "clear-history":
		valid = len(a) == 3 && a[1] == "-t" && target(2)
	case "list-sessions":
		valid = len(a) == 3 && a[1] == "-F" && a[2] == "#{session_name}"
	case "display-message":
		valid = len(a) == 5 && a[1] == "-t" && target(2) && a[3] == "-p" &&
			(a[4] == "#{session_attached}" || a[4] == "#{session_activity}")
	case "show-environment":
		valid = len(a) == 4 && a[1] == "-t" && target(2) && fixtureMetaKey(a[3])
	case "set-environment":
		valid = len(a) == 5 && a[1] == "-t" && target(2) &&
			((a[3] == "-u" && fixtureMetaKey(a[4])) || (fixtureMetaKey(a[3]) && fixtureMetaValue(a[4])))
	case "capture-pane":
		valid = len(a) == 6 && a[1] == "-t" && target(2) && a[3] == "-p" && a[4] == "-S" && a[5] == "-10"
	case "send-keys":
		valid = len(a) >= 4 && a[1] == "-t" && target(2) &&
			((len(a) == 4 && (a[3] == "Enter" || a[3] == "C-c")) || (len(a) == 5 && a[3] == "-l" && a[4] == "hello"))
	}
	if !valid {
		return nil, fmt.Errorf("unexpected fixture operation: %q", a)
	}
	return a, nil
}

func fixtureMetaKey(key string) bool {
	switch key {
	case "test-key", "nonexistent-key", "remove-me", "key", "key1", "key2":
		return true
	}
	return false
}

func fixtureMetaValue(value string) bool {
	switch value {
	case "test-value", "value", "v1", "v2", "val1", "val2":
		return true
	}
	return false
}

func (f *sshConformanceFixture) close() error {
	_ = f.listener.Close()
	<-f.accepted
	f.mu.Lock()
	for conn := range f.connections {
		_ = conn.Close()
	}
	f.mu.Unlock()
	f.workers.Wait()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.connections) != 0 || len(f.unexpected) != 0 {
		return fmt.Errorf("SSH fixture connections=%d unexpected=%v", len(f.connections), f.unexpected)
	}
	return nil
}

func sshFixtureHostLine(address string, public cryptossh.PublicKey) []byte {
	return []byte(address + " " + strings.TrimSpace(string(cryptossh.MarshalAuthorizedKey(public))) + "\n")
}

// writeFixtureFile never follows a pre-existing target in the private root.
func writeFixtureFile(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(data)
	return errors.Join(writeErr, file.Close())
}
