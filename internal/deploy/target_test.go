package deploy

import (
	"strings"
	"testing"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
)

func TestResolveTargetsUsesTagIntersectionAndHostUnion(t *testing.T) {
	hosts := []config.Host{
		{Alias: "one", Tags: []string{"prod", "web"}},
		{Alias: "two", Tags: []string{"prod"}},
		{Alias: "three", Tags: []string{"test"}},
	}
	selected, err := ResolveTargets(hosts, TargetSelector{Hosts: []string{"three"}, Tags: []string{"prod", "web"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 || selected[0].Alias != "one" || selected[1].Alias != "three" {
		t.Fatalf("selected = %+v", selected)
	}
}

func TestResolveTargetsRejectsAllMixedAndMissingValues(t *testing.T) {
	hosts := []config.Host{{Alias: "one", Tags: []string{"prod"}}}
	for _, selector := range []TargetSelector{
		{All: true, Hosts: []string{"one"}},
		{Hosts: []string{"missing"}},
		{Tags: []string{"missing"}},
	} {
		if _, err := ResolveTargets(hosts, selector); err == nil {
			t.Fatalf("selector should fail: %+v", selector)
		}
	}
	if _, err := ResolveTargets(hosts, TargetSelector{}); err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("empty selector error = %v", err)
	}
}
