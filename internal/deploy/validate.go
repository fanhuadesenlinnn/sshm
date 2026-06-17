package deploy

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/fanhuadesenlinnn/sshm/v6/internal/config"
)

var becomeUserPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

func ValidateCatalog(catalog *Catalog, hosts []config.Host) error {
	return validateCatalog(catalog, hosts, false)
}

func ValidateCatalogAllowEmptyTargetMatches(catalog *Catalog, hosts []config.Host) error {
	return validateCatalog(catalog, hosts, len(hosts) == 0)
}

func validateCatalog(catalog *Catalog, hosts []config.Host, allowEmptyTargetMatches bool) error {
	if len(catalog.Profiles) == 0 {
		return fmt.Errorf("deploy 配置不包含 profile")
	}
	for _, handler := range catalog.Handlers {
		if err := validateHandler(handler); err != nil {
			return fmt.Errorf("handler %q: %w", handler.Name, err)
		}
	}
	for _, profile := range catalog.Profiles {
		if err := validateProfile(profile, hosts, false, allowEmptyTargetMatches); err != nil {
			return fmt.Errorf("%s: profile %q: %w", profile.Source, profile.Name, err)
		}
		for _, step := range profile.Steps {
			for _, name := range step.Notify {
				if _, ok := catalog.HandlerByName[name]; !ok {
					return fmt.Errorf("%s: profile %q step %q 引用了不存在的 handler %q", profile.Source, profile.Name, step.Name, name)
				}
			}
		}
	}
	return nil
}

func ValidateProfile(profile Profile, hosts []config.Host, allowEmptyTargets bool) error {
	return validateProfile(profile, hosts, allowEmptyTargets, false)
}

func validateProfile(profile Profile, hosts []config.Host, allowEmptyTargets, allowEmptyTargetMatches bool) error {
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("名称不能为空")
	}
	if !profileNamePattern.MatchString(profile.Name) {
		return fmt.Errorf("名称只能包含字母、数字、点、下划线和短横线")
	}
	if len(profile.Steps) == 0 {
		return fmt.Errorf("至少需要一个 step")
	}
	if err := validateBatchFields(profile.Serial, profile.Parallel, profile.MaxFail, profile.MaxFailPercent); err != nil {
		return err
	}
	for index, step := range profile.Steps {
		if err := validateStep(step, false); err != nil {
			return fmt.Errorf("step %d (%s): %w", index+1, step.DisplayName(index), err)
		}
	}
	if profile.Targets.Empty() {
		if allowEmptyTargets {
			return nil
		}
		return fmt.Errorf("targets 不能为空")
	}
	if err := validateTargetSelectorShape(profile.Targets); err != nil {
		return err
	}
	if allowEmptyTargetMatches && len(hosts) == 0 {
		return nil
	}
	_, err := ResolveTargets(hosts, profile.Targets)
	return err
}

func validateHandler(handler Step) error {
	if strings.TrimSpace(handler.Name) == "" {
		return fmt.Errorf("名称不能为空")
	}
	if err := validateStep(handler, true); err != nil {
		return err
	}
	if handler.ActionType() == "confirm" {
		return fmt.Errorf("handler 不允许 confirm")
	}
	if len(handler.Notify) > 0 {
		return fmt.Errorf("handler 不允许继续 notify")
	}
	return nil
}

func validateBatchFields(serial, parallel, maxFail, maxFailPercent int) error {
	if serial < 0 {
		return fmt.Errorf("serial 必须大于 0")
	}
	if parallel < 0 || parallel > 128 {
		return fmt.Errorf("parallel 必须在 1 到 128 之间")
	}
	if maxFail < 0 {
		return fmt.Errorf("max_fail 必须大于 0")
	}
	if maxFailPercent < 0 || maxFailPercent > 100 {
		return fmt.Errorf("max_fail_percent 必须在 1 到 100 之间")
	}
	return nil
}

