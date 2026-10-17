package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/mail"
	"github.com/gastownhall/gascity/internal/session"
	beadslib "github.com/steveyegge/beads"
)

// The inbox's resolver must use the native session directory for names, rather
// than paying BdStore.Get's show + wisp-query miss before resolving the alias.
func TestMailAliasLookupDoesNotSpawnBd(t *testing.T) {
	for _, alias := range []string{"attention-fable", "attention-terra", "watch-officer", "rig/worker.name"} {
		t.Run(alias, func(t *testing.T) {
			upstream := &mailAliasStorage{rows: []*beadslib.Issue{{
				ID: "gc-1234", IssueType: "session", Status: "open",
				Metadata: []byte(fmt.Sprintf(`{"alias":%q,"session_name":"runtime-1234"}`, alias)),
			}}}
			var calls []string
			bd := beads.NewBdStore("/unused", func(_, _ string, args ...string) ([]byte, error) {
				calls = append(calls, strings.Join(args, " "))
				if args[0] == "show" {
					return nil, errors.New("no issue found matching " + args[len(args)-1])
				}
				return []byte("[]"), nil
			})
			native := beads.NewNativeDoltStoreOverStorageForTest(upstream)
			fixture := newProxiedScopeFixture(t)
			store, err := beads.NewProxiedStore(native, bd, fixture.admit(t, false))
			if err != nil {
				t.Fatal(err)
			}
			for _, identifier := range []string{alias, "gc-1234"} {
				target, err := resolveMailTargetsWithConfig("", nil, store, identifier)
				if err != nil {
					t.Fatalf("resolve %q: %v", identifier, err)
				}
				var out, stderr bytes.Buffer
				if code := doMailInboxTarget(emptyAliasInbox{}, target, &out, &stderr); code != 0 || out.String() != "No unread messages for "+alias+"\n" {
					t.Fatalf("inbox = %d, %q, %q", code, out.String(), stderr.String())
				}
			}
			front := session.NewStore(beads.SessionStore{Store: store})
			if info, err := front.ResolveMailboxAddress(alias, false); err != nil || info.ID != "gc-1234" {
				t.Fatalf("mailbox alias = %+v, %v", info, err)
			}
			if _, _, err := session.ResolveSessionRecordByExactID(store, alias); !errors.Is(err, session.ErrSessionNotFound) {
				t.Fatalf("alias is not an exact ID: %v", err)
			}
			upstream.rows[0].ID = "gc-wisp-1234"
			upstream.rows[0].Ephemeral = true
			if id, err := session.ResolveSessionID(store, "gc-wisp-1234"); err != nil || id != "gc-wisp-1234" {
				t.Fatalf("real wisp ID = %q, %v", id, err)
			}
			if _, err := session.ResolveSessionID(store, "gc-wisp-missing"); !errors.Is(err, session.ErrSessionNotFound) {
				t.Fatalf("missing ID: %v", err)
			}
			if len(calls) != 0 {
				t.Fatalf("alias/wisp inbox spawned bd: %v", calls)
			}
		})
	}
}

type mailAliasStorage struct {
	beadslib.Storage
	rows []*beadslib.Issue
}

func (s *mailAliasStorage) SearchIssues(_ context.Context, _ string, f beadslib.IssueFilter) ([]*beadslib.Issue, error) {
	// Deliberately over-return metadata matches: the native adapter must retain
	// its exact projection. IDs are filtered as the library filters them.
	var rows []*beadslib.Issue
	for _, row := range s.rows {
		if len(f.IDs) > 0 && f.IDs[0] != row.ID {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

type emptyAliasInbox struct{}

func (emptyAliasInbox) Inbox(string) ([]mail.Message, error) { return nil, nil }
