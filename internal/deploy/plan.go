package deploy

import (
	"fmt"
	"time"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

func BuildPlan(catalog *Catalog, name string, hosts []config.Host, overrides Overrides) (Plan, error) {
	profile, ok := catalog.ByName[name]
	if !ok {
		return Plan{}, fmt.Errorf("未找到 deploy profile: %s", name)
	}
	selector := profile.Targets
	if overrides.Targets != nil {
		selector = *overrides.Targets
	}
	resolved := profile
	resolved.Targets = selector
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
	handlers, err := ResolveHandlers(catalog.Handlers)
	if err != nil {
		return Plan{}, err
	}

	parallel := profile.Parallel
	if parallel == 0 {
		parallel = overrides.DefaultParallel
	}
	if parallel == 0 {
		parallel = 4
	}
	if overrides.Parallel > 0 {
		parallel = overrides.Parallel
	}
	serial := profile.Serial
	if overrides.Serial > 0 {
		serial = overrides.Serial
	}
	timeout := profile.Timeout
	if timeout.Duration == 0 {
		timeout = overrides.DefaultTimeout
	}
	if timeout.Duration == 0 {
		timeout.Duration = 30 * time.Second
	}
	connectTimeout := profile.ConnectTimeout
	if connectTimeout.Duration == 0 {
		connectTimeout = overrides.DefaultConnectTimeout
	}
	if connectTimeout.Duration == 0 {
		connectTimeout.Duration = 10 * time.Second
	}
	batchOptions := batch.Options{
		Parallel:       parallel,
		Serial:         serial,
		FailFast:       profile.FailFast || overrides.FailFast,
		MaxFail:        profile.MaxFail,
		MaxFailPercent: profile.MaxFailPercent,
	}
	if overrides.MaxFail > 0 {
		batchOptions.MaxFail = overrides.MaxFail
	}
	if overrides.MaxFailPercent > 0 {
		batchOptions.MaxFailPercent = overrides.MaxFailPercent
	}
	plan := Plan{
		Profile: name, Description: profile.Description, Config: profile.Source,
		Sources: append([]string(nil), catalog.Sources...), Targets: selector, Hosts: resolvedHosts,
		Batch: batchOptions, Timeout: timeout, ConnectTimeout: connectTimeout,
		Check: overrides.Check, Diff: overrides.Diff, Steps: steps, Handlers: handlers,
	}
	if err := plan.Validate(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (p Plan) JSON() PlanJSON {
	out := PlanJSON{
		Profile: p.Profile, Description: p.Description, Config: p.Config,
		Sources: append([]string(nil), p.Sources...), Selector: p.Targets,
		Batch: p.Batch, Timeout: p.Timeout, ConnectTimeout: p.ConnectTimeout,
		Check: p.Check, Diff: p.Diff, Steps: p.Steps, Handlers: p.Handlers,
	}
	for _, host := range p.Hosts {
		out.Targets = append(out.Targets, PlanHost{
			Alias: host.Alias, Address: fmt.Sprintf("%s@%s:%d", host.User, host.Host, host.Port),
		})
	}
	return out
}
