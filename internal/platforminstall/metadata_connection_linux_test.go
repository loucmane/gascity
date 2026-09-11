package platforminstall

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestMetadataAcceptedConnectionExactEvidence(t *testing.T) {
	host := MetadataHost{PID: 42, Listener: "123", Address: "127.0.0.1:4400"}
	socket := metadataSocket{Local: "0100007F:ABCD", Remote: "0100007F:1130", Inode: 456, Cookie: 789}
	for _, failure := range []string{"none", "wrong-accepter", "listener-not-accepted", "netns", "self-netns", "netns-after", "closed", "reused", "ambiguous", "duplicate-fd", "client-missing", "fd-error", "link-error", "table-error", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			links := map[string]string{"/proc/self/ns/net": "net:[1]", "/proc/42/ns/net": "net:[1]", "/proc/42/fd/7": "socket:[123]", "/proc/42/fd/8": "socket:[321]"}
			inode, state := "321", "01"
			if failure == "wrong-accepter" {
				links["/proc/42/fd/8"] = "socket:[999]"
			}
			if failure == "listener-not-accepted" {
				inode = "123"
			}
			if failure == "netns" {
				links["/proc/42/ns/net"] = "net:[2]"
			}
			if failure == "self-netns" {
				links["/proc/self/ns/net"] = "net:[2]"
			}
			if failure == "closed" {
				state = "08"
			}
			if failure == "reused" {
				inode = "999"
				links["/proc/42/fd/8"] = "socket:[999]"
			}
			table := fmt.Sprintf("sl local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode\n0: %s %s %s 0 0 0 0 0 %s\n", socket.Remote, socket.Local, state, inode)
			if failure == "ambiguous" {
				table += table
			}
			if failure != "client-missing" {
				table += fmt.Sprintf("1: %s %s 01 0 0 0 0 0 456\n", socket.Local, socket.Remote)
			}
			namespaceReads := 0
			reads := metadataHostReads{
				read: func(string) ([]byte, error) {
					if failure == "table-error" {
						return nil, errors.New("read refused")
					}
					return []byte(table), nil
				},
				link: func(path string) (string, error) {
					if path == "/proc/42/ns/net" {
						namespaceReads++
						if failure == "netns-after" && namespaceReads == 2 {
							return "net:[2]", nil
						}
					}
					if failure == "link-error" {
						return "", errors.New("readlink refused")
					}
					value, ok := links[path]
					if !ok {
						return "", errors.New("missing")
					}
					return value, nil
				},
				fds: func(string) ([]string, error) {
					if failure == "fd-error" {
						return nil, errors.New("enumeration refused")
					}
					if failure == "duplicate-fd" {
						return []string{"7", "8", "8"}, nil
					}
					return []string{"7", "8"}, nil
				},
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			actual, err := metadataAcceptedSocket(ctx, host, "net:[1]", socket, "321", reads)
			if (err == nil) != (failure == "none") {
				t.Fatalf("accepted=%q error=%v", actual, err)
			}
			if err == nil && actual != "321" {
				t.Fatal(actual)
			}
		})
	}
}

func TestMetadataConnectionRoutesAreClosed(t *testing.T) {
	for _, city := range []string{"fixture", "city-1", "City_2"} {
		for _, action := range []string{"health", "begin", "check"} {
			method, path, err := metadataConnectionRoute(city, action)
			if err != nil || method == "" || !strings.HasPrefix(path, "/") {
				t.Fatal(method, path, err)
			}
		}
	}
	for _, city := range []string{"", ".", "..", "../svc", "a/b", "a%2fb", "a?x", "a#x", "a\\b", "a\r\nUpgrade: websocket", strings.Repeat("a", 129)} {
		if _, _, err := metadataConnectionRoute(city, "begin"); err == nil {
			t.Fatalf("accepted city %q", city)
		}
	}
	for _, action := range []string{"CONNECT", "upgrade", "svc", "../check", "begin?x"} {
		if _, _, err := metadataConnectionRoute("fixture", action); err == nil {
			t.Fatalf("accepted action %q", action)
		}
	}
}

func TestMetadataSocketContinuityRefusesReuseAndReplacement(t *testing.T) {
	expected := metadataSocket{Local: "0100007F:ABCD", Remote: "0100007F:1130", Inode: 456, Cookie: 789}
	if err := validateMetadataSocketContinuity(expected, expected); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"local", "remote", "inode", "cookie"} {
		changed := expected
		switch field {
		case "local":
			changed.Local = "0100007F:ABCE"
		case "remote":
			changed.Remote = "0100007F:1131"
		case "inode":
			changed.Inode++
		case "cookie":
			changed.Cookie++
		}
		if err := validateMetadataSocketContinuity(expected, changed); err == nil {
			t.Fatalf("accepted %s replacement", field)
		}
	}
}