func validateStep(step Step, handler bool) error {
	actions := 0
	if step.Exec != "" {
		actions++
	}
	if step.Push != nil {
		actions++
	}
	if step.Pull != nil {
		actions++
	}
	if step.Mkdir != nil {
		actions++
	}
	if step.Wait != nil {
		actions++
	}
	if step.Confirm != "" {
		actions++
	}
	if actions != 1 {
		return fmt.Errorf("必须且只能包含一个 action")
	}
	if step.Timeout.Duration < 0 {
		return fmt.Errorf("timeout 不能为负数")
	}
	if step.Push != nil {
		if step.Push.Src == "" || step.Push.Dest == "" {
			return fmt.Errorf("push 需要 src 和 dest")
		}
		if err := validateRemotePath(step.Push.Dest); err != nil {
			return fmt.Errorf("push dest: %w", err)
		}
		if step.Push.Overwrite && step.Push.Backup {
			return fmt.Errorf("push overwrite 与 backup 不能同时使用")
		}
	}
	if step.Pull != nil {
		if step.Pull.Src == "" || step.Pull.Dest == "" {
			return fmt.Errorf("pull 需要 src 和 dest")
		}
		if err := validateRemotePath(step.Pull.Src); err != nil {
			return fmt.Errorf("pull src: %w", err)
		}
		if step.Pull.Overwrite && step.Pull.Backup {
			return fmt.Errorf("pull overwrite 与 backup 不能同时使用")
		}
	}
	if step.Mkdir != nil {
		if step.Mkdir.Path == "" {
			return fmt.Errorf("mkdir 需要 path")
		}
		if err := validateRemotePath(step.Mkdir.Path); err != nil {
			return fmt.Errorf("mkdir path: %w", err)
		}
		if step.Mkdir.Mode != "" {
			if _, err := strconv.ParseUint(step.Mkdir.Mode, 8, 12); err != nil {
				return fmt.Errorf("mkdir mode 必须是八进制权限字符串")
			}
		}
	}
	if step.Wait != nil && step.Wait.Duration <= 0 {
		return fmt.Errorf("wait 必须大于 0")
	}
	if step.ActionType() != "exec" {
		if step.Become || step.BecomeUser != "" || step.CheckSafe || step.FailedWhen != nil || step.ChangedWhen != nil {
			return fmt.Errorf("become、become_user、check_safe、failed_when 和 changed_when 仅适用于 exec")
		}
	}
	if step.BecomeUser != "" && !becomeUserPattern.MatchString(step.BecomeUser) {
		return fmt.Errorf("become_user 格式不安全")
	}
	for name, condition := range map[string]*Condition{"failed_when": step.FailedWhen, "changed_when": step.ChangedWhen} {
		if condition != nil {
			if err := validateCondition(*condition); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
	}
	if handler && step.IgnoreError {
		return fmt.Errorf("handler 不支持 ignore_error")
	}
	return nil
}

func validateRemotePath(value string) error {
	if strings.ContainsRune(value, '\x00') || value == "~" || strings.HasPrefix(value, "~/") {
		return fmt.Errorf("远端路径必须是明确路径，且不支持 ~ 或 NUL")
	}
	for _, component := range strings.Split(value, "/") {
		if component == ".." {
			return fmt.Errorf("远端路径不能包含上级目录组件")
		}
	}
	clean := path.Clean(value)
	if clean == "." || clean == "/" {
		return fmt.Errorf("远端路径必须指向明确文件或目录")
	}
	return nil
}

func validateCondition(condition Condition) error {
	if (len(condition.RCIn) == 0) == (len(condition.RCNotIn) == 0) {
		return fmt.Errorf("必须且只能包含 rc_in 或 rc_not_in")
	}
	return nil
}

func ResolveSteps(profile Profile) ([]Step, error) {
	steps := make([]Step, len(profile.Steps))
	for index, step := range profile.Steps {
		resolved := step
		if resolved.Push != nil {
			copy := *resolved.Push
			if !filepath.IsAbs(copy.Src) {
				copy.Src = filepath.Join(profile.BaseDir, copy.Src)
			}
			resolved.Push = &copy
		}
		if resolved.Pull != nil {
			copy := *resolved.Pull
			if !filepath.IsAbs(copy.Dest) {
				copy.Dest = filepath.Join(profile.BaseDir, copy.Dest)
			}
			resolved.Pull = &copy
		}
		resolved.BaseDir = profile.BaseDir
		if err := validateStep(resolved, false); err != nil {
			return nil, fmt.Errorf("step %d (%s): %w", index+1, step.DisplayName(index), err)
		}
		steps[index] = resolved
	}
	return steps, nil
}

func ResolveHandlers(handlers []Step) ([]Step, error) {
	resolved := make([]Step, len(handlers))
	for index, handler := range handlers {
		copy := handler
		if copy.Push != nil {
			action := *copy.Push
			if !filepath.IsAbs(action.Src) {
				action.Src = filepath.Join(copy.BaseDir, action.Src)
			}
			copy.Push = &action
		}
		if copy.Pull != nil {
			action := *copy.Pull
			if !filepath.IsAbs(action.Dest) {
				action.Dest = filepath.Join(copy.BaseDir, action.Dest)
			}
			copy.Pull = &action
		}
		if err := validateHandler(copy); err != nil {
			return nil, fmt.Errorf("handler %d (%s): %w", index+1, handler.Name, err)
		}
		resolved[index] = copy
	}
	return resolved, nil
}
