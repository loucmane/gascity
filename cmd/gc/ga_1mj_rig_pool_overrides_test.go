package main

import (
	"testing"

	"github.com/gastownhall/gascity/internal/config"
)

func TestReconciliationCity_GenericRigPoolAppliesPerRigOptionDefaults(t *testing.T) {
	maxSessions := 2
	cfg := &config.City{
		Rigs: []config.Rig{
			{
				Name: "blog",
				Path: "/tmp/blog",
				RigPatches: []config.AgentOverride{{
					Agent: "builder",
					OptionDefaults: map[string]string{
						"worklog_access": "classified-vault-and-blog-worktrees",
					},
				}},
			},
			{Name: "gascity", Path: "/tmp/gascity"},
		},
		Agents: []config.Agent{{
			Name:              "builder",
			Scope:             "rig",
			MaxActiveSessions: &maxSessions,
			OptionDefaults: map[string]string{
				"model":          "gpt-5.6-sol",
				"worklog_access": "classified-vault",
			},
		}},
	}

	effective := reconciliationCityWithExpandedGenericRigPools(cfg, nil)
	if effective == cfg {
		t.Fatal("generic rig pool expansion returned the caller config")
	}

	byIdentity := make(map[string]config.Agent, len(effective.Agents))
	for _, agent := range effective.Agents {
		byIdentity[agent.QualifiedName()] = agent
	}
	blog, ok := byIdentity["blog/builder"]
	if !ok {
		t.Fatalf("blog/builder missing from expanded agents: %v", byIdentity)
	}
	if got := blog.OptionDefaults["worklog_access"]; got != "classified-vault-and-blog-worktrees" {
		t.Fatalf("blog/builder worklog_access = %q, want Blog-scoped choice", got)
	}
	if got := blog.OptionDefaults["model"]; got != "gpt-5.6-sol" {
		t.Fatalf("blog/builder model = %q, want inherited generic default", got)
	}

	gasCity, ok := byIdentity["gascity/builder"]
	if !ok {
		t.Fatalf("gascity/builder missing from expanded agents: %v", byIdentity)
	}
	if got := gasCity.OptionDefaults["worklog_access"]; got != "classified-vault" {
		t.Fatalf("gascity/builder worklog_access = %q, want provider-safe default", got)
	}
	if got := cfg.Agents[0].OptionDefaults["worklog_access"]; got != "classified-vault" {
		t.Fatalf("caller config mutated: generic builder worklog_access = %q", got)
	}
}
