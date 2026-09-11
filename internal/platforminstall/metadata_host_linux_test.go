package platforminstall

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMetadataHostIdentityRequiresEveryObservation(t *testing.T) {
	expected := MetadataHost{PID: 42, Boot: "boot", Start: "99", Executable: "/fixture/gc", Address: "127.0.0.1:4400", Listener: "123", City: "fixture"}
	image := []byte("pinned executable")
	stat := "42 (name with spaces) S " + strings.Repeat("0 ", 18) + "99 0"
	files := map[string][]byte{"/proc/42/stat": []byte(stat), "/proc/sys/kernel/random/boot_id": []byte("boot\n"), "/proc/42/exe": image, "/proc/42/net/tcp": []byte("sl local_address rem_address st tx_queue rx_queue tr tm->when retrnsmt uid timeout inode\n0: 0100007F:1130 00000000:0000 0A 0 0 0 0 0 123\n")}
	links := map[string]string{"/proc/42/exe": "/fixture/gc", "/proc/42/fd/7": "socket:[123]"}
	for _, failure := range []string{"none", "stat", "boot", "exe-read", "exe-link", "fds", "fd-link", "tcp", "hash", "listener", "missing", "canceled"} {
		t.Run(failure, func(t *testing.T) {
			reads := metadataHostReads{
				read: func(path string) ([]byte, error) {
					for key, suffix := range map[string]string{"stat": "/stat", "boot": "boot_id", "exe-read": "/exe", "tcp": "/tcp"} {
						if failure == key && strings.HasSuffix(path, suffix) {
							return nil, errors.New("injected read error")
						}
					}
					data, exists := files[path]
					if !exists || failure == "missing" {
						return nil, errors.New("absent")
					}
					return data, nil
				},
				link: func(path string) (string, error) {
					if failure == "exe-link" && strings.HasSuffix(path, "/exe") || failure == "fd-link" && strings.Contains(path, "/fd/") {
						return "", errors.New("readlink failure")
					}
					return links[path], nil
				},
				fds: func(string) ([]string, error) {
					if failure == "fds" {
						return nil, errors.New("enumeration failure")
					}
					return []string{"7"}, nil
				},
			}
			want := expected
			hash := sha256Hex(image)
			if failure == "hash" {
				hash = strings.Repeat("0", 64)
			}
			if failure == "listener" {
				want.Listener = "456"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if failure == "canceled" {
				cancel()
			}
			err := inspectMetadataHost(ctx, want, hash, reads)
			if (err == nil) != (failure == "none") {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestMetadataHostRefusesEndpointAliases(t *testing.T) {
	for _, address := range []string{"localhost:4400", "0.0.0.0:4400", "127.0.0.1:0", "127.0.0.1:04400", "http://127.0.0.1:4400", "127.0.0.1:4400/path"} {
		if _, err := metadataEndpoint(address); err == nil {
			t.Fatalf("accepted %q", address)
		}
	}
	if port, err := metadataEndpoint("127.0.0.1:4400"); err != nil || port != 4400 {
		t.Fatal(port, err)
	}
}

func TestMetadataLeaseReplyExactAndNonrenewing(t *testing.T) {
	request := MetadataLeaseRequest{Transaction: strings.Repeat("a", 64), RequestSHA256: strings.Repeat("b", 64), Nonce: strings.Repeat("c", 64), Deadline: 30}
	proof := MetadataLeaseProof{Request: request, Instance: strings.Repeat("d", 64), Observed: 20}
	if err := validateMetadataLeaseReply(proof, request, proof.Instance, 10, 25); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 6; index++ {
		changed := proof
		switch index {
		case 0:
			changed.Request.Nonce = strings.Repeat("e", 64)
		case 1:
			changed.Request.Deadline++
		case 2:
			changed.Instance = strings.Repeat("e", 64)
		case 3:
			changed.Observed = 9
		case 4:
			changed.Observed = 26
		case 5:
			changed.Request.RequestSHA256 = strings.Repeat("e", 64)
		}
		if err := validateMetadataLeaseReply(changed, request, proof.Instance, 10, 25); err == nil {
			t.Fatalf("accepted mutation %d", index)
		}
	}
}
