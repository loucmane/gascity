package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func claudeTrustScreen(menu string) string {
	return "Accessing workspace:\n/work/reviewed\n\nQuick safety check: Is this a project you created or one you trust?\n\n" + menu + "\n\nEnter to confirm · Esc to cancel\n"
}

func runWorkspaceTrustPath(ctx context.Context, stream bool, content string, send func(...string) error) error {
	if !stream {
		return acceptWorkspaceTrustDialog(ctx, time.Second, func(int) (string, error) { return content, nil }, send)
	}
	snapshots := make(chan string, 1)
	snapshots <- content
	close(snapshots)
	_, err := acceptWorkspaceTrustDialogFromStream(ctx, time.Second, newReplayableSnapshotCursor(snapshots), send)
	return err
}

func TestClaudeWorkspaceTrustSelection(t *testing.T) {
	withZeroDialogTimings(t)
	cases := []struct {
		name, screen string
		keys         []string
	}{
		{"captured-no-default", claudeTrustScreen("❯ No, exit\n  Yes, I trust this folder"), []string{"Down", "Enter"}},
		{"yes-default", claudeTrustScreen("❯ Yes, I trust this folder\n  No, exit"), []string{"Enter"}},
		{"reversed-no-default", claudeTrustScreen("  Yes, I trust this folder\n❯ No, exit"), []string{"Up", "Enter"}},
		{"numbered-no-default", claudeTrustScreen("❯ 1. No, exit\n  2. Yes, I trust this folder"), []string{"Down", "Enter"}},
		{"missing-choice", claudeTrustScreen("❯ No, exit"), nil},
		{"missing-selection", claudeTrustScreen("No, exit\nYes, I trust this folder"), nil},
		{"two-selections", claudeTrustScreen("❯ No, exit\n❯ Yes, I trust this folder"), nil},
		{"duplicate-yes", claudeTrustScreen("❯ No, exit\nYes, I trust this folder\nYes, I trust this folder"), nil},
		{"unknown-selected", claudeTrustScreen("❯ Other action\nNo, exit\nYes, I trust this folder"), nil},
		{"unknown-after", claudeTrustScreen("❯ No, exit\nYes, I trust this folder\nOther action"), nil},
		{"unknown-between", claudeTrustScreen("❯ No, exit\nOther action\nYes, I trust this folder"), nil},
		{"mixed-numbering", claudeTrustScreen("❯ 1. No, exit\nYes, I trust this folder"), nil},
		{"unknown-number", claudeTrustScreen("❯ 3. No, exit\n4. Yes, I trust this folder"), nil},
		{"truncated", "Quick safety check\n❯ No, exit\nYes, I trust this folder", nil},
	}
	for _, stream := range []bool{false, true} {
		path := "poll"
		if stream {
			path = "stream"
		}
		for _, tc := range cases {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				var got []string
				err := runWorkspaceTrustPath(context.Background(), stream, tc.screen, func(keys ...string) error {
					got = append(got, keys...)
					return nil
				})
				if (err != nil) != (tc.keys == nil) {
					t.Fatalf("error = %v; expected refusal = %v", err, tc.keys == nil)
				}
				if !reflect.DeepEqual(got, tc.keys) {
					t.Fatalf("keys = %v, want %v", got, tc.keys)
				}
			})
		}
	}
}

func TestClaudeWorkspaceTrustSendFailureAndCancellation(t *testing.T) {
	withZeroDialogTimings(t)
	for _, stream := range []bool{false, true} {
		for _, failAt := range []int{1, 2} {
			want := errors.New("key transport failed")
			calls := 0
			err := runWorkspaceTrustPath(context.Background(), stream, claudeTrustScreen("❯ No, exit\nYes, I trust this folder"), func(...string) error {
				calls++
				if calls == failAt {
					return want
				}
				return nil
			})
			if !errors.Is(err, want) || calls != failAt {
				t.Fatalf("stream=%v failAt=%d calls=%d error=%v", stream, failAt, calls, err)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		err := runWorkspaceTrustPath(ctx, stream, claudeTrustScreen("❯ No, exit\nYes, I trust this folder"), func(...string) error { calls++; return nil })
		if !errors.Is(err, context.Canceled) || calls != 0 {
			t.Fatalf("stream=%v calls=%d error=%v", stream, calls, err)
		}
	}
}
