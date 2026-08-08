package deploy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/fanhuadesenlinnn/sshmd/v6/internal/batch"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/config"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/operation"
	"github.com/fanhuadesenlinnn/sshmd/v6/internal/ops"
)

// TaskContext carries per-host runtime state into a module.
type TaskContext struct {
	Ctx               context.Context
	Host              config.Host
	Vars              Vars
	Registers         map[string]any
	Facts             Vars
	Become            bool
	BecomeUser        string
	BecomePassword    string
	HasBecomePassword bool
	Env               map[string]string
	Check             bool
	Diff              bool
	CheckSafe         bool
	Confirm           func(message string) error
	ConfirmLazy       bool
	Timeout           time.Duration
	ConnectTimeout    time.Duration
	BaseDir           string
	LoopItem          any
	LoopIndex         int
	Executor          ops.Executor
	Visible           io.Writer
	PlayState         *PlayState
}

// PlayState carries mutable state shared by all hosts of one play.
type PlayState struct {
	mu       sync.Mutex
	prompted map[string]bool
}

func NewPlayState() *PlayState {
	return &PlayState{prompted: map[string]bool{}}
}

func (p *PlayState) MarkPrompted(key string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.prompted[key] {
		return false
	}
	p.prompted[key] = true
	return true
}

// ConfirmOnce prompts for message at most once per play, deduplicating
// concurrent hosts under free strategy. A nil confirm means interaction is
// unavailable, which is reported as an error exactly like linear mode.
func (p *PlayState) ConfirmOnce(message string, confirm func(string) error) error {
	if confirm == nil {
		return fmt.Errorf("deploy confirm 需要交互终端: %s", message)
	}
	p.mu.Lock()
	if p.prompted[message] {
		p.mu.Unlock()
		return nil
	}
	p.prompted[message] = true
	p.mu.Unlock()
	return confirm(message)
}

// ModuleResult is the normalized outcome of one module execution.
type ModuleResult struct {
	Status      batch.Status
	Output      string
	RC          int
	Register    any
	Err         error
	Stage       operation.FailureStage
	Changed     bool
	WouldChange bool
	Destination string
	SkipReason  string
}

// Module is one idempotent operation in the v3 registry.
type Module interface {
	Name() string
	// DecodeArgs strictly validates the module argument mapping. It must not
	// touch the network; path resolution happens inside Run.
	DecodeArgs(node *yaml.Node) (any, error)
	Run(tc TaskContext, args any) ModuleResult
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Module{}
)

// Register adds a module to the global registry.
func Register(module Module) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[module.Name()] = module
}

// Lookup returns a registered module by name.
func Lookup(name string) (Module, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	module, ok := registry[name]
	return module, ok
}

// ModuleNames returns the sorted names of all registered modules.
func ModuleNames() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// decodeStrict decodes a YAML node into out rejecting unknown fields.
func decodeStrict(node *yaml.Node, out any) error {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	if err := encoder.Encode(node); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(buffer.Bytes()))
	decoder.KnownFields(true)
	return decoder.Decode(out)
}

func failedModule(err error, stage operation.FailureStage) ModuleResult {
	if err == nil {
		err = fmt.Errorf("模块执行失败")
	}
	status := batch.StatusFailed
	if operation.IsConnectionFailure(stage) {
		status = batch.StatusUnreachable
	}
	return ModuleResult{Status: status, Err: err, Stage: stage, RC: 1}
}
