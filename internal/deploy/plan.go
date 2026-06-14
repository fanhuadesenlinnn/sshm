package deploy

import (
	"fmt"

	"github.com/fanhuadesenlinnn/sshm/v5/internal/config"
)

type Overrides struct {
	Targets     *TargetSelector
	Mode        string
	MaxParallel int
}

func BuildPlan(catalog *Catalog, name string, hosts []config.Host, overrides Overrides) (Plan, error) {
	profile, ok := catalog.ByName[name]
	if !ok {
		return Plan{}, fmt.Errorf("未找到 deploy profile: %s", name)
	}
	selector := profile.Targets
	if overrides.Targets != nil {
		selector = *overrides.Targets
	}
	strategy := profile.Strategy
	if overrides.Mode != "" {
		strategy.Mode = overrides.Mode
		if overrides.Mode == "visible" && overrides.MaxParallel == 0 {
			strategy.MaxParallel = 1
		}
	}
	if overrides.MaxParallel != 0 {
		strategy.MaxParallel = overrides.MaxParallel
	}
	if err := validateStrategy(strategy); err != nil {
		return Plan{}, err
	}
	resolved := profile
	resolved.Targets = selector
	resolved.Strategy = strategy
	if err := ValidateProfile(resolved, hosts, false); err != nil {
		return Plan{}, err
	}
	resolvedHosts, err := ResolveTargets(hosts, selector)
	if err != nil {
		return Plan{}, err
	}
	steps, err := ResolveSteps(resolved)
	if err != nil {
		return Plan{}, err
	}
	return Plan{
		Profile: name, Description: profile.Description, Config: profile.Source, Sources: append([]string(nil), catalog.Sources...),
		Targets: selector, Hosts: resolvedHosts,
		Strategy: strategy, Steps: steps,
	}, nil
}

func (p Plan) JSON() PlanJSON {
	out := PlanJSON{
		Profile: p.Profile, Description: p.Description, Config: p.Config, Sources: append([]string(nil), p.Sources...), Selector: p.Targets,
		Strategy: p.Strategy, Steps: p.Steps,
	}
	for _, host := range p.Hosts {
		out.Targets = append(out.Targets, PlanHost{
			Alias: host.Alias, Address: fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port),
		})
	}
	return out
}
